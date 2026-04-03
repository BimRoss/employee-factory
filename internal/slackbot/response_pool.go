package slackbot

import (
	"regexp"
	"strings"

	"github.com/bimross/employee-factory/internal/config"
)

var (
	reOnboardingIntent = regexp.MustCompile(`(?i)\b(welcome|welcoming|welcome aboard|onboard(?:ing)?|on board|glad to have you|great to have you|happy to have you)\b`)
	reWordTokenPool    = regexp.MustCompile(`[a-z0-9_-]+`)
)

// resolveBroadcastCandidatePool returns an ordered participant pool for broadcast routing.
// On onboarding-style messages, it excludes the detected target agent unless they were explicitly @mentioned.
func resolveBroadcastCandidatePool(rawText string, cfg *config.Config) []string {
	base := orderedBroadcastPool(cfg)
	if len(base) == 0 {
		return nil
	}
	if !isOnboardingWelcomeIntent(rawText) {
		return base
	}
	target, ok := detectOnboardingTargetKey(rawText, cfg, base)
	if !ok || target == "" {
		return base
	}
	if isExplicitlyMentionedKey(rawText, cfg, target) {
		// Explicit @mention always overrides pool filtering.
		return base
	}
	filtered := poolWithoutKey(base, target)
	// Keep multiagent sessions viable (minimum two participants).
	if len(filtered) < 2 {
		return base
	}
	return filtered
}

func orderedBroadcastPool(cfg *config.Config) []string {
	if cfg == nil {
		return nil
	}
	out := make([]string, 0, len(cfg.MultiagentOrder))
	seen := make(map[string]bool, len(cfg.MultiagentOrder))
	for _, key := range cfg.MultiagentOrder {
		trimmed := strings.ToLower(strings.TrimSpace(key))
		if trimmed == "" || seen[trimmed] {
			continue
		}
		seen[trimmed] = true
		out = append(out, trimmed)
	}
	return out
}

func isOnboardingWelcomeIntent(rawText string) bool {
	return reOnboardingIntent.MatchString(strings.TrimSpace(rawText))
}

func detectOnboardingTargetKey(rawText string, cfg *config.Config, pool []string) (string, bool) {
	mentionKeys := mentionedSquadKeys(rawText, cfg)
	if len(mentionKeys) == 1 {
		return strings.ToLower(strings.TrimSpace(mentionKeys[0])), true
	}
	if len(mentionKeys) > 1 {
		return "", false
	}

	tokenSet := tokenSet(rawText)
	var matched []string
	for _, key := range pool {
		if tokenSet[key] {
			matched = append(matched, key)
		}
	}
	if len(matched) != 1 {
		return "", false
	}
	return matched[0], true
}

func tokenSet(rawText string) map[string]bool {
	out := map[string]bool{}
	for _, tok := range reWordTokenPool.FindAllString(strings.ToLower(rawText), -1) {
		out[tok] = true
	}
	return out
}

func isExplicitlyMentionedKey(rawText string, cfg *config.Config, key string) bool {
	for _, mention := range mentionedSquadKeys(rawText, cfg) {
		if strings.EqualFold(strings.TrimSpace(mention), strings.TrimSpace(key)) {
			return true
		}
	}
	plainAt := regexp.MustCompile(`(?i)(?:^|\s)@` + regexp.QuoteMeta(strings.TrimSpace(key)) + `\b`)
	if plainAt.MatchString(rawText) {
		return true
	}
	return false
}

func poolWithoutKey(pool []string, key string) []string {
	key = strings.TrimSpace(key)
	out := make([]string, 0, len(pool))
	for _, candidate := range pool {
		if strings.EqualFold(strings.TrimSpace(candidate), key) {
			continue
		}
		out = append(out, candidate)
	}
	return out
}

func removedPoolKeys(before []string, after []string) []string {
	if len(before) == 0 {
		return nil
	}
	afterSet := make(map[string]bool, len(after))
	for _, key := range after {
		afterSet[strings.ToLower(strings.TrimSpace(key))] = true
	}
	var removed []string
	for _, key := range before {
		normalized := strings.ToLower(strings.TrimSpace(key))
		if normalized == "" {
			continue
		}
		if !afterSet[normalized] {
			removed = append(removed, normalized)
		}
	}
	return removed
}
