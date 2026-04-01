package slackbot

import (
	"math"
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

func TestCountSquadMessagesInRun(t *testing.T) {
	squad := map[string]bool{"U1": true, "U2": true}
	msgs := []slack.Message{
		{Msg: slack.Msg{User: "UH", Text: "hi"}},
		{Msg: slack.Msg{User: "U1", Text: "a"}},
		{Msg: slack.Msg{User: "U2", Text: "b"}},
		{Msg: slack.Msg{User: "U1", Text: "c"}},
	}
	if n := countSquadMessagesInRun(msgs, squad, 3); n != 3 {
		t.Fatalf("want 3 squad in run after human, got %d", n)
	}
	if n := countSquadMessagesInRun(msgs, squad, 2); n != 2 {
		t.Fatalf("want 2 through idx 2, got %d", n)
	}
	allSquad := []slack.Message{
		{Msg: slack.Msg{User: "U1"}},
		{Msg: slack.Msg{User: "U2"}},
	}
	if n := countSquadMessagesInRun(allSquad, squad, 1); n != 2 {
		t.Fatalf("no human in window: want 2, got %d", n)
	}
}

func TestSampleBroadcastRoundCount_meanMessagesNearTarget(t *testing.T) {
	const iters = 8000
	participants := 4
	target := 10
	maxR := 6
	var totalMsgs int
	for i := 0; i < iters; i++ {
		r := sampleBroadcastRoundCount(participants, target, maxR)
		totalMsgs += r * participants
	}
	mean := float64(totalMsgs) / float64(iters)
	if math.Abs(mean-float64(target)) > 1.5 {
		t.Fatalf("mean squad messages %.2f want ~%d (4 participants)", mean, target)
	}
}

func TestBroadcastMultiagentTrigger(t *testing.T) {
	if !broadcastMultiagentTrigger("<!everyone> hi") {
		t.Fatal("everyone token")
	}
	if !broadcastMultiagentTrigger("x <!channel|@channel> y") {
		t.Fatal("channel token")
	}
	if broadcastMultiagentTrigger("Hey everyone") {
		t.Fatal("plain everyone text does not start broadcast (only coordinator + Slack tokens)")
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
