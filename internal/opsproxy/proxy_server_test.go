package opsproxy

import "testing"

func TestRedisPrefixAllowed(t *testing.T) {
	s := &ProxyServer{
		cfg: &ProxyConfig{
			AllowedRedisPrefixes: []string{"thread_owner:", "ops:"},
		},
	}
	if !s.redisPrefixAllowed("thread_owner:C123") {
		t.Fatal("expected thread_owner prefix to be allowed")
	}
	if s.redisPrefixAllowed("other:key") {
		t.Fatal("did not expect unrelated prefix to be allowed")
	}
}
