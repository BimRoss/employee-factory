package slackbot

import "strings"

var generalAutoReplyAskPhrases = []string{
	"can you",
	"could you",
	"would you",
	"what do you think",
	"thoughts on",
	"help me",
	"please help",
	"how should",
	"what should",
}

var generalAutoReplyClosurePhrases = []string{
	"all good",
	"i am good",
	"i'm good",
	"im good",
	"we are good",
	"we're good",
	"were good",
	"no worries",
	"no worry",
	"no problem",
	"sounds good",
	"that helps",
	"this helps",
	"i should be good",
	"should be good",
	"done for now",
}

// shouldSkipGeneralAutoReply blocks low-signal closure/ack pings in #general so
// deterministic winner selection is reserved for substantive asks.
func shouldSkipGeneralAutoReply(rawText string) (bool, string) {
	text := strings.ToLower(strings.TrimSpace(rawText))
	if text == "" {
		return true, "empty_text"
	}
	if hasGeneralAutoReplyAskSignal(text) {
		return false, ""
	}
	words := len(strings.Fields(text))
	if words > 12 {
		return false, ""
	}
	for _, phrase := range generalAutoReplyClosurePhrases {
		if strings.Contains(text, phrase) {
			return true, "closure_intent"
		}
	}
	return false, ""
}

func hasGeneralAutoReplyAskSignal(text string) bool {
	if strings.Contains(text, "?") {
		return true
	}
	for _, phrase := range generalAutoReplyAskPhrases {
		if strings.Contains(text, phrase) {
			return true
		}
	}
	return false
}
