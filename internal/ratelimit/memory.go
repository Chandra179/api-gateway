package ratelimit

import (
	"context"
	"sync"
	"time"
)

// memBucket is the in-memory token-bucket state for one client.
type memBucket struct {
	tokens     float64
	lastRefill int64
}

// MemoryStore is a Store backed by in-process state. It is useful for local
// development without Redis and for unit tests. It is NOT shared across
// gateway instances, so it does not satisfy the distributed requirements of
// the design doc; use RedisStore in real deployments.
type MemoryStore struct {
	mu       sync.Mutex
	buckets  map[string]*memBucket
	quotas   map[string]int64
	quotaSet map[string]int64 // key -> unix second the window started
}

// NewMemoryStore creates an empty in-memory store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{
		buckets:  make(map[string]*memBucket),
		quotas:   make(map[string]int64),
		quotaSet: make(map[string]int64),
	}
}

// Check evaluates both limits against in-memory state.
func (s *MemoryStore) Check(_ context.Context, key string, l Limit, now time.Time) (Decision, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	nowSec := now.Unix()

	// --- Token bucket ---
	b, ok := s.buckets[key]
	if !ok {
		b = &memBucket{tokens: l.BurstSize, lastRefill: nowSec}
		s.buckets[key] = b
	}
	elapsed := float64(nowSec - b.lastRefill)
	b.tokens += elapsed * l.BurstRate
	if b.tokens > l.BurstSize {
		b.tokens = l.BurstSize
	}
	b.lastRefill = nowSec

	dec := Decision{Allowed: true, Reason: "ok", Remaining: b.tokens}

	if b.tokens < 1 {
		dec.Allowed = false
		dec.Reason = "burst"
		wait := (1 - b.tokens) / l.BurstRate
		if wait < 0 {
			wait = 0
		}
		dec.RetryAfter = time.Duration(wait) * time.Second
		return dec, nil
	}

	// --- Daily quota (fixed window, 24h) ---
	b.tokens -= 1
	win := int64(DailyQuotaWindow.Seconds())
	if start, ok := s.quotaSet[key]; !ok || nowSec-start >= win {
		s.quotas[key] = 0
		s.quotaSet[key] = nowSec
	}
	s.quotas[key]++
	dec.Remaining = b.tokens
	dec.QuotaCount = s.quotas[key]

	if dec.QuotaCount > l.DailyQuota {
		dec.Allowed = false
		dec.Reason = "quota"
	}
	return dec, nil
}

// Close is a no-op for the in-memory store.
func (s *MemoryStore) Close() error { return nil }
