package slackbot

import (
	"regexp"
	"strings"

	"github.com/bimross/employee-factory/internal/config"
	"github.com/slack-go/slack"
)

var (
	reDirectChallenge = regexp.MustCompile(`(?i)\b(?:you(?:'|’)re?\s+wrong|you\s+are\s+wrong|you\s+don(?:'|’)t\s+get\s+it|that(?:'|’)s\s+wrong|that\s+is\s+wrong|not\s+what\s+i\s+mean|you(?:'|’)re?\s+missing|you\s+missed)\b`)
	reChallengeTone   = regexp.MustCompile(`(?i)\b(?:wrong|don(?:'|’)t|get\s+it|miss(?:ed|ing)?|not|no|actually|instead)\b`)
	reWord            = regexp.MustCompile(`(?i)[a-z][a-z0-9_-]{2,}`)
)

var rethinkStopWords = map[string]bool{
	"that": true, "this": true, "with": true, "from": true, "your": true, "you": true, "are": true, "the": true, "and": true,
	"for": true, "not": true, "but": true, "have": true, "has": true, "had": true, "was": true, "were": true, "what": true,
	"when": true, "where": true, "why": true, "how": true, "our": true, "its": true, "it's": true, "into": true, "then": true,
	"them": true, "they": true, "will": true, "would": true, "should": true, "could": true, "about": true, "just": true,
}

const rethinkSystemCue = "Rethink trigger: the user challenged your previous position. First line: acknowledge reassessment and restate their correction briefly. Second line: update your position/confidence and give one concrete next action."

func shouldTriggerRethinkCue(userText, priorAgentText string) bool {
	userText = strings.TrimSpace(userText)
	priorAgentText = strings.TrimSpace(priorAgentText)
	if userText == "" || priorAgentText == "" {
		return false
	}
	if !reChallengeTone.MatchString(userText) {
		return false
	}
	if reDirectChallenge.MatchString(userText) {
		return true
	}
	overlap := lexicalOverlapCount(userText, priorAgentText)
	return overlap >= 2
}

func prependRethinkCue(payload, userText, priorAgentText string) string {
	if !shouldTriggerRethinkCue(userText, priorAgentText) {
		return payload
	}
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return rethinkSystemCue
	}
	return rethinkSystemCue + "\n\n" + payload
}

func lexicalOverlapCount(a, b string) int {
	aw := contentWordSet(a)
	bw := contentWordSet(b)
	n := 0
	for w := range aw {
		if bw[w] {
			n++
		}
	}
	return n
}

func contentWordSet(text string) map[string]bool {
	set := map[string]bool{}
	for _, m := range reWord.FindAllString(strings.ToLower(text), -1) {
		if rethinkStopWords[m] {
			continue
		}
		set[m] = true
	}
	return set
}

func latestPriorEmployeeMessageInThread(msgs []slack.Message, currentMsgTS, employeeKey, botUserID string, cfg *config.Config) string {
	emp := strings.ToLower(strings.TrimSpace(employeeKey))
	for i := len(msgs) - 1; i >= 0; i-- {
		m := msgs[i]
		if strings.TrimSpace(m.Timestamp) == strings.TrimSpace(currentMsgTS) {
			continue
		}
		text := strings.TrimSpace(m.Text)
		if text == "" {
			continue
		}
		if m.User == botUserID {
			return text
		}
		if k, ok := squadKeyForSlackUser(cfg, m.User); ok && strings.EqualFold(k, emp) {
			return text
		}
	}
	return ""
}

func latestPriorEmployeeMessageInSquadMessages(msgs []slack.Message, botUserID string) string {
	for i := len(msgs) - 1; i >= 0; i-- {
		if strings.TrimSpace(msgs[i].User) != strings.TrimSpace(botUserID) {
			continue
		}
		t := strings.TrimSpace(msgs[i].Text)
		if t != "" {
			return t
		}
	}
	return ""
}

