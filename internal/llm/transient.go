package llm

import (
	"context"
	"errors"
	"strings"

	"github.com/sashabaranov/go-openai"
)

const maxRetryBackoffMS = 8000

// IsTransientLLMError reports whether err is worth retrying or routing to a fallback model
// (rate limits, gateway errors, and temporary provider capacity).
func IsTransientLLMError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var req *openai.RequestError
	if errors.As(err, &req) && req != nil {
		switch req.HTTPStatusCode {
		case 429, 502, 503:
			return true
		}
	}
	var api *openai.APIError
	if errors.As(err, &api) && api != nil {
		switch api.HTTPStatusCode {
		case 429, 502, 503:
			return true
		}
	}
	s := err.Error()
	if strings.Contains(s, "No instances available") {
		return true
	}
	return false
}
