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
}

// New builds an LLM from config (base URL + key + model).
func New(cfg *config.Config) *EmployeeLLM {
	return &EmployeeLLM{
		inner:          cogito.NewOpenAILLM(cfg.LLMModel, cfg.LLMAPIKey, cfg.LLMBaseURL),
		maxTokens:      cfg.LLMMaxTokens,
		systemMaxRunes: cfg.LLMSystemMaxRunes,
	}
}

// Reply generates an assistant reply given system persona and user message text.
func (e *EmployeeLLM) Reply(ctx context.Context, systemPersona, userText string) (string, error) {
	userText = strings.TrimSpace(userText)
	if userText == "" {
		return "", nil
	}

	systemPersona = strings.TrimSpace(systemPersona)
	if e.systemMaxRunes >= 0 {
		systemPersona = truncateRunes(systemPersona, e.systemMaxRunes)
	}

	frag := cogito.NewEmptyFragment().
		AddMessage(cogito.SystemMessageRole, systemPersona).
		AddMessage(cogito.UserMessageRole, userText)

	messages := frag.GetMessages()
	req := openai.ChatCompletionRequest{
		Messages:    messages,
		MaxTokens:   e.maxTokens,
		Temperature: 0.55,
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

func truncateRunes(s string, maxRunes int) string {
	if maxRunes <= 0 || s == "" {
		return s
	}
	r := []rune(s)
	if len(r) <= maxRunes {
		return s
	}
	log.Printf("persona truncated from %d to %d runes to fit model context window", len(r), maxRunes)
	return string(r[:maxRunes]) + "\n\n[Persona truncated to fit the model context limit.]"
}
