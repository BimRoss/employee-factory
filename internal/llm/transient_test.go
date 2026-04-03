package llm

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/sashabaranov/go-openai"
)

func TestIsTransientLLMError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"request 503", &openai.RequestError{HTTPStatusCode: 503, Err: errors.New("x")}, true},
		{"request 502", &openai.RequestError{HTTPStatusCode: 502, Err: errors.New("x")}, true},
		{"request 429", &openai.RequestError{HTTPStatusCode: 429, Err: errors.New("x")}, true},
		{"request 401", &openai.RequestError{HTTPStatusCode: 401, Err: errors.New("x")}, false},
		{"api 503", &openai.APIError{HTTPStatusCode: 503}, true},
		{"api 400", &openai.APIError{HTTPStatusCode: 400}, false},
		{"chutes body string", fmt.Errorf(`error, status code: 503, body: {"detail":"No instances available (yet) for chute_id='x'"}`), true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsTransientLLMError(tc.err); got != tc.want {
				t.Fatalf("IsTransientLLMError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsProviderTimeoutLLMError(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		err  error
		want bool
	}{
		{"nil", nil, false},
		{"deadline", context.DeadlineExceeded, true},
		{"wrapped deadline", fmt.Errorf("request failed: %w", context.DeadlineExceeded), true},
		{"io timeout text", errors.New("dial tcp: i/o timeout"), true},
		{"client timeout text", errors.New("net/http: request canceled (Client.Timeout exceeded while awaiting headers)"), true},
		{"non-timeout", errors.New("status code: 400"), false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			if got := IsProviderTimeoutLLMError(tc.err); got != tc.want {
				t.Fatalf("IsProviderTimeoutLLMError(%v) = %v, want %v", tc.err, got, tc.want)
			}
		})
	}
}

func TestIsFallbackEligibleLLMError(t *testing.T) {
	t.Parallel()
	if !IsFallbackEligibleLLMError(context.DeadlineExceeded) {
		t.Fatal("expected timeout errors to be fallback-eligible")
	}
	if !IsFallbackEligibleLLMError(&openai.RequestError{HTTPStatusCode: 503, Err: errors.New("x")}) {
		t.Fatal("expected transient 503 errors to be fallback-eligible")
	}
	if IsFallbackEligibleLLMError(errors.New("status code: 400")) {
		t.Fatal("did not expect 400 errors to be fallback-eligible")
	}
}

func TestNewFallbackClientWhenModelsDiffer(t *testing.T) {
	t.Setenv("EMPLOYEE_ID", "alex")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("LLM_MODEL", "primary/model")
	t.Setenv("LLM_FALLBACK_MODEL", "fallback/model")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	llm := New(cfg)
	if llm.fallback == nil {
		t.Fatal("expected non-nil fallback client when LLM_FALLBACK_MODEL differs from LLM_MODEL")
	}
}

func TestNewNoFallbackWhenSameModel(t *testing.T) {
	t.Setenv("EMPLOYEE_ID", "alex")
	t.Setenv("LLM_API_KEY", "test-key")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("LLM_MODEL", "same/model")
	t.Setenv("LLM_FALLBACK_MODEL", "same/model")
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	llm := New(cfg)
	if llm.fallback != nil {
		t.Fatal("expected nil fallback when primary and fallback model ids match")
	}
}
