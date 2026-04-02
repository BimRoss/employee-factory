package slackbot

import (
	"testing"

	"github.com/bimross/employee-factory/internal/config"
)

func TestShouldRouteAsBroadcast(t *testing.T) {
	cfg := &config.Config{
		MultiagentEnabled: true,
		MultiagentBotUserIDs: map[string]string{
			"ross": "UROSS001",
			"tim":  "UTIM002",
		},
		MultiagentOrder: []string{"ross", "tim"},
	}
	if !shouldRouteAsBroadcast("<!everyone> check this", cfg) {
		t.Fatal("expected broadcast route for <!everyone>")
	}
	if !shouldRouteAsBroadcast("<!channel> check this", cfg) {
		t.Fatal("expected broadcast route for <!channel>")
	}
	if !shouldRouteAsBroadcast("<!everyone> ping <@UROSS001>", cfg) {
		t.Fatal("expected broadcast route precedence for mixed everyone+agent mention")
	}
	if shouldRouteAsBroadcast("<@UROSS001> only ross", cfg) {
		t.Fatal("single agent mention should not route as broadcast")
	}
}

