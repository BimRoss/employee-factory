package slackbot

import (
	"context"
	"testing"

	"github.com/bimross/employee-factory/internal/config"
)

func TestApplyAvailabilityRouter_Disabled(t *testing.T) {
	b := &Bot{
		cfg: &config.Config{
			EmployeeID:                   "ross",
			RouterAvailabilityEnabled:    false,
			RouterLogOnly:                false,
			SlackOutboundWindowSec:       60,
			SlackOutboundMaxPerWindow:    10,
			MultiagentBroadcastRounds:    1,
			MultiagentSlotSoftTimeoutSec: 12,
		},
	}
	handled := b.applyAvailabilityRouter(context.Background(), availabilityRouteEvent{
		Path:    "message",
		RawText: "I need to step away for the afternoon.",
		Phase:   routerPhaseIngress,
	})
	if handled {
		t.Fatal("router should not handle when both router flags are disabled")
	}
}

func TestApplyAvailabilityRouter_LogOnlyDoesNotSuppress(t *testing.T) {
	b := &Bot{
		cfg: &config.Config{
			EmployeeID:                "ross",
			RouterAvailabilityEnabled: false,
			RouterLogOnly:             true,
		},
	}
	handled := b.applyAvailabilityRouter(context.Background(), availabilityRouteEvent{
		Path:      "message",
		Channel:   "C123",
		MessageTS: "1740000000.000001",
		RawText:   "AFK for now, back later.",
		Phase:     routerPhaseIngress,
	})
	if handled {
		t.Fatal("router should not suppress in log-only mode")
	}
}

func TestApplyAvailabilityRouter_EnforceSuppresses(t *testing.T) {
	b := &Bot{
		cfg: &config.Config{
			EmployeeID:                "ross",
			RouterAvailabilityEnabled: true,
			RouterLogOnly:             false,
		},
	}
	handled := b.applyAvailabilityRouter(context.Background(), availabilityRouteEvent{
		Path:      "post_llm_channel",
		MessageTS: "1740000000.000002",
		RawText:   "Signing off for today.",
		Phase:     routerPhasePreLLM,
		// Empty channel intentionally avoids network calls in this unit test.
	})
	if !handled {
		t.Fatal("router should suppress when enforcement is enabled and cue is matched")
	}
}

func TestRouterAnchorPrefersThreadTS(t *testing.T) {
	if got := routerAnchor("1740000000.000100", "1740000000.000050"); got != "1740000000.000100" {
		t.Fatalf("expected thread ts anchor, got %q", got)
	}
	if got := routerAnchor("", "1740000000.000050"); got != "1740000000.000050" {
		t.Fatalf("expected message ts anchor fallback, got %q", got)
	}
}
