// Package config loads the service configuration from environment variables.
//
// It mirrors the C# ReceiverOptions: prefix RECEIVER__ with __ as the section
// separator (RECEIVER__RABBITMQ__HOST, RECEIVER__REDIS__CONNECTIONSTRING, ...),
// preserving 1:1 parity with the Wave 1 .env files and systemd unit. Defaults
// match the C# property defaults exactly.
package config

import (
	"time"

	"github.com/caarlos0/env/v11"
)

// Config is the full service configuration. Redis/RabbitMQ/Consul are only used
// when OutputMode == "rabbitmq" (wired in later milestones).
type Config struct {
	Port           int    `env:"PORT" envDefault:"5000"`
	DlxPath        string `env:"DLXPATH" envDefault:"./dlx"`
	OutputMode     string `env:"OUTPUTMODE" envDefault:"stdout"`
	OutputFilePath string `env:"OUTPUTFILEPATH" envDefault:"./output"`

	Redis    RedisConfig    `envPrefix:"REDIS__"`
	RabbitMQ RabbitMQConfig `envPrefix:"RABBITMQ__"`
	Consul   ConsulConfig   `envPrefix:"CONSUL__"`
}

// RedisConfig — durability buffer. The numeric fields are kept as bare ints to
// match the C# options and the Wave 1 env files (e.g. RETRYINTERVALSECONDS=30,
// not "30s"); use the helper methods for Go-native durations.
type RedisConfig struct {
	ConnectionString     string `env:"CONNECTIONSTRING" envDefault:"localhost:6379"`
	BufferTTLHours       int    `env:"BUFFERTLTHOURS" envDefault:"24"`
	RetryIntervalSeconds int    `env:"RETRYINTERVALSECONDS" envDefault:"30"`
	RetryMinAgeMinutes   int    `env:"RETRYMINAGEMINUTES" envDefault:"2"`
}

// BufferTTL returns the buffer entry TTL.
func (r RedisConfig) BufferTTL() time.Duration { return time.Duration(r.BufferTTLHours) * time.Hour }

// RetryInterval returns how often the RetryWorker scans the buffer.
func (r RedisConfig) RetryInterval() time.Duration {
	return time.Duration(r.RetryIntervalSeconds) * time.Second
}

// RetryMinAge returns the minimum entry age before a retry is attempted.
func (r RedisConfig) RetryMinAge() time.Duration {
	return time.Duration(r.RetryMinAgeMinutes) * time.Minute
}

// RabbitMQConfig — broker. Password is never set from .env; it comes from the
// secrets file in the systemd EnvironmentFile.
type RabbitMQConfig struct {
	Host                  string `env:"HOST" envDefault:"localhost"`
	Port                  int    `env:"PORT" envDefault:"5672"`
	VirtualHost           string `env:"VIRTUALHOST" envDefault:"/"`
	Username              string `env:"USERNAME" envDefault:"data_receiving"`
	Password              string `env:"PASSWORD"`
	Exchange              string `env:"EXCHANGE" envDefault:"sensors.raw"`
	ConfirmTimeoutSeconds int    `env:"CONFIRMTIMEOUTSECONDS" envDefault:"5"`
}

// ConfirmTimeout returns the publisher-confirm wait timeout.
func (r RabbitMQConfig) ConfirmTimeout() time.Duration {
	return time.Duration(r.ConfirmTimeoutSeconds) * time.Second
}

// ConsulConfig — service discovery. Token comes from the secrets file.
type ConsulConfig struct {
	Address        string `env:"ADDRESS" envDefault:"http://localhost:8500"`
	Token          string `env:"TOKEN"`
	InstanceID     string `env:"INSTANCEID" envDefault:"data-receiving-01"`
	HealthPort     int    `env:"HEALTHPORT" envDefault:"8080"`
	ServiceVersion string `env:"SERVICEVERSION" envDefault:"1.0.0"`
	Environment    string `env:"ENVIRONMENT" envDefault:"prod"`

	// HealthCheckHost is the host Consul uses to reach this service's /health
	// endpoint. Defaults to localhost (Consul agent on the same host, as in
	// Wave 1). When Consul runs in a container it cannot reach the host via
	// localhost, so set this to the service's reachable address (e.g. the
	// container name or host IP).
	HealthCheckHost string `env:"HEALTHCHECKHOST" envDefault:"localhost"`
}

// Load reads the configuration from the environment using the RECEIVER__ prefix.
func Load() (Config, error) {
	var cfg Config
	if err := env.ParseWithOptions(&cfg, env.Options{Prefix: "RECEIVER__"}); err != nil {
		return Config{}, err
	}
	return cfg, nil
}
