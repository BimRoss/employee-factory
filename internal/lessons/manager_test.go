package lessons

import (
	"context"
	"testing"
	"time"
)

type memoryStore struct {
	active map[string][]Lesson
	events map[string][]Event
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		active: map[string][]Lesson{},
		events: map[string][]Event{},
	}
}

func (m *memoryStore) AppendEvent(_ context.Context, employee string, event Event, _ int, _ time.Duration) error {
	m.events[employee] = append(m.events[employee], event)
	return nil
}

func (m *memoryStore) LoadActive(_ context.Context, employee string) ([]Lesson, error) {
	return append([]Lesson(nil), m.active[employee]...), nil
}

func (m *memoryStore) SaveActive(_ context.Context, employee string, lessons []Lesson, _ time.Duration) error {
	m.active[employee] = append([]Lesson(nil), lessons...)
	return nil
}

func TestCapturePromotesSterileOpenerLesson(t *testing.T) {
	store := newMemoryStore()
	m := New(Config{
		Enabled:        true,
		LogOnly:        false,
		AutoApply:      true,
		MinConfidence:  0.8,
		MaxActive:      3,
		TTL:            24 * time.Hour,
		MaxPromptRunes: 500,
		MaxEvents:      10,
	}, store)

	decision, err := m.Capture(context.Background(), Event{
		Employee:       "tim",
		Path:           "post_llm_channel",
		SourceUserText: "can you investigate this?",
		FinalReply:     "Understood, I will investigate now.",
		Timestamp:      time.Now().UTC(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Applied {
		t.Fatalf("expected applied lesson, got skipped=%t reason=%s", decision.Skipped, decision.SkipReason)
	}

	prefix, count, err := m.PromptPrefix(context.Background(), "tim")
	if err != nil {
		t.Fatal(err)
	}
	if count != 1 {
		t.Fatalf("expected 1 active lesson, got %d", count)
	}
	if prefix == "" {
		t.Fatal("expected non-empty prompt prefix")
	}
}

func TestCaptureLogOnlyDoesNotPromote(t *testing.T) {
	store := newMemoryStore()
	m := New(Config{
		Enabled:        true,
		LogOnly:        true,
		AutoApply:      true,
		MinConfidence:  0.8,
		MaxActive:      3,
		TTL:            24 * time.Hour,
		MaxPromptRunes: 500,
		MaxEvents:      10,
	}, store)

	decision, err := m.Capture(context.Background(), Event{
		Employee:       "tim",
		SourceUserText: "can you investigate this?",
		FinalReply:     "Understood, I will investigate now.",
	})
	if err != nil {
		t.Fatal(err)
	}
	if !decision.Skipped || decision.SkipReason != "log_only" {
		t.Fatalf("expected log_only skip, got skipped=%t reason=%s", decision.Skipped, decision.SkipReason)
	}
	if len(store.active["tim"]) != 0 {
		t.Fatalf("expected no active lessons in log-only mode, got %d", len(store.active["tim"]))
	}
}

func TestPromptPrefixRespectsRuneCap(t *testing.T) {
	active := []Lesson{
		{Text: "Open with the concrete move."},
		{Text: "Keep it concise and action-first."},
		{Text: "Avoid extra delegation asks."},
	}
	out := buildPromptPrefix(active, 40)
	if out == "" {
		t.Fatal("expected truncated prefix, got empty")
	}
	if len([]rune(out)) > 41 { // includes trailing ellipsis
		t.Fatalf("expected truncated output <= 41 runes, got %d", len([]rune(out)))
	}
}

func TestFilterExpiredLessons(t *testing.T) {
	now := time.Now().UTC()
	in := []Lesson{
		{ID: "new", UpdatedAt: now.Add(-1 * time.Hour)},
		{ID: "old", UpdatedAt: now.Add(-48 * time.Hour)},
	}
	got := filterExpired(in, now, 24*time.Hour)
	if len(got) != 1 || got[0].ID != "new" {
		t.Fatalf("unexpected filtered set: %+v", got)
	}
}
