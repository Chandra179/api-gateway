package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/koala/atlas/api-gateway/internal/ratelimit"
)

func newTestAuth() *AuthHandler {
	logger := zap.NewNop()
	limiter := ratelimit.NewLimiter(ratelimit.NewMemoryStore())
	return NewAuthHandler(limiter, logger)
}

func doAuth(h *AuthHandler, opts ...func(*http.Request)) *httptest.ResponseRecorder {
	r := httptest.NewRequest(http.MethodGet, "/greet", nil)
	for _, o := range opts {
		o(r)
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func withAPIKey(k string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set(HeaderAPIKey, k) }
}

func withIP(ip string) func(*http.Request) {
	return func(r *http.Request) { r.Header.Set(HeaderForwardedFor, ip) }
}

func TestAuthAllowsThenRejectsFreeKey(t *testing.T) {
	h := newTestAuth()
	free := ratelimit.LimitFor(ratelimit.TierFree)

	// Burst size worth of requests are allowed.
	for i := int64(0); i < int64(free.BurstSize); i++ {
		if w := doAuth(h, withAPIKey("anon-key")); w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, w.Code)
		}
	}

	// The next request exceeds the burst and is rejected with 429.
	w := doAuth(h, withAPIKey("anon-key"))
	if w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected 429, got %d", w.Code)
	}
	if w.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header on rejection")
	}
}

func TestAuthPaidTierHasHigherLimit(t *testing.T) {
	h := newTestAuth()
	w := doAuth(h, withAPIKey("paid-key-1"))
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	paid := ratelimit.LimitFor(ratelimit.TierPaid)
	if got := w.Header().Get("X-RateLimit-Limit"); got != formatAmount(paid.BurstSize) {
		t.Fatalf("expected paid limit %s, got %s", formatAmount(paid.BurstSize), got)
	}
}

func TestAuthFallsBackToIPWithoutKey(t *testing.T) {
	h := newTestAuth()
	free := ratelimit.LimitFor(ratelimit.TierFree)

	// Exhaust one IP's burst.
	for i := int64(0); i < int64(free.BurstSize); i++ {
		if w := doAuth(h, withIP("203.0.113.7")); w.Code != http.StatusOK {
			t.Fatalf("request %d: expected 200, got %d", i, w.Code)
		}
	}
	if w := doAuth(h, withIP("203.0.113.7")); w.Code != http.StatusTooManyRequests {
		t.Fatalf("expected first IP to be rate limited, got %d", w.Code)
	}

	// A different IP is limited independently.
	if w := doAuth(h, withIP("198.51.100.9")); w.Code != http.StatusOK {
		t.Fatalf("expected independent IP to be allowed, got %d", w.Code)
	}
}

func TestAuthFailOpenAllowsWhenStoreDown(t *testing.T) {
	logger := zap.NewNop()
	limiter := ratelimit.NewLimiter(failingStoreForAPI{}, ratelimit.WithFailOpen(true))
	h := NewAuthHandler(limiter, logger)
	if w := doAuth(h, withAPIKey("any")); w.Code != http.StatusOK {
		t.Fatalf("expected fail-open 200, got %d", w.Code)
	}
}

// failingStoreForAPI lets the api package test fail-open behaviour.
type failingStoreForAPI struct{}

func (failingStoreForAPI) Check(_ context.Context, _ string, _ ratelimit.Limit, _ time.Time) (ratelimit.Decision, error) {
	return ratelimit.Decision{}, ratelimit.ErrStoreUnavailable
}
func (failingStoreForAPI) Close() error { return nil }
