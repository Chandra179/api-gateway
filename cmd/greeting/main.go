// Command greeting runs the greeting module as a standalone, independently
// deployable backend service behind the mesh. It carries no rate-limit or
// Redis dependency: enforcement happens at the edge (cmd/gateway) and
// transport security happens in the mesh (Consul Connect + Envoy sidecar).
package main

import (
	"context"
	"errors"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"

	"github.com/koala/atlas/api-gateway/internal/config"
	"github.com/koala/atlas/api-gateway/internal/discovery"
	"github.com/koala/atlas/api-gateway/internal/httpserver"
	"github.com/koala/atlas/api-gateway/internal/module/greeting"
)

func main() {
	logger, err := newLogger()
	if err != nil {
		logger.Error("init logger error", zap.Error(err))
		os.Exit(1)
	}
	if err := run(logger); err != nil {
		logger.Error("greeting exited with error", zap.Error(err))
		os.Exit(1)
	}
}

func newLogger() (*zap.Logger, error) {
	cfg := zap.NewProductionConfig()
	cfg.OutputPaths = []string{"stderr"}
	return cfg.Build()
}

func run(logger *zap.Logger) error {
	cfg := config.Load()

	mux := http.NewServeMux()
	mux.Handle("/greet", httpserver.Recover(greeting.New(), logger))
	mux.Handle("/healthz", httpserver.HealthHandler())

	srv := httpserver.New(cfg.GreetingAddr, mux)
	ln, err := net.Listen("tcp", srv.Addr)
	if err != nil {
		return err
	}
	go func() { _ = srv.Serve(ln) }()
	logger.Info("greeting listening", zap.String("addr", cfg.GreetingAddr))

	var deregister func(context.Context) error
	if cfg.ConsulEnabled {
		deregister, err = discovery.Register(discovery.Config{
			Addr:          cfg.ConsulAddr,
			ServiceName:   orDefault(cfg.ServiceName, "greeting"),
			AdvertiseAddr: cfg.AdvertiseAddr,
			Port:          httpserver.PortFrom(cfg.GreetingAddr),
			Connect:       cfg.ConsulConnect,
		}, logger)
		if err != nil {
			return err
		}
	}

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
	logger.Info("greeting stopped")
	return nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
