package slackbot

import (
	"fmt"
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

func TestFormatOutgoingSlackMessage_addressedBareNameGetsMention(t *testing.T) {
	cfg := &config.Config{
		EmployeeID: "tim",
		MultiagentBotUserIDs: map[string]string{
			"ross":  "U0APX108QE7",
			"tim":   "U0AQ10R2H8E",
			"alex":  "U0APSMH05B5",
			"garth": "UGARTH0001",
		},
	}
	in := "I agree. Ross, can you help with this checklist?"
	out := formatOutgoingSlackMessage(in, cfg, "U0AQ10R2H8E")
	if !strings.Contains(out, "<@U0APX108QE7>, can you help with this checklist?") {
		t.Fatalf("expected addressed bare name to become mention, got %q", out)
	}
	if strings.Contains(out, " Ross,") {
		t.Fatalf("expected plain addressed name removed, got %q", out)
	}
}

func TestFormatOutgoingSlackMessage_narrativeNameDoesNotAutoMention(t *testing.T) {
	cfg := &config.Config{
		EmployeeID: "tim",
		MultiagentBotUserIDs: map[string]string{
			"ross":  "U0APX108QE7",
			"tim":   "U0AQ10R2H8E",
			"alex":  "U0APSMH05B5",
			"garth": "UGARTH0001",
		},
	}
	in := "Alex asked Ross to review rollout status."
	out := formatOutgoingSlackMessage(in, cfg, "U0AQ10R2H8E")
	if strings.Contains(out, "<@U0APX108QE7>") {
		t.Fatalf("expected narrative reference not to auto-mention, got %q", out)
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

func TestNormalizeSlackReply_capsLength(t *testing.T) {
	cfg := &config.Config{}
	in := "Line one with useful content.\nLine two with details.\nLine three still allowed.\nLine four still allowed.\nLine five should be dropped."
	out := normalizeSlackReply(in, cfg, "")
	lines := strings.Split(out, "\n")
	if len(lines) > 4 {
		t.Fatalf("expected <=4 lines, got %d: %q", len(lines), out)
	}
	if strings.Contains(out, "Line five") {
		t.Fatalf("expected overflow lines dropped, got %q", out)
	}
}

func TestNormalizeSlackReply_truncationEndsAsCompleteSentence(t *testing.T) {
	cfg := &config.Config{}
	in := strings.Repeat("This is a long sentence without punctuation ", 40) + "final thought"
	out := normalizeSlackReply(in, cfg, "")
	if strings.HasSuffix(out, "...") {
		t.Fatalf("expected no ellipsis truncation tail, got %q", out)
	}
	last := out[len(out)-1]
	if last != '.' && last != '!' && last != '?' && last != '"' && last != '\'' {
		t.Fatalf("expected sentence-safe ending punctuation, got %q", out)
	}
}

func TestNormalizeSlackReply_rewritesSelfNameToFirstPersonGrammar(t *testing.T) {
	cfg := &config.Config{
		EmployeeID: "ross",
	}
	in := "Ross can take this one. If you need logs, ask Ross."
	out := normalizeSlackReply(in, cfg, "")
	if strings.Contains(strings.ToLower(out), "ross") {
		t.Fatalf("expected self-name rewritten, got %q", out)
	}
	if !strings.Contains(out, "I can take this one.") {
		t.Fatalf("expected subject self-reference to use I, got %q", out)
	}
	if !strings.Contains(out, "ask me.") {
		t.Fatalf("expected object self-reference to use me, got %q", out)
	}
}

func TestNormalizeSlackReply_preservesOtherAgentNamesAndMentions(t *testing.T) {
	cfg := &config.Config{
		EmployeeID: "ross",
		MultiagentBotUserIDs: map[string]string{
			"ross":  "U0APX108QE7",
			"tim":   "U0AQ10R2H8E",
			"alex":  "U0APSMH05B5",
			"garth": "UGARTH0001",
		},
	}
	in := "Tim asked Ross to handle this; ping <@U0AQ10R2H8E|Tim> if needed."
	out := normalizeSlackReply(in, cfg, "U0APX108QE7")
	if strings.Contains(strings.ToLower(out), "ross") {
		t.Fatalf("expected self-name rewritten, got %q", out)
	}
	if !strings.Contains(out, "Tim asked me") {
		t.Fatalf("expected self-name rewrite with other names preserved, got %q", out)
	}
	if !strings.Contains(out, "<@U0AQ10R2H8E") {
		t.Fatalf("expected mention token preserved, got %q", out)
	}
}

func TestNormalizeSlackReply_repairsAwkwardMeIsPattern(t *testing.T) {
	cfg := &config.Config{EmployeeID: "ross"}
	in := "me is ready to work."
	out := normalizeSlackReply(in, cfg, "")
	if strings.Contains(strings.ToLower(out), "me is") {
		t.Fatalf("expected awkward first-person grammar repaired, got %q", out)
	}
	if !strings.Contains(out, "I am ready to work.") {
		t.Fatalf("expected repaired grammar, got %q", out)
	}
}

func TestFormatOutgoingSlackMessage_stripsSpeakerPrefixes(t *testing.T) {
	in := "**Garth:** Keep this tight.\nRoss: Next move is ship."
	out := formatOutgoingSlackMessage(in, nil, "")
	if strings.Contains(strings.ToLower(out), "garth:") || strings.Contains(strings.ToLower(out), "ross:") {
		t.Fatalf("expected speaker prefixes removed, got %q", out)
	}
	if !strings.Contains(out, "Keep this tight.") || !strings.Contains(out, "Next move is ship.") {
		t.Fatalf("unexpected sanitized output: %q", out)
	}
}

func TestEnforceMultiagentMentionPolicy_requireHandoff(t *testing.T) {
	cfg := &config.Config{
		EmployeeID: "ross",
		MultiagentBotUserIDs: map[string]string{
			"ross":  "U0APX108QE7",
			"tim":   "U0AQ10R2H8E",
			"alex":  "U0APSMH05B5",
			"garth": "UGARTH0001",
		},
	}
	in := "Keep this scoped and fast."
	out := enforceMultiagentMentionPolicy(in, cfg, "U0APX108QE7", true)
	matches := reSlackMention.FindAllStringSubmatch(out, -1)
	if len(matches) != 1 {
		t.Fatalf("expected exactly one mention, got %d: %q", len(matches), out)
	}
	if matches[0][1] == "U0APX108QE7" {
		t.Fatalf("should not mention self: %q", out)
	}
}

func TestEnforceMultiagentMentionPolicy_noHandoff(t *testing.T) {
	cfg := &config.Config{
		EmployeeID: "tim",
		MultiagentBotUserIDs: map[string]string{
			"ross":  "U0APX108QE7",
			"tim":   "U0AQ10R2H8E",
			"alex":  "U0APSMH05B5",
			"garth": "UGARTH0001",
		},
	}
	in := "Loop back with @ross and @alex, maybe <@UGARTH0001> too."
	out := enforceMultiagentMentionPolicy(in, cfg, "U0AQ10R2H8E", false)
	if strings.Contains(out, "<@U0APX108QE7>") || strings.Contains(out, "<@U0APSMH05B5>") || strings.Contains(out, "<@UGARTH0001>") {
		t.Fatalf("expected squad mentions removed, got %q", out)
	}
	if strings.Contains(strings.ToLower(out), "@ross") || strings.Contains(strings.ToLower(out), "@alex") || strings.Contains(strings.ToLower(out), "@garth") {
		t.Fatalf("expected plain squad mentions removed, got %q", out)
	}
}

func TestEnforceMultiagentMentionPolicy_requireHandoff_singleRossMentionBalanced(t *testing.T) {
	cfg := &config.Config{
		EmployeeID: "tim",
		MultiagentBotUserIDs: map[string]string{
			"ross":  "U0APX108QE7",
			"tim":   "U0AQ10R2H8E",
			"alex":  "U0APSMH05B5",
			"garth": "UGARTH0001",
		},
	}
	seen := make(map[string]bool)
	for i := 0; i < 20; i++ {
		in := fmt.Sprintf("Need a fast check from <@U0APX108QE7> (%d)", i)
		out := enforceMultiagentMentionPolicy(in, cfg, "U0AQ10R2H8E", true)
		matches := reSlackMention.FindAllStringSubmatch(out, -1)
		if len(matches) != 1 {
			t.Fatalf("expected exactly one mention, got %d: %q", len(matches), out)
		}
		uid := matches[0][1]
		if uid == "U0AQ10R2H8E" {
			t.Fatalf("should never mention self: %q", out)
		}
		seen[uid] = true
	}
	if len(seen) < 2 {
		t.Fatalf("expected balanced distribution across mentions, got only %v", seen)
	}
	if !seen["U0APSMH05B5"] && !seen["UGARTH0001"] {
		t.Fatalf("expected at least one non-Ross handoff when seed varies, got %v", seen)
	}
}
