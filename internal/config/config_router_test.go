package config

import "testing"

func TestRouterAvailabilityDefaults(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("ROUTER_AVAILABILITY_ENABLED", "")
	t.Setenv("ROUTER_LOG_ONLY", "")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "")
	t.Setenv("MULTIAGENT_ORDER", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RouterAvailabilityEnabled {
		t.Fatal("RouterAvailabilityEnabled: expected false by default")
	}
	if cfg.RouterLogOnly {
		t.Fatal("RouterLogOnly: expected false by default")
	}
	if cfg.RouterAvailabilityActive() {
		t.Fatal("RouterAvailabilityActive: expected false by default")
	}
}

func TestRouterAvailabilityFlags(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("ROUTER_AVAILABILITY_ENABLED", "true")
	t.Setenv("ROUTER_LOG_ONLY", "true")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "")
	t.Setenv("MULTIAGENT_ORDER", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.RouterAvailabilityEnabled {
		t.Fatal("RouterAvailabilityEnabled: expected true")
	}
	if !cfg.RouterLogOnly {
		t.Fatal("RouterLogOnly: expected true")
	}
	if !cfg.RouterAvailabilityActive() {
		t.Fatal("RouterAvailabilityActive: expected true when either flag is on")
	}
}
