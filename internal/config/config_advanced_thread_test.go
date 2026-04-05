package config

import "testing"

func TestAdvancedThreadDefaults(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "")
	t.Setenv("MULTIAGENT_ORDER", "")
	t.Setenv("ADVANCED_TOOLING_THREAD_ENFORCEMENT", "")
	t.Setenv("ADVANCED_TOOLING_SEED_THREAD_ON_TOPLEVEL", "")
	t.Setenv("ADVANCED_TOOLING_THREAD_TASK_TTL_SEC", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AdvancedToolingThreadEnforcement != "off" {
		t.Fatalf("expected default enforcement off, got %q", cfg.AdvancedToolingThreadEnforcement)
	}
	if !cfg.AdvancedToolingSeedThreadOnTopLevel {
		t.Fatal("expected seed thread on top-level default true")
	}
	if cfg.AdvancedToolingThreadTaskTTLSec != 1200 {
		t.Fatalf("unexpected default task ttl: %d", cfg.AdvancedToolingThreadTaskTTLSec)
	}
}

func TestAdvancedThreadFlagParsing(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "")
	t.Setenv("MULTIAGENT_ORDER", "")
	t.Setenv("ADVANCED_TOOLING_THREAD_ENFORCEMENT", "enforce")
	t.Setenv("ADVANCED_TOOLING_SEED_THREAD_ON_TOPLEVEL", "false")
	t.Setenv("ADVANCED_TOOLING_THREAD_TASK_TTL_SEC", "3600")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.AdvancedToolingThreadEnforcement != "enforce" {
		t.Fatalf("expected enforce mode, got %q", cfg.AdvancedToolingThreadEnforcement)
	}
	if cfg.AdvancedToolingSeedThreadOnTopLevel {
		t.Fatal("expected seed thread flag false")
	}
	if cfg.AdvancedToolingThreadTaskTTLSec != 3600 {
		t.Fatalf("unexpected configured task ttl: %d", cfg.AdvancedToolingThreadTaskTTLSec)
	}
}
