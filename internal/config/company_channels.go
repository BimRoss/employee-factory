package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

// CompanyChannelRuntime defines the first-pass runtime contract for one Slack channel/company.
type CompanyChannelRuntime struct {
	CompanySlug             string `json:"company_slug"`
	ChannelID               string `json:"channel_id"`
	DisplayName             string `json:"display_name,omitempty"`
	PrimaryOwner            string `json:"primary_owner,omitempty"`
	ThreadsEnabled          bool   `json:"threads_enabled"`
	GeneralAutoReplyEnabled bool   `json:"general_auto_reply_enabled"`
}

func parseCompanyChannelsEnv(cfg *Config) error {
	if cfg == nil {
		return nil
	}
	cfg.CompanyChannelsEnforce = parseBoolEnv("COMPANY_CHANNELS_ENFORCE", false)
	raw := strings.TrimSpace(os.Getenv("COMPANY_CHANNELS_JSON"))
	if raw == "" {
		cfg.CompanyChannels = nil
		return nil
	}
	m, err := parseCompanyChannelsJSON(raw)
	if err != nil {
		return err
	}
	cfg.CompanyChannels = m
	return nil
}

func parseCompanyChannelsJSON(raw string) (map[string]CompanyChannelRuntime, error) {
	var entries []CompanyChannelRuntime
	if err := json.Unmarshal([]byte(raw), &entries); err != nil {
		return nil, fmt.Errorf("company-channels: invalid COMPANY_CHANNELS_JSON: %w", err)
	}
	if len(entries) == 0 {
		return nil, fmt.Errorf("company-channels: COMPANY_CHANNELS_JSON parsed empty")
	}
	out := make(map[string]CompanyChannelRuntime, len(entries))
	for i, e := range entries {
		channelID := strings.TrimSpace(e.ChannelID)
		if channelID == "" {
			return nil, fmt.Errorf("company-channels: entry %d missing channel_id", i)
		}
		e.ChannelID = channelID
		e.CompanySlug = strings.TrimSpace(e.CompanySlug)
		e.DisplayName = strings.TrimSpace(e.DisplayName)
		e.PrimaryOwner = strings.TrimSpace(e.PrimaryOwner)
		out[channelID] = e
	}
	return out, nil
}

func (c *Config) CompanyChannelForID(channelID string) (CompanyChannelRuntime, bool) {
	if c == nil || len(c.CompanyChannels) == 0 {
		return CompanyChannelRuntime{}, false
	}
	ch, ok := c.CompanyChannels[strings.TrimSpace(channelID)]
	return ch, ok
}

func (c *Config) ChannelAllowed(channelID string) bool {
	if c == nil {
		return false
	}
	if !c.CompanyChannelsEnforce || len(c.CompanyChannels) == 0 {
		return true
	}
	_, ok := c.CompanyChannelForID(channelID)
	return ok
}
