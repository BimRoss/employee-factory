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

func TestOutboundNeedsRepair_detectsSpeakerPrefixLeak(t *testing.T) {
	flagged, reason := outboundNeedsRepair("**Garth:** Here is the rewritten Slack reply:")
	if !flagged {
		t.Fatal("expected speaker prefix leak to be flagged")
	}
	if reason == "" {
		t.Fatal("expected reason for speaker prefix leak")
	}
}

func TestOutboundNeedsRepair_detectsAwkwardFirstPersonGrammar(t *testing.T) {
	flagged, reason := outboundNeedsRepair("me is ready to work.")
	if !flagged {
		t.Fatal("expected awkward first-person grammar to be flagged")
	}
	if reason != "awkward_first_person_grammar" {
		t.Fatalf("expected awkward grammar reason, got %q", reason)
	}
}

func TestOutboundNeedsRepair_detectsBracketAssignmentArtifact(t *testing.T) {
	flagged, reason := outboundNeedsRepair("[a=me] Point being, ship it.")
	if !flagged {
		t.Fatal("expected bracket assignment artifact to be flagged")
	}
	if reason != "bracket_assignment_artifact" {
		t.Fatalf("expected bracket assignment artifact reason, got %q", reason)
	}
}
