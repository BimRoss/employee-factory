package slackbot

import (
	"context"
	"testing"
	"time"

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

func TestBroadcastActivityTracking(t *testing.T) {
	b := &Bot{
		activeBroadcastByChannel: map[string]int{},
	}
	channel := "CGENERAL"
	if b.isBroadcastActive(channel) {
		t.Fatal("expected no active broadcast initially")
	}
	b.beginBroadcast(channel)
	if !b.isBroadcastActive(channel) {
		t.Fatal("expected active broadcast after begin")
	}
	b.beginBroadcast(channel)
	b.endBroadcast(channel)
	if !b.isBroadcastActive(channel) {
		t.Fatal("expected one active session remaining after one end")
	}
	b.endBroadcast(channel)
	if b.isBroadcastActive(channel) {
		t.Fatal("expected no active sessions after matching end calls")
	}
}

func TestWithLLMTimeoutUsesConfig(t *testing.T) {
	b := &Bot{cfg: &config.Config{LLMReplyTimeoutSec: 2}}
	ctx, cancel := b.withLLMTimeout(context.Background())
	defer cancel()
	deadline, ok := ctx.Deadline()
	if !ok {
		t.Fatal("expected deadline from timeout helper")
	}
	remaining := time.Until(deadline)
	if remaining <= time.Second || remaining > 3*time.Second {
		t.Fatalf("expected ~2s timeout, got %s", remaining)
	}
}

