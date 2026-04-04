package slackbot

import (
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"math/rand"
	"regexp"
	"strconv"
	"strings"
	"time"

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
		return humanTimeoutMessage("deadline_exceeded")
	}
	if errors.Is(err, context.Canceled) {
		return "The model request was interrupted. Please retry."
	}
	if llm.IsProviderTimeoutLLMError(err) {
		return humanTimeoutMessage("provider_timeout")
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
			return humanTimeoutMessage(fmt.Sprintf("http_%d", status))
		default:
			return fmt.Sprintf("The model provider returned HTTP %d. Ross needs to check logs.", status)
		}
	}
	if llm.IsTransientLLMError(err) {
		return humanTimeoutMessage("transient_overload")
	}
	return "I hit a model error. Please retry once; if it repeats, Ross will check logs."
}

func humanTimeoutMessage(seed string) string {
	// Keep this intentionally human and light in-channel.
	choices := []string{
		"Sorry, I was in the bathroom for a sec. Try me again.",
		"Sorry, I zoned out for a moment. Can you send that one more time?",
		"Sorry, I lagged there. Hit me again and I'll jump right in.",
		"Sorry, I hiccuped for a second. Give me one more ping.",
	}
	return pickHumanChoice(choices, seed)
}

func pickHumanChoice(choices []string, seed string) string {
	if len(choices) == 0 {
		return "Sorry, I glitched for a second. Try me again."
	}
	s := strings.TrimSpace(seed)
	if s == "" {
		s = time.Now().Format(time.RFC3339Nano)
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(s))
	// Mix in runtime jitter so repeated identical errors can still vary across posts.
	x := int(h.Sum32()) ^ rand.New(rand.NewSource(time.Now().UnixNano())).Int()
	if x < 0 {
		x = -x
	}
	return choices[x%len(choices)]
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
