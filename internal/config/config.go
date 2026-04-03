package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// DefaultOpenRouterModel is used when LLM_MODEL is unset with the default OpenRouter base URL.
const DefaultOpenRouterModel = "google/gemini-2.0-flash-001"

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
	// LLMMaxRetries is extra completion attempts on the primary model after transient errors (429/502/503, temporary capacity).
	LLMMaxRetries int
	// LLMRetryBackoffMS is the base delay before the first retry; later retries double it (capped in the LLM package).
	LLMRetryBackoffMS int
	// LLMFallbackModel is a smaller or warmer OpenAI-compatible model id (same LLM_BASE_URL and LLM_API_KEY). Empty disables fallback.
	LLMFallbackModel string
	// LLMFallbackTimeoutSec bounds fallback completion time when fallback is attempted.
	LLMFallbackTimeoutSec int
	// LLMReplyTimeoutSec bounds each completion call so one stalled provider request cannot block event handling forever.
	LLMReplyTimeoutSec int

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
	MultiagentOrder          []string          // employee keys, e.g. ross, tim, alex, garth
	MultiagentPollInterval   int               // milliseconds between Slack polls while waiting
	MultiagentWaitTimeoutSec int               // max wait for a predecessor to post
	// MultiagentSlotSoftTimeoutSec bounds how long a bot waits for its predecessor before degraded start.
	MultiagentSlotSoftTimeoutSec int
	// MultiagentAllowDegradedStart allows downstream slots to continue when prior slot is missing after soft timeout.
	MultiagentAllowDegradedStart bool
	// MultiagentHandoffProbability: per scheduled multiagent reply, chance (0–1) to nudge @mention handoff vs self-contained.
	MultiagentHandoffProbability float64
	// MultiagentBroadcastHandoffProbability: same as above for @everyone / <!channel> multiagent runs only.
	// When unset in env, defaults to MultiagentHandoffProbability (typically 0.5).
	MultiagentBroadcastHandoffProbability float64
	// MultiagentBroadcastBranchingEnabled enables deterministic branch-mode fan-out for @everyone runs.
	MultiagentBroadcastBranchingEnabled bool
	// MultiagentBroadcastBranchingProbability is the deterministic chance [0..1] to use branch-mode handoff probability.
	MultiagentBroadcastBranchingProbability float64
	// MultiagentBroadcastBranchingHandoffProbability is the per-reply handoff chance used when branch mode is selected.
	MultiagentBroadcastBranchingHandoffProbability float64
	// MultiagentShuffleSecret: optional; mixed into the SHA-256 seed for <!everyone> / <!channel> turn order (set the same on all squad pods).
	MultiagentShuffleSecret string
	// MultiagentBroadcastRounds: number of full passes over the shuffled squad per broadcast (default 1: each agent replies once).
	MultiagentBroadcastRounds int
	// MultiagentSquadRunMax: max squad-bot messages in the current run (after last non-squad user); 0 = no cap.
	MultiagentSquadRunMax int

	// Threaded #chat (CEO-only): Socket Mode already receives thread replies; no extra webhooks.
	// ThreadsEnabled when ChatAllowedUserID and SlackChatChannelID are both non-empty.
	ChatAllowedUserID  string // Slack user id of the only human who may drive thread sessions
	SlackChatChannelID string // e.g. C01234567 — thread routing only in this channel
	// General auto-reply (non-mention plain messages): single deterministic squad winner.
	SlackGeneralChannelID                 string  // e.g. C0GENERAL — plain-message random auto-reply channel gate
	MultiagentGeneralAutoReplyEnabled     bool    // enable plain-message auto-reply selector
	MultiagentGeneralAutoReplyProbability float64 // deterministic trigger chance [0..1] per qualifying message
	RedisURL                              string  // for persisting thread owner on human-root threads; optional but required for that case
	ThreadOwnerTTLSec                     int     // Redis TTL for owner key (default 30d)

	LLMChannelIncludeThreads   bool // enrich main-channel context with recent thread reply snippets
	LLMChannelThreadParentScan int  // how many recent top-level messages to scan for reply_count (default 4)
	LLMChannelThreadRepliesMax int  // max replies pulled per parent thread (default 15)
	// Context recency weighting for channel/thread/squad context blocks.
	// Newest context line weight is 1.0; older lines decay by factor per step.
	LLMContextWeightDecay  float64 // default 0.5 (latest, 0.5x, 0.25x...)
	LLMContextWeightWindow int     // default 3 (cap decay steps for older history)

	// Handoff mention probability range used per reply.
	// Runtime samples within this bounded range to make cross-agent pinging feel organic.
	MultiagentHandoffMinProbability float64 // default 0.25
	MultiagentHandoffMaxProbability float64 // default 0.75

	// CompanyChannels is an optional channel->company runtime contract map loaded from COMPANY_CHANNELS_JSON.
	// This is the first-pass scaffold for "one Slack channel = one company runtime".
	CompanyChannels map[string]CompanyChannelRuntime
	// CompanyChannelsEnforce drops events for channels that are not present in CompanyChannels.
	// Keep false during migration so existing single-channel behavior remains unchanged.
	CompanyChannelsEnforce bool

	// RouterAvailabilityEnabled enforces availability/signoff ack-only behavior for Slack ingress + pre-LLM guards.
	RouterAvailabilityEnabled bool
	// RouterLogOnly keeps router classification + decision traces enabled but does not enforce suppression.
	RouterLogOnly bool
}

// Load reads environment variables. Canonical keys (LLM_*, SLACK_*) take precedence;
// if unset and EMPLOYEE_ID is set, falls back to OPENROUTER_KEY / {EMPLOYEE_ID}_OPENROUTER_API_KEY / _OPENROUTER_KEY / _CHUTES_KEY / _MODEL style keys.
// LLM model: LLM_MODEL, then {EMPLOYEE}_MODEL (e.g. ALEX_MODEL when id is alex), else ALEX_MODEL only if EMPLOYEE_ID is unset, else default OpenRouter model.
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
		llmModel = DefaultOpenRouterModel
	}

	cfg := &Config{
		EmployeeID: empID,
		HTTPAddr:   getEnv("HTTP_ADDR", ":8080"),
		LLMBaseURL: getEnv("LLM_BASE_URL", "https://openrouter.ai/api/v1"),
		LLMModel:   llmModel,
		LLMAPIKey: strings.TrimSpace(firstNonEmpty(
			os.Getenv("LLM_API_KEY"),
			os.Getenv("OPENROUTER_API_KEY"),
			os.Getenv("OPENROUTER_KEY"),
			employeePrefixed(empID, "OPENROUTER_API_KEY"),
			employeePrefixed(empID, "OPENROUTER_KEY"),
			employeePrefixed(empID, "CHUTES_KEY"),
			os.Getenv("ALEX_OPENROUTER_API_KEY"),
			os.Getenv("ALEX_OPENROUTER_KEY"),
			os.Getenv("ALEX_CHUTES_KEY"),
		)),
		LLMSystemMaxRunes: parseIntEnvSigned("LLM_SYSTEM_MAX_RUNES", 48000),
		// Higher default budget reduces model-side truncation; brevity is still governed by Slack response policy.
		LLMMaxTokens:              parseIntEnvMin("LLM_MAX_TOKENS", 900, 1),
		LLMTemperature:            parseFloat32Env("LLM_TEMPERATURE", 0.55),
		LLMTopP:                   parseOptionalFloat32("LLM_TOP_P"),
		LLMMaxRetries:             parseIntEnvMinAllowZero("LLM_MAX_RETRIES", 2, 0),
		LLMRetryBackoffMS:         parseIntEnvMin("LLM_RETRY_BACKOFF_MS", 400, 50),
		LLMFallbackModel:          strings.TrimSpace(os.Getenv("LLM_FALLBACK_MODEL")),
		LLMFallbackTimeoutSec:     parseIntEnvMin("LLM_FALLBACK_TIMEOUT_SEC", 8, 1),
		LLMReplyTimeoutSec:        parseIntEnvMin("LLM_REPLY_TIMEOUT_SEC", 35, 5),
		LLMThreadMaxMessages:      parseIntEnvMin("LLM_THREAD_MAX_MESSAGES", 25, 1),
		LLMThreadMaxRunes:         parseIntEnvMin("LLM_THREAD_MAX_RUNES", 16000, 256),
		LLMAlexHints:              parseBoolEnv("LLM_ALEX_HINTS", true),
		SlackOutboundWindowSec:    parseIntEnvMin("SLACK_OUTBOUND_WINDOW_SEC", 60, 1),
		SlackOutboundMaxPerWindow: parseIntEnvDefaultOrZero("SLACK_OUTBOUND_MAX_PER_WINDOW", 10),
		SlackBotToken:             strings.TrimSpace(firstNonEmpty(os.Getenv("SLACK_BOT_TOKEN"), employeePrefixed(empID, "SLACK_BOT_TOKEN"), os.Getenv("ALEX_SLACK_BOT_TOKEN"))),
		SlackAppToken:             strings.TrimSpace(firstNonEmpty(os.Getenv("SLACK_APP_TOKEN"), employeePrefixed(empID, "SLACK_APP_TOKEN"), os.Getenv("ALEX_SLACK_APP_TOKEN"))),
		PersonaPath:               getEnv("PERSONA_PATH", "/config/persona.md"),
		PersonaReloadMS:           parseIntEnv("PERSONA_RELOAD_MS", 60000),
		// Canonical: CHAT_ALLOWED_USER_ID, SLACK_CHAT_CHANNEL_ID. Aliases for local/.env convenience:
		// SLACK_CEO_USER_ID, SLACK_CHANNEL_ID (first non-empty wins in firstNonEmpty order).
		ChatAllowedUserID:                     strings.TrimSpace(firstNonEmpty(os.Getenv("CHAT_ALLOWED_USER_ID"), os.Getenv("SLACK_CEO_USER_ID"))),
		SlackChatChannelID:                    strings.TrimSpace(firstNonEmpty(os.Getenv("SLACK_CHAT_CHANNEL_ID"), os.Getenv("SLACK_CHANNEL_ID"))),
		SlackGeneralChannelID:                 strings.TrimSpace(os.Getenv("SLACK_GENERAL_CHANNEL_ID")),
		MultiagentGeneralAutoReplyEnabled:     parseBoolEnv("MULTIAGENT_GENERAL_AUTO_REPLY_ENABLED", false),
		MultiagentGeneralAutoReplyProbability: parseFloat64EnvClamp("MULTIAGENT_GENERAL_AUTO_REPLY_PROBABILITY", 0.4, 0, 1),
		RedisURL:                              strings.TrimSpace(os.Getenv("REDIS_URL")),
		ThreadOwnerTTLSec:                     parseIntEnvMin("THREAD_OWNER_TTL_SEC", 30*24*3600, 60),
		LLMChannelIncludeThreads:              parseBoolEnv("LLM_CHANNEL_INCLUDE_THREADS", false),
		LLMChannelThreadParentScan:            parseIntEnvMin("LLM_CHANNEL_THREAD_PARENT_SCAN", 4, 1),
		LLMChannelThreadRepliesMax:            parseIntEnvMin("LLM_CHANNEL_THREAD_REPLIES_MAX", 15, 1),
		LLMContextWeightDecay:                 parseFloat64EnvClamp("LLM_CONTEXT_WEIGHT_DECAY", 0.5, 0.1, 1.0),
		LLMContextWeightWindow:                parseIntEnvMin("LLM_CONTEXT_WEIGHT_WINDOW", 3, 1),
		RouterAvailabilityEnabled:             parseBoolEnv("ROUTER_AVAILABILITY_ENABLED", false),
		RouterLogOnly:                         parseBoolEnv("ROUTER_LOG_ONLY", false),
	}

	if err := parseMultiagentEnv(cfg); err != nil {
		return nil, err
	}
	if err := parseCompanyChannelsEnv(cfg); err != nil {
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
		return nil, fmt.Errorf("set LLM_API_KEY, OPENROUTER_API_KEY, or OPENROUTER_KEY (or %s_OPENROUTER_* / %s_CHUTES_KEY, or ALEX_OPENROUTER_* / ALEX_CHUTES_KEY for local Alex)", strings.ToUpper(empID), strings.ToUpper(empID))
	}
	if cfg.SlackBotToken == "" {
		return nil, fmt.Errorf("set SLACK_BOT_TOKEN or employee-prefixed _SLACK_BOT_TOKEN")
	}
	if cfg.SlackAppToken == "" {
		return nil, fmt.Errorf("set SLACK_APP_TOKEN or employee-prefixed _SLACK_APP_TOKEN")
	}

	return cfg, nil
}

// ThreadsEnabled is true when CEO + channel allowlist are set so thread routing runs.
func (c *Config) ThreadsEnabled() bool {
	if c == nil {
		return false
	}
	return c.ChatAllowedUserID != "" && c.SlackChatChannelID != ""
}

// MultiagentConfigured reports whether multi-agent sequencing can activate (squad map + order present and enabled).
func (c *Config) MultiagentConfigured() bool {
	if c == nil || !c.MultiagentEnabled {
		return false
	}
	return len(c.MultiagentBotUserIDs) > 0 && len(c.MultiagentOrder) > 0
}

// RouterAvailabilityActive returns true when availability/sentiment routing should classify traffic.
func (c *Config) RouterAvailabilityActive() bool {
	if c == nil {
		return false
	}
	return c.RouterAvailabilityEnabled || c.RouterLogOnly
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
	cfg.MultiagentSlotSoftTimeoutSec = parseIntEnvMin("MULTIAGENT_SLOT_SOFT_TIMEOUT_SEC", 12, 1)
	cfg.MultiagentAllowDegradedStart = parseBoolEnv("MULTIAGENT_ALLOW_DEGRADED_START", true)
	cfg.MultiagentEnabled = parseBoolEnv("MULTIAGENT_ENABLED", true)
	cfg.MultiagentHandoffProbability = parseFloat64EnvClamp("MULTIAGENT_HANDOFF_PROBABILITY", 0.5, 0, 1)
	cfg.MultiagentBroadcastHandoffProbability = cfg.MultiagentHandoffProbability
	if v := strings.TrimSpace(os.Getenv("MULTIAGENT_BROADCAST_HANDOFF_PROBABILITY")); v != "" {
		if f, err := strconv.ParseFloat(v, 64); err == nil {
			if f < 0 {
				f = 0
			}
			if f > 1 {
				f = 1
			}
			cfg.MultiagentBroadcastHandoffProbability = f
		}
	} else {
		cfg.MultiagentBroadcastHandoffProbability = 0.35
	}
	cfg.MultiagentBroadcastBranchingEnabled = parseBoolEnv("MULTIAGENT_BROADCAST_BRANCHING_ENABLED", true)
	cfg.MultiagentBroadcastBranchingProbability = parseFloat64EnvClamp("MULTIAGENT_BROADCAST_BRANCHING_PROBABILITY", 0.5, 0, 1)
	cfg.MultiagentBroadcastBranchingHandoffProbability = parseFloat64EnvClamp(
		"MULTIAGENT_BROADCAST_BRANCHING_HANDOFF_PROBABILITY",
		0.6,
		0,
		1,
	)
	cfg.MultiagentShuffleSecret = strings.TrimSpace(os.Getenv("MULTIAGENT_SHUFFLE_SECRET"))
	cfg.MultiagentBroadcastRounds = parseIntEnvMin("MULTIAGENT_BROADCAST_ROUNDS", 1, 1)
	if cfg.MultiagentBroadcastRounds > 24 {
		cfg.MultiagentBroadcastRounds = 24
	}
	cfg.MultiagentHandoffMinProbability = parseFloat64EnvClamp("MULTIAGENT_HANDOFF_MIN_PROBABILITY", 0.25, 0, 1)
	cfg.MultiagentHandoffMaxProbability = parseFloat64EnvClamp("MULTIAGENT_HANDOFF_MAX_PROBABILITY", 0.75, 0, 1)
	if cfg.MultiagentHandoffMaxProbability < cfg.MultiagentHandoffMinProbability {
		cfg.MultiagentHandoffMinProbability, cfg.MultiagentHandoffMaxProbability = cfg.MultiagentHandoffMaxProbability, cfg.MultiagentHandoffMinProbability
	}
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

// parseIntEnvMinAllowZero is like parseIntEnvMin but allows n == min when min is 0.
func parseIntEnvMinAllowZero(key string, def, min int) int {
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
