package lessons

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// RedisStore keeps lesson state in Redis with employee-scoped keys.
type RedisStore struct {
	c *redis.Client
}

// NewRedisStore creates a lessons store from REDIS_URL.
func NewRedisStore(redisURL string) (*RedisStore, error) {
	if redisURL == "" {
		return nil, fmt.Errorf("empty redis URL")
	}
	opt, err := redis.ParseURL(redisURL)
	if err != nil {
		return nil, err
	}
	return &RedisStore{c: redis.NewClient(opt)}, nil
}

func (r *RedisStore) activeKey(employee string) string {
	return fmt.Sprintf("employee-factory:lessons:%s:active", employee)
}

func (r *RedisStore) eventsKey(employee string) string {
	return fmt.Sprintf("employee-factory:lessons:%s:events", employee)
}

func (r *RedisStore) AppendEvent(ctx context.Context, employee string, event Event, maxEvents int, ttl time.Duration) error {
	if r == nil || r.c == nil || employee == "" {
		return nil
	}
	if maxEvents < 1 {
		maxEvents = 200
	}
	raw, err := json.Marshal(event)
	if err != nil {
		return err
	}
	pipe := r.c.TxPipeline()
	pipe.LPush(ctx, r.eventsKey(employee), raw)
	pipe.LTrim(ctx, r.eventsKey(employee), 0, int64(maxEvents-1))
	if ttl > 0 {
		pipe.Expire(ctx, r.eventsKey(employee), ttl*2)
	}
	_, err = pipe.Exec(ctx)
	return err
}

func (r *RedisStore) LoadActive(ctx context.Context, employee string) ([]Lesson, error) {
	if r == nil || r.c == nil || employee == "" {
		return nil, nil
	}
	raw, err := r.c.Get(ctx, r.activeKey(employee)).Result()
	if err == redis.Nil {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if raw == "" {
		return nil, nil
	}
	var out []Lesson
	if err := json.Unmarshal([]byte(raw), &out); err != nil {
		return nil, err
	}
	return out, nil
}

func (r *RedisStore) SaveActive(ctx context.Context, employee string, lessons []Lesson, ttl time.Duration) error {
	if r == nil || r.c == nil || employee == "" {
		return nil
	}
	raw, err := json.Marshal(lessons)
	if err != nil {
		return err
	}
	if ttl <= 0 {
		ttl = 7 * 24 * time.Hour
	}
	return r.c.Set(ctx, r.activeKey(employee), raw, ttl).Err()
}

func (r *RedisStore) Close() error {
	if r == nil || r.c == nil {
		return nil
	}
	return r.c.Close()
}
