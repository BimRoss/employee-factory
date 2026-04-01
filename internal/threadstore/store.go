package threadstore

import (
	"context"
	"time"
)

// OwnerStore persists thread owner employee key for human-root Slack threads (channel_id + thread_ts).
type OwnerStore interface {
	Get(ctx context.Context, channelID, threadTS string) (ownerKey string, ok bool, err error)
	Set(ctx context.Context, channelID, threadTS, ownerKey string, ttl time.Duration) error
}

// Noop returns empty Get; Set is a no-op (human-root threads cannot persist owner).
type Noop struct{}

var _ OwnerStore = Noop{}

func (Noop) Get(context.Context, string, string) (string, bool, error) {
	return "", false, nil
}

func (Noop) Set(context.Context, string, string, string, time.Duration) error {
	return nil
}
