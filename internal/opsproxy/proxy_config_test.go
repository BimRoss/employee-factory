package opsproxy

import "testing"

func TestLoadProxyConfigFromEnv_Validation(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("OPS_PROXY_AUTH_TOKEN", "")
	t.Setenv("OPS_PROXY_ALLOWED_NAMESPACES", "")
	t.Setenv("OPS_PROXY_ALLOWED_REDIS_PREFIXES", "")

	if _, err := LoadProxyConfigFromEnv(); err == nil {
		t.Fatal("expected validation error without required env")
	}
}

func TestLoadProxyConfigFromEnv_Success(t *testing.T) {
	t.Setenv("SKIP_DOTENV", "1")
	t.Setenv("OPS_PROXY_AUTH_TOKEN", "secret")
	t.Setenv("OPS_PROXY_ALLOWED_NAMESPACES", "employee-factory,subnet-signal")
	t.Setenv("OPS_PROXY_ALLOWED_REDIS_PREFIXES", "thread_owner:,ops:")

	cfg, err := LoadProxyConfigFromEnv()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.AuthToken != "secret" {
		t.Fatalf("auth token mismatch: %q", cfg.AuthToken)
	}
	if len(cfg.AllowedNamespaces) != 2 {
		t.Fatalf("namespace allowlist mismatch: %+v", cfg.AllowedNamespaces)
	}
	if len(cfg.AllowedRedisPrefixes) != 2 {
		t.Fatalf("redis prefix allowlist mismatch: %+v", cfg.AllowedRedisPrefixes)
	}
}
