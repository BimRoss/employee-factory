package config

import "testing"

func TestGeneralAutoReactionDefaults(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "ross:U1,tim:U2")
	t.Setenv("MULTIAGENT_ORDER", "ross,tim")
	t.Setenv("SLACK_GENERAL_CHANNEL_ID", "")
	t.Setenv("MULTIAGENT_GENERAL_AUTO_REACTION_ENABLED", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SlackGeneralChannelID != "" {
		t.Fatalf("SlackGeneralChannelID: got %q want empty", cfg.SlackGeneralChannelID)
	}
	if cfg.MultiagentGeneralAutoReactionEnabled {
		t.Fatal("MultiagentGeneralAutoReactionEnabled: expected false by default")
	}
	if cfg.MultiagentSlotSoftTimeoutSec != 12 {
		t.Fatalf("MultiagentSlotSoftTimeoutSec: got %d want 12", cfg.MultiagentSlotSoftTimeoutSec)
	}
	if !cfg.MultiagentAllowDegradedStart {
		t.Fatal("MultiagentAllowDegradedStart: expected true by default")
	}
	if cfg.LLMFallbackTimeoutSec != 8 {
		t.Fatalf("LLMFallbackTimeoutSec: got %d want 8", cfg.LLMFallbackTimeoutSec)
	}
}

func TestGeneralAutoReactionEnv(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "ross:U1,tim:U2")
	t.Setenv("MULTIAGENT_ORDER", "ross,tim")
	t.Setenv("SLACK_GENERAL_CHANNEL_ID", "CGENERAL")
	t.Setenv("MULTIAGENT_GENERAL_AUTO_REACTION_ENABLED", "true")
	t.Setenv("MULTIAGENT_SLOT_SOFT_TIMEOUT_SEC", "7")
	t.Setenv("MULTIAGENT_ALLOW_DEGRADED_START", "false")
	t.Setenv("LLM_FALLBACK_TIMEOUT_SEC", "3")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SlackGeneralChannelID != "CGENERAL" {
		t.Fatalf("SlackGeneralChannelID: got %q", cfg.SlackGeneralChannelID)
	}
	if !cfg.MultiagentGeneralAutoReactionEnabled {
		t.Fatal("MultiagentGeneralAutoReactionEnabled: expected true")
	}
	if cfg.MultiagentSlotSoftTimeoutSec != 7 {
		t.Fatalf("MultiagentSlotSoftTimeoutSec: got %d want 7", cfg.MultiagentSlotSoftTimeoutSec)
	}
	if cfg.MultiagentAllowDegradedStart {
		t.Fatal("MultiagentAllowDegradedStart: expected false")
	}
	if cfg.LLMFallbackTimeoutSec != 3 {
		t.Fatalf("LLMFallbackTimeoutSec: got %d want 3", cfg.LLMFallbackTimeoutSec)
	}
}

func TestMultiagentChatterCap_Default25Percent(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "ross:U1,tim:U2")
	t.Setenv("MULTIAGENT_ORDER", "ross,tim")
	t.Setenv("MULTIAGENT_HANDOFF_MIN_PROBABILITY", "0.25")
	t.Setenv("MULTIAGENT_HANDOFF_MAX_PROBABILITY", "0.75")
	t.Setenv("MULTIAGENT_BROADCAST_HANDOFF_PROBABILITY", "0.35")
	t.Setenv("MULTIAGENT_BROADCAST_BRANCHING_HANDOFF_PROBABILITY", "0.60")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MultiagentChatterCap != 0.25 {
		t.Fatalf("MultiagentChatterCap: got %.2f want 0.25", cfg.MultiagentChatterCap)
	}
	if cfg.MultiagentHandoffMaxProbability != 0.25 {
		t.Fatalf("MultiagentHandoffMaxProbability: got %.2f want 0.25", cfg.MultiagentHandoffMaxProbability)
	}
	if cfg.MultiagentBroadcastHandoffProbability != 0.25 {
		t.Fatalf("MultiagentBroadcastHandoffProbability: got %.2f want 0.25", cfg.MultiagentBroadcastHandoffProbability)
	}
	if cfg.MultiagentBroadcastBranchingHandoffProbability != 0.25 {
		t.Fatalf("MultiagentBroadcastBranchingHandoffProbability: got %.2f want 0.25", cfg.MultiagentBroadcastBranchingHandoffProbability)
	}
}

func TestMultiagentChatterCap_Override(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "ross:U1,tim:U2")
	t.Setenv("MULTIAGENT_ORDER", "ross,tim")
	t.Setenv("MULTIAGENT_CHATTER_CAP", "0.40")
	t.Setenv("MULTIAGENT_HANDOFF_MAX_PROBABILITY", "0.75")
	t.Setenv("MULTIAGENT_BROADCAST_HANDOFF_PROBABILITY", "0.35")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.MultiagentChatterCap != 0.40 {
		t.Fatalf("MultiagentChatterCap: got %.2f want 0.40", cfg.MultiagentChatterCap)
	}
	if cfg.MultiagentHandoffMaxProbability != 0.40 {
		t.Fatalf("MultiagentHandoffMaxProbability: got %.2f want 0.40", cfg.MultiagentHandoffMaxProbability)
	}
	if cfg.MultiagentBroadcastHandoffProbability != 0.35 {
		t.Fatalf("MultiagentBroadcastHandoffProbability: got %.2f want 0.35", cfg.MultiagentBroadcastHandoffProbability)
	}
}
