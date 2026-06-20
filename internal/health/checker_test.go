package health

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
)

type fakeTCP struct{ up bool }

func (f fakeTCP) IsListening() bool { return f.up }

type fakePinger struct{ err error }

func (f fakePinger) Ping(context.Context) error { return f.err }

func decode(t *testing.T, body []byte) Result {
	t.Helper()
	var r Result
	if err := json.Unmarshal(body, &r); err != nil {
		t.Fatalf("body not JSON: %v (%s)", err, body)
	}
	return r
}

func TestChecker_AllHealthy(t *testing.T) {
	c := NewChecker(fakeTCP{up: true}, fakePinger{}, fakePinger{})
	code, body := c.Check(context.Background())
	if code != 200 {
		t.Fatalf("code = %d, want 200", code)
	}
	r := decode(t, body)
	if r.Status != "ok" || !r.Checks["tcp"] || !r.Checks["redis"] || !r.Checks["rabbitmq"] {
		t.Fatalf("unexpected result: %+v", r)
	}
}

func TestChecker_RedisDown_503(t *testing.T) {
	c := NewChecker(fakeTCP{up: true}, fakePinger{err: errors.New("redis down")}, fakePinger{})
	code, body := c.Check(context.Background())
	if code != 503 {
		t.Fatalf("code = %d, want 503", code)
	}
	r := decode(t, body)
	if r.Status != "unavailable" || r.Checks["redis"] {
		t.Fatalf("redis should be false: %+v", r)
	}
}

func TestChecker_RabbitMQDown_503(t *testing.T) {
	c := NewChecker(fakeTCP{up: true}, fakePinger{}, fakePinger{err: errors.New("broker down")})
	code, _ := c.Check(context.Background())
	if code != 503 {
		t.Fatalf("code = %d, want 503", code)
	}
}

func TestChecker_TCPNotListening_503(t *testing.T) {
	c := NewChecker(fakeTCP{up: false}, fakePinger{}, fakePinger{})
	code, body := c.Check(context.Background())
	if code != 503 {
		t.Fatalf("code = %d, want 503", code)
	}
	if decode(t, body).Checks["tcp"] {
		t.Fatal("tcp should be false")
	}
}

func TestChecker_NilDepsOnlyChecksTCP(t *testing.T) {
	// Non-rabbitmq modes pass nil pingers; only tcp is checked.
	c := NewChecker(fakeTCP{up: true}, nil, nil)
	code, body := c.Check(context.Background())
	if code != 200 {
		t.Fatalf("code = %d, want 200", code)
	}
	r := decode(t, body)
	if _, ok := r.Checks["redis"]; ok {
		t.Fatal("redis check should be absent when pinger is nil")
	}
}
