package ratelimit

import (
	"context"
	"time"
)

// Option configures a Limiter.
type Option func(*Limiter)

// WithFailOpen sets whether the limiter allows traffic through when the store
// is unavailable. The design doc mandates fail-open (availability over strict
// accuracy), so the default is true.
func WithFailOpen(failOpen bool) Option {
	return func(l *Limiter) { l.failOpen = failOpen }
}

// Limiter runs rate-limit checks against a shared Store and applies the
// fail-open / fail-closed policy when the store is unavailable.
type Limiter struct {
	store    Store
	failOpen bool
}

// NewLimiter constructs a Limiter backed by store.
func NewLimiter(store Store, opts ...Option) *Limiter {
	l := &Limiter{store: store, failOpen: true}
	for _, o := range opts {
		o(l)
	}
	return l
}

// Check evaluates the limits for a client identified by key. If the store is
// unavailable and fail-open, the request is allowed (over-admission during an
// outage is preferable to blocking all traffic). If fail-closed, it is
// rejected.
func (l *Limiter) Check(ctx context.Context, key string, tier Tier) Decision {
	return l.CheckAt(ctx, key, tier, time.Now())
}

// CheckAt is Check with an injectable clock, used by tests.
func (l *Limiter) CheckAt(ctx context.Context, key string, tier Tier, now time.Time) Decision {
	limit := LimitFor(tier)
	dec, err := l.store.Check(ctx, key, limit, now)
	if err != nil {
		if l.failOpen {
			// Fail open: allow the request through, unmetered.
			return Decision{Allowed: true, Reason: "fail-open", Limit: limit}
		}
		// Fail closed: reject rather than risk unbounded abuse.
		return Decision{Allowed: false, Reason: "store-unavailable", Limit: limit}
	}
	dec.Limit = limit
	return dec
}
