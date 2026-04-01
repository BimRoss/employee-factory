package slackbot

import (
	"strings"
	"testing"

	"github.com/bimross/employee-factory/internal/config"
)

func TestFormatOutgoingSlackMessage_GitHubBold(t *testing.T) {
	in := "Start with **health checks** and **telemetry**."
	out := formatOutgoingSlackMessage(in, nil, "")
	want := "Start with *health checks* and *telemetry*."
	if out != want {
		t.Fatalf("got %q want %q", out, want)
	}
}

func TestFormatOutgoingSlackMessage_strayDoubleStars(t *testing.T) {
	in := "**Next: Subnet Signal**—measure before build."
	out := formatOutgoingSlackMessage(in, nil, "")
	if strings.Contains(out, "**") {
		t.Fatalf("expected no ** left, got %q", out)
	}
	if !strings.Contains(out, "Next: Subnet Signal") {
		t.Fatalf("got %q", out)
	}
}

func TestFormatOutgoingSlackMessage_heading(t *testing.T) {
	in := "## Next step\nDo the thing."
	out := formatOutgoingSlackMessage(in, nil, "")
	if strings.Contains(out, "#") {
		t.Fatalf("expected hashes stripped, got %q", out)
	}
	if !strings.Contains(out, "Next step") || !strings.Contains(out, "Do the thing") {
		t.Fatalf("unexpected: %q", out)
	}
}

func TestFormatOutgoingSlackMessage_link(t *testing.T) {
	in := "See [docs](https://example.com) for more."
	out := formatOutgoingSlackMessage(in, nil, "")
	if out != "See docs for more." {
		t.Fatalf("got %q", out)
	}
}

func TestFormatOutgoingSlackMessage_squadMentions(t *testing.T) {
	cfg := &config.Config{
		MultiagentBotUserIDs: map[string]string{
			"ross":  "U0APX108QE7",
			"tim":   "U0AQ10R2H8E",
			"alex":  "U0APSMH05B5",
			"garth": "UGARTH0001",
		},
	}
	in := "@ross agrees — ask @TIM, @Alex, and @garth too."
	out := formatOutgoingSlackMessage(in, cfg, "")
	if !strings.Contains(out, "<@U0APX108QE7>") || !strings.Contains(out, "<@U0AQ10R2H8E>") || !strings.Contains(out, "<@U0APSMH05B5>") || !strings.Contains(out, "<@UGARTH0001>") {
		t.Fatalf("expected Slack mention tokens, got %q", out)
	}
	if strings.Contains(out, "@ross") || strings.Contains(out, "@TIM") {
		t.Fatalf("expected @name replaced, got %q", out)
	}
}

func TestFormatOutgoingSlackMessage_stripsSelfMention(t *testing.T) {
	cfg := &config.Config{
		EmployeeID: "tim",
		MultiagentBotUserIDs: map[string]string{
			"ross":  "U0APX108QE7",
			"tim":   "U0AQ10R2H8E",
			"alex":  "U0APSMH05B5",
			"garth": "UGARTH0001",
		},
	}
	in := "Hey @ross — @Tim, one question? Also <@U0AQ10R2H8E|Tim> ping."
	out := formatOutgoingSlackMessage(in, cfg, "U0AQ10R2H8E")
	if strings.Contains(out, "U0AQ10R2H8E") || strings.Contains(out, "@tim") || strings.Contains(out, "@Tim") {
		t.Fatalf("expected self-mentions stripped, got %q", out)
	}
	if !strings.Contains(out, "<@U0APX108QE7>") {
		t.Fatalf("expected other squad mention preserved, got %q", out)
	}
}
