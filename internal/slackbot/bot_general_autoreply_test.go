package slackbot

import (
	"testing"

	"github.com/bimross/employee-factory/internal/config"
)

func TestGeneralAutoReplyEligible_grantOnly(t *testing.T) {
	cfg := &config.Config{
		MultiagentEnabled:                 true,
		MultiagentGeneralAutoReplyEnabled: true,
		SlackGeneralChannelID:             "CGENERAL",
		ChatAllowedUserID:                 "UGRANT",
		MultiagentBotUserIDs: map[string]string{
			"ross": "UROSS",
			"tim":  "UTIM",
		},
		MultiagentOrder: []string{"ross", "tim"},
	}
	if !generalAutoReplyEligible(cfg, "CGENERAL", "UGRANT") {
		t.Fatal("expected Grant message in #general to be eligible")
	}
	if generalAutoReplyEligible(cfg, "CGENERAL", "UOTHER") {
		t.Fatal("expected non-Grant user to be ineligible")
	}
}

func TestGeneralAutoReplyEligible_channelGate(t *testing.T) {
	cfg := &config.Config{
		MultiagentEnabled:                 true,
		MultiagentGeneralAutoReplyEnabled: true,
		SlackGeneralChannelID:             "CGENERAL",
		ChatAllowedUserID:                 "UGRANT",
		MultiagentBotUserIDs: map[string]string{
			"ross": "UROSS",
			"tim":  "UTIM",
		},
		MultiagentOrder: []string{"ross", "tim"},
	}
	if generalAutoReplyEligible(cfg, "CRANDOM", "UGRANT") {
		t.Fatal("expected non-general channel to be ineligible")
	}
}

func TestGeneralAutoReplyProbabilityAndWinner_uniqueness(t *testing.T) {
	order := []string{"ross", "tim", "alex", "garth"}
	if shouldTriggerGeneralAutoReply("1743491234.567890", order, "secret", 0.0) {
		t.Fatal("probability 0 should never trigger")
	}
	winner := selectSingleGeneralParticipant("1743491234.567890", order, "secret")
	if winner == "" {
		t.Fatal("expected winner")
	}
	n := 0
	for _, key := range order {
		if key == winner {
			n++
		}
	}
	if n != 1 {
		t.Fatalf("winner should appear exactly once in order: winner=%q count=%d", winner, n)
	}
}
