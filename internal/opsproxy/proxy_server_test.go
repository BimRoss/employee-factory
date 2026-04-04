package opsproxy

import "testing"

func TestRedisPrefixAllowed(t *testing.T) {
	s := &ProxyServer{
		cfg: &ProxyConfig{
			AllowedRedisPrefixes: []string{"thread_owner:", "ops:"},
			WaitlistPrefixes:     []string{"waitlist:"},
		},
	}
	if !s.redisPrefixAllowed("thread_owner:C123") {
		t.Fatal("expected thread_owner prefix to be allowed")
	}
	if s.redisPrefixAllowed("other:key") {
		t.Fatal("did not expect unrelated prefix to be allowed")
	}
	if !s.waitlistPrefixAllowed("waitlist:user:1") {
		t.Fatal("expected waitlist prefix to be allowed")
	}
	if s.waitlistPrefixAllowed("thread_owner:C123") {
		t.Fatal("did not expect thread_owner key in waitlist prefix set")
	}
}
