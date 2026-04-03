package slackbot

import (
	"testing"

	"github.com/slack-go/slack"
)

func TestShouldEnforceGrantBoundary(t *testing.T) {
	if shouldEnforceGrantBoundary("UGRANT", "UGRANT") {
		t.Fatal("grant trigger should not enforce boundary")
	}
	if !shouldEnforceGrantBoundary("UAGENT", "UGRANT") {
		t.Fatal("agent trigger should enforce boundary")
	}
}

func TestClipMessagesToGrantBoundary_hardCut(t *testing.T) {
	msgs := []slack.Message{
		{Msg: slack.Msg{User: "UHUMAN", Text: "older"}},
		{Msg: slack.Msg{User: "UGRANT", Text: "anchor"}},
		{Msg: slack.Msg{User: "UAGENT", Text: "after anchor"}},
	}
	out := clipMessagesToGrantBoundary(msgs, "UGRANT", true)
	if len(out) != 2 {
		t.Fatalf("expected 2 messages through grant anchor, got %d", len(out))
	}
	if out[1].User != "UGRANT" {
		t.Fatalf("expected last retained message to be grant, got %q", out[1].User)
	}
}

func TestClipMessagesToGrantBoundary_noGrantMessageYieldsEmpty(t *testing.T) {
	msgs := []slack.Message{
		{Msg: slack.Msg{User: "UAGENT1", Text: "one"}},
		{Msg: slack.Msg{User: "UAGENT2", Text: "two"}},
	}
	out := clipMessagesToGrantBoundary(msgs, "UGRANT", true)
	if len(out) != 0 {
		t.Fatalf("expected empty context when no grant anchor exists, got %d", len(out))
	}
}
