package slackbot

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"
	"time"

	"github.com/bimross/employee-factory/internal/emailaction"
	"github.com/bimross/employee-factory/internal/gmailsender"
	"github.com/sashabaranov/go-openai/jsonschema"
)

var reLikelyEmail = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)

const grantFallbackRecipientEmail = "grant@bimross.com"
const joanneEmailPendingTTL = 20 * time.Minute

type joannePendingEmail struct {
	To           string
	Subject      string
	Body         string
	Goal         string
	ThreadTS     string
	ActionSource string
	CreatedAt    time.Time
	ExpiresAt    time.Time
}

type joanneEmailActionExtract struct {
	Intent          string  `json:"intent"`
	To              string  `json:"to,omitempty"`
	Subject         string  `json:"subject,omitempty"`
	BodyInstruction string  `json:"body_instruction,omitempty"`
	BodyText        string  `json:"body_text,omitempty"`
	Confidence      float64 `json:"confidence,omitempty"`
	Reason          string  `json:"reason,omitempty"`
}

func joanneEmailActionSchema() jsonschema.Definition {
	return jsonschema.Definition{
		Type:                 jsonschema.Object,
		AdditionalProperties: false,
		Properties: map[string]jsonschema.Definition{
			"intent": {
				Type:        jsonschema.String,
				Enum:        []string{emailaction.IntentSendEmail, "none"},
				Description: "Action intent. Use send_email only when the user explicitly asks to send or draft an email.",
			},
			"to": {
				Type:        jsonschema.String,
				Description: "Explicit recipient email address if provided.",
			},
			"subject": {
				Type:        jsonschema.String,
				Description: "Email subject/title if provided.",
			},
			"body_instruction": {
				Type:        jsonschema.String,
				Description: "Instruction for drafting body text if body text is not provided.",
			},
			"body_text": {
				Type:        jsonschema.String,
				Description: "Final body text if explicitly provided.",
			},
			"confidence": {
				Type:        jsonschema.Number,
				Description: "Confidence score from 0.0 to 1.0.",
			},
			"reason": {
				Type:        jsonschema.String,
				Description: "One short reason for the selected intent.",
			},
		},
		Required: []string{"intent"},
	}
}

func (b *Bot) tryHandleJoanneSendEmail(ctx context.Context, channel, rawText, requestUserID, messageTS, threadTS string) bool {
	if b == nil || b.cfg == nil {
		return false
	}
	if !b.cfg.JoanneEmailEnabled {
		return false
	}
	if !strings.EqualFold(strings.TrimSpace(b.cfg.EmployeeID), "joanne") {
		return false
	}
	if !shouldHandleTargetedSquadMessage(rawText, b.cfg, "joanne") {
		log.Printf("joanne_email: skip message_ts=%s reason=targeted_to_other_employee", strings.TrimSpace(messageTS))
		return false
	}
	cmdText := strings.TrimSpace(rawText)
	if b.botUserID != "" {
		cmdText = strings.TrimSpace(strings.ReplaceAll(cmdText, "<@"+b.botUserID+">", ""))
	}
	if isJoanneEmailConfirmText(cmdText) || isJoanneEmailCancelText(cmdText) {
		decision := b.decideAdvancedTaskRouting(ctx, channel, requestUserID, messageTS, threadTS, advancedTaskJoanneEmail)
		if decision.ConsumeEvent || !decision.AllowExecution {
			return true
		}
		if strings.TrimSpace(decision.ExecutionTS) != "" {
			threadTS = strings.TrimSpace(decision.ExecutionTS)
		}
	}
	if b.tryHandleJoanneEmailConfirmation(ctx, channel, requestUserID, cmdText, threadTS, messageTS) {
		return true
	}

	extract, extractErr := b.extractJoanneEmailAction(ctx, cmdText)
	action, matched, parseErr, source := resolveJoanneEmailAction(cmdText, extract, extractErr)
	if !matched {
		return false
	}
	log.Printf(
		"joanne_email: accepted message_ts=%s source=%s confidence=%.2f parse_err=%t extract_err=%t reason=%q",
		strings.TrimSpace(messageTS),
		source,
		extract.Confidence,
		parseErr != nil,
		extractErr != nil,
		strings.TrimSpace(extract.Reason),
	)
	decision := b.decideAdvancedTaskRouting(ctx, channel, requestUserID, messageTS, threadTS, advancedTaskJoanneEmail)
	if decision.ConsumeEvent || !decision.AllowExecution {
		return true
	}
	if strings.TrimSpace(decision.ExecutionTS) != "" {
		threadTS = strings.TrimSpace(decision.ExecutionTS)
	}
	go b.handleJoanneSendEmailSafely(ctx, channel, requestUserID, messageTS, threadTS, action, parseErr, source)
	return true
}

func resolveJoanneEmailAction(raw string, extract joanneEmailActionExtract, extractErr error) (emailaction.SendEmailAction, bool, error, string) {
	if extractErr == nil && strings.EqualFold(strings.TrimSpace(extract.Intent), emailaction.IntentSendEmail) {
		action := emailaction.SendEmailAction{
			Intent:          emailaction.IntentSendEmail,
			To:              strings.TrimSpace(extract.To),
			Subject:         strings.TrimSpace(extract.Subject),
			BodyInstruction: strings.TrimSpace(extract.BodyInstruction),
			BodyText:        strings.TrimSpace(extract.BodyText),
		}
		if action.BodyText == "" && action.BodyInstruction == "" {
			return action, true, fmt.Errorf("missing email content"), "extractor"
		}
		return action, true, nil, "extractor"
	}
	action, matched, err := emailaction.ParseSendEmailAction(raw)
	if !matched {
		return emailaction.SendEmailAction{}, false, nil, "none"
	}
	return action, true, err, "parser"
}

func (b *Bot) extractJoanneEmailAction(ctx context.Context, cmdText string) (joanneEmailActionExtract, error) {
	var out joanneEmailActionExtract
	extractCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()

	systemPrompt := "You classify whether a Slack message is a send-email command for Joanne. Respond only with schema-compliant JSON."
	userPrompt := "Message:\n" + strings.TrimSpace(cmdText) + "\n\nRules:\n- intent=send_email only for explicit requests to send or draft an email.\n- If unsure or not an email request, set intent=none.\n- Map title as subject.\n- If direct body text is provided, place it in body_text.\n- If only drafting guidance is provided, place it in body_instruction.\n- Do not invent recipients."
	err := b.llm.ExtractStructured(extractCtx, systemPrompt, userPrompt, joanneEmailActionSchema(), &out, "joanne_send_email")
	if err != nil {
		return joanneEmailActionExtract{}, err
	}
	out.Intent = strings.ToLower(strings.TrimSpace(out.Intent))
	out.To = strings.TrimSpace(out.To)
	out.Subject = strings.TrimSpace(out.Subject)
	out.BodyText = strings.TrimSpace(out.BodyText)
	out.BodyInstruction = strings.TrimSpace(out.BodyInstruction)
	out.Reason = strings.TrimSpace(out.Reason)
	if out.Confidence < 0 {
		out.Confidence = 0
	}
	if out.Confidence > 1 {
		out.Confidence = 1
	}
	return out, nil
}

func (b *Bot) handleJoanneSendEmailSafely(parent context.Context, channel, requestUserID, messageTS, threadTS string, action emailaction.SendEmailAction, parseErr error, source string) {
	defer func() {
		if r := recover(); r != nil {
			log.Printf("joanne_email: panic recovered message_ts=%s source=%s panic=%v", strings.TrimSpace(messageTS), source, r)
		}
	}()
	b.handleJoanneSendEmail(parent, channel, requestUserID, messageTS, threadTS, action, parseErr, source)
}

func (b *Bot) handleJoanneSendEmail(parent context.Context, channel, requestUserID, messageTS, threadTS string, action emailaction.SendEmailAction, parseErr error, actionSource string) {
	commandCtx, cancel := context.WithTimeout(parent, 45*time.Second)
	defer cancel()

	if b.gmailSender == nil {
		b.postJoanneEmailStatus(commandCtx, channel, threadTS, "Email tooling is enabled but Gmail auth is not ready in runtime config yet.")
		return
	}
	missingRecipient := strings.TrimSpace(action.To) == ""
	hasBodyContent := strings.TrimSpace(action.BodyText) != "" || strings.TrimSpace(action.BodyInstruction) != ""
	missingGoal := !hasBodyContent
	if parseErr != nil && !hasBodyContent {
		log.Printf("joanne_email: parse_err message_ts=%s source=%s err=%v", strings.TrimSpace(messageTS), strings.TrimSpace(actionSource), parseErr)
	}
	if missingRecipient || missingGoal {
		b.postJoanneEmailStatus(commandCtx, channel, threadTS, b.buildJoanneEmailMissingInfoPrompt(missingRecipient, missingGoal))
		return
	}

	to, recipientSource, err := b.resolveJoanneEmailRecipient(commandCtx, action.To, requestUserID)
	if err != nil {
		b.postJoanneEmailStatus(commandCtx, channel, threadTS, "I couldn't confirm the recipient address yet. Who should receive this email? Please share a direct email address.")
		return
	}

	subject := strings.TrimSpace(action.Subject)
	if subject == "" {
		subject = "Note from Joanne"
	}

	body := normalizeJoannePlainText(action.BodyText)
	if body == "" {
		body, err = b.generateJoanneEmailBody(commandCtx, action.BodyInstruction, to)
		if err != nil {
			log.Printf("joanne_email: body generation failed message_ts=%s err=%v", strings.TrimSpace(messageTS), err)
			b.postJoanneEmailStatus(commandCtx, channel, threadTS, "I hit a drafting error while composing that email. Please retry with `body:` text.")
			return
		}
	}
	body = normalizeJoannePlainText(body)
	if body == "" {
		b.postJoanneEmailStatus(commandCtx, channel, threadTS, "I still need the email body content. Please provide it with `body:` or `instruction:`.")
		return
	}

	goal := deriveJoanneEmailGoal(action, body)
	b.setJoannePendingEmail(channel, requestUserID, threadTS, joannePendingEmail{
		To:           to,
		Subject:      subject,
		Body:         body,
		Goal:         goal,
		ThreadTS:     strings.TrimSpace(threadTS),
		ActionSource: actionSource + "/" + recipientSource,
		CreatedAt:    time.Now().UTC(),
		ExpiresAt:    time.Now().UTC().Add(joanneEmailPendingTTL),
	})
	b.postJoanneEmailStatus(commandCtx, channel, threadTS, buildJoanneEmailConfirmationPrompt(to, subject, goal))
}

func (b *Bot) tryHandleJoanneEmailConfirmation(ctx context.Context, channel, requestUserID, cmdText, threadTS, messageTS string) bool {
	pending, ok := b.getJoannePendingEmail(channel, requestUserID, threadTS)
	if !ok {
		return false
	}
	text := strings.TrimSpace(cmdText)
	if isJoanneEmailCancelText(text) {
		b.clearJoannePendingEmail(channel, requestUserID, threadTS)
		b.postJoanneEmailStatus(ctx, channel, pending.ThreadTS, "Stopped. I canceled that queued email.")
		return true
	}
	if !isJoanneEmailConfirmText(text) {
		return false
	}
	if b.gmailSender == nil {
		b.postJoanneEmailStatus(ctx, channel, pending.ThreadTS, "Email tooling is enabled but Gmail auth is not ready in runtime config yet.")
		return true
	}
	sendCtx, cancel := context.WithTimeout(ctx, 45*time.Second)
	defer cancel()
	result, err := b.gmailSender.Send(sendCtx, gmailsender.SendInput{
		To:      pending.To,
		Subject: pending.Subject,
		Body:    pending.Body,
	})
	if err != nil {
		log.Printf("joanne_email: confirm send failed message_ts=%s action_source=%s err=%v", strings.TrimSpace(messageTS), pending.ActionSource, err)
		b.postJoanneEmailStatus(sendCtx, channel, pending.ThreadTS, "I couldn't send that Gmail message yet. Reply `confirm send` to retry, or `cancel` to stop.")
		return true
	}
	b.clearJoannePendingEmail(channel, requestUserID, threadTS)
	log.Printf(
		"joanne_email: confirm send success message_ts=%s action_source=%s message_id=%s thread_id=%s",
		strings.TrimSpace(messageTS),
		pending.ActionSource,
		strings.TrimSpace(result.MessageID),
		strings.TrimSpace(result.ThreadID),
	)
	b.postJoanneEmailStatus(sendCtx, channel, pending.ThreadTS, fmt.Sprintf("Email sent to `%s` from `%s`.", pending.To, b.cfg.GoogleSenderEmail))
	return true
}

func (b *Bot) resolveJoanneEmailRecipient(ctx context.Context, explicitTo, requestUserID string) (email string, source string, err error) {
	if to := strings.TrimSpace(explicitTo); to != "" {
		if b.shouldUseGrantRecipientFallback(requestUserID, to) {
			return grantFallbackRecipientEmail, "grant_me_alias", nil
		}
		cleanTo := normalizeEmailAddress(to)
		if !isValidEmail(cleanTo) {
			if b.shouldUseGrantRecipientFallback(requestUserID, "") {
				return grantFallbackRecipientEmail, "grant_user_fallback_invalid_explicit", nil
			}
			return "", "", fmt.Errorf("invalid explicit recipient")
		}
		return cleanTo, "explicit", nil
	}
	userID := strings.TrimSpace(requestUserID)
	if userID == "" {
		return "", "", fmt.Errorf("missing requesting user id")
	}
	if b.shouldUseGrantRecipientFallback(userID, "") {
		return grantFallbackRecipientEmail, "grant_user_fallback", nil
	}
	user, err := b.api.GetUserInfoContext(ctx, userID)
	if err != nil {
		if b.shouldUseGrantRecipientFallback(userID, "") {
			return grantFallbackRecipientEmail, "grant_user_fallback", nil
		}
		return "", "", err
	}
	if user == nil || user.Profile.Email == "" {
		if b.shouldUseGrantRecipientFallback(userID, "") {
			return grantFallbackRecipientEmail, "grant_user_fallback", nil
		}
		return "", "", fmt.Errorf("requesting user has no email in Slack profile")
	}
	to := strings.TrimSpace(user.Profile.Email)
	to = normalizeEmailAddress(to)
	if !isValidEmail(to) {
		return "", "", fmt.Errorf("invalid inferred recipient")
	}
	return to, "inferred_from_slack_user", nil
}

func (b *Bot) shouldUseGrantRecipientFallback(requestUserID, explicitTo string) bool {
	if b == nil || b.cfg == nil {
		return false
	}
	allowed := strings.TrimSpace(b.cfg.ChatAllowedUserID)
	if allowed == "" || strings.TrimSpace(requestUserID) != allowed {
		return false
	}
	explicit := strings.ToLower(strings.TrimSpace(explicitTo))
	if explicit == "" {
		return true
	}
	switch explicit {
	case "me", "myself", "grant", "grant foster":
		return true
	default:
		return false
	}
}

func (b *Bot) generateJoanneEmailBody(ctx context.Context, instruction, recipient string) (string, error) {
	persona := b.persona.String()
	if strings.TrimSpace(persona) == "" {
		persona = "You are Joanne."
	}
	prompt := strings.TrimSpace(instruction)
	if prompt == "" {
		return "", fmt.Errorf("missing email instruction")
	}
	userText := fmt.Sprintf(
		"Draft an email body in Joanne's voice.\nRecipient: %s\nInstruction: %s\n\nReturn only the email body text (no subject line, no markdown).",
		recipient,
		prompt,
	)
	reply, err := b.llm.Reply(ctx, persona, "Write concise, plain-text email body only.", userText)
	if err != nil {
		return "", err
	}
	reply = strings.TrimSpace(reply)
	if reply == "" {
		return "", fmt.Errorf("empty generated body")
	}
	return normalizeJoannePlainText(reply), nil
}

func (b *Bot) postJoanneEmailStatus(ctx context.Context, channel, threadTS, text string) {
	postCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	if err := b.postSlackResponse(postCtx, channel, slackResponse{
		Text:     strings.TrimSpace(text),
		ThreadTS: strings.TrimSpace(threadTS),
	}); err != nil {
		log.Printf("joanne_email: slack status post failed: %v", err)
	}
}

func isValidEmail(v string) bool {
	return reLikelyEmail.MatchString(strings.TrimSpace(v))
}

func normalizeEmailAddress(raw string) string {
	s := strings.TrimSpace(raw)
	// Slack often wraps addresses as <mailto:user@example.com|user@example.com>.
	if strings.HasPrefix(s, "<mailto:") && strings.HasSuffix(s, ">") {
		s = strings.TrimPrefix(s, "<mailto:")
		s = strings.TrimSuffix(s, ">")
		if i := strings.Index(s, "|"); i > 0 {
			s = s[:i]
		}
	}
	if m := reLikelyEmail.FindString(s); strings.TrimSpace(m) != "" {
		return strings.TrimSpace(m)
	}
	return strings.TrimSpace(strings.Trim(s, ">,;"))
}

func (b *Bot) buildJoanneEmailMissingInfoPrompt(missingRecipient, missingGoal bool) string {
	if missingRecipient && missingGoal {
		return "I can send that. Before I do, I still need two things: who should receive it, and what outcome you want from the email. Share it naturally, for example: `send to grant@bimross.com and the goal is confirming Monday at 10am`."
	}
	if missingRecipient {
		return "I can draft that now. Who should receive this email? Please share the recipient address."
	}
	return "I have the recipient. What is the goal of this email so I can draft the right message?"
}

func buildJoanneEmailConfirmationPrompt(to, subject, goal string) string {
	return fmt.Sprintf(
		"I have everything I need. Before I send, please confirm this looks right:\n- Recipient: `%s`\n- Goal: %s\n- Subject: %s\n\nReply `confirm send` to send it, or `cancel` to stop.",
		strings.TrimSpace(to),
		strings.TrimSpace(goal),
		strings.TrimSpace(subject),
	)
}

func normalizeJoannePlainText(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return ""
	}
	s = strings.ReplaceAll(s, "\r\n", "\n")
	s = strings.ReplaceAll(s, "\r", "\n")
	s = strings.ReplaceAll(s, "```", "")
	lines := strings.Split(s, "\n")
	cleaned := make([]string, 0, len(lines))
	blankStreak := 0
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if line == "" {
			blankStreak++
			if blankStreak > 1 {
				continue
			}
			cleaned = append(cleaned, "")
			continue
		}
		blankStreak = 0
		cleaned = append(cleaned, line)
	}
	return strings.TrimSpace(strings.Join(cleaned, "\n"))
}

func deriveJoanneEmailGoal(action emailaction.SendEmailAction, body string) string {
	if goal := strings.TrimSpace(action.BodyInstruction); goal != "" {
		return goal
	}
	body = strings.TrimSpace(body)
	if body == "" {
		body = strings.TrimSpace(action.BodyText)
	}
	if body == "" {
		return "Draft and send the requested message."
	}
	if len(body) > 120 {
		return body[:117] + "..."
	}
	return body
}

func isJoanneEmailConfirmText(v string) bool {
	text := strings.TrimSpace(strings.ToLower(v))
	switch text {
	case "confirm", "confirm send", "send now":
		return true
	default:
		return false
	}
}

func isJoanneEmailCancelText(v string) bool {
	text := strings.TrimSpace(strings.ToLower(v))
	switch text {
	case "cancel", "cancel send", "stop":
		return true
	default:
		return false
	}
}

func joannePendingKey(channel, requestUserID, threadTS string) string {
	return strings.TrimSpace(channel) + "|" + strings.TrimSpace(requestUserID) + "|" + strings.TrimSpace(threadTS)
}

func (b *Bot) setJoannePendingEmail(channel, requestUserID, threadTS string, pending joannePendingEmail) {
	if b == nil {
		return
	}
	key := joannePendingKey(channel, requestUserID, threadTS)
	b.joanneEmailPendingMu.Lock()
	defer b.joanneEmailPendingMu.Unlock()
	if b.joanneEmailPending == nil {
		b.joanneEmailPending = map[string]joannePendingEmail{}
	}
	b.joanneEmailPending[key] = pending
}

func (b *Bot) getJoannePendingEmail(channel, requestUserID, threadTS string) (joannePendingEmail, bool) {
	if b == nil {
		return joannePendingEmail{}, false
	}
	key := joannePendingKey(channel, requestUserID, threadTS)
	now := time.Now().UTC()
	b.joanneEmailPendingMu.Lock()
	defer b.joanneEmailPendingMu.Unlock()
	pending, ok := b.joanneEmailPending[key]
	if !ok {
		return joannePendingEmail{}, false
	}
	if !pending.ExpiresAt.IsZero() && pending.ExpiresAt.Before(now) {
		delete(b.joanneEmailPending, key)
		return joannePendingEmail{}, false
	}
	return pending, true
}

func (b *Bot) clearJoannePendingEmail(channel, requestUserID, threadTS string) {
	if b == nil {
		return
	}
	key := joannePendingKey(channel, requestUserID, threadTS)
	b.joanneEmailPendingMu.Lock()
	defer b.joanneEmailPendingMu.Unlock()
	delete(b.joanneEmailPending, key)
}
