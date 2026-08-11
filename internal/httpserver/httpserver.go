// Package httpserver holds the small pieces of HTTP server plumbing shared by
// every gateway binary (cmd/gateway, cmd/greeting, and any future service):
// a configured *http.Server, a health check handler, and panic recovery.
package httpserver

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"go.uber.org/zap"
)

// New builds an *http.Server with sane timeouts for a given address/handler.
func New(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

// PortFrom extracts the numeric port from a bind address like ":8098" or
// "0.0.0.0:8098", returning 0 if it can't be parsed.
func PortFrom(addr string) int {
	idx := strings.LastIndex(addr, ":")
	if idx < 0 {
		return 0
	}
	p, err := strconv.Atoi(addr[idx+1:])
	if err != nil {
		return 0
	}
	return p
}

// HealthHandler answers liveness/readiness probes (e.g. Docker healthcheck,
// Consul HTTP health check) with a bare 200.
func HealthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}

// Recover logs panics from the wrapped handler and returns 500 instead of
// crashing the process.
func Recover(next http.Handler, logger *zap.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic in handler",
					zap.Any("panic", rec),
					zap.String("path", r.URL.Path),
				)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
