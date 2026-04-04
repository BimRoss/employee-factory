package slackbot

import "testing"

func TestResolvePresentation_ChatDefaultsToPlainText(t *testing.T) {
	mode, reason := ResolvePresentation(ResponseKindChat, PresentationOptions{})
	if mode != PresentationModePlainText {
		t.Fatalf("mode mismatch: got=%s", mode)
	}
	if reason == "" {
		t.Fatal("expected non-empty reason")
	}
}

func TestResolvePresentation_BoundedMetricsUseBlocksWhenEnabled(t *testing.T) {
	mode, _ := ResolvePresentation(ResponseKindOpsMetrics, PresentationOptions{
		EnableBlocks:  true,
		MaxBlockItems: 8,
		ItemCount:     4,
		HasStructured: true,
	})
	if mode != PresentationModeSlackBlocks {
		t.Fatalf("mode mismatch: got=%s want=%s", mode, PresentationModeSlackBlocks)
	}
}

func TestResolvePresentation_StructuredJSONFencedWhenAuto(t *testing.T) {
	mode, _ := ResolvePresentation(ResponseKindStructuredJSON, PresentationOptions{
		JSONMode: "auto",
	})
	if mode != PresentationModeFencedJSON {
		t.Fatalf("mode mismatch: got=%s want=%s", mode, PresentationModeFencedJSON)
	}
}

func TestResolvePresentation_ForceTextWins(t *testing.T) {
	mode, _ := ResolvePresentation(ResponseKindOpsWaitlist, PresentationOptions{
		ForceText:    true,
		EnableBlocks: true,
		JSONMode:     "force_for_structured",
	})
	if mode != PresentationModePlainText {
		t.Fatalf("mode mismatch: got=%s want=%s", mode, PresentationModePlainText)
	}
}
