// Package discovery registers gateway binaries with Consul so the service
// mesh (Consul Connect + Envoy) can route to them dynamically, and so
// operators get a single source of truth for what's running and healthy.
package discovery

import (
	"context"
	"fmt"
	"net"
	"time"

	consulapi "github.com/hashicorp/consul/api"
	"go.uber.org/zap"
)

// Config describes how a service instance should register itself.
type Config struct {
	// Addr is the Consul agent's HTTP API address, e.g. "consul:8500".
	Addr string

	// ServiceName is the logical service name other components discover
	// (e.g. "greeting", "gateway-check").
	ServiceName string

	// ServiceID uniquely identifies this instance. Defaults to ServiceName.
	// NOTE: this only holds for one replica per service (the current
	// topology) — `consul connect envoy -sidecar-for <id>` needs a
	// predictable ID at container-start time. Once services run multiple
	// replicas, this needs a per-instance scheme again (e.g. hostname
	// suffix) and the sidecar command needs a matching lookup strategy.
	ServiceID string

	// AdvertiseAddr is the address other services/the mesh use to reach this
	// instance (e.g. the Docker Compose service name).
	AdvertiseAddr string

	// Port is the TCP port this instance listens on.
	Port int

	// Connect registers a managed Envoy sidecar for this service so it
	// participates in the Consul Connect mesh (mTLS, intentions). Services
	// not yet meshed (e.g. the rate-limit check service in this phase) leave
	// this false and register as a plain Consul service.
	Connect bool

	// HealthPath is the HTTP path Consul polls for liveness. Defaults to
	// "/healthz".
	HealthPath string

	// Interval and Timeout control the health check cadence. Default to 5s
	// and 2s respectively.
	Interval time.Duration
	Timeout  time.Duration

	// DeregisterAfter removes a service that has been failing its health
	// check for this long, as a safety net for instances that die without
	// deregistering. Defaults to 1 minute.
	DeregisterAfter time.Duration
}

// Register registers the calling service with Consul and returns a function
// that deregisters it; call the returned function during graceful shutdown.
func Register(cfg Config, logger *zap.Logger) (func(context.Context) error, error) {
	client, err := consulapi.NewClient(&consulapi.Config{Address: cfg.Addr})
	if err != nil {
		return nil, fmt.Errorf("consul client: %w", err)
	}

	id := orDefault(cfg.ServiceID, cfg.ServiceName)

	// Envoy's EDS wants a literal IP for each endpoint, not a DNS name -
	// resolve AdvertiseAddr (typically a Docker Compose service name) up
	// front so Connect-enabled registrations propagate a real IP.
	advertiseAddr := cfg.AdvertiseAddr
	if ip := net.ParseIP(advertiseAddr); ip == nil && advertiseAddr != "" {
		if ips, err := net.LookupHost(advertiseAddr); err == nil && len(ips) > 0 {
			advertiseAddr = ips[0]
		} else {
			return nil, fmt.Errorf("resolve advertise addr %q: %w", cfg.AdvertiseAddr, err)
		}
	}

	reg := &consulapi.AgentServiceRegistration{
		ID:      id,
		Name:    cfg.ServiceName,
		Address: advertiseAddr,
		Port:    cfg.Port,
		Check: &consulapi.AgentServiceCheck{
			HTTP:                           fmt.Sprintf("http://%s:%d%s", cfg.AdvertiseAddr, cfg.Port, orDefault(cfg.HealthPath, "/healthz")),
			Interval:                       durOrDefault(cfg.Interval, 5*time.Second).String(),
			Timeout:                        durOrDefault(cfg.Timeout, 2*time.Second).String(),
			DeregisterCriticalServiceAfter: durOrDefault(cfg.DeregisterAfter, time.Minute).String(),
		},
	}

	if cfg.Connect {
		// A managed sidecar: Consul spins up (and this repo's Compose file
		// separately runs, via `consul connect envoy -sidecar-for <id>`) an
		// Envoy proxy that terminates mTLS on this service's behalf. No
		// Upstreams are declared yet — nothing in this phase calls out
		// through the mesh; add them here once a meshed service needs to
		// reach another meshed service (e.g. gateway-check -> greeting).
		reg.Connect = &consulapi.AgentServiceConnect{
			SidecarService: &consulapi.AgentServiceRegistration{},
		}
	}

	if err := client.Agent().ServiceRegister(reg); err != nil {
		return nil, fmt.Errorf("consul register %s: %w", id, err)
	}
	logger.Info("registered with consul",
		zap.String("id", id),
		zap.String("name", cfg.ServiceName),
		zap.String("advertise_addr", cfg.AdvertiseAddr),
		zap.Int("port", cfg.Port),
		zap.Bool("connect", cfg.Connect),
	)

	return func(_ context.Context) error {
		if err := client.Agent().ServiceDeregister(id); err != nil {
			return fmt.Errorf("consul deregister %s: %w", id, err)
		}
		return nil
	}, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}

func durOrDefault(v, def time.Duration) time.Duration {
	if v <= 0 {
		return def
	}
	return v
}
