package config

import "testing"

func TestSlackPresentationDefaults(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "")
	t.Setenv("MULTIAGENT_ORDER", "")
	t.Setenv("SLACK_PRESENTATION_ENABLE_BLOCKS", "")
	t.Setenv("SLACK_PRESENTATION_JSON_MODE", "")
	t.Setenv("SLACK_PRESENTATION_MAX_BLOCK_ITEMS", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SlackPresentationEnableBlocks {
		t.Fatal("expected SLACK_PRESENTATION_ENABLE_BLOCKS default false")
	}
	if cfg.SlackPresentationJSONMode != "off" {
		t.Fatalf("expected JSON mode off, got %q", cfg.SlackPresentationJSONMode)
	}
	if cfg.SlackPresentationMaxBlockItems != 8 {
		t.Fatalf("expected max block items 8, got %d", cfg.SlackPresentationMaxBlockItems)
	}
}

func TestSlackPresentationEnvParsing(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "")
	t.Setenv("MULTIAGENT_ORDER", "")
	t.Setenv("SLACK_PRESENTATION_ENABLE_BLOCKS", "true")
	t.Setenv("SLACK_PRESENTATION_JSON_MODE", "force_for_structured")
	t.Setenv("SLACK_PRESENTATION_MAX_BLOCK_ITEMS", "12")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.SlackPresentationEnableBlocks {
		t.Fatal("expected blocks enabled")
	}
	if cfg.SlackPresentationJSONMode != "force_for_structured" {
		t.Fatalf("unexpected json mode: %q", cfg.SlackPresentationJSONMode)
	}
	if cfg.SlackPresentationMaxBlockItems != 12 {
		t.Fatalf("unexpected max block items: %d", cfg.SlackPresentationMaxBlockItems)
	}
}
