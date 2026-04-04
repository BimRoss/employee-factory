package slackbot

import (
	"context"
	"strings"

	"github.com/slack-go/slack"
)

type slackResponse struct {
	Text     string
	Blocks   []slack.Block
	ThreadTS string
}

func (b *Bot) postSlackResponse(ctx context.Context, channel string, resp slackResponse) error {
	text := strings.TrimSpace(resp.Text)
	if text == "" {
		text = "…"
	}
	opts := []slack.MsgOption{slack.MsgOptionText(text, false)}
	if len(resp.Blocks) > 0 {
		opts = append(opts, slack.MsgOptionBlocks(resp.Blocks...))
	}
	if ts := strings.TrimSpace(resp.ThreadTS); ts != "" {
		opts = append(opts, slack.MsgOptionTS(ts))
	}
	_, _, err := b.api.PostMessageContext(ctx, channel, opts...)
	return err
}
