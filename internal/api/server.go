package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/koala/atlas/api-gateway/internal/config"
	"github.com/koala/atlas/api-gateway/internal/module/greeting"
	"github.com/koala/atlas/api-gateway/internal/ratelimit"
)

// Server runs the two HTTP listeners of the gateway process:
//
//   - the check listener, which Traefik's ForwardAuth middleware calls for
//     every request (the rate-limit check at the edge), and
//   - the gateway listener, which serves the modular monolith's protected
//     module endpoints after the check passes.
type Server struct {
	check     *http.Server
	gateway   *http.Server
	checkLn   net.Listener
	gatewayLn net.Listener
	logger    *zap.Logger
}

// NewServer wires the routing for both listeners.
func NewServer(cfg config.Config, limiter *ratelimit.Limiter, logger *zap.Logger) *Server {
	auth := NewAuthHandler(limiter, logger)
	greet := greeting.New()

	checkMux := http.NewServeMux()
	checkMux.Handle("/auth", auth)
	checkMux.Handle("/healthz", healthHandler())

	gatewayMux := http.NewServeMux()
	gatewayMux.Handle("/api/v1/greet", recoverMiddleware(greet, logger))
	gatewayMux.Handle("/healthz", healthHandler())

	return &Server{
		check:   newHTTPServer(cfg.CheckAddr, checkMux),
		gateway: newHTTPServer(cfg.GatewayAddr, gatewayMux),
		logger:  logger,
	}
}

// Start binds both listeners up front (so a bind failure, e.g. a port already
// in use, is surfaced synchronously) and then serves on both.
func (s *Server) Start() error {
	checkLn, err := net.Listen("tcp", s.check.Addr)
	if err != nil {
		return fmt.Errorf("check listener %s: %w", s.check.Addr, err)
	}
	gatewayLn, err := net.Listen("tcp", s.gateway.Addr)
	if err != nil {
		_ = checkLn.Close()
		return fmt.Errorf("gateway listener %s: %w", s.gateway.Addr, err)
	}
	s.checkLn, s.gatewayLn = checkLn, gatewayLn

	errCh := make(chan error, 2)
	go func() { _ = s.check.Serve(checkLn) }()
	go func() { _ = s.gateway.Serve(gatewayLn) }()

	// Any immediate serve error (beyond the already-bound listener) is fatal.
	select {
	case err := <-errCh:
		if err != nil {
			return err
		}
	case <-time.After(50 * time.Millisecond):
	}
	return nil
}

// Shutdown gracefully stops both listeners.
func (s *Server) Shutdown(ctx context.Context) error {
	var firstErr error
	if err := s.gateway.Shutdown(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	if err := s.check.Shutdown(ctx); err != nil && firstErr == nil {
		firstErr = err
	}
	return firstErr
}

func newHTTPServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
}

func healthHandler() http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}
}

// recoverMiddleware logs panics from modules and returns 500 instead of
// crashing the whole monolith process.
func recoverMiddleware(next http.Handler, logger *zap.Logger) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic in module handler",
					zap.Any("panic", rec),
					zap.String("path", r.URL.Path),
				)
				http.Error(w, http.StatusText(http.StatusInternalServerError), http.StatusInternalServerError)
			}
		}()
		next.ServeHTTP(w, r)
	})
}
