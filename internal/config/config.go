package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// DefaultChutesModel is used when LLM_MODEL is unset and matches the default Chutes base URL.
const DefaultChutesModel = "unsloth/Llama-3.2-1B-Instruct"

// Config holds runtime settings for one employee instance.
type Config struct {
	EmployeeID string

	HTTPAddr string

	LLMBaseURL string
	LLMModel   string
	LLMAPIKey  string
	// LLMSystemMaxRunes caps the system (persona) prompt size. -1 disables truncation (e.g. large-context models).
	LLMSystemMaxRunes int
	// LLMMaxTokens is passed as max_tokens on chat completions (generation budget).
	LLMMaxTokens int

	SlackBotToken string
	SlackAppToken string

	PersonaPath     string
	PersonaReloadMS int
}

// Load reads environment variables. Canonical keys (LLM_*, SLACK_*) take precedence;
// if unset and EMPLOYEE_ID is set, falls back to {EMPLOYEE_ID}_CHUTES_KEY / _MODEL style keys.
// LLM model: LLM_MODEL, then {EMPLOYEE}_MODEL (e.g. ALEX_MODEL when id is alex), else ALEX_MODEL only if EMPLOYEE_ID is unset, else default Chutes model.
func Load() (*Config, error) {
	_ = os.Getenv("SKIP_DOTENV") // documented no-op if caller loads dotenv first

	empID := strings.TrimSpace(os.Getenv("EMPLOYEE_ID"))

	llmModel := strings.TrimSpace(firstNonEmpty(
		os.Getenv("LLM_MODEL"),
		employeePrefixed(empID, "MODEL"),
	))
	if llmModel == "" && empID == "" {
		// Local convenience when EMPLOYEE_ID is unset: same as ALEX_* for keys.
		llmModel = strings.TrimSpace(os.Getenv("ALEX_MODEL"))
	}
	if llmModel == "" {
		llmModel = DefaultChutesModel
	}

	cfg := &Config{
		EmployeeID:        empID,
		HTTPAddr:          getEnv("HTTP_ADDR", ":8080"),
		LLMBaseURL:        getEnv("LLM_BASE_URL", "https://llm.chutes.ai/v1"),
		LLMModel:          llmModel,
		LLMAPIKey:         strings.TrimSpace(firstNonEmpty(os.Getenv("LLM_API_KEY"), employeePrefixed(empID, "CHUTES_KEY"), os.Getenv("ALEX_CHUTES_KEY"))),
		LLMSystemMaxRunes: parseIntEnvSigned("LLM_SYSTEM_MAX_RUNES", 48000),
		// Slack: 512 tokens still allows essay-length output; default lower—override for long-form.
		LLMMaxTokens:    parseIntEnvMin("LLM_MAX_TOKENS", 256, 1),
		SlackBotToken:   strings.TrimSpace(firstNonEmpty(os.Getenv("SLACK_BOT_TOKEN"), employeePrefixed(empID, "SLACK_BOT_TOKEN"), os.Getenv("ALEX_SLACK_BOT_TOKEN"))),
		SlackAppToken:   strings.TrimSpace(firstNonEmpty(os.Getenv("SLACK_APP_TOKEN"), employeePrefixed(empID, "SLACK_APP_TOKEN"), os.Getenv("ALEX_SLACK_APP_TOKEN"))),
		PersonaPath:     getEnv("PERSONA_PATH", "/config/persona.md"),
		PersonaReloadMS: parseIntEnv("PERSONA_RELOAD_MS", 60000),
	}

	if cfg.LLMAPIKey == "" {
		return nil, fmt.Errorf("set LLM_API_KEY, or %s_CHUTES_KEY, or ALEX_CHUTES_KEY for local Alex", strings.ToUpper(empID))
	}
	if cfg.SlackBotToken == "" {
		return nil, fmt.Errorf("set SLACK_BOT_TOKEN or employee-prefixed _SLACK_BOT_TOKEN")
	}
	if cfg.SlackAppToken == "" {
		return nil, fmt.Errorf("set SLACK_APP_TOKEN or employee-prefixed _SLACK_APP_TOKEN")
	}

	return cfg, nil
}

func getEnv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func firstNonEmpty(vals ...string) string {
	for _, s := range vals {
		if strings.TrimSpace(s) != "" {
			return strings.TrimSpace(s)
		}
	}
	return ""
}

func employeePrefixed(empID, suffix string) string {
	empID = strings.TrimSpace(empID)
	if empID == "" {
		return ""
	}
	key := strings.ToUpper(strings.ReplaceAll(empID, "-", "_")) + "_" + suffix
	return strings.TrimSpace(os.Getenv(key))
}

func parseIntEnv(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	var n int
	_, err := fmt.Sscanf(v, "%d", &n)
	if err != nil || n <= 0 {
		return def
	}
	return n
}

// parseIntEnvSigned parses an int env; empty uses def. Invalid values use def.
// Negative values are allowed (e.g. -1 means disable truncation for LLM_SYSTEM_MAX_RUNES).
func parseIntEnvSigned(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	return n
}

// parseIntEnvMin parses a positive int; empty uses def; values below min use def.
func parseIntEnvMin(key string, def, min int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil || n < min {
		return def
	}
	return n
}
