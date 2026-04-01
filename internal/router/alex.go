package router

import (
	"strings"
)

// AlexUserPrefix is a deterministic, zero–LLM-call hint so the model leans on the right
// alex-*.mdc modules already present in persona.md. The model must not quote this block.
const alexHintIntro = "Internal routing hint (do not quote this line or label it; use the persona frameworks it points to):\n"

// WrapAlexUserMessage prepends a keyword-based hint for Alex (Hormozi) Slack only.
func WrapAlexUserMessage(userText string) string {
	userText = strings.TrimSpace(userText)
	if userText == "" {
		return userText
	}
	h := alexKeywordHint(userText)
	if h == "" {
		return userText
	}
	return alexHintIntro + h + "\n\nUser message:\n" + userText
}

func alexKeywordHint(s string) string {
	lower := strings.ToLower(s)
	switch {
	case containsAny(lower, []string{"closer", "objection", "sales call", "close the", "closing"}):
		return "Prefer CLOSER / sales-call frameworks from the persona (e.g. alex-sales-closer)."
	case containsAny(lower, []string{"core four", "obscurity", "outreach", "cold email", "content", "run ads", "paid ads", "advertising"}):
		return "Prefer acquisition framing still in the persona (e.g. alex-stair-step-bucket, alex-one-channel-avatar-product, boom vs optimization ideas where present)."
	case containsAny(lower, []string{"price", "pricing", "raise price", "anchor", "grandfather"}):
		return "Prefer pricing frameworks (e.g. alex-raising-prices, alex-anchor-expensive, alex-pricing-extremes)."
	case containsAny(lower, []string{"stair step", "bucket", "100k", "advertise", "ads first"}):
		return "Prefer growth-stage framing (e.g. alex-stair-step-bucket)."
	case containsAny(lower, []string{"stress", "quit", "burnout", "anxious"}):
		return "Prefer mindset framing (e.g. alex-stress-mindset, alex-region-beta-anxiety-cost)."
	case containsAny(lower, []string{"word of mouth", "referral", "retention", "churn"}):
		return "Prefer reputation / retention framing (e.g. alex-more-word-of-mouth, alex-look-back-window)."
	case containsAny(lower, []string{"offer", "value equation", "grand slam"}):
		return "Prefer offer math (e.g. alex-offer-math)."
	default:
		return ""
	}
}

func containsAny(hay string, needles []string) bool {
	for _, n := range needles {
		if strings.Contains(hay, n) {
			return true
		}
	}
	return false
}
