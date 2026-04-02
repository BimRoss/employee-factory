package slackbot

import (
	"context"
	"log"
	"regexp"
	"strings"
)

var (
	reInternalArtifactToken = regexp.MustCompile(`(?i)\b(?:alex|tim|ross|garth)-[a-z0-9-]{3,}\b`)
	reInternalArtifactLine  = regexp.MustCompile(`(?i)^\s*(?:\*{1,2}\s*)?(?:current state:|closer\b|the rule\b|tim-systems-delegation\b|here is the rewritten slack reply:|conversation summary\b|step\s+\d+\s*:|ross\s*:|alex\s*:|garth\s*:|tim\s*:|assistant\s*:)\s*`)
	reLikelyCutoffTail      = regexp.MustCompile(`(?i)\b(?:and|or|but|so|because|with|without|to|for|of|in|on|at|from|by|about|into|onto|whats|what's)\s*$`)
	reAwkwardFirstPerson    = regexp.MustCompile(`(?i)(^|[.!?\n]\s*)me\s+(?:is|am|are|was|were|have|had|will|can|could|should|would|do|did|need|want|think|know|feel|recommend|prefer|agree|disagree|support|understand|write|wrote|plan|guess|see|saw|hear|heard|believe)\b`)
)

const outboundRepairSuffix = `
Outbound repair mode:
- Return one complete user-facing Slack reply only.
- Remove internal labels, prompt fragments, and rule slugs.
- Keep it concise (1-3 short lines) and end with a complete sentence.`

func outboundNeedsRepair(reply string) (bool, string) {
	s := strings.TrimSpace(reply)
	if s == "" {
		return true, "empty"
	}
	if reInternalArtifactToken.MatchString(s) {
		return true, "internal_artifact_token"
	}
	lines := strings.Split(s, "\n")
	for _, line := range lines {
		if reInternalArtifactLine.MatchString(strings.TrimSpace(line)) {
			return true, "internal_artifact_line"
		}
	}
	if strings.HasSuffix(s, "<@") || strings.HasSuffix(s, ":") || strings.HasSuffix(s, "-") {
		return true, "cutoff_tail"
	}
	if reLikelyCutoffTail.MatchString(s) {
		return true, "dangling_tail_word"
	}
	if reAwkwardFirstPerson.MatchString(s) {
		return true, "awkward_first_person_grammar"
	}
	last := s[len(s)-1]
	if (last >= 'a' && last <= 'z') || (last >= 'A' && last <= 'Z') || (last >= '0' && last <= '9') {
		// Plain alnum ending can be valid, but in this Slack bot it often correlates with clipped tails.
		// Keep it permissive: only flag very short or suspiciously terse outputs.
		if len([]rune(s)) < 20 {
			return true, "suspiciously_short_tail"
		}
	}
	return false, ""
}

func stripInternalArtifactLines(s string) string {
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(s), "\n") {
		t := strings.TrimSpace(line)
		if t == "" {
			continue
		}
		if reInternalArtifactLine.MatchString(t) || reInternalArtifactToken.MatchString(t) {
			continue
		}
		out = append(out, t)
	}
	return strings.TrimSpace(strings.Join(out, "\n"))
}

func fallbackCompleteReply(candidate string) string {
	clean := stripInternalArtifactLines(candidate)
	if clean == "" {
		return "Quick take: send that again and I will answer directly in one clean pass."
	}
	last := clean[len(clean)-1]
	if last != '.' && last != '!' && last != '?' && last != '"' && last != '\'' {
		clean += "."
	}
	return clean
}

func (b *Bot) repairOutboundReply(ctx context.Context, persona, userPayload, draft string) string {
	needsRepair, reason := outboundNeedsRepair(draft)
	if !needsRepair {
		return draft
	}
	prompt := "Original user payload:\n" + strings.TrimSpace(userPayload) +
		"\n\nDraft reply flagged by quality gate (" + reason + "):\n" + strings.TrimSpace(draft) +
		"\n\nRewrite it as a clean, complete Slack reply."
	fixed, err := b.llm.Reply(ctx, persona, slackReplySuffix+"\n\n"+outboundRepairSuffix, prompt)
	if err != nil {
		log.Printf("outbound repair: rewrite failed: %v", err)
		return fallbackCompleteReply(draft)
	}
	fixed = strings.TrimSpace(fixed)
	if fixed == "" {
		return fallbackCompleteReply(draft)
	}
	if flagged, _ := outboundNeedsRepair(fixed); flagged {
		return fallbackCompleteReply(fixed)
	}
	return fixed
}
