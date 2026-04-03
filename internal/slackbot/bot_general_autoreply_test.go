package slackbot

import (
	"testing"
	"time"

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

func TestGeneralAutoReplyNoSquadMentions(t *testing.T) {
	cfg := &config.Config{
		MultiagentBotUserIDs: map[string]string{
			"ross": "UROSS",
			"tim":  "UTIM",
		},
		MultiagentOrder: []string{"ross", "tim"},
	}

	if !generalAutoReplyNoSquadMentions("who is ready to work", cfg) {
		t.Fatal("expected plain message with no mentions to be eligible for random auto-reply path")
	}

	if generalAutoReplyNoSquadMentions("what do you think <@UROSS>?", cfg) {
		t.Fatal("expected explicit squad mention to disable random auto-reply path")
	}
}

func TestGeneralAutoReplyFailoverDelay_DeterministicByOrder(t *testing.T) {
	order := []string{"ross", "tim", "alex", "garth"}
	if got := generalAutoReplyFailoverDelay("ross", order); got != 5*time.Second {
		t.Fatalf("ross delay mismatch: got=%s", got)
	}
	if got := generalAutoReplyFailoverDelay("tim", order); got != 6*time.Second {
		t.Fatalf("tim delay mismatch: got=%s", got)
	}
	if got := generalAutoReplyFailoverDelay("garth", order); got != 8*time.Second {
		t.Fatalf("garth delay mismatch: got=%s", got)
	}
	if got := generalAutoReplyFailoverDelay("unknown", order); got != 9*time.Second {
		t.Fatalf("unknown delay mismatch: got=%s", got)
	}
}

func TestGeneralAutoReplyWinnerShouldPost(t *testing.T) {
	if !generalAutoReplyWinnerShouldPost(generalAutoReplyClaimAcquired) {
		t.Fatal("winner should post when claim acquired")
	}
	if !generalAutoReplyWinnerShouldPost(generalAutoReplyClaimBackendDown) {
		t.Fatal("winner should post when claim backend unavailable")
	}
	if generalAutoReplyWinnerShouldPost(generalAutoReplyClaimAlreadyClaimed) {
		t.Fatal("winner should not post when already claimed")
	}
}

func TestGeneralAutoReplyFailoverShouldPost(t *testing.T) {
	if !generalAutoReplyFailoverShouldPost(generalAutoReplyClaimAcquired) {
		t.Fatal("failover should post only when claim acquired")
	}
	if generalAutoReplyFailoverShouldPost(generalAutoReplyClaimBackendDown) {
		t.Fatal("failover should not post when claim backend unavailable")
	}
	if generalAutoReplyFailoverShouldPost(generalAutoReplyClaimAlreadyClaimed) {
		t.Fatal("failover should not post when already claimed")
	}
}

func TestShouldSkipGeneralAutoReply_ClosureIntent(t *testing.T) {
	cases := []string{
		"I should be good thanks garth",
		"all good thanks",
		"No problem",
		"that helps, appreciate it",
	}
	for _, tc := range cases {
		skip, reason := shouldSkipGeneralAutoReply(tc)
		if !skip {
			t.Fatalf("expected skip for closure text: %q", tc)
		}
		if reason != "closure_intent" {
			t.Fatalf("expected closure_intent reason for %q, got %q", tc, reason)
		}
	}
}

func TestShouldSkipGeneralAutoReply_AskSignalOverridesClosure(t *testing.T) {
	cases := []string{
		"thanks, can you draft a quick experiment?",
		"all good, but what do you think we should ship next?",
		"appreciate it - could you outline the next step?",
	}
	for _, tc := range cases {
		skip, reason := shouldSkipGeneralAutoReply(tc)
		if skip {
			t.Fatalf("did not expect skip for ask text: %q reason=%q", tc, reason)
		}
	}
}
