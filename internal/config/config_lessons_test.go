package config

import "testing"

func TestLessonsDefaults(t *testing.T) {
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
	if cfg.LessonsEnabled {
		t.Fatal("LessonsEnabled: expected false by default")
	}
	if !cfg.LessonsLogOnly {
		t.Fatal("LessonsLogOnly: expected true by default")
	}
	if !cfg.LessonsAutoApply {
		t.Fatal("LessonsAutoApply: expected true by default")
	}
	if cfg.LessonsMinConfidence != 0.8 {
		t.Fatalf("LessonsMinConfidence: got %.2f want 0.80", cfg.LessonsMinConfidence)
	}
	if cfg.LessonsMaxActive != 3 {
		t.Fatalf("LessonsMaxActive: got %d want 3", cfg.LessonsMaxActive)
	}
	if cfg.LessonsTTLSeconds != 604800 {
		t.Fatalf("LessonsTTLSeconds: got %d want 604800", cfg.LessonsTTLSeconds)
	}
	if cfg.LessonsMaxEvents != 200 {
		t.Fatalf("LessonsMaxEvents: got %d want 200", cfg.LessonsMaxEvents)
	}
	if cfg.LessonsMaxPromptRunes != 600 {
		t.Fatalf("LessonsMaxPromptRunes: got %d want 600", cfg.LessonsMaxPromptRunes)
	}
}

func TestLessonsEnvClamps(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("LLM_API_KEY", "x")
	t.Setenv("SLACK_BOT_TOKEN", "xoxb-test")
	t.Setenv("SLACK_APP_TOKEN", "xapp-test")
	t.Setenv("MULTIAGENT_BOT_USER_IDS", "")
	t.Setenv("MULTIAGENT_ORDER", "")
	t.Setenv("LESSONS_ENABLED", "true")
	t.Setenv("LESSONS_LOG_ONLY", "false")
	t.Setenv("LESSONS_AUTO_APPLY", "false")
	t.Setenv("LESSONS_MIN_CONFIDENCE", "2.2")
	t.Setenv("LESSONS_MAX_ACTIVE", "0")
	t.Setenv("LESSONS_TTL_SEC", "1")
	t.Setenv("LESSONS_MAX_EVENTS", "1")
	t.Setenv("LESSONS_MAX_PROMPT_RUNES", "10")

	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if !cfg.LessonsEnabled {
		t.Fatal("LessonsEnabled: expected true")
	}
	if cfg.LessonsLogOnly {
		t.Fatal("LessonsLogOnly: expected false")
	}
	if cfg.LessonsAutoApply {
		t.Fatal("LessonsAutoApply: expected false")
	}
	if cfg.LessonsMinConfidence != 1.0 {
		t.Fatalf("LessonsMinConfidence clamp: got %.2f want 1.00", cfg.LessonsMinConfidence)
	}
	if cfg.LessonsMaxActive != 3 {
		t.Fatalf("LessonsMaxActive min clamp: got %d want 3", cfg.LessonsMaxActive)
	}
	if cfg.LessonsTTLSeconds != 604800 {
		t.Fatalf("LessonsTTLSeconds min clamp: got %d want 604800", cfg.LessonsTTLSeconds)
	}
	if cfg.LessonsMaxEvents != 200 {
		t.Fatalf("LessonsMaxEvents min clamp: got %d want 200", cfg.LessonsMaxEvents)
	}
	if cfg.LessonsMaxPromptRunes != 600 {
		t.Fatalf("LessonsMaxPromptRunes min clamp: got %d want 600", cfg.LessonsMaxPromptRunes)
	}
}
