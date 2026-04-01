package slackbot

import (
	"hash/fnv"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/bimross/employee-factory/internal/config"
)

// Slack message text uses "mrkdwn" (not GitHub/MD). Bold is *like this* (single asterisks).
// Models often emit **double** asterisks, which Slack shows literally—never as bold.
// See: https://api.slack.com/reference/surfaces/formatting
var (
	reGitHubBold    = regexp.MustCompile(`\*\*([^*\n]+)\*\*`)
	reMDHeading     = regexp.MustCompile(`(?m)^#{1,6}\s+`)
	reBracketLinkMD = regexp.MustCompile(`\[([^\]]+)\]\([^)]+\)`)
	reSlackMention  = regexp.MustCompile(`<@(U[A-Za-z0-9]+)(?:\|[^>]+)?>`)
)

const (
	slackReplyMaxLines = 2
	slackReplyMaxRunes = 240
)

type squadMember struct {
	key string
	uid string
}

// formatOutgoingSlackMessage normalizes common LLM Markdown habits so Slack renders cleanly.
// Product intent: keep expressive Slack-native mrkdwn (bold, code, quotes)—not “plain ASCII only.”
// Block Kit modals / richer interactive surfaces stay out of scope until we wire them deliberately.
// It converts **bold** to *bold* (Slack mrkdwn), strips ATX # headings, simplifies [text](url) to label text,
// and rewrites @ross / @tim / @alex / @garth (any configured squad keys) to Slack mention tokens when squad IDs are configured.
// selfSlackUserID is this process's Slack bot user id (from auth.test); pass "" in tests. Used to strip self-<@U…> if the model emits it.
func formatOutgoingSlackMessage(s string, cfg *config.Config, selfSlackUserID string) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	s = convertGitHubBoldToSlack(s)
	s = reBracketLinkMD.ReplaceAllString(s, "$1")
	s = reMDHeading.ReplaceAllString(s, "")
	s = substituteSquadAtMentions(s, cfg)
	s = stripOutgoingSelfMentions(s, cfg, selfSlackUserID)
	return strings.TrimSpace(s)
}

// normalizeSlackReply applies Slack formatting fixes plus a strict short-form cap.
func normalizeSlackReply(s string, cfg *config.Config, selfSlackUserID string) string {
	s = formatOutgoingSlackMessage(s, cfg, selfSlackUserID)
	s = capSlackReplyLength(s, slackReplyMaxLines, slackReplyMaxRunes)
	return strings.TrimSpace(s)
}

func capSlackReplyLength(s string, maxLines int, maxRunes int) string {
	s = strings.TrimSpace(s)
	if s == "" {
		return s
	}
	if maxLines < 1 {
		maxLines = 1
	}
	if maxRunes < 8 {
		maxRunes = 8
	}
	rawLines := strings.Split(s, "\n")
	lines := make([]string, 0, len(rawLines))
	for _, line := range rawLines {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		lines = append(lines, reCollapseSpaces.ReplaceAllString(t, " "))
	}
	if len(lines) == 0 {
		return ""
	}
	if len(lines) > maxLines {
		lines = lines[:maxLines]
	}
	out := strings.Join(lines, "\n")
	if utf8.RuneCountInString(out) <= maxRunes {
		return out
	}
	r := []rune(out)
	if maxRunes <= 3 {
		return string(r[:maxRunes])
	}
	return strings.TrimSpace(string(r[:maxRunes-3])) + "..."
}

func convertGitHubBoldToSlack(s string) string {
	for i := 0; i < 32 && strings.Contains(s, "**"); i++ {
		next := reGitHubBold.ReplaceAllString(s, `*$1*`)
		if next == s {
			break
		}
		s = next
	}
	// Models still emit stray ** (nested emphasis, odd spans, punctuation edge cases).
	// Repeatedly collapse ** → * until none remain so Slack never shows literal double asterisks.
	for i := 0; i < 32 && strings.Contains(s, "**"); i++ {
		s = strings.ReplaceAll(s, "**", "*")
	}
	return s
}

// substituteSquadAtMentions turns @ross (any case) into <@U…> so Slack notifies and linkifies correctly.
// Plain @name without a user id does not reliably ping; see https://api.slack.com/reference/surfaces/formatting
// The current employee (cfg.EmployeeID) is never converted—stripOutgoingSelfMentions removes any @self / <@self> afterward.
func substituteSquadAtMentions(s string, cfg *config.Config) string {
	if cfg == nil || len(cfg.MultiagentBotUserIDs) == 0 {
		return s
	}
	selfKey := strings.ToLower(strings.TrimSpace(cfg.EmployeeID))
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
		if selfKey != "" && strings.EqualFold(key, selfKey) {
			continue
		}
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

var reCollapseSpaces = regexp.MustCompile(`[ \t]{2,}`)

// enforceMultiagentMentionPolicy keeps cross-agent loops organic while bounded:
// requireHandoff=true  => exactly one other-agent mention in final text.
// requireHandoff=false => no squad mentions in final text.
func enforceMultiagentMentionPolicy(s string, cfg *config.Config, selfSlackUserID string, requireHandoff bool) string {
	if cfg == nil || len(cfg.MultiagentBotUserIDs) == 0 {
		return strings.TrimSpace(s)
	}
	selfKey := strings.ToLower(strings.TrimSpace(cfg.EmployeeID))
	selfID := strings.TrimSpace(selfSlackUserID)
	if selfID == "" && selfKey != "" {
		selfID = strings.TrimSpace(cfg.MultiagentBotUserIDs[selfKey])
	}

	var members []squadMember
	for k, uid := range cfg.MultiagentBotUserIDs {
		key := strings.ToLower(strings.TrimSpace(k))
		uid = strings.TrimSpace(uid)
		if key == "" || uid == "" {
			continue
		}
		if key == selfKey || (selfID != "" && uid == selfID) {
			continue
		}
		members = append(members, squadMember{key: key, uid: uid})
	}
	if len(members) == 0 {
		return strings.TrimSpace(s)
	}
	sort.Slice(members, func(i, j int) bool { return members[i].key < members[j].key })

	keepUID := ""
	if requireHandoff {
		matches := reSlackMention.FindAllStringSubmatch(s, -1)
		valid := make(map[string]bool, len(members))
		for _, m := range members {
			valid[m.uid] = true
		}
		for _, m := range matches {
			if len(m) < 2 {
				continue
			}
			uid := strings.TrimSpace(m[1])
			if valid[uid] {
				keepUID = uid
				break
			}
		}
	}

	out := s
	for _, m := range members {
		reTok := regexp.MustCompile(`<@` + regexp.QuoteMeta(m.uid) + `(?:\|[^>]+)?>`)
		out = reTok.ReplaceAllString(out, "")
		rePlain := regexp.MustCompile(`(?i)@` + regexp.QuoteMeta(m.key) + `\b`)
		out = rePlain.ReplaceAllString(out, "")
	}
	out = reCollapseSpaces.ReplaceAllString(out, " ")
	out = strings.ReplaceAll(out, " ,", ",")
	out = strings.ReplaceAll(out, " .", ".")
	out = strings.ReplaceAll(out, " ?", "?")
	out = strings.ReplaceAll(out, " !", "!")
	out = strings.TrimSpace(out)

	if !requireHandoff {
		if out == "" {
			return "..."
		}
		return out
	}

	if keepUID == "" {
		keepUID = pickMentionUID(members, out)
	}
	mention := "<@" + keepUID + ">"
	if out == "" {
		return mention + " quick take?"
	}
	if strings.HasSuffix(out, "?") {
		return out + " " + mention
	}
	return out + " " + mention + " quick take?"
}

func pickMentionUID(members []squadMember, seed string) string {
	if len(members) == 0 {
		return ""
	}
	h := fnv.New32a()
	_, _ = h.Write([]byte(seed))
	idx := int(h.Sum32() % uint32(len(members)))
	return members[idx].uid
}

// stripOutgoingSelfMentions removes pings to this bot: plain @employee_id and Slack tokens <@U…> (optional |label).
func stripOutgoingSelfMentions(s string, cfg *config.Config, selfSlackUserID string) string {
	if cfg == nil {
		return s
	}
	selfKey := strings.ToLower(strings.TrimSpace(cfg.EmployeeID))
	if selfKey == "" {
		return s
	}
	out := s
	uid := strings.TrimSpace(selfSlackUserID)
	if uid == "" && len(cfg.MultiagentBotUserIDs) > 0 {
		uid = strings.TrimSpace(cfg.MultiagentBotUserIDs[selfKey])
	}
	if uid != "" {
		reTok := regexp.MustCompile(`<@` + regexp.QuoteMeta(uid) + `(?:\|[^>]+)?>`)
		out = reTok.ReplaceAllString(out, "")
	}
	rePlain := regexp.MustCompile(`(?i)@` + regexp.QuoteMeta(selfKey) + `\b`)
	out = rePlain.ReplaceAllString(out, "")
	out = reCollapseSpaces.ReplaceAllString(out, " ")
	out = strings.ReplaceAll(out, " ,", ",")
	out = strings.ReplaceAll(out, " .", ".")
	out = strings.ReplaceAll(out, " ?", "?")
	out = strings.ReplaceAll(out, " !", "!")
	return strings.TrimSpace(out)
}
