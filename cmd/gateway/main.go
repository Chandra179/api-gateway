// Command gateway runs the API gateway: a modular monolith Go process that
// exposes (1) a rate-limit check service consumed by Traefik's ForwardAuth
// middleware and (2) the protected module endpoints behind the gateway.
package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/koala/atlas/api-gateway/internal/api"
	"github.com/koala/atlas/api-gateway/internal/config"
	"github.com/koala/atlas/api-gateway/internal/ratelimit"
)

func main() {
	logger, err := newLogger()
	if err != nil {
		// Fall back to a no-op logger so a broken zap config can't mask the
		// real startup failure.
		logger = zap.NewNop()
	}
	if err := run(logger); err != nil {
		logger.Error("gateway exited with error", zap.Error(err))
		os.Exit(1)
	}
}

// newLogger builds the structured JSON logger. It writes to stderr and the
// level is configurable via the ZAP_LEVEL environment variable (defaults to
// "info").
func newLogger() (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{"stderr"}
	return cfg.Build()
}

func run(logger *zap.Logger) error {
	cfg := config.Load()

	// Pick the store: Redis for distributed deployments, in-memory otherwise so
	// the process runs standalone for local development.
	var store ratelimit.Store
	if cfg.RedisAddr != "" && cfg.RedisAddr != "none" {
		rs, err := ratelimit.NewRedisStore(cfg.RedisAddr)
		if err != nil {
			return err
		}
		defer func() { _ = rs.Close() }()
		store = rs
		logger.Info("using Redis store", zap.String("addr", cfg.RedisAddr))
	} else {
		store = ratelimit.NewMemoryStore()
		logger.Info("using in-memory store (single instance only)")
	}

	limiter := ratelimit.NewLimiter(store, ratelimit.WithFailOpen(cfg.FailOpen))
	srv := api.NewServer(cfg, limiter, logger)

	if err := srv.Start(); err != nil {
		return err
	}
	logger.Info("gateway listening",
		zap.String("check", cfg.CheckAddr),
		zap.String("gateway", cfg.GatewayAddr))

	// Graceful shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	logger.Info("gateway stopped")
	return nil
}
