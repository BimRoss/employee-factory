package slackbot

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strconv"

	"github.com/bimross/employee-factory/internal/llm"
	"github.com/sashabaranov/go-openai"
)

// go-openai RequestError.Error() and APIError.Error() both embed "status code: NNN".
var reOpenAIStatusCode = regexp.MustCompile(`(?i)status code:\s*(\d{3})\b`)

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
		case 404:
			return "The model or endpoint was not found (404). Ross should verify LLM_BASE_URL and LLM_MODEL match the provider."
		case 413:
			return "The chat request was too large for the provider (413). Ross can lower context, LLM_MAX_TOKENS, or LLM_SYSTEM_MAX_RUNES."
		case 422:
			return "The model rejected this payload (422). Ross needs to inspect logs."
		case 429:
			return "The model provider is rate-limiting right now (429). Please retry shortly."
		case 400:
			return "The model rejected this request format (400). Ross needs to inspect logs."
		case 500, 502, 503, 504:
			return "The model provider is temporarily unavailable. Please retry in a few seconds."
		default:
			return fmt.Sprintf("The model provider returned HTTP %d. Ross needs to check logs.", status)
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
	// Some wrappers stringify inner errors; go-openai bodies still embed "status code: NNN".
	if m := reOpenAIStatusCode.FindStringSubmatch(err.Error()); len(m) == 2 {
		if n, convErr := strconv.Atoi(m[1]); convErr == nil && n > 0 {
			return n, true
		}
	}
	return 0, false
}
