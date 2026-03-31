package llm

import (
	"strings"
	"testing"
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
