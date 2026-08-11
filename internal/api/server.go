// Package api exposes the rate-limit check service consumed by Traefik's
// ForwardAuth middleware. Protected backend services (e.g. cmd/greeting) are
// separate deployables discovered by Traefik via Consul; this package no
// longer serves them directly.
package api

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"time"

	"go.uber.org/zap"

	"github.com/koala/atlas/api-gateway/internal/config"
	"github.com/koala/atlas/api-gateway/internal/httpserver"
	"github.com/koala/atlas/api-gateway/internal/ratelimit"
)

// Server runs the check listener that Traefik's ForwardAuth middleware calls
// for every request (the rate-limit check at the edge).
type Server struct {
	check   *http.Server
	checkLn net.Listener
	logger  *zap.Logger
}

// NewServer wires the routing for the check listener.
func NewServer(cfg config.Config, limiter *ratelimit.Limiter, logger *zap.Logger) *Server {
	auth := NewAuthHandler(limiter, logger)

	checkMux := http.NewServeMux()
	checkMux.Handle("/auth", auth)
	checkMux.Handle("/healthz", httpserver.HealthHandler())

	return &Server{
		check:  httpserver.New(cfg.CheckAddr, checkMux),
		logger: logger,
	}
}

// Start binds the listener up front (so a bind failure, e.g. a port already
// in use, is surfaced synchronously) and then serves it.
func (s *Server) Start() error {
	checkLn, err := net.Listen("tcp", s.check.Addr)
	if err != nil {
		return fmt.Errorf("check listener %s: %w", s.check.Addr, err)
	}
	s.checkLn = checkLn

	errCh := make(chan error, 1)
	go func() { errCh <- s.check.Serve(checkLn) }()

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

// Shutdown gracefully stops the listener.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.check.Shutdown(ctx)
}
