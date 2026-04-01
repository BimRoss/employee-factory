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
	// LLMMaxTokens is passed as max_tokens on chat completions (generation budget ceiling).
	LLMMaxTokens int
	// LLMTemperature is sampling temperature for chat completions (default 0.55).
	LLMTemperature float32
	// LLMTopP is optional nucleus sampling; nil means omit (provider default).
	LLMTopP *float32

	// Recent channel history (conversations.history) for linear context; env keys LLM_THREAD_*.
	LLMThreadMaxMessages int
	LLMThreadMaxRunes    int
	// LLMAlexHints enables deterministic keyword hints prepended to the user message (Alex only).
	LLMAlexHints bool

	// Slack outbound rate limit (per bot instance): max posts in a rolling window.
	// Prevents runaway reply loops when agents @mention each other. 0 = disabled.
	SlackOutboundWindowSec    int
	SlackOutboundMaxPerWindow int

	SlackBotToken string
	SlackAppToken string

	PersonaPath     string
	PersonaReloadMS int

	// Multiagent: shared Slack user IDs + order for sequential multi-bot channel sessions (see slackbot).
	// When unset, multi-agent mode is off and behavior matches single-bot replies.
	MultiagentEnabled        bool
	MultiagentBotUserIDs     map[string]string // employee key -> Slack user ID (from MULTIAGENT_BOT_USER_IDS or ROSS_SLACK_BOT_ID / TIM_SLACK_BOT_ID / …)
	MultiagentOrder          []string          // employee keys, e.g. ross, tim, alex
	MultiagentPollInterval   int               // milliseconds between Slack polls while waiting
	MultiagentWaitTimeoutSec int               // max wait for a predecessor to post
	// MultiagentHandoffProbability: per scheduled multiagent reply, chance (0–1) to nudge @mention handoff vs self-contained.
	MultiagentHandoffProbability float64
	// MultiagentSquadRunMax: max squad-bot messages in the current run (after last non-squad user); 0 = no cap.
	MultiagentSquadRunMax int
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
		// Default ceiling avoids mid-sentence cutoffs; brevity comes from the Slack system suffix, not a tiny cap.
		LLMMaxTokens:              parseIntEnvMin("LLM_MAX_TOKENS", 512, 1),
		LLMTemperature:            parseFloat32Env("LLM_TEMPERATURE", 0.55),
		LLMTopP:                   parseOptionalFloat32("LLM_TOP_P"),
		LLMThreadMaxMessages:      parseIntEnvMin("LLM_THREAD_MAX_MESSAGES", 25, 1),
		LLMThreadMaxRunes:         parseIntEnvMin("LLM_THREAD_MAX_RUNES", 16000, 256),
		LLMAlexHints:              parseBoolEnv("LLM_ALEX_HINTS", true),
		SlackOutboundWindowSec:    parseIntEnvMin("SLACK_OUTBOUND_WINDOW_SEC", 60, 1),
		SlackOutboundMaxPerWindow: parseIntEnvDefaultOrZero("SLACK_OUTBOUND_MAX_PER_WINDOW", 10),
		SlackBotToken:             strings.TrimSpace(firstNonEmpty(os.Getenv("SLACK_BOT_TOKEN"), employeePrefixed(empID, "SLACK_BOT_TOKEN"), os.Getenv("ALEX_SLACK_BOT_TOKEN"))),
		SlackAppToken:             strings.TrimSpace(firstNonEmpty(os.Getenv("SLACK_APP_TOKEN"), employeePrefixed(empID, "SLACK_APP_TOKEN"), os.Getenv("ALEX_SLACK_APP_TOKEN"))),
		PersonaPath:               getEnv("PERSONA_PATH", "/config/persona.md"),
		PersonaReloadMS:           parseIntEnv("PERSONA_RELOAD_MS", 60000),
	}

	if err := parseMultiagentEnv(cfg); err != nil {
		return nil, err
	}

	// Optional per-employee sampling temperature (e.g. ROSS_LLM_TEMPERATURE) to diversify squad replies.
	if empID != "" {
		if v := strings.TrimSpace(employeePrefixed(empID, "LLM_TEMPERATURE")); v != "" {
			if f, err := strconv.ParseFloat(v, 32); err == nil {
				cfg.LLMTemperature = float32(f)
			}
		}
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

// MultiagentConfigured reports whether multi-agent sequencing can activate (squad map + order present and enabled).
func (c *Config) MultiagentConfigured() bool {
	if c == nil || !c.MultiagentEnabled {
		return false
	}
	return len(c.MultiagentBotUserIDs) > 0 && len(c.MultiagentOrder) > 0
}

func parseMultiagentEnv(cfg *Config) error {
	raw := strings.TrimSpace(os.Getenv("MULTIAGENT_BOT_USER_IDS"))
	orderRaw := strings.TrimSpace(os.Getenv("MULTIAGENT_ORDER"))
	if raw == "" && orderRaw == "" {
		cfg.MultiagentEnabled = false
		return nil
	}
	if orderRaw == "" {
		return fmt.Errorf("multi-agent: set MULTIAGENT_ORDER plus MULTIAGENT_BOT_USER_IDS or each ROSS_SLACK_BOT_ID / TIM_SLACK_BOT_ID / …, or omit multi-agent env vars")
	}

	var order []string
	for _, part := range strings.Split(orderRaw, ",") {
		part = strings.ToLower(strings.TrimSpace(part))
		if part == "" {
			continue
		}
		order = append(order, part)
	}
	if len(order) == 0 {
		return fmt.Errorf("multi-agent: MULTIAGENT_ORDER empty")
	}

	m := make(map[string]string)
	if raw != "" {
		for _, part := range strings.Split(raw, ",") {
			part = strings.TrimSpace(part)
			if part == "" {
				continue
			}
			kv := strings.SplitN(part, ":", 2)
			if len(kv) != 2 {
				return fmt.Errorf("multi-agent: invalid MULTIAGENT_BOT_USER_IDS entry %q (want ross:U123,...)", part)
			}
			key := strings.ToLower(strings.TrimSpace(kv[0]))
			uid := strings.TrimSpace(kv[1])
			if key == "" || uid == "" {
				return fmt.Errorf("multi-agent: empty key or user id in %q", part)
			}
			m[key] = uid
		}
		if len(m) == 0 {
			return fmt.Errorf("multi-agent: MULTIAGENT_BOT_USER_IDS parsed empty")
		}
	} else {
		for _, key := range order {
			uid := strings.TrimSpace(employeePrefixed(key, "SLACK_BOT_ID"))
			if uid == "" {
				envName := strings.ToUpper(strings.ReplaceAll(key, "-", "_")) + "_SLACK_BOT_ID"
				return fmt.Errorf("multi-agent: set %s for employee %q (or set MULTIAGENT_BOT_USER_IDS)", envName, key)
			}
			m[key] = uid
		}
	}

	for _, part := range order {
		if _, ok := m[part]; !ok {
			return fmt.Errorf("multi-agent: MULTIAGENT_ORDER key %q missing from squad user id map", part)
		}
	}
	for k := range m {
		found := false
		for _, o := range order {
			if o == k {
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf("multi-agent: employee %q in map but not in MULTIAGENT_ORDER", k)
		}
	}

	cfg.MultiagentBotUserIDs = m
	cfg.MultiagentOrder = order
	cfg.MultiagentPollInterval = parseIntEnvMin("MULTIAGENT_POLL_INTERVAL_MS", 800, 50)
	cfg.MultiagentWaitTimeoutSec = parseIntEnvMin("MULTIAGENT_WAIT_TIMEOUT_SEC", 300, 5)
	cfg.MultiagentEnabled = parseBoolEnv("MULTIAGENT_ENABLED", true)
	cfg.MultiagentHandoffProbability = parseFloat64EnvClamp("MULTIAGENT_HANDOFF_PROBABILITY", 0.5, 0, 1)
	cfg.MultiagentSquadRunMax = parseIntEnvDefaultOrZero("MULTIAGENT_SQUAD_RUN_MAX", 12)
	return nil
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

func parseFloat32Env(key string, def float32) float32 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return def
	}
	return float32(f)
}

func parseOptionalFloat32(key string) *float32 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return nil
	}
	f, err := strconv.ParseFloat(v, 32)
	if err != nil {
		return nil
	}
	x := float32(f)
	return &x
}

// parseIntEnvDefaultOrZero returns def when unset; 0 when set to "0" (disable); invalid uses def.
func parseIntEnvDefaultOrZero(key string, def int) int {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return def
	}
	if n < 0 {
		return def
	}
	return n
}

func parseBoolEnv(key string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(key)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

// parseFloat64EnvClamp parses a float env; empty uses def; invalid uses def; clamps to [min, max].
func parseFloat64EnvClamp(key string, def, min, max float64) float64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return def
	}
	if f < min {
		return min
	}
	if f > max {
		return max
	}
	return f
}
