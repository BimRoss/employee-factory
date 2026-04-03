package router

import (
	"strings"
	"testing"
)

func TestClassifyAvailability_AvailabilityCue(t *testing.T) {
	decision := ClassifyAvailability("Woof I might need to step away for the afternoon guys")
	if decision.Intent != AvailabilityIntentAvailability {
		t.Fatalf("expected availability intent, got %q", decision.Intent)
	}
	if decision.Action != AvailabilityActionAckOnly {
		t.Fatalf("expected ack_only action, got %q", decision.Action)
	}
	if decision.Confidence < 0.95 {
		t.Fatalf("expected high confidence, got %.2f", decision.Confidence)
	}
	if len(decision.MatchedTerms) == 0 {
		t.Fatal("expected matched terms for availability cue")
	}
}

func TestClassifyAvailability_SignoffWinsPrecedence(t *testing.T) {
	decision := ClassifyAvailability("I am AFK and signing off now.")
	if decision.Intent != AvailabilityIntentSignoff {
		t.Fatalf("expected signoff intent, got %q", decision.Intent)
	}
	if decision.Action != AvailabilityActionAckOnly {
		t.Fatalf("expected ack_only action, got %q", decision.Action)
	}
}

func TestClassifyAvailability_NormalMessage(t *testing.T) {
	decision := ClassifyAvailability("Can you scope the next experiment for this channel?")
	if decision.Intent != AvailabilityIntentNormal {
		t.Fatalf("expected normal intent, got %q", decision.Intent)
	}
	if decision.Action != AvailabilityActionNormal {
		t.Fatalf("expected normal action, got %q", decision.Action)
	}
}

func TestClassifyAvailability_AvoidsOfflineArchitectureFalsePositive(t *testing.T) {
	decision := ClassifyAvailability("We should evaluate offline-first architecture for the mobile app.")
	if decision.Intent != AvailabilityIntentNormal {
		t.Fatalf("expected normal intent, got %q", decision.Intent)
	}
}

func TestBuildAsyncSafeAck_Contract(t *testing.T) {
	msg := BuildAsyncSafeAck(AvailabilityIntentAvailability)
	if strings.TrimSpace(msg) == "" {
		t.Fatal("expected non-empty async-safe availability acknowledgment")
	}
	if strings.Contains(msg, "@") {
		t.Fatalf("router ack should not contain mentions: %q", msg)
	}
	if strings.Count(msg, "?") > 0 {
		t.Fatalf("router ack should not ask new questions: %q", msg)
	}
}

func TestClassifyPresenceCheck_DirectPing(t *testing.T) {
	decision := ClassifyPresenceCheck("@everyone are you guys online")
	if !decision.IsPresenceCheck {
		t.Fatalf("expected presence check, got %+v", decision)
	}
	if decision.Confidence < 0.95 {
		t.Fatalf("expected high confidence, got %.2f", decision.Confidence)
	}
	if len(decision.MatchedTerms) == 0 {
		t.Fatal("expected matched terms for presence check")
	}
}

func TestClassifyPresenceCheck_IgnoresOfflineArchitecture(t *testing.T) {
	decision := ClassifyPresenceCheck("We should evaluate offline-first architecture for this feature.")
	if decision.IsPresenceCheck {
		t.Fatalf("expected non-presence decision, got %+v", decision)
	}
}
