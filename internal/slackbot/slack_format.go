package slackbot

import (
	"regexp"
	"sort"
	"strings"

	"github.com/bimross/employee-factory/internal/config"
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
// It converts **bold** to *bold* (Slack mrkdwn), strips ATX # headings, simplifies [text](url) to label text,
// and rewrites @ross / @tim / @alex to Slack mention tokens when squad IDs are configured.
func formatOutgoingSlackMessage(s string, cfg *config.Config) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	s = convertGitHubBoldToSlack(s)
	s = reBracketLinkMD.ReplaceAllString(s, "$1")
	s = reMDHeading.ReplaceAllString(s, "")
	s = substituteSquadAtMentions(s, cfg)
	return strings.TrimSpace(s)
}

func convertGitHubBoldToSlack(s string) string {
	for i := 0; i < 32 && strings.Contains(s, "**"); i++ {
		next := reGitHubBold.ReplaceAllString(s, `*$1*`)
		if next == s {
			break
		}
		s = next
	}
	return s
}

// substituteSquadAtMentions turns @ross (any case) into <@U…> so Slack notifies and linkifies correctly.
// Plain @name without a user id does not reliably ping; see https://api.slack.com/reference/surfaces/formatting
func substituteSquadAtMentions(s string, cfg *config.Config) string {
	if cfg == nil || len(cfg.MultiagentBotUserIDs) == 0 {
		return s
	}
	var keys []string
	for k := range cfg.MultiagentBotUserIDs {
		uid := strings.TrimSpace(cfg.MultiagentBotUserIDs[k])
		if uid == "" {
			continue
		}
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool { return len(keys[i]) > len(keys[j]) })
	out := s
	for _, key := range keys {
		uid := strings.TrimSpace(cfg.MultiagentBotUserIDs[key])
		if uid == "" {
			continue
		}
		token := "<@" + uid + ">"
		re := regexp.MustCompile(`(?i)@` + regexp.QuoteMeta(key) + `\b`)
		out = re.ReplaceAllString(out, token)
	}
	return out
}
