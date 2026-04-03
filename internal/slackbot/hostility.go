package slackbot

import (
	"regexp"
	"strings"
)

var reDirectHostility = regexp.MustCompile(`(?i)\b(?:fuck\s+off|fuck\s+you|shut\s+up|you\s+suck|you(?:'|’)re\s+(?:an?\s+)?(?:idiot|moron|stupid)|go\s+to\s+hell|piss\s+off|eat\s+shit)\b`)

const hostilitySystemCue = "Hostile user input detected: respond with one brief firm pushback line (light emotional mirroring), avoid appeasing filler, then immediately redirect to a concrete next action."

func isDirectlyHostileToAgent(text string) bool {
	text = strings.TrimSpace(text)
	if text == "" {
		return false
	}
	return reDirectHostility.MatchString(text)
}

func prependHostilityCue(payload string, latestUserText string) string {
	if !isDirectlyHostileToAgent(latestUserText) {
		return payload
	}
	payload = strings.TrimSpace(payload)
	if payload == "" {
		return hostilitySystemCue
	}
	return hostilitySystemCue + "\n\n" + payload
}
