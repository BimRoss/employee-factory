package slackbot

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
)

type generalAutoReplyLocker struct {
	c *redis.Client
}

type generalAutoReplyClaimStatus string

const (
	generalAutoReplyClaimAcquired       generalAutoReplyClaimStatus = "acquired"
	generalAutoReplyClaimAlreadyClaimed generalAutoReplyClaimStatus = "already_claimed"
	generalAutoReplyClaimBackendDown    generalAutoReplyClaimStatus = "backend_unavailable"
)

func newGeneralAutoReplyLocker(redisURL string) *generalAutoReplyLocker {
	redisURL = strings.TrimSpace(redisURL)
	if redisURL == "" {
		return nil
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil
	}
	return &generalAutoReplyLocker{c: redis.NewClient(opt)}
}

func (l *generalAutoReplyLocker) key(channelID, anchorTS string) string {
	return fmt.Sprintf("employee-factory:general_auto_reaction:%s:%s", strings.TrimSpace(channelID), strings.TrimSpace(anchorTS))
}

func (l *generalAutoReplyLocker) TryClaim(ctx context.Context, channelID, anchorTS, claimant string, ttl time.Duration) (generalAutoReplyClaimStatus, error) {
	if l == nil || l.c == nil {
		return generalAutoReplyClaimBackendDown, nil
	}
	if ttl <= 0 {
		ttl = 90 * time.Second
	}
	ok, err := l.c.SetNX(ctx, l.key(channelID, anchorTS), claimant, ttl).Result()
	if err != nil {
		return generalAutoReplyClaimBackendDown, err
	}
	if ok {
		return generalAutoReplyClaimAcquired, nil
	}
	return generalAutoReplyClaimAlreadyClaimed, nil
}

func (l *generalAutoReplyLocker) ReleaseIfOwned(ctx context.Context, channelID, anchorTS, claimant string) error {
	if l == nil || l.c == nil {
		return nil
	}
	k := l.key(channelID, anchorTS)
	v, err := l.c.Get(ctx, k).Result()
	if err == redis.Nil {
		return nil
	}
	if err != nil {
		return err
	}
	if strings.TrimSpace(v) != strings.TrimSpace(claimant) {
		return nil
	}
	return l.c.Del(ctx, k).Err()
}
