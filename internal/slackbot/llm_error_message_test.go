package slackbot

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/sashabaranov/go-openai"
)

func TestLLMErrorUserMessage(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		wantAny []string
	}{
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			wantAny: []string{
				"sorry",
				"try me again",
				"one more time",
				"one more ping",
			},
		},
		{
			name: "rate limited",
			err: &openai.RequestError{
				HTTPStatusCode: 429,
				Err:            errors.New("too many requests"),
			},
			wantAny: []string{"rate-limiting"},
		},
		{
			name: "auth failure",
			err: &openai.RequestError{
				HTTPStatusCode: 401,
				Err:            errors.New("unauthorized"),
			},
			wantAny: []string{"auth/config issue"},
		},
		{
			name:    "generic fallback",
			err:     errors.New("boom"),
			wantAny: []string{"please retry once"},
		},
		{
			name: "http 413 from APIError",
			err: &openai.APIError{
				HTTPStatusCode: 413,
				Message:        "too large",
			},
			wantAny: []string{"too large for the provider (413)"},
		},
		{
			name: "status code embedded in string only",
			err:  errors.New(`error, status code: 502, status: , message: upstream`),
			wantAny: []string{
				"sorry",
				"try me again",
				"one more time",
				"one more ping",
			},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := llmErrorUserMessage(tc.err)
			gotLower := strings.ToLower(got)
			ok := false
			for _, w := range tc.wantAny {
				if strings.Contains(gotLower, strings.ToLower(w)) {
					ok = true
					break
				}
			}
			if !ok {
				t.Fatalf("llmErrorUserMessage()=%q want one of %v", got, tc.wantAny)
			}
		})
	}
}
