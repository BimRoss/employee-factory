package config

import "testing"

func TestParseCompanyChannelsJSON(t *testing.T) {
	raw := `[{"company_slug":"acme","channel_id":"C123","display_name":"Acme Inc","threads_enabled":true,"general_auto_reaction_enabled":true}]`
	got, err := parseCompanyChannelsJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	ch, ok := got["C123"]
	if !ok {
		t.Fatalf("expected C123 in map: %#v", got)
	}
	if ch.CompanySlug != "acme" || ch.DisplayName != "Acme Inc" {
		t.Fatalf("unexpected channel runtime: %#v", ch)
	}
}

func TestParseCompanyChannelsJSON_RequiresChannelID(t *testing.T) {
	raw := `[{"company_slug":"acme"}]`
	if _, err := parseCompanyChannelsJSON(raw); err == nil {
		t.Fatal("expected error for missing channel_id")
	}
}

func TestChannelAllowed(t *testing.T) {
	cfg := &Config{
		CompanyChannelsEnforce: true,
		CompanyChannels: map[string]CompanyChannelRuntime{
			"C123": {CompanySlug: "acme", ChannelID: "C123"},
		},
	}
	if !cfg.ChannelAllowed("C123") {
		t.Fatal("expected configured channel to be allowed")
	}
	if cfg.ChannelAllowed("C999") {
		t.Fatal("expected unknown channel to be denied when enforcement enabled")
	}
	cfg.CompanyChannelsEnforce = false
	if !cfg.ChannelAllowed("C999") {
		t.Fatal("expected unknown channel to be allowed when enforcement disabled")
	}
}
