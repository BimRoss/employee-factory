package llm

import (
	"context"
	"log"
	"strings"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/mudler/cogito"
	"github.com/sashabaranov/go-openai"
)

// EmployeeLLM wraps Cogito OpenAI-compatible client (Chutes, OpenRouter, etc.).
type EmployeeLLM struct {
	inner          cogito.LLM
	maxTokens      int
	systemMaxRunes int
	temperature    float32
	topP           *float32
}

// New builds an LLM from config (base URL + key + model).
func New(cfg *config.Config) *EmployeeLLM {
	return &EmployeeLLM{
		inner:          cogito.NewOpenAILLM(cfg.LLMModel, cfg.LLMAPIKey, cfg.LLMBaseURL),
		maxTokens:      cfg.LLMMaxTokens,
		systemMaxRunes: cfg.LLMSystemMaxRunes,
		temperature:    cfg.LLMTemperature,
		topP:           cfg.LLMTopP,
	}
}

// Reply generates an assistant reply. personaBody is truncated to fit systemMaxRunes minus
// slackSystemSuffix so Slack formatting rules are never dropped by truncation.
func (e *EmployeeLLM) Reply(ctx context.Context, personaBody, slackSystemSuffix, userText string) (string, error) {
	userText = strings.TrimSpace(userText)
	if userText == "" {
		return "", nil
	}

	personaBody = strings.TrimSpace(personaBody)
	suffix := strings.TrimSpace(slackSystemSuffix)
	system := composeSystemPrompt(personaBody, suffix, e.systemMaxRunes)

	frag := cogito.NewEmptyFragment().
		AddMessage(cogito.SystemMessageRole, system).
		AddMessage(cogito.UserMessageRole, userText)

	messages := frag.GetMessages()
	req := openai.ChatCompletionRequest{
		Messages:    messages,
		MaxTokens:   e.maxTokens,
		Temperature: e.temperature,
	}
	if e.topP != nil {
		req.TopP = *e.topP
	}

	resp, err := e.inner.CreateChatCompletion(ctx, req)
	if err != nil {
		return "", err
	}
	if len(resp.Choices) == 0 {
		return "", nil
	}
	content := strings.TrimSpace(resp.Choices[0].Message.Content)
	return content, nil
}

func composeSystemPrompt(personaBody, suffix string, systemMaxRunes int) string {
	persona := strings.TrimSpace(personaBody)
	suffix = strings.TrimSpace(suffix)
	if systemMaxRunes < 0 {
		if suffix == "" {
			return persona
		}
		if persona == "" {
			return suffix
		}
		return persona + "\n\n" + suffix
	}
	sr := len([]rune(suffix))
	if sr > systemMaxRunes {
		log.Printf("slack suffix longer than LLM_SYSTEM_MAX_RUNES (%d); truncating suffix", systemMaxRunes)
		return truncateRunes(suffix, systemMaxRunes)
	}
	if persona == "" {
		return suffix
	}
	if suffix == "" {
		return truncateRunes(persona, systemMaxRunes)
	}
	sep := 2 // "\n\n" between persona and Slack rules
	budget := systemMaxRunes - sr - sep
	if budget <= 0 {
		log.Printf("system prompt: no room for persona after Slack suffix; using suffix only")
		return suffix
	}
	persona = truncateRunes(persona, budget)
	if persona == "" {
		return suffix
	}
	return persona + "\n\n" + suffix
}

func truncateRunes(s string, maxRunes int) string {
	if s == "" {
		return ""
	}
	if maxRunes <= 0 {
		return ""
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	log.Printf("persona truncated from %d to %d runes to fit model context window", len(r), maxRunes)
	return string(r[:maxRunes])
}
