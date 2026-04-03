package slackbot

import (
	"strings"

	"github.com/slack-go/slack"
)

// clipMessagesToGrantBoundary applies the hard Grant-last boundary.
// When enforce is true, only messages up to and including the latest Grant
// message are retained. If no Grant message exists in the window, context is
// intentionally empty to prevent agent-only loops.
func clipMessagesToGrantBoundary(msgs []slack.Message, grantUserID string, enforce bool) []slack.Message {
	if !enforce {
		return msgs
	}
	grantUserID = strings.TrimSpace(grantUserID)
	if grantUserID == "" || len(msgs) == 0 {
		return nil
	}
	lastGrantIdx := -1
	for i := len(msgs) - 1; i >= 0; i-- {
		if strings.TrimSpace(msgs[i].User) == grantUserID {
			lastGrantIdx = i
			break
		}
	}
	if lastGrantIdx < 0 {
		return nil
	}
	clipped := make([]slack.Message, lastGrantIdx+1)
	copy(clipped, msgs[:lastGrantIdx+1])
	return clipped
}

func shouldEnforceGrantBoundary(triggerUserID, grantUserID string) bool {
	trigger := strings.TrimSpace(triggerUserID)
	grant := strings.TrimSpace(grantUserID)
	if trigger == "" || grant == "" {
		return false
	}
	return trigger != grant
}
