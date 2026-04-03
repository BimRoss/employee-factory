package slackbot

import "testing"

func TestIsDirectlyHostileToAgent(t *testing.T) {
	cases := []string{
		"fuck off",
		"you suck",
		"shut up and listen",
	}
	for _, tc := range cases {
		if !isDirectlyHostileToAgent(tc) {
			t.Fatalf("expected hostile detection true for %q", tc)
		}
	}
}

func TestIsDirectlyHostileToAgent_nonHostile(t *testing.T) {
	cases := []string{
		"what should we ship next?",
		"this plan is weak, can you tighten it?",
		"I disagree with that suggestion",
	}
	for _, tc := range cases {
		if isDirectlyHostileToAgent(tc) {
			t.Fatalf("expected hostile detection false for %q", tc)
		}
	}
}

func TestPrependHostilityCue(t *testing.T) {
	payload := "Current user question: what next?"
	out := prependHostilityCue(payload, "fuck off")
	if out == payload {
		t.Fatal("expected hostility cue to be prepended")
	}
	if out[:len(hostilitySystemCue)] != hostilitySystemCue {
		t.Fatalf("expected hostility cue prefix, got %q", out)
	}
}

func TestPrependHostilityCue_nonHostilePassthrough(t *testing.T) {
	payload := "Current user question: what next?"
	out := prependHostilityCue(payload, "what should we do next?")
	if out != payload {
		t.Fatalf("expected passthrough payload, got %q", out)
	}
}
