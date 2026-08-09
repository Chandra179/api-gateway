// Package config loads runtime configuration for the API gateway from the
// environment. Every value has a sane default so the service runs with zero
// configuration for local development.
package config

import (
	"os"
	"strconv"
	"time"
)

// Config holds the runtime settings for the gateway process.
type Config struct {
	// GatewayAddr is the bind address of the protected module API (the
	// downstream endpoints that Traefik forwards allowed requests to).
	GatewayAddr string

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
}

// Load reads configuration from environment variables, falling back to
// development-friendly defaults.
func Load() Config {
	return Config{
		GatewayAddr: env("GATEWAY_ADDR", ":8098"),
		CheckAddr:   env("CHECK_ADDR", ":8099"),
		RedisAddr:   env("REDIS_ADDR", "localhost:6379"),
		FailOpen:    envBool("FAIL_OPEN", true),
		CheckTimeout: envDuration(
			"CHECK_TIMEOUT",
			time.Duration(50)*time.Millisecond,
		),
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
