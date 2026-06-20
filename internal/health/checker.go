// Package health serves the /health endpoint and checks real dependency
// liveness. Unlike the C# HealthEndpoint (which only reports whether the TCP
// listener is up), the checker also pings Redis and RabbitMQ, per CLAUDE.md
// §1.6/§4: a health check must verify the DB and broker, not return a static 200.
package health

import (
	"context"
	"encoding/json"
	"time"
)

// tcpStatus reports whether the TCP listener is accepting connections.
type tcpStatus interface{ IsListening() bool }

// pinger verifies liveness of a dependency. Satisfied by the Redis buffer and
// the RabbitMQ health pinger.
type pinger interface {
	Ping(ctx context.Context) error
}

// Checker aggregates the liveness of the TCP listener, Redis and RabbitMQ.
type Checker struct {
	tcp   tcpStatus
	redis pinger
	rmq   pinger
}

// NewChecker wires a Checker. redis and rmq may be nil in non-rabbitmq modes, in
// which case only the TCP listener is checked.
func NewChecker(tcp tcpStatus, redis, rmq pinger) *Checker {
	return &Checker{tcp: tcp, redis: redis, rmq: rmq}
}

// Result is the JSON body returned by /health.
type Result struct {
	Status string          `json:"status"`
	Checks map[string]bool `json:"checks"`
}

// Check runs all configured checks and reports an HTTP status code plus a JSON
// body. It returns 200 only when every configured dependency is healthy,
// otherwise 503 (CLAUDE.md §4: 503 if a critical dependency is down).
func (c *Checker) Check(ctx context.Context) (int, []byte) {
	checks := map[string]bool{
		"tcp": c.tcp != nil && c.tcp.IsListening(),
	}
	if c.redis != nil {
		checks["redis"] = pingOK(ctx, c.redis)
	}
	if c.rmq != nil {
		checks["rabbitmq"] = pingOK(ctx, c.rmq)
	}

	healthy := true
	for _, ok := range checks {
		if !ok {
			healthy = false
			break
		}
	}

	status := "ok"
	code := 200
	if !healthy {
		status = "unavailable"
		code = 503
	}

	body, _ := json.Marshal(Result{Status: status, Checks: checks})
	return code, body
}

func pingOK(ctx context.Context, p pinger) bool {
	ctx, cancel := context.WithTimeout(ctx, 2*time.Second)
	defer cancel()
	return p.Ping(ctx) == nil
}
