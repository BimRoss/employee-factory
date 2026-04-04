package lessons

import (
	"context"
	"time"
)

// Event captures one completed outbound turn that can be distilled into lessons.
type Event struct {
	Employee       string    `json:"employee"`
	Path           string    `json:"path"`
	Channel        string    `json:"channel,omitempty"`
	ThreadTS       string    `json:"thread_ts,omitempty"`
	MessageTS      string    `json:"message_ts,omitempty"`
	SourceUserText string    `json:"source_user_text"`
	FinalReply     string    `json:"final_reply"`
	Timestamp      time.Time `json:"timestamp"`
}

// Lesson is an active runtime lesson considered for prompt injection.
type Lesson struct {
	ID         string    `json:"id"`
	AnchorKey  string    `json:"anchor_key"`
	Text       string    `json:"text"`
	Confidence float64   `json:"confidence"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Store persists lesson events and active lessons.
type Store interface {
	AppendEvent(ctx context.Context, employee string, event Event, maxEvents int, ttl time.Duration) error
	LoadActive(ctx context.Context, employee string) ([]Lesson, error)
	SaveActive(ctx context.Context, employee string, lessons []Lesson, ttl time.Duration) error
}

// NoopStore safely disables runtime persistence.
type NoopStore struct{}

func (NoopStore) AppendEvent(context.Context, string, Event, int, time.Duration) error { return nil }
func (NoopStore) LoadActive(context.Context, string) ([]Lesson, error)                 { return nil, nil }
func (NoopStore) SaveActive(context.Context, string, []Lesson, time.Duration) error    { return nil }
