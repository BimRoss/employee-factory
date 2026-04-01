package slackbot

import "testing"

func TestOutboundNeedsRepair_detectsInternalArtifact(t *testing.T) {
	flagged, reason := outboundNeedsRepair("tim-systems-delegation\nCurrent state: Revenue: $1M")
	if !flagged {
		t.Fatal("expected repair gate to flag internal artifact")
	}
	if reason == "" {
		t.Fatal("expected non-empty repair reason")
	}
}

func TestOutboundNeedsRepair_detectsDanglingTail(t *testing.T) {
	flagged, _ := outboundNeedsRepair("One more thing and")
	if !flagged {
		t.Fatal("expected dangling tail to be flagged")
	}
}

func TestFallbackCompleteReply_stripsArtifactsAndCompletesSentence(t *testing.T) {
	out := fallbackCompleteReply("CLOSER\ntim-systems-delegation\nShip this now")
	if out != "Ship this now." {
		t.Fatalf("unexpected fallback output: %q", out)
	}
}
