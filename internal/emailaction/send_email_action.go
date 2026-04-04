package emailaction

import (
	"fmt"
	"regexp"
	"strings"
)

const IntentSendEmail = "send_email"

var (
	reEmail = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	reTitle = regexp.MustCompile(`(?i)\b(?:subject|title)\s*:\s*([^;\n,]+)`)
	reBody  = regexp.MustCompile(`(?i)\b(?:body)\s*:\s*([^;\n]+)`)
)

// SendEmailAction is the first typed action contract for Joanne email tooling.
type SendEmailAction struct {
	Intent          string `json:"intent"`
	To              string `json:"to,omitempty"`
	Subject         string `json:"subject,omitempty"`
	BodyInstruction string `json:"body_instruction,omitempty"`
	BodyText        string `json:"body_text,omitempty"`
}

// ParseSendEmailAction parses a send-email command.
// Returns matched=false when the text is not a send-email intent.
func ParseSendEmailAction(raw string) (SendEmailAction, bool, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return SendEmailAction{}, false, nil
	}

	lower := strings.ToLower(text)
	if !looksLikeSendEmailIntent(lower) {
		return SendEmailAction{}, false, nil
	}

	action := SendEmailAction{Intent: IntentSendEmail}
	segments := splitCommandSegments(text)
	remaining := make([]string, 0, len(segments))
	for _, seg := range segments {
		key, val, ok := parseKeyValueSegment(seg)
		if !ok {
			remaining = append(remaining, seg)
			continue
		}
		switch key {
		case "to":
			action.To = strings.TrimSpace(val)
		case "subject", "title":
			action.Subject = strings.TrimSpace(val)
		case "body":
			action.BodyText = strings.TrimSpace(val)
		case "body_instruction", "instruction":
			action.BodyInstruction = strings.TrimSpace(val)
		default:
			remaining = append(remaining, seg)
		}
	}

	if action.To == "" {
		if m := reEmail.FindString(text); strings.TrimSpace(m) != "" {
			action.To = strings.TrimSpace(m)
		}
	}
	if action.Subject == "" {
		if m := reTitle.FindStringSubmatch(text); len(m) > 1 {
			action.Subject = strings.TrimSpace(strings.Trim(m[1], `"'`))
		}
	}
	if action.BodyText == "" {
		if m := reBody.FindStringSubmatch(text); len(m) > 1 {
			action.BodyText = strings.TrimSpace(strings.Trim(m[1], `"'`))
		}
	}

	rem := normalizeResidual(strings.Join(remaining, " "))
	if action.BodyText == "" && action.BodyInstruction == "" && rem != "" {
		action.BodyInstruction = rem
	}

	if action.BodyText == "" && action.BodyInstruction == "" {
		return SendEmailAction{}, true, fmt.Errorf("missing email content: include instruction:... or body:...")
	}

	return action, true, nil
}

func looksLikeSendEmailIntent(lower string) bool {
	switch {
	case strings.Contains(lower, "send email"):
		return true
	case strings.Contains(lower, "email me"):
		return true
	case strings.Contains(lower, "please email"):
		return true
	case strings.HasPrefix(lower, "email "):
		return true
	case strings.Contains(lower, "draft email"):
		return true
	case strings.Contains(lower, "send an email"):
		return true
	case strings.Contains(lower, "email ") && (strings.Contains(lower, "body:") || strings.Contains(lower, "title:") || strings.Contains(lower, "subject:")):
		return true
	default:
		return false
	}
}

func splitCommandSegments(s string) []string {
	parts := strings.FieldsFunc(s, func(r rune) bool {
		return r == '\n' || r == ';'
	})
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	if len(out) == 0 {
		return []string{s}
	}
	return out
}

func parseKeyValueSegment(seg string) (key, val string, ok bool) {
	i := strings.Index(seg, ":")
	if i <= 0 {
		return "", "", false
	}
	k := strings.ToLower(strings.TrimSpace(seg[:i]))
	v := strings.TrimSpace(seg[i+1:])
	if k == "" || v == "" {
		return "", "", false
	}
	switch k {
	case "to", "subject", "title", "body", "instruction", "body_instruction":
		return k, strings.Trim(v, `"'`), true
	default:
		return "", "", false
	}
}

func normalizeResidual(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	intentRe := regexp.MustCompile(`(?i)\b(send an email|send email|draft email)\b`)
	s = strings.TrimSpace(intentRe.ReplaceAllString(s, ""))
	fieldRe := regexp.MustCompile(`(?i)\b(to|subject|body|instruction|body_instruction)\s*:\s*[^;\n]+`)
	s = strings.TrimSpace(fieldRe.ReplaceAllString(s, ""))
	s = strings.TrimSpace(strings.Trim(s, "-:"))
	return s
}
