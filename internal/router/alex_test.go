package router

import (
	"strings"
	"testing"
)

func TestWrapAlexUserMessage_addsHintForCloser(t *testing.T) {
	out := WrapAlexUserMessage("How do I handle objections on a sales call?")
	if !strings.Contains(out, "CLOSER") {
		t.Fatalf("expected CLOSER hint, got: %s", out)
	}
	if !strings.Contains(out, "User message:") {
		t.Fatalf("expected user message section")
	}
}

func TestWrapAlexUserMessage_noHint(t *testing.T) {
	out := WrapAlexUserMessage("Hello")
	if strings.Contains(out, "Internal routing hint") {
		t.Fatalf("did not expect hint for generic greeting")
	}
}
