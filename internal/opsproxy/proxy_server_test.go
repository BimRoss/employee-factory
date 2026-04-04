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

func TestResolveWaitlistPrefixes_DefaultPrioritizesMakeACompany(t *testing.T) {
	s := &ProxyServer{
		cfg: &ProxyConfig{
			WaitlistPrefixes: []string{"waitlist:", "makeacompany:waitlist:", "legacy:waitlist:"},
		},
	}
	prefixes, err := s.resolveWaitlistPrefixes("")
	if err != nil {
		t.Fatalf("resolve default prefixes: %v", err)
	}
	if len(prefixes) != 3 {
		t.Fatalf("expected 3 prefixes, got %d", len(prefixes))
	}
	if prefixes[0] != "makeacompany:waitlist:" {
		t.Fatalf("expected makeacompany prefix first, got %q", prefixes[0])
	}
}

func TestResolveWaitlistPrefixes_ExplicitAllowed(t *testing.T) {
	s := &ProxyServer{
		cfg: &ProxyConfig{
			WaitlistPrefixes: []string{"waitlist:", "makeacompany:waitlist:"},
		},
	}
	prefixes, err := s.resolveWaitlistPrefixes("waitlist:")
	if err != nil {
		t.Fatalf("resolve explicit prefix: %v", err)
	}
	if len(prefixes) != 1 || prefixes[0] != "waitlist:" {
		t.Fatalf("unexpected explicit prefixes result: %#v", prefixes)
	}
}
