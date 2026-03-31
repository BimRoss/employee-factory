package slackbot

import (
	"time"
)

// outboundGate limits how often this bot posts to Slack using a rolling maximum
// per time window—useful when multiple agents @mention each other so conversations
// cannot run away. Further messages simply wait for the next human (or later) ping.
type outboundGate struct {
	window   time.Duration
	maxPosts int

	times []time.Time
}

func newOutboundGate(window time.Duration, maxPosts int) *outboundGate {
	if maxPosts <= 0 {
		return nil
	}
	return &outboundGate{
		window:   window,
		maxPosts: maxPosts,
	}
}

// allow reports whether an outbound message may be sent now. It does not record a send.
func (g *outboundGate) allow(now time.Time) bool {
	if g == nil {
		return true
	}
	g.prune(now)
	return len(g.times) < g.maxPosts
}

// record notes a successful outbound post at now (call only after PostMessage succeeds).
func (g *outboundGate) record(now time.Time) {
	if g == nil {
		return
	}
	g.prune(now)
	g.times = append(g.times, now)
}

func (g *outboundGate) prune(now time.Time) {
	if g.window <= 0 {
		return
	}
	cutoff := now.Add(-g.window)
	i := 0
	for i < len(g.times) && g.times[i].Before(cutoff) {
		i++
	}
	if i > 0 {
		g.times = append([]time.Time(nil), g.times[i:]...)
	}
}
