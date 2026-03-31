package slackbot

import (
	"testing"
	"time"
)

func TestOutboundGate_maxPerWindow(t *testing.T) {
	window := time.Minute
	g := newOutboundGate(window, 3)
	if g == nil {
		t.Fatal("expected non-nil gate")
	}

	t0 := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		if !g.allow(t0.Add(time.Duration(i) * time.Second)) {
			t.Fatalf("post %d should be allowed", i+1)
		}
		g.record(t0.Add(time.Duration(i) * time.Second))
	}

	if g.allow(t0.Add(3 * time.Second)) {
		t.Fatal("fourth post within window should be blocked")
	}
}

func TestOutboundGate_slidingWindow(t *testing.T) {
	g := newOutboundGate(time.Minute, 3)
	t0 := time.Date(2026, 3, 31, 12, 0, 0, 0, time.UTC)

	for i := 0; i < 3; i++ {
		if !g.allow(t0.Add(time.Duration(i) * time.Second)) {
			t.Fatalf("post %d should be allowed", i+1)
		}
		g.record(t0.Add(time.Duration(i) * time.Second))
	}
	if g.allow(t0.Add(3 * time.Second)) {
		t.Fatal("fourth post in same minute should be blocked")
	}
	// After 61s from first post, oldest drops off — allow one more.
	t61 := t0.Add(61 * time.Second)
	if !g.allow(t61) {
		t.Fatal("after window slides, post should be allowed")
	}
}

func TestOutboundGate_disabled(t *testing.T) {
	if newOutboundGate(time.Minute, 0) != nil {
		t.Fatal("maxPosts 0 should disable gate")
	}
}
