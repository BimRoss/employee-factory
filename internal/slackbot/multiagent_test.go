package slackbot

import (
	"testing"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/slack-go/slack"
)

func testCfgSquad() *config.Config {
	return &config.Config{
		MultiagentEnabled: true,
		MultiagentBotUserIDs: map[string]string{
			"ross": "UROSS001",
			"tim":  "UTIM002",
			"alex": "UALEX003",
		},
		MultiagentOrder: []string{"ross", "tim", "alex"},
	}
}

func TestParseMentionedUserIDs(t *testing.T) {
	text := "Hi <@UALEX003> and <@UTIM002> <@UROSS001> repeat <@UALEX003>"
	got := parseMentionedUserIDs(text)
	want := []string{"UALEX003", "UTIM002", "UROSS001"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestMentionedSquadKeys_order(t *testing.T) {
	cfg := testCfgSquad()
	raw := "<@UTIM002> <@UROSS001> what next?"
	got := mentionedSquadKeys(raw, cfg)
	want := []string{"ross", "tim"}
	if len(got) != len(want) {
		t.Fatalf("got %v want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("idx %d: got %q want %q", i, got[i], want[i])
		}
	}
}

func TestBuildSlots_rounds(t *testing.T) {
	cfg := testCfgSquad()
	participants := []string{"ross", "tim"}
	slots := buildSlots(participants, 2, cfg.MultiagentBotUserIDs)
	want := []string{"UROSS001", "UTIM002", "UROSS001", "UTIM002"}
	if len(slots) != len(want) {
		t.Fatalf("got %v want %v", slots, want)
	}
	for i := range want {
		if slots[i] != want[i] {
			t.Fatalf("idx %d: got %q want %q", i, slots[i], want[i])
		}
	}
}

func TestEveryoneTriggerTwoRounds(t *testing.T) {
	if !everyoneTriggerTwoRounds("Hey everyone — thoughts?") {
		t.Fatal("expected natural language everyone")
	}
	if !everyoneTriggerTwoRounds("Ping <!channel> please") {
		t.Fatal("expected channel token")
	}
	if !everyoneTriggerTwoRounds("Use <!everyone> for this") {
		t.Fatal("expected everyone token")
	}
	if everyoneTriggerTwoRounds("everything is fine") {
		t.Fatal("should not match everything")
	}
}

func TestStripSquadUserMentions(t *testing.T) {
	cfg := testCfgSquad()
	squad := squadUserIDSet(cfg)
	text := "<@UROSS001> <@UTIM002> hello <!channel>"
	out := stripSquadUserMentions(text, squad)
	if out != "hello" {
		t.Fatalf("got %q want %q", out, "hello")
	}
}

func TestPrefixMatchesSquadSlots(t *testing.T) {
	slots := []string{"U1", "U2", "U3"}
	msgs := []slack.Message{
		{Msg: slack.Msg{User: "U1"}},
		{Msg: slack.Msg{User: "U2"}},
	}
	if !prefixMatchesSquadSlots(msgs, slots, 2) {
		t.Fatal("expected match for k=2")
	}
	if prefixMatchesSquadSlots(msgs, slots, 3) {
		t.Fatal("should not match k=3")
	}
	if !prefixMatchesSquadSlots(nil, slots, 0) {
		t.Fatal("k=0 empty ok")
	}
}
