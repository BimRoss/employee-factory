package lessons

import (
	"context"
	"crypto/sha1"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"
)

var (
	reNonAlphaNum     = regexp.MustCompile(`[^a-z0-9]+`)
	reLikelySecret    = regexp.MustCompile(`(?i)(api[_-]?key|token|secret|password|bearer\s+[a-z0-9._-]+|xox[baprs]-)`)
	reOpsProxyFailure = regexp.MustCompile(`(?i)could not .*kubernetes.*ops proxy`)
)

// Config controls lesson capture and auto-apply behavior.
type Config struct {
	Enabled        bool
	LogOnly        bool
	AutoApply      bool
	MinConfidence  float64
	MaxActive      int
	MaxEvents      int
	TTL            time.Duration
	MaxPromptRunes int
}

// Manager handles runtime lesson capture and prompt prefix generation.
type Manager struct {
	cfg   Config
	store Store
	clock func() time.Time
}

// Decision reports what happened during lesson capture.
type Decision struct {
	Employee   string
	AnchorKey  string
	Applied    bool
	Skipped    bool
	SkipReason string
	Confidence float64
}

func New(cfg Config, store Store) *Manager {
	if store == nil {
		store = NoopStore{}
	}
	if cfg.MaxActive < 1 {
		cfg.MaxActive = 3
	}
	if cfg.MaxEvents < 1 {
		cfg.MaxEvents = 200
	}
	if cfg.TTL <= 0 {
		cfg.TTL = 7 * 24 * time.Hour
	}
	if cfg.MaxPromptRunes < 64 {
		cfg.MaxPromptRunes = 600
	}
	if cfg.MinConfidence <= 0 {
		cfg.MinConfidence = 0.8
	}
	return &Manager{
		cfg:   cfg,
		store: store,
		clock: time.Now,
	}
}

func (m *Manager) Capture(ctx context.Context, event Event) (Decision, error) {
	decision := Decision{Employee: strings.TrimSpace(event.Employee), Skipped: true}
	if m == nil || !m.cfg.Enabled {
		decision.SkipReason = "disabled"
		return decision, nil
	}
	employee := strings.TrimSpace(strings.ToLower(event.Employee))
	if employee == "" {
		decision.SkipReason = "empty_employee"
		return decision, nil
	}
	if event.Timestamp.IsZero() {
		event.Timestamp = m.clock().UTC()
	}
	event.Employee = employee
	if err := m.store.AppendEvent(ctx, employee, event, m.cfg.MaxEvents, m.cfg.TTL); err != nil {
		return decision, err
	}

	candidate, ok := deriveLesson(event)
	if !ok {
		decision.SkipReason = "no_candidate"
		return decision, nil
	}
	decision.AnchorKey = candidate.AnchorKey
	decision.Confidence = candidate.Confidence
	if candidate.Confidence < m.cfg.MinConfidence {
		decision.SkipReason = "below_confidence"
		return decision, nil
	}
	if m.cfg.LogOnly {
		decision.SkipReason = "log_only"
		return decision, nil
	}
	if !m.cfg.AutoApply {
		decision.SkipReason = "auto_apply_disabled"
		return decision, nil
	}
	if candidateRejected(candidate) {
		decision.SkipReason = "candidate_rejected"
		return decision, nil
	}

	active, err := m.store.LoadActive(ctx, employee)
	if err != nil {
		return decision, err
	}
	now := m.clock().UTC()
	active = filterExpired(active, now, m.cfg.TTL)
	active = upsertLesson(active, candidate, now, m.cfg.MaxActive)
	if err := m.store.SaveActive(ctx, employee, active, m.cfg.TTL); err != nil {
		return decision, err
	}
	decision.Applied = true
	decision.Skipped = false
	decision.SkipReason = ""
	return decision, nil
}

func (m *Manager) PromptPrefix(ctx context.Context, employee string) (string, int, error) {
	if m == nil || !m.cfg.Enabled {
		return "", 0, nil
	}
	employee = strings.TrimSpace(strings.ToLower(employee))
	if employee == "" {
		return "", 0, nil
	}
	active, err := m.store.LoadActive(ctx, employee)
	if err != nil {
		return "", 0, err
	}
	active = filterExpired(active, m.clock().UTC(), m.cfg.TTL)
	if len(active) == 0 {
		return "", 0, nil
	}
	if len(active) > m.cfg.MaxActive {
		active = active[:m.cfg.MaxActive]
	}
	prefix := buildPromptPrefix(active, m.cfg.MaxPromptRunes)
	return prefix, len(active), nil
}

func buildPromptPrefix(active []Lesson, maxRunes int) string {
	if len(active) == 0 {
		return ""
	}
	lines := []string{"Runtime lessons (apply subtly):"}
	for _, l := range active {
		text := strings.TrimSpace(l.Text)
		if text == "" {
			continue
		}
		lines = append(lines, "- "+text)
	}
	if len(lines) == 1 {
		return ""
	}
	out := strings.Join(lines, "\n")
	runes := []rune(out)
	if len(runes) > maxRunes {
		out = string(runes[:maxRunes])
		out = strings.TrimSpace(out) + "…"
	}
	return out
}

func filterExpired(in []Lesson, now time.Time, ttl time.Duration) []Lesson {
	if ttl <= 0 {
		return in
	}
	cutoff := now.Add(-ttl)
	out := make([]Lesson, 0, len(in))
	for _, l := range in {
		if l.UpdatedAt.IsZero() || l.UpdatedAt.After(cutoff) {
			out = append(out, l)
		}
	}
	return out
}

func upsertLesson(active []Lesson, candidate Lesson, now time.Time, maxActive int) []Lesson {
	candidate.UpdatedAt = now
	for i := range active {
		if active[i].ID == candidate.ID {
			active[i].Text = candidate.Text
			if candidate.Confidence > active[i].Confidence {
				active[i].Confidence = candidate.Confidence
			}
			active[i].UpdatedAt = now
			sortLessons(active)
			if len(active) > maxActive {
				active = active[:maxActive]
			}
			return active
		}
	}
	active = append(active, candidate)
	sortLessons(active)
	if len(active) > maxActive {
		active = active[:maxActive]
	}
	return active
}

func sortLessons(items []Lesson) {
	sort.Slice(items, func(i, j int) bool {
		if items[i].Confidence != items[j].Confidence {
			return items[i].Confidence > items[j].Confidence
		}
		return items[i].UpdatedAt.After(items[j].UpdatedAt)
	})
}

func deriveLesson(event Event) (Lesson, bool) {
	source := strings.TrimSpace(strings.ToLower(event.SourceUserText))
	reply := strings.TrimSpace(strings.ToLower(event.FinalReply))
	if source == "" && reply == "" {
		return Lesson{}, false
	}

	switch {
	case startsWithSterileAck(reply):
		return newLesson(
			"style_opener_hygiene",
			"Open with the concrete move/answer; avoid sterile acknowledgment stubs like 'understood' or 'noted.'",
			0.97,
		), true
	case looksLikeAvailabilityCue(source):
		return newLesson(
			"availability_ack_only",
			"For availability/signoff cues, send one short acknowledgment and avoid stacking asks or delegations.",
			0.92,
		), true
	case strings.Contains(source, "kubernetes") && reOpsProxyFailure.MatchString(reply):
		return newLesson(
			"kubernetes_ops_proxy_blocker",
			"When Kubernetes access fails, state the ops proxy blocker clearly and route to Ross for investigation.",
			0.89,
		), true
	}
	return Lesson{}, false
}

func newLesson(anchor, text string, confidence float64) Lesson {
	h := sha1.Sum([]byte(anchor + "|" + canonicalTokenKey(text)))
	return Lesson{
		ID:         fmt.Sprintf("%x", h[:8]),
		AnchorKey:  anchor,
		Text:       text,
		Confidence: confidence,
	}
}

func candidateRejected(c Lesson) bool {
	text := strings.TrimSpace(c.Text)
	if text == "" {
		return true
	}
	if reLikelySecret.MatchString(text) {
		return true
	}
	return false
}

func startsWithSterileAck(reply string) bool {
	reply = strings.TrimSpace(strings.ToLower(reply))
	stubs := []string{
		"understood",
		"noted",
		"acknowledged",
		"sounds good",
		"got it",
	}
	for _, s := range stubs {
		if strings.HasPrefix(reply, s) {
			return true
		}
	}
	return false
}

func looksLikeAvailabilityCue(source string) bool {
	cues := []string{
		"afk",
		"step away",
		"back later",
		"sign off",
		"go to bed",
		"go to sleep",
	}
	for _, c := range cues {
		if strings.Contains(source, c) {
			return true
		}
	}
	return false
}

func canonicalTokenKey(s string) string {
	s = strings.ToLower(strings.TrimSpace(s))
	s = reNonAlphaNum.ReplaceAllString(s, " ")
	s = strings.Join(strings.Fields(s), " ")
	return s
}
