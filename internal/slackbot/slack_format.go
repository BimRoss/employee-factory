package slackbot

import (
	"regexp"
	"strings"
)

// Slack message text uses "mrkdwn" (not GitHub/MD). Bold is *like this* (single asterisks).
// Models often emit **double** asterisks, which Slack shows literally—never as bold.
// See: https://api.slack.com/reference/surfaces/formatting
var (
	reGitHubBold    = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
	reMDHeading     = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	reBracketLinkMD = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
)

// formatOutgoingSlackMessage normalizes common LLM Markdown habits so Slack renders cleanly.
// Product intent: keep expressive Slack-native mrkdwn (bold, code, quotes)—not “plain ASCII only.”
// Block Kit modals / richer interactive surfaces stay out of scope until we wire them deliberately.
// It converts **bold** to *bold* (Slack mrkdwn), strips ATX # headings, and simplifies [text](url) to label text.
func formatOutgoingSlackMessage(s string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	s = reGitHubBold.ReplaceAllString(s, `*$1*`)
	s = reBracketLinkMD.ReplaceAllString(s, "$1")
	s = reMDHeading.ReplaceAllString(s, "")
	return strings.TrimSpace(s)
}
