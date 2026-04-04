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
	"github.com/slack-go/slack"
)

var reLikelyEmail = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)

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
	cmdText := strings.TrimSpace(rawText)
	if b.botUserID != "" {
		cmdText = strings.TrimSpace(strings.ReplaceAll(cmdText, "<@"+b.botUserID+">", ""))
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

	if parseErr != nil {
		b.postJoanneEmailStatus(commandCtx, channel, threadTS, "I can send that email once you give me content via `instruction:` or `body:`.")
		return
	}
	if b.gmailSender == nil {
		b.postJoanneEmailStatus(commandCtx, channel, threadTS, "Email tooling is enabled but Gmail auth is not ready in runtime config yet.")
		return
	}

	to, recipientSource, err := b.resolveJoanneEmailRecipient(commandCtx, action.To, requestUserID)
	if err != nil {
		b.postJoanneEmailStatus(commandCtx, channel, threadTS, "I couldn't resolve the recipient email from this request. Add `to: name@example.com` and retry.")
		return
	}

	subject := strings.TrimSpace(action.Subject)
	if subject == "" {
		subject = "Note from Joanne"
	}

	body := strings.TrimSpace(action.BodyText)
	if body == "" {
		body, err = b.generateJoanneEmailBody(commandCtx, action.BodyInstruction, to)
		if err != nil {
			log.Printf("joanne_email: body generation failed message_ts=%s err=%v", strings.TrimSpace(messageTS), err)
			b.postJoanneEmailStatus(commandCtx, channel, threadTS, "I hit a drafting error while composing that email. Please retry with `body:` text.")
			return
		}
	}

	result, err := b.gmailSender.Send(commandCtx, gmailsender.SendInput{
		To:      to,
		Subject: subject,
		Body:    body,
	})
	if err != nil {
		log.Printf("joanne_email: send failed message_ts=%s action_source=%s recipient_source=%s err=%v", strings.TrimSpace(messageTS), actionSource, recipientSource, err)
		b.postJoanneEmailStatus(commandCtx, channel, threadTS, "I couldn't send the Gmail message. Please check OAuth/runtime setup and retry.")
		return
	}

	log.Printf(
		"joanne_email: send success message_ts=%s action_source=%s recipient_source=%s message_id=%s thread_id=%s",
		strings.TrimSpace(messageTS),
		actionSource,
		recipientSource,
		strings.TrimSpace(result.MessageID),
		strings.TrimSpace(result.ThreadID),
	)
	b.postJoanneEmailStatus(commandCtx, channel, threadTS, fmt.Sprintf("Email sent to `%s` from `%s`.", to, b.cfg.GoogleSenderEmail))
}

func (b *Bot) resolveJoanneEmailRecipient(ctx context.Context, explicitTo, requestUserID string) (email string, source string, err error) {
	if to := strings.TrimSpace(explicitTo); to != "" {
		cleanTo := normalizeEmailAddress(to)
		if !isValidEmail(cleanTo) {
			return "", "", fmt.Errorf("invalid explicit recipient")
		}
		return cleanTo, "explicit", nil
	}
	userID := strings.TrimSpace(requestUserID)
	if userID == "" {
		return "", "", fmt.Errorf("missing requesting user id")
	}
	user, err := b.api.GetUserInfoContext(ctx, userID)
	if err != nil {
		return "", "", err
	}
	if user == nil || user.Profile.Email == "" {
		return "", "", fmt.Errorf("requesting user has no email in Slack profile")
	}
	to := strings.TrimSpace(user.Profile.Email)
	to = normalizeEmailAddress(to)
	if !isValidEmail(to) {
		return "", "", fmt.Errorf("invalid inferred recipient")
	}
	return to, "inferred_from_slack_user", nil
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
	return reply, nil
}

func (b *Bot) postJoanneEmailStatus(ctx context.Context, channel, threadTS, text string) {
	postCtx, cancel := context.WithTimeout(ctx, 8*time.Second)
	defer cancel()
	opts := []slack.MsgOption{slack.MsgOptionText(strings.TrimSpace(text), false)}
	if strings.TrimSpace(threadTS) != "" {
		opts = append(opts, slack.MsgOptionTS(strings.TrimSpace(threadTS)))
	}
	if _, _, err := b.api.PostMessageContext(postCtx, channel, opts...); err != nil {
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
