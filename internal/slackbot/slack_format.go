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
	reSpeakerPrefix = regexp.MustCompile(`(?i)^\s*(?:\*{1,2}\s*)?(?:alex|tim|ross|garth|assistant)\s*:\s*`)
	reWordToken     = regexp.MustCompile(`(?i)[a-z][a-z'_-]*`)
)

const (
	slackReplyMaxLines = 4
	slackReplyMaxRunes = 600
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
	s = stripSpeakerPrefixes(s)
	s = convertGitHubBoldToSlack(s)
	s = reBracketLinkMD.ReplaceAllString(s, "$1")
	s = reMDHeading.ReplaceAllString(s, "")
	s = substituteSquadAtMentions(s, cfg)
	s = stripOutgoingSelfMentions(s, cfg, selfSlackUserID)
	return strings.TrimSpace(s)
}

func stripSpeakerPrefixes(s string) string {
	lines := strings.Split(s, "\n")
	out := make([]string, 0, len(lines))
	for _, line := range lines {
		clean := strings.TrimSpace(reSpeakerPrefix.ReplaceAllString(line, ""))
		if clean == "" {
			continue
		}
		out = append(out, clean)
	}
	return strings.Join(out, "\n")
}

// normalizeSlackReply applies Slack formatting fixes plus a strict short-form cap.
func normalizeSlackReply(s string, cfg *config.Config, selfSlackUserID string) string {
	s = formatOutgoingSlackMessage(s, cfg, selfSlackUserID)
	s = normalizeSelfReferencePlainText(s, cfg)
	s = capSlackReplyLength(s, slackReplyMaxLines, slackReplyMaxRunes)
	return strings.TrimSpace(s)
}

// normalizeSelfReferencePlainText rewrites plain self-name references to "me"
// so agent replies use first-person voice ("me"/"I") instead of persona names.
// It deliberately skips Slack mention tokens (<@U...>) to avoid breaking pings.
func normalizeSelfReferencePlainText(s string, cfg *config.Config) string {
	if cfg == nil {
		return s
	}
	selfKey := strings.TrimSpace(cfg.EmployeeID)
	if selfKey == "" {
		return s
	}
	reSelf := regexp.MustCompile(`(?i)\b` + regexp.QuoteMeta(selfKey) + `\b`)
	parts := reSlackMention.Split(s, -1)
	if len(parts) == 0 {
		return s
	}
	mentions := reSlackMention.FindAllString(s, -1)
	for i := range parts {
		parts[i] = rewriteSelfNameToPronoun(parts[i], reSelf)
		parts[i] = normalizeAwkwardFirstPersonGrammar(parts[i])
	}

	var b strings.Builder
	for i := 0; i < len(parts); i++ {
		b.WriteString(parts[i])
		if i < len(mentions) {
			b.WriteString(mentions[i])
		}
	}

	out := reCollapseSpaces.ReplaceAllString(b.String(), " ")
	out = strings.ReplaceAll(out, " ,", ",")
	out = strings.ReplaceAll(out, " .", ".")
	out = strings.ReplaceAll(out, " ?", "?")
	out = strings.ReplaceAll(out, " !", "!")
	return strings.TrimSpace(out)
}

func rewriteSelfNameToPronoun(s string, reSelf *regexp.Regexp) string {
	if s == "" || reSelf == nil {
		return s
	}
	matches := reSelf.FindAllStringIndex(s, -1)
	if len(matches) == 0 {
		return s
	}
	var b strings.Builder
	last := 0
	for _, m := range matches {
		if len(m) != 2 || m[0] < last {
			continue
		}
		start, end := m[0], m[1]
		b.WriteString(s[last:start])
		if isSentenceStartPosition(s, start) {
			b.WriteString("I")
		} else {
			b.WriteString("me")
		}
		last = end
	}
	b.WriteString(s[last:])
	return b.String()
}

func isSentenceStartPosition(s string, idx int) bool {
	if idx <= 0 {
		return true
	}
	for i := idx - 1; i >= 0; i-- {
		switch s[i] {
		case ' ', '\t':
			continue
		case '\n', '.', '!', '?':
			return true
		default:
			return false
		}
	}
	return true
}

func normalizeAwkwardFirstPersonGrammar(s string) string {
	if s == "" {
		return s
	}
	matches := reWordToken.FindAllStringIndex(s, -1)
	if len(matches) < 2 {
		return s
	}
	buf := []byte(s)
	for i := 0; i < len(matches)-1; i++ {
		aStart, aEnd := matches[i][0], matches[i][1]
		bStart, bEnd := matches[i+1][0], matches[i+1][1]
		first := strings.ToLower(string(buf[aStart:aEnd]))
		second := strings.ToLower(string(buf[bStart:bEnd]))
		if first != "me" {
			continue
		}
		replacement := ""
		switch second {
		case "is", "am", "are":
			replacement = "I am"
		case "was", "were":
			replacement = "I was"
		case "have", "had", "will", "can", "could", "should", "would", "do", "did", "need", "want", "think", "know", "feel", "recommend", "prefer", "agree", "disagree", "support", "understand", "write", "wrote", "plan", "guess", "see", "saw", "hear", "heard", "believe":
			replacement = "I " + second
		default:
			continue
		}
		buf = append(buf[:aStart], append([]byte(replacement), buf[bEnd:]...)...)
		return normalizeAwkwardFirstPersonGrammar(string(buf))
	}
	return string(buf)
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
	truncated := strings.TrimSpace(string(r[:maxRunes]))
	if truncated == "" {
		return ""
	}
	// Prefer a complete sentence boundary over hard rune clipping.
	lastPunct := strings.LastIndexAny(truncated, ".!?")
	if lastPunct >= 0 {
		safe := strings.TrimSpace(truncated[:lastPunct+1])
		if safe != "" {
			return safe
		}
	}
	last := truncated[len(truncated)-1]
	if last != '.' && last != '!' && last != '?' && last != '"' && last != '\'' {
		truncated += "."
	}
	return truncated
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
