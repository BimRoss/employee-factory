package config

import "testing"

func TestRossOpsDefaults(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "")
	t.Setenv("MULTIAGENT_ORDER", "")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.RossOpsEnabled {
		t.Fatal("RossOpsEnabled: expected false by default")
	}
	if cfg.RossOpsLogOnly {
		t.Fatal("RossOpsLogOnly: expected false by default")
	}
}

func TestRossOpsValidation(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("EMPLOYEE_ID", "ross")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "")
	t.Setenv("MULTIAGENT_ORDER", "")
	t.Setenv("ROSS_OPS_ENABLED", "true")
	t.Setenv("ROSS_OPS_PROXY_URL", "")
	t.Setenv("ROSS_OPS_PROXY_TOKEN", "")

	if _, err := Load(); err == nil {
		t.Fatal("expected validation error when ross ops config is missing")
	}
}
