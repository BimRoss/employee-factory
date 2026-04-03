package slackbot

import (
	"context"
	"fmt"
	"log"
	"regexp"
	"strings"

	"github.com/bimross/employee-factory/internal/emailaction"
	"github.com/bimross/employee-factory/internal/gmailsender"
	"github.com/slack-go/slack"
)

var reLikelyEmail = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)

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
	action, matched, err := emailaction.ParseSendEmailAction(cmdText)
	if !matched {
		return false
	}
	if err != nil {
		b.postJoanneEmailStatus(ctx, channel, threadTS, "I can send that email once you give me content via `instruction:` or `body:`.")
		return true
	}
	if b.gmailSender == nil {
		b.postJoanneEmailStatus(ctx, channel, threadTS, "Email tooling is enabled but Gmail auth is not ready in runtime config yet.")
		return true
	}

	to, source, err := b.resolveJoanneEmailRecipient(ctx, action.To, requestUserID)
	if err != nil {
		b.postJoanneEmailStatus(ctx, channel, threadTS, "I couldn't resolve the recipient email from this request. Add `to: name@example.com` and retry.")
		return true
	}

	subject := strings.TrimSpace(action.Subject)
	if subject == "" {
		subject = "Note from Joanne"
	}

	body := strings.TrimSpace(action.BodyText)
	if body == "" {
		body, err = b.generateJoanneEmailBody(ctx, action.BodyInstruction, to)
		if err != nil {
			log.Printf("joanne_email: body generation failed message_ts=%s err=%v", strings.TrimSpace(messageTS), err)
			b.postJoanneEmailStatus(ctx, channel, threadTS, "I hit a drafting error while composing that email. Please retry with `body:` text.")
			return true
		}
	}

	err = b.gmailSender.Send(ctx, gmailsender.SendInput{
		To:      to,
		Subject: subject,
		Body:    body,
	})
	if err != nil {
		log.Printf("joanne_email: send failed message_ts=%s recipient_source=%s err=%v", strings.TrimSpace(messageTS), source, err)
		b.postJoanneEmailStatus(ctx, channel, threadTS, "I couldn't send the Gmail message. Please check OAuth/runtime setup and retry.")
		return true
	}

	log.Printf("joanne_email: send success message_ts=%s recipient_source=%s", strings.TrimSpace(messageTS), source)
	b.postJoanneEmailStatus(ctx, channel, threadTS, fmt.Sprintf("Email sent to `%s` from `%s`.", to, b.cfg.GoogleSenderEmail))
	return true
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
	opts := []slack.MsgOption{slack.MsgOptionText(strings.TrimSpace(text), false)}
	if strings.TrimSpace(threadTS) != "" {
		opts = append(opts, slack.MsgOptionTS(strings.TrimSpace(threadTS)))
	}
	if _, _, err := b.api.PostMessageContext(ctx, channel, opts...); err != nil {
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
