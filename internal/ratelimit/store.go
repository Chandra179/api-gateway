package ratelimit

import (
	"context"
	"errors"
	"time"
)

// ErrStoreUnavailable is returned by a Store when the backing store cannot be
// reached. Callers use this to decide whether to fail open or fail closed.
var ErrStoreUnavailable = errors.New("rate-limit store unavailable")

// Store is the shared, atomic state backend for the rate limiter. A single
// Check call evaluates both the token bucket and the daily quota in one atomic
// operation so that concurrent gateway instances cannot double-count a client.
type Store interface {
	// Check atomically evaluates the burst (token bucket) and daily quota
	// (fixed window) limits for a client key and returns the outcome.
	Check(ctx context.Context, key string, l Limit, now time.Time) (Decision, error)

	// Close releases any resources held by the store.
	Close() error
}
