package slackbot

import (
	"strings"
	"testing"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/slack-go/slack"
)

func TestShouldTriggerRethinkCue_directChallenge(t *testing.T) {
	user := "Alex, you're wrong. We already proved product volume."
	prior := "Lead gen is the bottleneck right now."
	if !shouldTriggerRethinkCue(user, prior) {
		t.Fatalf("expected rethink trigger for direct challenge")
	}
}

func TestShouldTriggerRethinkCue_semanticOverlapChallenge(t *testing.T) {
	user := "Actually the demo loop isn't the issue; we already have volume."
	prior := "The bottleneck is demo volume in the lead gen automation loop."
	if !shouldTriggerRethinkCue(user, prior) {
		t.Fatalf("expected rethink trigger for semantic contradiction overlap")
	}
}

func TestShouldTriggerRethinkCue_nonContradictory(t *testing.T) {
	user := "Can you outline the next two steps?"
	prior := "Lead gen is the bottleneck right now."
	if shouldTriggerRethinkCue(user, prior) {
		t.Fatalf("expected no rethink trigger for neutral follow-up")
	}
}

func TestPrependRethinkCue(t *testing.T) {
	payload := "Current user question: what should we do next?"
	out := prependRethinkCue(payload, "you're wrong", "lead gen is the bottleneck")
	if !strings.HasPrefix(out, rethinkSystemCue) {
		t.Fatalf("expected rethink cue prefix, got: %q", out)
	}
	if !strings.Contains(out, payload) {
		t.Fatalf("expected original payload preserved, got: %q", out)
	}
}

func TestLatestPriorEmployeeMessageInThread(t *testing.T) {
	cfg := &config.Config{
		EmployeeID: "alex",
		MultiagentBotUserIDs: map[string]string{
			"alex": "UALEX",
			"tim":  "UTIM",
		},
	}
	msgs := []slack.Message{
		{Msg: slack.Msg{Timestamp: "100.1", User: "UGRANT", Text: "If you were CEO what now?"}},
		{Msg: slack.Msg{Timestamp: "100.2", User: "UALEX", Text: "Lead gen is the bottleneck."}},
		{Msg: slack.Msg{Timestamp: "100.3", User: "UGRANT", Text: "You're wrong, we already have volume."}},
	}
	got := latestPriorEmployeeMessageInThread(msgs, "100.3", "alex", "UALEX", cfg)
	if got != "Lead gen is the bottleneck." {
		t.Fatalf("unexpected prior self message: %q", got)
	}
}

