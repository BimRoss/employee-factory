package emailaction

import (
	"fmt"
	"regexp"
	"strings"
)

const IntentSendEmail = "send_email"

var (
	reEmail       = regexp.MustCompile(`(?i)\b[A-Z0-9._%+\-]+@[A-Z0-9.\-]+\.[A-Z]{2,}\b`)
	reFieldMarker = regexp.MustCompile(`(?is)\b(to|subject|title|body|instruction|body_instruction)\s*:\s*`)
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
	fields, residual := parseLabeledFields(text)
	if v := strings.TrimSpace(fields["to"]); v != "" {
		action.To = v
	}
	if v := strings.TrimSpace(fields["subject"]); v != "" {
		action.Subject = v
	}
	if v := strings.TrimSpace(fields["title"]); v != "" {
		action.Subject = v
	}
	if v := strings.TrimSpace(fields["body"]); v != "" {
		action.BodyText = v
	}
	if v := strings.TrimSpace(fields["instruction"]); v != "" {
		action.BodyInstruction = v
	}
	if v := strings.TrimSpace(fields["body_instruction"]); v != "" {
		action.BodyInstruction = v
	}

	if action.To == "" {
		if m := reEmail.FindString(text); strings.TrimSpace(m) != "" {
			action.To = strings.TrimSpace(m)
		}
	}

	rem := normalizeResidual(residual)
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

func parseLabeledFields(s string) (map[string]string, string) {
	matches := reFieldMarker.FindAllStringSubmatchIndex(s, -1)
	if len(matches) == 0 {
		return map[string]string{}, s
	}
	fields := map[string]string{}
	consumed := make([][2]int, 0, len(matches))
	for i, m := range matches {
		if len(m) < 4 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(s[m[2]:m[3]]))
		valueStart := m[1]
		valueEnd := len(s)
		if i+1 < len(matches) {
			valueEnd = matches[i+1][0]
		}
		value := cleanFieldValue(s[valueStart:valueEnd])
		if value != "" {
			fields[key] = value
		}
		consumed = append(consumed, [2]int{m[0], valueEnd})
	}
	residual := removeRanges(s, consumed)
	return fields, residual
}

func cleanFieldValue(raw string) string {
	value := strings.TrimSpace(raw)
	value = strings.Trim(value, " \t\r\n;,")
	value = strings.Trim(value, `"'`)
	return strings.TrimSpace(value)
}

func removeRanges(s string, ranges [][2]int) string {
	if len(ranges) == 0 {
		return s
	}
	var builder strings.Builder
	last := 0
	for _, rg := range ranges {
		start, end := rg[0], rg[1]
		if start < last {
			start = last
		}
		if start > len(s) {
			start = len(s)
		}
		if end > len(s) {
			end = len(s)
		}
		if start > last {
			builder.WriteString(s[last:start])
		}
		if builder.Len() > 0 {
			builder.WriteString(" ")
		}
		last = end
	}
	if last < len(s) {
		builder.WriteString(s[last:])
	}
	return builder.String()
}

func normalizeResidual(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	intentRe := regexp.MustCompile(`(?i)\b(send an email|send email|draft email)\b`)
	s = strings.TrimSpace(intentRe.ReplaceAllString(s, ""))
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.TrimSpace(strings.Trim(s, "-:"))
	s = strings.Join(strings.Fields(s), " ")
	return s
}
