package opsproxy

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"
)

type ProxyConfig struct {
	HTTPAddr             string
	AuthToken            string
	RedisURL             string
	AllowedNamespaces    []string
	AllowedRedisPrefixes []string
	WaitlistPrefixes     []string
	DefaultStatusLimit   int
	MaxStatusLimit       int
	DefaultRedisLimit    int
	MaxRedisLimit        int
	DefaultWaitlistLimit int
	MaxWaitlistLimit     int
	DefaultLogTailLines  int64
	MaxLogTailLines      int64
	MaxLogBytes          int
	RequestTimeout       time.Duration
}

func LoadProxyConfigFromEnv() (*ProxyConfig, error) {
	cfg := &ProxyConfig{
		HTTPAddr:             strings.TrimSpace(getEnv("HTTP_ADDR", ":8080")),
		AuthToken:            strings.TrimSpace(firstNonEmpty(os.Getenv("OPS_PROXY_AUTH_TOKEN"), os.Getenv("ROSS_OPS_PROXY_TOKEN"))),
		RedisURL:             strings.TrimSpace(os.Getenv("REDIS_URL")),
		AllowedNamespaces:    splitCSV(os.Getenv("OPS_PROXY_ALLOWED_NAMESPACES")),
		AllowedRedisPrefixes: splitCSV(os.Getenv("OPS_PROXY_ALLOWED_REDIS_PREFIXES")),
		WaitlistPrefixes:     splitCSV(os.Getenv("OPS_PROXY_WAITLIST_PREFIXES")),
		DefaultStatusLimit:   parseIntEnvMin("OPS_PROXY_DEFAULT_STATUS_LIMIT", 8, 1),
		MaxStatusLimit:       parseIntEnvMin("OPS_PROXY_MAX_STATUS_LIMIT", 25, 1),
		DefaultRedisLimit:    parseIntEnvMin("OPS_PROXY_DEFAULT_REDIS_LIMIT", 10, 1),
		MaxRedisLimit:        parseIntEnvMin("OPS_PROXY_MAX_REDIS_LIMIT", 50, 1),
		DefaultWaitlistLimit: parseIntEnvMin("OPS_PROXY_DEFAULT_WAITLIST_LIMIT", 50, 1),
		MaxWaitlistLimit:     parseIntEnvMin("OPS_PROXY_MAX_WAITLIST_LIMIT", 250, 1),
		DefaultLogTailLines:  parseInt64EnvMin("OPS_PROXY_DEFAULT_LOG_TAIL_LINES", 200, 1),
		MaxLogTailLines:      parseInt64EnvMin("OPS_PROXY_MAX_LOG_TAIL_LINES", 500, 1),
		MaxLogBytes:          parseIntEnvMin("OPS_PROXY_MAX_LOG_BYTES", 20000, 1024),
		RequestTimeout:       time.Duration(parseIntEnvMin("OPS_PROXY_REQUEST_TIMEOUT_SEC", 15, 1)) * time.Second,
	}
	if cfg.MaxStatusLimit < cfg.DefaultStatusLimit {
		cfg.MaxStatusLimit = cfg.DefaultStatusLimit
	}
	if cfg.MaxRedisLimit < cfg.DefaultRedisLimit {
		cfg.MaxRedisLimit = cfg.DefaultRedisLimit
	}
	if cfg.MaxWaitlistLimit < cfg.DefaultWaitlistLimit {
		cfg.MaxWaitlistLimit = cfg.DefaultWaitlistLimit
	}
	if cfg.MaxLogTailLines < cfg.DefaultLogTailLines {
		cfg.MaxLogTailLines = cfg.DefaultLogTailLines
	}
	if cfg.AuthToken == "" {
		return nil, fmt.Errorf("set OPS_PROXY_AUTH_TOKEN (or ROSS_OPS_PROXY_TOKEN)")
	}
	if len(cfg.AllowedNamespaces) == 0 {
		return nil, fmt.Errorf("set OPS_PROXY_ALLOWED_NAMESPACES with at least one namespace")
	}
	if len(cfg.AllowedRedisPrefixes) == 0 {
		return nil, fmt.Errorf("set OPS_PROXY_ALLOWED_REDIS_PREFIXES with at least one prefix")
	}
	if len(cfg.WaitlistPrefixes) == 0 {
		return nil, fmt.Errorf("set OPS_PROXY_WAITLIST_PREFIXES with at least one prefix")
	}
	return cfg, nil
}

func splitCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	seen := map[string]struct{}{}
	for _, part := range parts {
		v := strings.TrimSpace(part)
		if v == "" {
			continue
		}
		if _, ok := seen[v]; ok {
			continue
		}
		seen[v] = struct{}{}
		out = append(out, v)
	}
	return out
}

func getEnv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if strings.TrimSpace(v) != "" {
			return strings.TrimSpace(v)
		}
	}
	return ""
}

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

func parseInt64EnvMin(key string, def, min int64) int64 {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil || n < min {
		return def
	}
	return n
}
