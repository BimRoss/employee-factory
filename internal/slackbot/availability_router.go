package slackbot

import (
	"context"
	"log"
	"strings"
	"time"

	"github.com/bimross/employee-factory/internal/router"
)

type routerPhase string

const (
	routerPhaseIngress routerPhase = "ingress"
	routerPhasePreLLM  routerPhase = "pre_llm"
)

type availabilityRouteEvent struct {
	Path      string
	Channel   string
	MessageTS string
	ThreadTS  string
	RawText   string
	Phase     routerPhase
}

func (b *Bot) applyAvailabilityRouter(ctx context.Context, ev availabilityRouteEvent) bool {
	if b == nil || b.cfg == nil || !b.cfg.RouterAvailabilityActive() {
		return false
	}

	decision := router.ClassifyAvailability(ev.RawText)
	if decision.Intent == router.AvailabilityIntentNormal {
		return false
	}

	enforce := b.cfg.RouterAvailabilityEnabled && !b.cfg.RouterLogOnly
	b.logRouterDecision(ev, decision, enforce)
	if !enforce || decision.Action != router.AvailabilityActionAckOnly {
		return false
	}

	reply := router.BuildAsyncSafeAck(decision.Intent)
	if strings.TrimSpace(reply) == "" {
		return true
	}
	if err := b.postAvailabilityRouterAck(ctx, strings.TrimSpace(ev.Channel), strings.TrimSpace(ev.ThreadTS), reply); err != nil {
		log.Printf("router_decision: phase=%s path=%s employee=%s channel=%s anchor=%s thread_ts=%s action=ack_only post_err=%q",
			ev.Phase,
			strings.TrimSpace(ev.Path),
			strings.TrimSpace(b.cfg.EmployeeID),
			strings.TrimSpace(ev.Channel),
			strings.TrimSpace(routerAnchor(ev.ThreadTS, ev.MessageTS)),
			strings.TrimSpace(ev.ThreadTS),
			err.Error(),
		)
	}
	return true
}

func (b *Bot) logRouterDecision(ev availabilityRouteEvent, decision router.AvailabilityDecision, enforce bool) {
	matchedTerms := strings.Join(decision.MatchedTerms, ",")
	log.Printf(
		"router_decision: phase=%s path=%s employee=%s channel=%s anchor=%s thread_ts=%s intent=%s action=%s confidence=%.2f reason=%s matched_terms=%q enforce=%t log_only=%t",
		ev.Phase,
		strings.TrimSpace(ev.Path),
		strings.TrimSpace(b.cfg.EmployeeID),
		strings.TrimSpace(ev.Channel),
		strings.TrimSpace(routerAnchor(ev.ThreadTS, ev.MessageTS)),
		strings.TrimSpace(ev.ThreadTS),
		decision.Intent,
		decision.Action,
		decision.Confidence,
		decision.Reason,
		matchedTerms,
		enforce,
		b.cfg.RouterLogOnly,
	)
}

func routerAnchor(threadTS, messageTS string) string {
	if strings.TrimSpace(threadTS) != "" {
		return strings.TrimSpace(threadTS)
	}
	return strings.TrimSpace(messageTS)
}

func (b *Bot) postAvailabilityRouterAck(ctx context.Context, channel, threadTS, text string) error {
	if b == nil || strings.TrimSpace(channel) == "" || strings.TrimSpace(text) == "" {
		return nil
	}

	b.mu.Lock()
	defer b.mu.Unlock()

	now := time.Now()
	if b.outbound != nil && !b.outbound.allow(now) {
		return nil
	}

	err := b.postSlackResponse(ctx, channel, slackResponse{
		Text:     text,
		ThreadTS: strings.TrimSpace(threadTS),
	})
	if err != nil {
		return err
	}
	if b.outbound != nil {
		b.outbound.record(time.Now())
	}
	return nil
}
