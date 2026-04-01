package config

import (
	"testing"
)

func TestChatEnvAliases(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("CHAT_ALLOWED_USER_ID", "")
	t.Setenv("SLACK_CHAT_CHANNEL_ID", "")
	t.Setenv("SLACK_CEO_USER_ID", "UCEO")
	t.Setenv("SLACK_CHANNEL_ID", "C123")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "")
	t.Setenv("MULTIAGENT_ORDER", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ChatAllowedUserID != "UCEO" {
		t.Fatalf("ChatAllowedUserID: got %q", cfg.ChatAllowedUserID)
	}
	if cfg.SlackChatChannelID != "C123" {
		t.Fatalf("SlackChatChannelID: got %q", cfg.SlackChatChannelID)
	}
}

func TestCanonicalOverridesAlias(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("CHAT_ALLOWED_USER_ID", "UCANON")
	t.Setenv("SLACK_CHAT_CHANNEL_ID", "CCANON")
	t.Setenv("SLACK_CEO_USER_ID", "UCEO")
	t.Setenv("SLACK_CHANNEL_ID", "C123")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "")
	t.Setenv("MULTIAGENT_ORDER", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ChatAllowedUserID != "UCANON" || cfg.SlackChatChannelID != "CCANON" {
		t.Fatalf("expected canonical keys to win: %+v", cfg)
	}
}
