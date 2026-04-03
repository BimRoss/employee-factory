package llm

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/mudler/cogito"
	"github.com/sashabaranov/go-openai"
)

// EmployeeLLM wraps a Cogito OpenAI-compatible client (OpenRouter, Chutes, etc.).
type EmployeeLLM struct {
	primary        cogito.LLM
	fallback       cogito.LLM
	maxRetries     int
	retryBackoffMS int
	fallbackTO     time.Duration
	maxTokens      int
	systemMaxRunes int
	temperature    float32
	topP           *float32
}

// New builds an LLM from config (base URL + key + model) with optional retry and fallback model.
func New(cfg *config.Config) *EmployeeLLM {
	e := &EmployeeLLM{
		primary:        cogito.NewOpenAILLM(cfg.LLMModel, cfg.LLMAPIKey, cfg.LLMBaseURL),
		maxRetries:     cfg.LLMMaxRetries,
		retryBackoffMS: cfg.LLMRetryBackoffMS,
		fallbackTO:     time.Duration(cfg.LLMFallbackTimeoutSec) * time.Second,
		maxTokens:      cfg.LLMMaxTokens,
		systemMaxRunes: cfg.LLMSystemMaxRunes,
		temperature:    cfg.LLMTemperature,
		topP:           cfg.LLMTopP,
	}
	fb := strings.TrimSpace(cfg.LLMFallbackModel)
	if fb != "" && fb != cfg.LLMModel {
		e.fallback = cogito.NewOpenAILLM(fb, cfg.LLMAPIKey, cfg.LLMBaseURL)
	}
	return e
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

	maxAttempts := 1 + e.maxRetries
	if maxAttempts < 1 {
		maxAttempts = 1
	}
	var lastErr error
	for attempt := 0; attempt < maxAttempts; attempt++ {
		if attempt > 0 {
			if lastErr == nil || !IsTransientLLMError(lastErr) {
				break
			}
			delay := e.retryBackoffMS * (1 << (attempt - 1))
			if delay > maxRetryBackoffMS {
				delay = maxRetryBackoffMS
			}
			select {
			case <-ctx.Done():
				return "", ctx.Err()
			case <-time.After(time.Duration(delay) * time.Millisecond):
			}
			log.Printf("llm: retrying primary completion after transient error (attempt %d/%d)", attempt+1, maxAttempts)
		}

		resp, err := e.primary.CreateChatCompletion(ctx, req)
		if err == nil {
			return chatCompletionText(resp), nil
		}
		lastErr = err
		if !IsTransientLLMError(err) {
			break
		}
	}

	if e.fallback != nil && lastErr != nil && IsFallbackEligibleLLMError(lastErr) {
		log.Printf("llm: attempting fallback model after primary failure: %v", lastErr)
		timeout := e.fallbackTO
		if timeout <= 0 {
			timeout = 8 * time.Second
		}
		fallbackBase := ctx
		// Primary timeout can cancel the parent ctx; use a short detached budget for fallback.
		if IsProviderTimeoutLLMError(lastErr) {
			fallbackBase = context.Background()
		}
		fallbackCtx, cancelFallback := context.WithTimeout(fallbackBase, timeout)
		resp, err := e.fallback.CreateChatCompletion(fallbackCtx, req)
		cancelFallback()
		if err == nil {
			log.Printf("llm: fallback model completion succeeded")
			return chatCompletionText(resp), nil
		}
		return "", err
	}

	if lastErr != nil {
		return "", lastErr
	}
	return "", nil
}

func chatCompletionText(resp openai.ChatCompletionResponse) string {
	if len(resp.Choices) == 0 {
		return ""
	}
	return strings.TrimSpace(resp.Choices[0].Message.Content)
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
