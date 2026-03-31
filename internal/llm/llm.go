package llm

import (
	"context"
	"strings"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/mudler/cogito"
)

// EmployeeLLM wraps Cogito OpenAI-compatible client (Chutes, OpenRouter, etc.).
type EmployeeLLM struct {
	inner cogito.LLM
}

// New builds an LLM from config (base URL + key + model).
func New(cfg *config.Config) *EmployeeLLM {
	return &EmployeeLLM{
		inner: cogito.NewOpenAILLM(cfg.LLMModel, cfg.LLMAPIKey, cfg.LLMBaseURL),
	}
}

// Reply generates an assistant reply given system persona and user message text.
func (e *EmployeeLLM) Reply(ctx context.Context, systemPersona, userText string) (string, error) {
	userText = strings.TrimSpace(userText)
	if userText == "" {
		return "", nil
	}
	frag := cogito.NewEmptyFragment().
		AddMessage(cogito.SystemMessageRole, systemPersona).
		AddMessage(cogito.UserMessageRole, userText)

	out, err := e.inner.Ask(ctx, frag)
	if err != nil {
		return "", err
	}
	last := out.LastMessage()
	if last == nil {
		return "", nil
	}
	return strings.TrimSpace(last.Content), nil
}
