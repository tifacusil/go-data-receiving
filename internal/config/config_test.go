package config

import (
	"testing"
	"time"
)

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Port != 5000 {
		t.Errorf("Port = %d, want 5000", cfg.Port)
	}
	if cfg.OutputMode != "stdout" {
		t.Errorf("OutputMode = %q, want stdout", cfg.OutputMode)
	}
	if cfg.DlxPath != "./dlx" {
		t.Errorf("DlxPath = %q, want ./dlx", cfg.DlxPath)
	}
	if cfg.Redis.RetryInterval() != 30*time.Second {
		t.Errorf("RetryInterval = %v, want 30s", cfg.Redis.RetryInterval())
	}
	if cfg.Redis.RetryMinAge() != 2*time.Minute {
		t.Errorf("RetryMinAge = %v, want 2m", cfg.Redis.RetryMinAge())
	}
	if cfg.Redis.BufferTTL() != 24*time.Hour {
		t.Errorf("BufferTTL = %v, want 24h", cfg.Redis.BufferTTL())
	}
	if cfg.RabbitMQ.Exchange != "sensors.raw" {
		t.Errorf("Exchange = %q, want sensors.raw", cfg.RabbitMQ.Exchange)
	}
	if cfg.Consul.HealthPort != 8080 {
		t.Errorf("HealthPort = %d, want 8080", cfg.Consul.HealthPort)
	}
}

func TestLoad_EnvOverridesNested(t *testing.T) {
	// The __ section separator must map RECEIVER__RABBITMQ__HOST onto RabbitMQ.Host.
	t.Setenv("RECEIVER__PORT", "5002")
	t.Setenv("RECEIVER__OUTPUTMODE", "rabbitmq")
	t.Setenv("RECEIVER__RABBITMQ__HOST", "204.168.203.32")
	t.Setenv("RECEIVER__REDIS__CONNECTIONSTRING", "redis-host:6379,password=secret")
	t.Setenv("RECEIVER__REDIS__RETRYINTERVALSECONDS", "45")
	t.Setenv("RECEIVER__CONSUL__INSTANCEID", "data-receiving-02")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Port != 5002 {
		t.Errorf("Port = %d, want 5002", cfg.Port)
	}
	if cfg.OutputMode != "rabbitmq" {
		t.Errorf("OutputMode = %q, want rabbitmq", cfg.OutputMode)
	}
	if cfg.RabbitMQ.Host != "204.168.203.32" {
		t.Errorf("RabbitMQ.Host = %q", cfg.RabbitMQ.Host)
	}
	if cfg.Redis.ConnectionString != "redis-host:6379,password=secret" {
		t.Errorf("Redis.ConnectionString = %q", cfg.Redis.ConnectionString)
	}
	if cfg.Redis.RetryInterval() != 45*time.Second {
		t.Errorf("RetryInterval = %v, want 45s", cfg.Redis.RetryInterval())
	}
	if cfg.Consul.InstanceID != "data-receiving-02" {
		t.Errorf("Consul.InstanceID = %q", cfg.Consul.InstanceID)
	}
}
