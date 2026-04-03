package router

import (
	"regexp"
	"strings"
)

type AvailabilityIntent string

const (
	AvailabilityIntentNormal       AvailabilityIntent = "normal"
	AvailabilityIntentAvailability AvailabilityIntent = "availability"
	AvailabilityIntentSignoff      AvailabilityIntent = "signoff"
)

type AvailabilityAction string

const (
	AvailabilityActionNormal  AvailabilityAction = "normal"
	AvailabilityActionAckOnly AvailabilityAction = "ack_only"
)

type AvailabilityDecision struct {
	Intent       AvailabilityIntent
	Action       AvailabilityAction
	Confidence   float64
	Reason       string
	MatchedTerms []string
}

type PresenceDecision struct {
	IsPresenceCheck bool
	Confidence      float64
	Reason          string
	MatchedTerms    []string
}

type phraseRule struct {
	phrase string
	re     *regexp.Regexp
}

var availabilityRules = []phraseRule{
	{phrase: "step away", re: regexp.MustCompile(`\bstep(?:ping)?\s+away\b`)},
	{phrase: "afk", re: regexp.MustCompile(`\bafk\b`)},
	{phrase: "away for the afternoon", re: regexp.MustCompile(`\baway\s+for\s+(?:the\s+)?afternoon\b`)},
	{phrase: "away for a bit", re: regexp.MustCompile(`\baway\s+for\s+(?:a\s+)?bit\b`)},
	{phrase: "back later", re: regexp.MustCompile(`\bback\s+later\b`)},
	{phrase: "brb", re: regexp.MustCompile(`\bbrb\b`)},
	{phrase: "going offline", re: regexp.MustCompile(`\b(?:going|go)\s+offline\b`)},
	{phrase: "i am offline", re: regexp.MustCompile(`\bi\s+am\s+offline\b`)},
	{phrase: "i'm offline", re: regexp.MustCompile(`\bi['’]m\s+offline\b`)},
}

var signoffRules = []phraseRule{
	{phrase: "go to bed", re: regexp.MustCompile(`\bgo(?:ing)?\s+to\s+bed\b`)},
	{phrase: "go to sleep", re: regexp.MustCompile(`\bgo(?:ing)?\s+to\s+sleep\b`)},
	{phrase: "sign off", re: regexp.MustCompile(`\bsign(?:ing)?\s+off\b`)},
	{phrase: "log off", re: regexp.MustCompile(`\blog(?:ging)?\s+off\b`)},
	{phrase: "call it a night", re: regexp.MustCompile(`\bcall(?:ing)?\s+it\s+a\s+night\b`)},
	{phrase: "heading to bed", re: regexp.MustCompile(`\bheading\s+to\s+bed\b`)},
}

var presenceCheckRules = []phraseRule{
	{phrase: "are you online", re: regexp.MustCompile(`\bare\s+you\s+(?:guys\s+)?online\b`)},
	{phrase: "you guys online", re: regexp.MustCompile(`\byou\s+guys\s+online\b`)},
	{phrase: "who is online", re: regexp.MustCompile(`\bwho(?:'s|\s+is)\s+online\b`)},
	{phrase: "everyone online", re: regexp.MustCompile(`\beveryone\s+online\b`)},
	{phrase: "online check", re: regexp.MustCompile(`\bonline\s+check\b`)},
	{phrase: "roll call", re: regexp.MustCompile(`\broll\s+call\b`)},
}

func ClassifyAvailability(text string) AvailabilityDecision {
	normalized := strings.TrimSpace(text)
	if normalized == "" {
		return AvailabilityDecision{
			Intent:     AvailabilityIntentNormal,
			Action:     AvailabilityActionNormal,
			Confidence: 1.0,
			Reason:     "empty_text",
		}
	}

	signoffMatches := matchPhrases(normalized, signoffRules)
	availabilityMatches := matchPhrases(normalized, availabilityRules)

	if len(signoffMatches) > 0 {
		return AvailabilityDecision{
			Intent:       AvailabilityIntentSignoff,
			Action:       AvailabilityActionAckOnly,
			Confidence:   confidenceForMatches(len(signoffMatches), len(availabilityMatches)),
			Reason:       "matched_signoff_cue",
			MatchedTerms: uniqueTerms(append(signoffMatches, availabilityMatches...)),
		}
	}
	if len(availabilityMatches) > 0 {
		return AvailabilityDecision{
			Intent:       AvailabilityIntentAvailability,
			Action:       AvailabilityActionAckOnly,
			Confidence:   confidenceForMatches(len(availabilityMatches), 0),
			Reason:       "matched_availability_cue",
			MatchedTerms: uniqueTerms(availabilityMatches),
		}
	}

	return AvailabilityDecision{
		Intent:     AvailabilityIntentNormal,
		Action:     AvailabilityActionNormal,
		Confidence: 1.0,
		Reason:     "no_availability_signal",
	}
}

func BuildAsyncSafeAck(intent AvailabilityIntent) string {
	switch intent {
	case AvailabilityIntentSignoff:
		return "Understood - I will go async here. When you are back, I will pick up from your latest priority in this thread."
	case AvailabilityIntentAvailability:
		return "Understood - I will switch to async while you are away. When you are back, I will continue from your latest priority in this thread."
	default:
		return ""
	}
}

func ClassifyPresenceCheck(text string) PresenceDecision {
	normalized := strings.TrimSpace(text)
	if normalized == "" {
		return PresenceDecision{
			IsPresenceCheck: false,
			Confidence:      1.0,
			Reason:          "empty_text",
		}
	}

	matches := matchPhrases(normalized, presenceCheckRules)
	if len(matches) == 0 {
		return PresenceDecision{
			IsPresenceCheck: false,
			Confidence:      1.0,
			Reason:          "no_presence_signal",
		}
	}

	return PresenceDecision{
		IsPresenceCheck: true,
		Confidence:      confidenceForMatches(len(matches), 0),
		Reason:          "matched_presence_cue",
		MatchedTerms:    uniqueTerms(matches),
	}
}

func matchPhrases(text string, rules []phraseRule) []string {
	lower := strings.ToLower(text)
	matches := make([]string, 0, len(rules))
	for _, rule := range rules {
		if rule.re.MatchString(lower) {
			matches = append(matches, rule.phrase)
		}
	}
	return matches
}

func confidenceForMatches(primaryMatches, secondaryMatches int) float64 {
	if primaryMatches >= 2 || secondaryMatches >= 1 {
		return 0.99
	}
	if primaryMatches == 1 {
		return 0.96
	}
	return 0.8
}

func uniqueTerms(terms []string) []string {
	if len(terms) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(terms))
	out := make([]string, 0, len(terms))
	for _, term := range terms {
		term = strings.TrimSpace(term)
		if term == "" {
			continue
		}
		if _, ok := seen[term]; ok {
			continue
		}
		seen[term] = struct{}{}
		out = append(out, term)
	}
	return out
}
