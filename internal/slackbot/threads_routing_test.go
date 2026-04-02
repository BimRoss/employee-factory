package slackbot

import (
	"strings"
	"testing"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/slack-go/slack"
)

func TestThreadRoutingShouldReply(t *testing.T) {
	t.Parallel()
	if !threadRoutingShouldReply("ross", "ross", nil) {
		t.Fatal("owner plain text")
	}
	if threadRoutingShouldReply("tim", "ross", nil) {
		t.Fatal("non-owner should not reply without mentions")
	}
	if !threadRoutingShouldReply("tim", "ross", []string{"tim"}) {
		t.Fatal("mentioned should reply")
	}
	if threadRoutingShouldReply("ross", "ross", []string{"tim"}) {
		t.Fatal("owner should not reply when mentions target others only")
	}
}

func TestSquadKeyForSlackUser(t *testing.T) {
	t.Parallel()
	cfg := &config.Config{
		MultiagentBotUserIDs: map[string]string{
			"ross": "U111",
			"alex": "U222",
		},
	}
	if k, ok := squadKeyForSlackUser(cfg, "U111"); !ok || k != "ross" {
		t.Fatalf("got %q %v", k, ok)
	}
	if _, ok := squadKeyForSlackUser(cfg, "U999"); ok {
		t.Fatal("expected miss")
	}
}

func TestThreadTranscriptBefore_includesRecencyWeights(t *testing.T) {
	cfg := &config.Config{
		LLMContextWeightDecay:  0.5,
		LLMContextWeightWindow: 3,
		MultiagentBotUserIDs: map[string]string{
			"ross": "U111",
		},
	}
	msgs := []slack.Message{
		{Msg: slack.Msg{Timestamp: "1.000", User: "UHUMAN", Text: "first"}},
		{Msg: slack.Msg{Timestamp: "2.000", User: "U111", Text: "second"}},
		{Msg: slack.Msg{Timestamp: "3.000", User: "UHUMAN", Text: "third"}},
	}
	out := threadTranscriptBefore(cfg, "U999", msgs, "1.000", "9.000", 5000)
	if !strings.Contains(out, "[w=1.00]") || !strings.Contains(out, "[w=0.50]") {
		t.Fatalf("expected weighted context markers, got %q", out)
	}
}
