package slackbot

import (
	"testing"

	"github.com/bimross/employee-factory/internal/config"
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
