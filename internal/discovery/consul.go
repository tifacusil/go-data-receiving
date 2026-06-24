// Package discovery registers the service in Consul with an HTTP health check
// and deregisters it on shutdown. Port of the C# ConsulRegistration IHostedService.
package discovery

import (
	"context"
	"fmt"
	"log/slog"

	consul "github.com/hashicorp/consul/api"

	"github.com/AngelloFD/go-data-receiving/internal/config"
)

// Registration manages this instance's Consul service entry.
type Registration struct {
	client    *consul.Client
	serviceID string
	cfg       config.ConsulConfig
	logger    *slog.Logger
}

// New builds a Consul client from config and returns a Registration. The service
// ID matches the C#: data-receiving-{InstanceId}.
func New(cfg config.ConsulConfig, logger *slog.Logger) (*Registration, error) {
	if logger == nil {
		logger = slog.Default()
	}
	apiCfg := consul.DefaultConfig()
	apiCfg.Address = cfg.Address
	if cfg.Token != "" {
		apiCfg.Token = cfg.Token
	}
	client, err := consul.NewClient(apiCfg)
	if err != nil {
		return nil, fmt.Errorf("consul client: %w", err)
	}
	return &Registration{
		client:    client,
		serviceID: "data-receiving-" + cfg.InstanceID,
		cfg:       cfg,
		logger:    logger,
	}, nil
}

// Run registers the service, blocks until ctx is cancelled, then deregisters.
// Designed to run as an errgroup goroutine alongside the other workers.
func (r *Registration) Run(ctx context.Context) error {
	if err := r.register(); err != nil {
		return err
	}
	<-ctx.Done()
	r.deregister()
	return nil
}

// healthHost returns the host Consul should use to reach /health, defaulting to
// localhost when unset (Wave 1 layout: Consul agent on the same host).
func (r *Registration) healthHost() string {
	if r.cfg.HealthCheckHost == "" {
		return "localhost"
	}
	return r.cfg.HealthCheckHost
}

func (r *Registration) register() error {
	reg := &consul.AgentServiceRegistration{
		ID:   r.serviceID,
		Name: "data-receiving",
		// Address debe ser la dirección por la que el resto del sistema alcanza
		// esta instancia (mismo host que el check). Sin esto Consul devuelve
		// ServiceAddress vacío y consul-template no puede armar el upstream de
		// nginx → 502. Usa healthHost() (hostname del contenedor en la red).
		Address: r.healthHost(),
		Port:    r.cfg.HealthPort,
		Tags: []string{
			r.cfg.Environment,
			"version:" + r.cfg.ServiceVersion,
			"instance:" + r.cfg.InstanceID,
		},
		Check: &consul.AgentServiceCheck{
			HTTP:                           fmt.Sprintf("http://%s:%d/health/", r.healthHost(), r.cfg.HealthPort),
			Interval:                       "10s",
			Timeout:                        "5s",
			DeregisterCriticalServiceAfter: "1m",
		},
	}
	if err := r.client.Agent().ServiceRegister(reg); err != nil {
		return fmt.Errorf("consul register %s: %w", r.serviceID, err)
	}
	r.logger.Info("registered with consul", slog.String("service_id", r.serviceID))
	return nil
}

// deregister removes the service entry. Uses a fresh background context since the
// run context is already cancelled at shutdown.
func (r *Registration) deregister() {
	if err := r.client.Agent().ServiceDeregister(r.serviceID); err != nil {
		r.logger.Warn("consul deregister failed", slog.String("service_id", r.serviceID), slog.Any("error", err))
		return
	}
	r.logger.Info("deregistered from consul", slog.String("service_id", r.serviceID))
}
