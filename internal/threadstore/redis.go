package threadstore

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

var _ OwnerStore = (*RedisOwnerStore)(nil)

// RedisOwnerStore stores owner keys under employee-factory:thread_owner:{channel}:{thread_ts}
type RedisOwnerStore struct {
	c *redis.Client
}

// NewRedis connects from REDIS_URL (redis://user:pass@host:port/db).
func NewRedis(redisURL string) (*RedisOwnerStore, error) {
	if redisURL == "" {
		return nil, fmt.Errorf("empty redis URL")
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return &RedisOwnerStore{c: redis.NewClient(opt)}, nil
}

func (r *RedisOwnerStore) key(channelID, threadTS string) string {
	return fmt.Sprintf("employee-factory:thread_owner:%s:%s", channelID, threadTS)
}

// Get returns stored owner employee key.
func (r *RedisOwnerStore) Get(ctx context.Context, channelID, threadTS string) (string, bool, error) {
	if r == nil || r.c == nil {
		return "", false, nil
	}
	s, err := r.c.Get(ctx, r.key(channelID, threadTS)).Result()
	if err == redis.Nil {
		return "", false, nil
	}
	if err != nil {
		return "", false, err
	}
	if s == "" {
		return "", false, nil
	}
	return s, true, nil
}

// Set stores owner with TTL.
func (r *RedisOwnerStore) Set(ctx context.Context, channelID, threadTS, ownerKey string, ttl time.Duration) error {
	if r == nil || r.c == nil {
		return nil
	}
	return r.c.Set(ctx, r.key(channelID, threadTS), ownerKey, ttl).Err()
}

// Close closes the Redis client.
func (r *RedisOwnerStore) Close() error {
	if r == nil || r.c == nil {
		return nil
	}
	return r.c.Close()
}
