package api

import (
	"context"
	"net"
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap"

	"github.com/koala/atlas/api-gateway/internal/config"
	"github.com/koala/atlas/api-gateway/internal/ratelimit"
)

func discardLogger() *zap.Logger {
	return zap.NewNop()
}

// occupiedAddr returns an address that is already bound, so a Server trying to
// bind it must fail.
func occupiedAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("bind: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	return ln.Addr().String()
}

func TestStartReturnsErrorOnBindFailure(t *testing.T) {
	occupied := occupiedAddr(t)
	limiter := ratelimit.NewLimiter(ratelimit.NewMemoryStore())
	cfg := config.Config{CheckAddr: occupied}
	srv := NewServer(cfg, limiter, discardLogger())

	if err := srv.Start(); err == nil {
		t.Fatal("expected Start to return a bind error, got nil")
	}
}

func TestStartAndShutdown(t *testing.T) {
	limiter := ratelimit.NewLimiter(ratelimit.NewMemoryStore())
	cfg := config.Config{CheckAddr: "127.0.0.1:0"}
	srv := NewServer(cfg, limiter, discardLogger())

	if err := srv.Start(); err != nil {
		t.Fatalf("Start: %v", err)
	}

	resp, err := http.Get("http://" + srv.checkLn.Addr().String() + "/healthz")
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /healthz: status %d", resp.StatusCode)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown: %v", err)
	}
}
