package ratelimit

import (
	"context"
	"errors"
	"testing"
	"time"
)

// smallLimit is a tiny limit so tests can exhaust it quickly.
var smallLimit = Limit{Name: "test", BurstRate: 1, BurstSize: 3, DailyQuota: 5}

func ctx() context.Context { return context.Background() }

func TestMemoryBurstAllowsThenRejects(t *testing.T) {
	s := NewMemoryStore()
	now := time.Unix(1_700_000_000, 0)

	// Burst size 3: three requests allowed.
	for i := 0; i < 3; i++ {
		d, err := s.Check(ctx(), "cli", smallLimit, now)
		if err != nil {
			t.Fatalf("check %d: %v", i, err)
		}
		if !d.Allowed {
			t.Fatalf("check %d: expected allowed, got %s", i, d.Reason)
		}
	}

	// Fourth request within the same second exceeds the bucket.
	d, err := s.Check(ctx(), "cli", smallLimit, now)
	if err != nil {
		t.Fatalf("check: %v", err)
	}
	if d.Allowed {
		t.Fatal("expected rejection when burst exhausted")
	}
	if d.Reason != "burst" {
		t.Fatalf("expected reason burst, got %q", d.Reason)
	}
}

func TestMemoryBurstRefills(t *testing.T) {
	s := NewMemoryStore()
	t0 := time.Unix(1_700_000_000, 0)

	// Exhaust the bucket.
	for i := 0; i < 3; i++ {
		if d, _ := s.Check(ctx(), "cli", smallLimit, t0); !d.Allowed {
			t.Fatalf("request %d not allowed", i)
		}
	}
	if d, _ := s.Check(ctx(), "cli", smallLimit, t0); d.Allowed {
		t.Fatal("expected exhausted")
	}

	// After 1 second the bucket refills 1 token (rate=1/s), so one request passes.
	t1 := t0.Add(time.Second)
	d, err := s.Check(ctx(), "cli", smallLimit, t1)
	if err != nil {
		t.Fatal(err)
	}
	if !d.Allowed {
		t.Fatalf("expected refill to allow, got %s", d.Reason)
	}

	// Immediately after, the bucket is empty again.
	if d, _ := s.Check(ctx(), "cli", smallLimit, t1); d.Allowed {
		t.Fatal("expected exhausted after using refilled token")
	}
}

// quotaLimit has a large burst so the daily quota is the binding constraint.
var quotaLimit = Limit{Name: "quota", BurstRate: 1000, BurstSize: 1000, DailyQuota: 5}

func TestMemoryDailyQuota(t *testing.T) {
	s := NewMemoryStore()
	now := time.Unix(1_700_000_000, 0)

	// quotaLimit.DailyQuota == 5. First 5 allowed.
	for i := 0; i < 5; i++ {
		if d, _ := s.Check(ctx(), "cli", quotaLimit, now); !d.Allowed {
			t.Fatalf("request %d not allowed", i)
		}
	}
	// 6th request exceeds quota.
	d, err := s.Check(ctx(), "cli", quotaLimit, now)
	if err != nil {
		t.Fatal(err)
	}
	if d.Allowed {
		t.Fatal("expected quota rejection")
	}
	if d.Reason != "quota" {
		t.Fatalf("expected reason quota, got %q", d.Reason)
	}

	// A new day (past 24h window) resets the quota.
	nextDay := now.Add(DailyQuotaWindow + time.Second)
	if d, _ := s.Check(ctx(), "cli", quotaLimit, nextDay); !d.Allowed {
		t.Fatalf("expected quota reset next day, got %s", d.Reason)
	}
}

// failingStore simulates an unavailable store.
type failingStore struct{}

func (failingStore) Check(context.Context, string, Limit, time.Time) (Decision, error) {
	return Decision{}, ErrStoreUnavailable
}
func (failingStore) Close() error { return errors.New("nop") }

func TestFailOpenAllowsWhenStoreDown(t *testing.T) {
	l := NewLimiter(failingStore{}, WithFailOpen(true))
	d := l.CheckAt(ctx(), "cli", TierFree, time.Now())
	if !d.Allowed {
		t.Fatalf("expected fail-open to allow, got %s", d.Reason)
	}
	if d.Reason != "fail-open" {
		t.Fatalf("expected reason fail-open, got %q", d.Reason)
	}
}

func TestFailClosedRejectsWhenStoreDown(t *testing.T) {
	l := NewLimiter(failingStore{}, WithFailOpen(false))
	d := l.CheckAt(ctx(), "cli", TierFree, time.Now())
	if d.Allowed {
		t.Fatal("expected fail-closed to reject")
	}
	if d.Reason != "store-unavailable" {
		t.Fatalf("expected reason store-unavailable, got %q", d.Reason)
	}
}

func TestLimitForUnknownTierDefaultsToFree(t *testing.T) {
	if got := LimitFor(Tier("premium")); got != DefaultLimits[TierFree] {
		t.Fatalf("unknown tier should default to free, got %+v", got)
	}
	if got := LimitFor(TierPaid); got.BurstSize < DefaultLimits[TierFree].BurstSize {
		t.Fatalf("paid tier should be larger than free, got %+v", got)
	}
}
