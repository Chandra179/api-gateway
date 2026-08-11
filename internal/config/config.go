// Package config loads runtime configuration for the API gateway from the
// environment. Every value has a sane default so the service runs with zero
// configuration for local development.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds the runtime settings shared by every gateway binary. Not every
// field is used by every binary (e.g. cmd/greeting ignores RedisAddr).
type Config struct {
	// GreetingAddr is the bind address of the greeting service, the example
	// standalone backend behind the gateway.
	GreetingAddr string

	// CheckAddr is the bind address of the rate-limit check service that
	// Traefik's ForwardAuth middleware calls with every request.
	CheckAddr string

	// RedisAddr is the shared Redis endpoint (token bucket + daily quota).
	RedisAddr string

	// FailOpen controls behaviour when Redis is unavailable. When true the
	// limiter allows all requests through (per the design doc); when false it
	// rejects them. Defaults to true.
	FailOpen bool

	// CheckTimeout bounds the time spent on a single rate-limit check so a slow
	// Redis cannot blow the 5ms p99 latency budget for the whole request.
	CheckTimeout time.Duration

	// ConsulEnabled turns on self-registration with Consul. Off by default so
	// a bare `go run` needs no external dependencies.
	ConsulEnabled bool

	// ConsulConnect additionally registers a managed Envoy sidecar (Consul
	// Connect) so this instance participates in the mTLS mesh. Ignored if
	// ConsulEnabled is false.
	ConsulConnect bool

	// ConsulAddr is the Consul agent's HTTP API address.
	ConsulAddr string

	// ServiceName is the name this instance registers under in Consul. Each
	// binary supplies its own default (e.g. "gateway-check", "greeting").
	ServiceName string

	// AdvertiseAddr is the hostname/address other Consul-registered
	// consumers use to reach this instance (e.g. the Docker Compose service
	// name).
	AdvertiseAddr string
}

// Load reads configuration from environment variables, falling back to
// development-friendly defaults.
func Load() Config {
	return Config{
		GreetingAddr: env("GREETING_ADDR", ":8098"),
		CheckAddr:    env("CHECK_ADDR", ":8099"),
		RedisAddr:    env("REDIS_ADDR", "localhost:6379"),
		FailOpen:     envBool("FAIL_OPEN", true),
		CheckTimeout: envDuration(
			"CHECK_TIMEOUT",
			time.Duration(50)*time.Millisecond,
		),
		ConsulEnabled: envBool("CONSUL_ENABLED", false),
		ConsulConnect: envBool("CONSUL_CONNECT", false),
		ConsulAddr:    env("CONSUL_ADDR", "127.0.0.1:8500"),
		ServiceName:   env("SERVICE_NAME", ""),
		AdvertiseAddr: env("ADVERTISE_ADDR", ""),
	}
}

func env(key, def string) string {
	if v, ok := os.LookupEnv(key); ok && v != "" {
		return v
	}
	return def
}

func envBool(key string, def bool) bool {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return def
	}
	return b
}

func envDuration(key string, def time.Duration) time.Duration {
	v, ok := os.LookupEnv(key)
	if !ok || v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}
