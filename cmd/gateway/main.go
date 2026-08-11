// Command gateway runs the rate-limit check service consumed by the mesh at
// the edge: it implements checkLimit(api_key) -> allow | reject over HTTP.
// It is not yet Connect-enabled — see internal/discovery for the plain
// Consul registration used here, and the note below on ext_authz.
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
	"github.com/koala/atlas/api-gateway/internal/discovery"
	"github.com/koala/atlas/api-gateway/internal/httpserver"
	"github.com/koala/atlas/api-gateway/internal/ratelimit"
)

func main() {
	logger, err := newLogger()
	if err != nil {
		// Fall back to a no-op logger so a broken zap config can't mask the
		// real startup failure.
		logger.Error("init logger error", zap.Error(err))
		os.Exit(1)
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
	store, err := ratelimit.NewRedisStore(cfg.RedisAddr)
	if err != nil {
		return err
	}
	defer func() { _ = store.Close() }()
	logger.Info("using Redis store", zap.String("addr", cfg.RedisAddr))

	limiter := ratelimit.NewLimiter(store, ratelimit.WithFailOpen(cfg.FailOpen))
	srv := api.NewServer(cfg, limiter, logger)

	if err := srv.Start(); err != nil {
		return err
	}
	logger.Info("gateway listening", zap.String("check", cfg.CheckAddr))

	// Not yet part of the Connect mesh: this service stays a plain HTTP
	// endpoint for now. Once ext_authz wiring lands, mesh sidecars will call
	// http://gateway:8099/auth directly for the rate-limit decision.
	var deregister func(context.Context) error
	if cfg.ConsulEnabled {
		deregister, err = discovery.Register(discovery.Config{
			Addr:          cfg.ConsulAddr,
			ServiceName:   orDefault(cfg.ServiceName, "gateway-check"),
			AdvertiseAddr: cfg.AdvertiseAddr,
			Port:          httpserver.PortFrom(cfg.CheckAddr),
			Connect:       false,
		}, logger)
		if err != nil {
			return err
		}
	}

	// Graceful shutdown on SIGINT/SIGTERM.
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)
	<-stop

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if deregister != nil {
		if derr := deregister(ctx); derr != nil {
			logger.Warn("consul deregister failed", zap.Error(derr))
		}
	}
	if err := srv.Shutdown(ctx); err != nil && !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	logger.Info("gateway stopped")
	return nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
