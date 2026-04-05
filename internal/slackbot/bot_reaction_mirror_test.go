package slackbot

import (
	"testing"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/slack-go/slack/slackevents"
)

func TestShouldMirrorThumbsUpReaction(t *testing.T) {
	baseCfg := &config.Config{
		ChatAllowedUserID: "UGRANT",
	}
	baseEvent := &slackevents.ReactionAddedEvent{
		User:     "UGRANT",
		Reaction: "+1",
		ItemUser: "UROSS",
		Item:     slackevents.Item{Type: "message", Channel: "CCHAT", Timestamp: "1740000000.000100"},
	}

	tests := []struct {
		name  string
		cfg   *config.Config
		botID string
		ev    *slackevents.ReactionAddedEvent
		want  bool
	}{
		{
			name:  "mirrors grant thumbs up on bot message",
			cfg:   baseCfg,
			botID: "UROSS",
			ev:    baseEvent,
			want:  true,
		},
		{
			name:  "skips non thumbs up reactions",
			cfg:   baseCfg,
			botID: "UROSS",
			ev: &slackevents.ReactionAddedEvent{
				User: "UGRANT", Reaction: "rocket", ItemUser: "UROSS",
				Item: slackevents.Item{Type: "message", Channel: "CCHAT", Timestamp: "1740000000.000100"},
			},
			want: false,
		},
		{
			name:  "skips reactions from non grant user",
			cfg:   baseCfg,
			botID: "UROSS",
			ev: &slackevents.ReactionAddedEvent{
				User: "UOTHER", Reaction: "+1", ItemUser: "UROSS",
				Item: slackevents.Item{Type: "message", Channel: "CCHAT", Timestamp: "1740000000.000100"},
			},
			want: false,
		},
		{
			name:  "skips reactions on other bot messages",
			cfg:   baseCfg,
			botID: "UROSS",
			ev: &slackevents.ReactionAddedEvent{
				User: "UGRANT", Reaction: "+1", ItemUser: "UTIM",
				Item: slackevents.Item{Type: "message", Channel: "CCHAT", Timestamp: "1740000000.000100"},
			},
			want: false,
		},
		{
			name:  "skips non message item",
			cfg:   baseCfg,
			botID: "UROSS",
			ev: &slackevents.ReactionAddedEvent{
				User: "UGRANT", Reaction: "+1", ItemUser: "UROSS",
				Item: slackevents.Item{Type: "file", Channel: "CCHAT", Timestamp: "1740000000.000100"},
			},
			want: false,
		},
		{
			name: "skips channel outside allowlist when enforced",
			cfg: &config.Config{
				ChatAllowedUserID:      "UGRANT",
				CompanyChannelsEnforce: true,
				CompanyChannels: map[string]config.CompanyChannelRuntime{
					"CALLOWED": {ChannelID: "CALLOWED"},
				},
			},
			botID: "UROSS",
			ev: &slackevents.ReactionAddedEvent{
				User: "UGRANT", Reaction: "+1", ItemUser: "UROSS",
				Item: slackevents.Item{Type: "message", Channel: "CDENIED", Timestamp: "1740000000.000100"},
			},
			want: false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := shouldMirrorThumbsUpReaction(tc.cfg, tc.botID, tc.ev)
			if got != tc.want {
				t.Fatalf("shouldMirrorThumbsUpReaction() = %t, want %t", got, tc.want)
			}
		})
	}
}
