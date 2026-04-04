package docsaction

import (
	"fmt"
	"regexp"
	"strings"
)

const IntentCreateDoc = "create_doc"

var (
	reDocTitle = regexp.MustCompile(`(?i)\b(?:title|subject)\s*:\s*([^;\n,]+)`)
	reDocBody  = regexp.MustCompile(`(?i)\b(?:body|content)\s*:\s*([^;\n]+)`)
)

// CreateDocAction is the typed action contract for Joanne Google Docs tooling.
type CreateDocAction struct {
	Intent          string `json:"intent"`
	Title           string `json:"title,omitempty"`
	BodyInstruction string `json:"body_instruction,omitempty"`
	BodyText        string `json:"body_text,omitempty"`
}

// ParseCreateDocAction parses a create-doc command.
// Returns matched=false when the text is not a create-doc intent.
func ParseCreateDocAction(raw string) (CreateDocAction, bool, error) {
	text := strings.TrimSpace(raw)
	if text == "" {
		return CreateDocAction{}, false, nil
	}
	lower := strings.ToLower(text)
	if !looksLikeCreateDocIntent(lower) {
		return CreateDocAction{}, false, nil
	}
	action := CreateDocAction{Intent: IntentCreateDoc}
	segments := splitCommandSegments(text)
	remaining := make([]string, 0, len(segments))
	for _, seg := range segments {
		key, val, ok := parseKeyValueSegment(seg)
		if !ok {
			remaining = append(remaining, seg)
			continue
		}
		switch key {
		case "title", "subject":
			action.Title = strings.TrimSpace(val)
		case "body", "content":
			action.BodyText = strings.TrimSpace(val)
		case "instruction", "body_instruction":
			action.BodyInstruction = strings.TrimSpace(val)
		default:
			remaining = append(remaining, seg)
		}
	}

	if action.Title == "" {
		if m := reDocTitle.FindStringSubmatch(text); len(m) > 1 {
			action.Title = strings.TrimSpace(strings.Trim(m[1], `"'`))
		}
	}
	if action.BodyText == "" {
		if m := reDocBody.FindStringSubmatch(text); len(m) > 1 {
			action.BodyText = strings.TrimSpace(strings.Trim(m[1], `"'`))
		}
	}

	rem := normalizeResidual(strings.Join(remaining, " "))
	if action.BodyText == "" && action.BodyInstruction == "" && rem != "" {
		action.BodyInstruction = rem
	}
	if action.BodyText == "" && action.BodyInstruction == "" {
		return CreateDocAction{}, true, fmt.Errorf("missing doc content: include instruction:... or body:...")
	}
	return action, true, nil
}

func looksLikeCreateDocIntent(lower string) bool {
	switch {
	case strings.Contains(lower, "create google doc"):
		return true
	case strings.Contains(lower, "create a google doc"):
		return true
	case strings.Contains(lower, "make google doc"):
		return true
	case strings.Contains(lower, "new google doc"):
		return true
	case strings.Contains(lower, "draft google doc"):
		return true
	case strings.Contains(lower, "create doc"):
		return true
	case strings.Contains(lower, "draft doc"):
		return true
	case strings.Contains(lower, "google doc") && (strings.Contains(lower, "instruction:") || strings.Contains(lower, "body:") || strings.Contains(lower, "content:")):
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
	case "title", "subject", "body", "content", "instruction", "body_instruction":
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
	intentRe := regexp.MustCompile(`(?i)\b(create google doc|create a google doc|make google doc|new google doc|draft google doc|create doc|draft doc)\b`)
	s = strings.TrimSpace(intentRe.ReplaceAllString(s, ""))
	fieldRe := regexp.MustCompile(`(?i)\b(title|subject|body|content|instruction|body_instruction)\s*:\s*[^;\n]+`)
	s = strings.TrimSpace(fieldRe.ReplaceAllString(s, ""))
	s = strings.TrimSpace(strings.Trim(s, "-:"))
	return s
}
