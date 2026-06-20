//go:build integration

package integration

import (
	"context"
	"testing"
	"time"

	consul "github.com/hashicorp/consul/api"

	"github.com/AngelloFD/go-data-receiving/internal/config"
	"github.com/AngelloFD/go-data-receiving/internal/discovery"
)

func consulAddr() string { return envOr("IT_CONSUL_ADDR", "http://localhost:8500") }

func consulClient(t *testing.T) *consul.Client {
	t.Helper()
	cfg := consul.DefaultConfig()
	cfg.Address = consulAddr()
	if tok := envOr("IT_CONSUL_TOKEN", ""); tok != "" {
		cfg.Token = tok
	}
	c, err := consul.NewClient(cfg)
	if err != nil {
		t.Fatalf("consul client: %v", err)
	}
	if _, err := c.Agent().Self(); err != nil {
		t.Skipf("consul not reachable at %s: %v", consulAddr(), err)
	}
	return c
}

func TestConsul_RegisterAndDeregister(t *testing.T) {
	client := consulClient(t)

	instanceID := "it-" + time.Now().UTC().Format("150405")
	cfg := config.ConsulConfig{
		Address:        consulAddr(),
		Token:          envOr("IT_CONSUL_TOKEN", ""),
		InstanceID:     instanceID,
		HealthPort:     18080,
		ServiceVersion: "1.0.0",
		Environment:    "test",
	}
	serviceID := "data-receiving-" + instanceID

	reg, err := discovery.New(cfg, nil)
	if err != nil {
		t.Fatalf("discovery.New: %v", err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- reg.Run(ctx) }()

	// Wait for registration to appear in the catalog.
	if !waitForService(t, client, serviceID, true, 3*time.Second) {
		cancel()
		<-done
		t.Fatal("service did not appear in consul catalog")
	}

	// Verify tags.
	svcs, _, err := client.Catalog().Service("data-receiving", "", nil)
	if err != nil {
		t.Fatalf("catalog service: %v", err)
	}
	var found *consul.CatalogService
	for _, s := range svcs {
		if s.ServiceID == serviceID {
			found = s
		}
	}
	if found == nil {
		t.Fatal("registered service not found")
	}
	if !hasTag(found.ServiceTags, "version:1.0.0") || !hasTag(found.ServiceTags, "instance:"+instanceID) {
		t.Fatalf("tags missing: %v", found.ServiceTags)
	}

	// Cancel -> Run deregisters on shutdown.
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("Run returned %v", err)
	}
	if waitForService(t, client, serviceID, false, 3*time.Second) {
		// present==false means it's gone
	} else {
		t.Fatal("service still registered after deregister")
	}
}

func waitForService(t *testing.T, c *consul.Client, serviceID string, wantPresent bool, timeout time.Duration) bool {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		svcs, _, err := c.Catalog().Service("data-receiving", "", nil)
		if err != nil {
			t.Fatalf("catalog: %v", err)
		}
		present := false
		for _, s := range svcs {
			if s.ServiceID == serviceID {
				present = true
			}
		}
		if present == wantPresent {
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

func hasTag(tags []string, want string) bool {
	for _, tg := range tags {
		if tg == want {
			return true
		}
	}
	return false
}
