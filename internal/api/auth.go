// Package api exposes the rate-limit check service (consumed by Traefik's
// ForwardAuth middleware) and the protected module endpoints that make up the
// modular monolith behind the gateway.
package api

import (
	"net"
	"net/http"
	"strconv"
	"strings"

	"go.uber.org/zap"

	"github.com/koala/atlas/api-gateway/internal/ratelimit"
)

// Header names used by the gateway.
const (
	// HeaderAPIKey carries the client's API key on inbound requests.
	HeaderAPIKey = "X-API-Key"
	// HeaderForwardedFor is set by Traefik/the LB with the original client IP.
	HeaderForwardedFor = "X-Forwarded-For"
)

// tierFor maps an API key to a rate-limit tier. In a real system this would be
// a lookup against the client/tier registry; here a small static map
// demonstrates tiered limits. Unknown keys fall back to the free tier.
func tierFor(apiKey string) ratelimit.Tier {
	switch apiKey {
	case "paid-key-1", "paid-key-2":
		return ratelimit.TierPaid
	default:
		return ratelimit.TierFree
	}
}

// AuthHandler is the ForwardAuth endpoint that Traefik calls for every request.
// It implements the checkLimit(api_key) -> allow | reject interface from the
// design doc: it returns 200 to allow the request through, or 429 (with a
// Retry-After header) to reject it before it reaches the modular monolith.
type AuthHandler struct {
	limiter *ratelimit.Limiter
	logger  *zap.Logger
}

// NewAuthHandler builds an AuthHandler.
func NewAuthHandler(limiter *ratelimit.Limiter, logger *zap.Logger) *AuthHandler {
	return &AuthHandler{limiter: limiter, logger: logger}
}

// ServeHTTP decides whether to allow the request.
func (h *AuthHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	key, tier := h.resolve(r)
	dec := h.limiter.Check(r.Context(), key, tier)

	if !dec.Allowed {
		h.logger.Warn("rate limit exceeded",
			zap.String("key", key),
			zap.String("tier", string(tier)),
			zap.String("reason", dec.Reason),
			zap.Float64("retry_after", dec.RetryAfter.Seconds()),
		)
		w.Header().Set("Retry-After", retryAfter(dec))
		w.Header().Set("X-RateLimit-Limit", formatAmount(dec.Limit.BurstSize))
		w.Header().Set("X-RateLimit-Remaining", "0")
		http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
		return
	}

	// Allowed: tell Traefik to forward the original request downstream.
	w.Header().Set("X-RateLimit-Limit", formatAmount(dec.Limit.BurstSize))
	w.Header().Set("X-RateLimit-Remaining", formatAmount(dec.Remaining))
	w.WriteHeader(http.StatusOK)
}

// resolve picks the client identity and tier: the API key when present, else
// the client IP (secondary IP-based limiting for anonymous traffic).
func (h *AuthHandler) resolve(r *http.Request) (key string, tier ratelimit.Tier) {
	if k := strings.TrimSpace(r.Header.Get(HeaderAPIKey)); k != "" {
		return k, tierFor(k)
	}
	ip := clientIP(r.RemoteAddr, r.Header.Get(HeaderForwardedFor))
	return "ip:" + ip, ratelimit.TierFree
}

// clientIP extracts the originating client IP, preferring the first entry in
// X-Forwarded-For (set by Traefik) and falling back to RemoteAddr.
func clientIP(remoteAddr, forwarded string) string {
	if fwd := firstIP(forwarded); fwd != "" {
		return fwd
	}
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return strings.TrimSpace(remoteAddr)
	}
	return host
}

func firstIP(forwarded string) string {
	for _, part := range strings.Split(forwarded, ",") {
		if ip := strings.TrimSpace(part); ip != "" {
			return ip
		}
	}
	return ""
}

// retryAfter renders a cardinal delta-seconds Retry-After value.
func retryAfter(d ratelimit.Decision) string {
	secs := int64(d.RetryAfter.Seconds())
	if secs < 1 {
		secs = 1
	}
	return strconv.FormatInt(secs, 10)
}

// formatAmount renders a float64 without trailing zeroes when it is integral.
func formatAmount(v float64) string {
	if v == float64(int64(v)) {
		return strconv.FormatInt(int64(v), 10)
	}
	return strconv.FormatFloat(v, 'f', -1, 64)
}
