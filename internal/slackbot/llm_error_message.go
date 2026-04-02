package slackbot

import (
	"context"
	"errors"

	"github.com/bimross/employee-factory/internal/llm"
	"github.com/sashabaranov/go-openai"
)

func llmErrorUserMessage(err error) string {
	if err == nil {
		return "I hit a model error. Please retry once."
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "I hit a model timeout. Please retry in a few seconds."
	}
	if errors.Is(err, context.Canceled) {
		return "The model request was interrupted. Please retry."
	}
	if status, ok := llmHTTPStatus(err); ok {
		switch status {
		case 401, 403:
			return "I hit a model auth/config issue (401/403). Ross needs to check credentials."
		case 429:
			return "The model provider is rate-limiting right now (429). Please retry shortly."
		case 400:
			return "The model rejected this request format (400). Ross needs to inspect logs."
		case 500, 502, 503, 504:
			return "The model provider is temporarily unavailable. Please retry in a few seconds."
		}
	}
	if llm.IsTransientLLMError(err) {
		return "The model provider is temporarily overloaded. Please retry in a few seconds."
	}
	return "I hit a model error. Please retry once; if it repeats, Ross will check logs."
}

func llmHTTPStatus(err error) (int, bool) {
	var req *openai.RequestError
	if errors.As(err, &req) && req != nil && req.HTTPStatusCode > 0 {
		return req.HTTPStatusCode, true
	}
	var api *openai.APIError
	if errors.As(err, &api) && api != nil && api.HTTPStatusCode > 0 {
		return api.HTTPStatusCode, true
	}
	return 0, false
}
