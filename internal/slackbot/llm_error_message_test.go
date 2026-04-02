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
		name string
		err  error
		want string
	}{
		{
			name: "deadline exceeded",
			err:  context.DeadlineExceeded,
			want: "timeout",
		},
		{
			name: "rate limited",
			err: &openai.RequestError{
				HTTPStatusCode: 429,
				Err:            errors.New("too many requests"),
			},
			want: "rate-limiting",
		},
		{
			name: "auth failure",
			err: &openai.RequestError{
				HTTPStatusCode: 401,
				Err:            errors.New("unauthorized"),
			},
			want: "auth/config issue",
		},
		{
			name: "generic fallback",
			err:  errors.New("boom"),
			want: "Please retry once",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := llmErrorUserMessage(tc.err)
			if !strings.Contains(strings.ToLower(got), strings.ToLower(tc.want)) {
				t.Fatalf("llmErrorUserMessage()=%q want substring %q", got, tc.want)
			}
		})
	}
}
