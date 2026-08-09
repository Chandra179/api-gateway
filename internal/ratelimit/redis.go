package ratelimit

import (
	"context"
	"time"

	"github.com/redis/go-redis/v9"
)

// Key prefixes separate token-bucket state from daily-quota counters in Redis.
const (
	bucketKeyPrefix = "rl:bucket:"
	quotaKeyPrefix  = "rl:quota:"
)

// DailyQuotaWindow is the fixed window length for the daily quota (24h).
const DailyQuotaWindow = 24 * time.Hour

// RedisStore is a Store backed by a shared Redis instance. The whole check is
// a single Lua script evaluated via EVAL, which is atomic by construction, so
// concurrent gateway instances cannot race on a client's counters.
type RedisStore struct {
	client *redis.Client
}

// NewRedisStore creates a Redis-backed store. It verifies connectivity with a
// ping immediately so a misconfiguration is surfaced at startup.
func NewRedisStore(addr string) (*RedisStore, error) {
	client := redis.NewClient(&redis.Options{Addr: addr})
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := client.Ping(ctx).Err(); err != nil {
		_ = client.Close()
		return nil, err
	}
	return &RedisStore{client: client}, nil
}

// Check evaluates both limits in a single atomic Lua script.
func (s *RedisStore) Check(ctx context.Context, key string, l Limit, now time.Time) (Decision, error) {
	nowSec := now.Unix()
	res, err := s.client.Eval(ctx, checkScript,
		[]string{bucketKeyPrefix + key, quotaKeyPrefix + key},
		l.BurstRate, l.BurstSize, l.DailyQuota, nowSec, int64(DailyQuotaWindow.Seconds()),
	).Result()
	if err != nil {
		// Redis unreachable or script error: surface as store-unavailable so
		// the limiter can fail open.
		if err == redis.Nil {
			return Decision{}, errStoreNil
		}
		return Decision{}, err
	}

	// The script returns {allowed, reason, tokens, quotaCount, retryAfter}.
	arr, ok := res.([]interface{})
	if !ok || len(arr) < 5 {
		return Decision{}, errBadScriptResult
	}

	allowed := toInt64(arr[0]) == 1
	reason := toString(arr[1])
	tokens := toFloat64(arr[2])
	quota := toInt64(arr[3])
	retry := toInt64(arr[4])

	return Decision{
		Allowed:    allowed,
		Reason:     reason,
		Remaining:  tokens,
		QuotaCount: quota,
		RetryAfter: time.Duration(retry) * time.Second,
	}, nil
}

// Close releases the Redis connection pool.
func (s *RedisStore) Close() error {
	return s.client.Close()
}
