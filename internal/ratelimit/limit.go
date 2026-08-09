// Package ratelimit implements the distributed rate limiter described in the
// rate-limiter.md design document.
//
// Two independent limits are enforced for every client, both must pass:
//
//   - A per-second burst limit enforced with a token bucket (refills
//     continuously, so there is no fixed-window boundary burst).
//   - A daily quota enforced with a fixed-window counter (24h sliding clock).
//
// Rate-limit state lives in a shared store (Redis by default) so that multiple
// gateway instances agree on a single global count per client. See limiter.go
// for fail-open behaviour.
package ratelimit

import (
	"fmt"
	"time"
)

// Tier identifies a client's rate-limit tier. Tiers allow different limits per
// client class (e.g. free vs paid).
type Tier string

const (
	// TierFree is the default tier for anonymous/IP-limited traffic and
	// unregistered API keys.
	TierFree Tier = "free"
	// TierPaid is a higher-throughput tier, typically for registered clients.
	TierPaid Tier = "paid"
)

// Limit describes the two independent limits applied to a client.
type Limit struct {
	// Name is a human-readable label.
	Name string

	// BurstRate is the token refill rate in tokens per second.
	BurstRate float64

	// BurstSize is the token bucket capacity (maximum burst size).
	BurstSize float64

	// DailyQuota is the maximum number of accepted requests per 24h window.
	DailyQuota int64
}

// DefaultLimits maps each tier to its limits. These mirror the numbers in the
// design document (100 req/s burst / 1M/day) and can be overridden per client.
var DefaultLimits = map[Tier]Limit{
	TierFree: {
		Name:       "free",
		BurstRate:  100,
		BurstSize:  100,
		DailyQuota: 1_000_000,
	},
	TierPaid: {
		Name:       "paid",
		BurstRate:  1_000,
		BurstSize:  1_000,
		DailyQuota: 10_000_000,
	},
}

// LimitFor returns the configured limits for a tier, or the free tier's limits
// if the tier is unknown (fail-safe default).
func LimitFor(t Tier) Limit {
	if l, ok := DefaultLimits[t]; ok {
		return l
	}
	return DefaultLimits[TierFree]
}

// Decision is the outcome of a single rate-limit check.
type Decision struct {
	// Allowed reports whether the request may proceed.
	Allowed bool

	// Reason is "burst" when the token bucket was exhausted, "quota" when the
	// daily quota was exhausted, or "ok" when the request passed both checks.
	Reason string

	// Remaining is the number of tokens left in the burst bucket after the
	// check (0 if the request was rejected on the bucket).
	Remaining float64

	// QuotaCount is the running request count for the current daily window.
	QuotaCount int64

	// RetryAfter is how long the client should wait before retrying, in
	// seconds. Only meaningful when the request was rejected.
	RetryAfter time.Duration

	// Limit is the tier limits that produced this decision.
	Limit Limit
}

// String renders the decision for logging.
func (d Decision) String() string {
	return fmt.Sprintf("allowed=%v reason=%s remaining=%.1f quota=%d",
		d.Allowed, d.Reason, d.Remaining, d.QuotaCount)
}
