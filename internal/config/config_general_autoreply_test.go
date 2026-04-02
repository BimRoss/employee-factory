package config

import "testing"

func TestGeneralAutoReplyDefaults(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "")
	t.Setenv("MULTIAGENT_ORDER", "")
	t.Setenv("SLACK_GENERAL_CHANNEL_ID", "")
	t.Setenv("MULTIAGENT_GENERAL_AUTO_REPLY_ENABLED", "")
	t.Setenv("MULTIAGENT_GENERAL_AUTO_REPLY_PROBABILITY", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SlackGeneralChannelID != "" {
		t.Fatalf("SlackGeneralChannelID: got %q want empty", cfg.SlackGeneralChannelID)
	}
	if cfg.MultiagentGeneralAutoReplyEnabled {
		t.Fatal("MultiagentGeneralAutoReplyEnabled: expected false by default")
	}
	if cfg.MultiagentGeneralAutoReplyProbability != 0.4 {
		t.Fatalf("MultiagentGeneralAutoReplyProbability: got %.2f want 0.40", cfg.MultiagentGeneralAutoReplyProbability)
	}
}

func TestGeneralAutoReplyEnvAndClamp(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "")
	t.Setenv("MULTIAGENT_ORDER", "")
	t.Setenv("SLACK_GENERAL_CHANNEL_ID", "CGENERAL")
	t.Setenv("MULTIAGENT_GENERAL_AUTO_REPLY_ENABLED", "true")
	t.Setenv("MULTIAGENT_GENERAL_AUTO_REPLY_PROBABILITY", "2.5")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SlackGeneralChannelID != "CGENERAL" {
		t.Fatalf("SlackGeneralChannelID: got %q", cfg.SlackGeneralChannelID)
	}
	if !cfg.MultiagentGeneralAutoReplyEnabled {
		t.Fatal("MultiagentGeneralAutoReplyEnabled: expected true")
	}
	if cfg.MultiagentGeneralAutoReplyProbability != 1 {
		t.Fatalf("MultiagentGeneralAutoReplyProbability clamp: got %.2f want 1.00", cfg.MultiagentGeneralAutoReplyProbability)
	}
}
