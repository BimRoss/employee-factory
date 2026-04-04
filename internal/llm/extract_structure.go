package llm

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"

	"github.com/mudler/cogito"
	"github.com/mudler/cogito/structures"
	"github.com/sashabaranov/go-openai"
	"github.com/sashabaranov/go-openai/jsonschema"
)

const extractStructureNudge = `Your previous response did not use the required json tool call with complete arguments matching the schema.

Call the json function exactly once with valid JSON arguments only (no markdown, no prose). If the interface does not show tools, output a single raw JSON object in your reply that matches the schema.`

// ExtractStructured runs schema extraction through Cogito and includes compatibility fallback
// for providers that return valid JSON in assistant content instead of tool_calls.
func (e *EmployeeLLM) ExtractStructured(ctx context.Context, systemPrompt, userPrompt string, schema jsonschema.Definition, out any, label string) error {
	if e == nil || e.primary == nil {
		return fmt.Errorf("llm extractor is not initialized")
	}
	frag := cogito.NewEmptyFragment().
		AddMessage(cogito.SystemMessageRole, strings.TrimSpace(systemPrompt)).
		AddMessage(cogito.UserMessageRole, strings.TrimSpace(userPrompt))
	structure := structures.Structure{
		Schema: schema,
		Object: out,
	}
	return extractStructureCompat(ctx, e.primary, frag, structure, label)
}

func extractStructureCompat(ctx context.Context, model cogito.LLM, frag cogito.Fragment, structure structures.Structure, label string) error {
	err := frag.ExtractStructure(ctx, model, structure)
	if err == nil {
		return nil
	}
	if !isExtractStructureToolFailure(err) {
		return err
	}

	retryFrag := frag.AddMessage(cogito.UserMessageRole, extractStructureNudge)
	retryErr := retryFrag.ExtractStructure(ctx, model, structure)
	if retryErr == nil {
		if label != "" {
			log.Printf("llm_extract: fallback=extract_retry label=%s", label)
		}
		return nil
	}
	if !isExtractStructureToolFailure(retryErr) {
		return retryErr
	}

	if uerr := unmarshalFromAssistantContent(frag, structure.Object); uerr == nil {
		if label != "" {
			log.Printf("llm_extract: fallback=assistant_json label=%s", label)
		}
		return nil
	}
	return err
}

func isExtractStructureToolFailure(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "no tool calls") ||
		strings.Contains(msg, "no choices:")
}

func unmarshalFromAssistantContent(frag cogito.Fragment, dest any) error {
	if dest == nil {
		return errors.New("nil destination")
	}
	for i := len(frag.Messages) - 1; i >= 0; i-- {
		m := frag.Messages[i]
		if m.Role != openai.ChatMessageRoleAssistant {
			continue
		}
		payload := extractJSONPayload(m.Content)
		if payload == "" {
			continue
		}
		if err := json.Unmarshal([]byte(payload), dest); err != nil {
			return err
		}
		return nil
	}
	return errors.New("no assistant JSON content")
}

func extractJSONPayload(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return ""
	}
	if strings.HasPrefix(s, "```") {
		rest := strings.TrimSpace(s[3:])
		rest = strings.TrimPrefix(rest, "json")
		rest = strings.TrimSpace(rest)
		if end := strings.Index(rest, "```"); end >= 0 {
			s = strings.TrimSpace(rest[:end])
		} else {
			s = rest
		}
	}
	s = strings.TrimSpace(s)
	if json.Valid([]byte(s)) {
		return s
	}
	start := strings.Index(s, "{")
	if start < 0 {
		return ""
	}
	depth := 0
	inString := false
	escape := false
	for i := start; i < len(s); i++ {
		c := s[i]
		if inString {
			if escape {
				escape = false
				continue
			}
			if c == '\\' {
				escape = true
				continue
			}
			if c == '"' {
				inString = false
			}
			continue
		}
		if c == '"' {
			inString = true
			continue
		}
		switch c {
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				candidate := s[start : i+1]
				if json.Valid([]byte(candidate)) {
					return candidate
				}
				return ""
			}
		}
	}
	return ""
}
