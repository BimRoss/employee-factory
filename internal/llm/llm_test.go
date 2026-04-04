package llm

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestComposeSystemPrompt_keepsSlackSuffixWhenPersonaTruncated(t *testing.T) {
	persona := strings.Repeat("x", 100)
	suffix := "SLACK_RULES_TAIL"
	out := composeSystemPrompt(persona, suffix, 60)
	if len([]rune(out)) > 60 {
		t.Fatalf("output longer than max: %d", len([]rune(out)))
	}
	if !strings.HasSuffix(out, suffix) {
		t.Fatalf("suffix not preserved at end: %q", out)
	}
}

func TestComposeSystemPrompt_suffixOnlyWhenNoBudget(t *testing.T) {
	suffix := "AB" // 2 runes
	out := composeSystemPrompt("persona", suffix, 2)
	if out != suffix {
		t.Fatalf("expected suffix only, got %q", out)
	}
}

func TestPrimaryAttemptContextSlicesDeadline(t *testing.T) {
	t.Parallel()
	parent, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	attemptCtx, cancelAttempt := primaryAttemptContext(parent, 3)
	defer cancelAttempt()

	d, ok := attemptCtx.Deadline()
	if !ok {
		t.Fatal("expected attempt context to have a deadline")
	}
	remaining := time.Until(d)
	if remaining > 16*time.Second {
		t.Fatalf("expected per-attempt cap around 15s, got %s", remaining)
	}
	if remaining < 9*time.Second {
		t.Fatalf("expected attempt budget to reserve room for retries, got %s", remaining)
	}
}

func TestPrimaryAttemptContextNoSliceWhenSingleAttempt(t *testing.T) {
	t.Parallel()
	parent, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	attemptCtx, cancelAttempt := primaryAttemptContext(parent, 1)
	defer cancelAttempt()
	if attemptCtx != parent {
		t.Fatal("expected single-attempt context to remain unchanged")
	}
}
