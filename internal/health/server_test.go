package health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestServer_Handle_200(t *testing.T) {
	s := NewServer(0, NewChecker(fakeTCP{up: true}, fakePinger{}, fakePinger{}), nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/", nil)

	s.handle(rec, req)

	if rec.Code != 200 {
		t.Fatalf("status = %d, want 200", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Fatalf("content-type = %q", ct)
	}
	decode(t, rec.Body.Bytes())
}

func TestServer_Handle_503(t *testing.T) {
	s := NewServer(0, NewChecker(fakeTCP{up: false}, nil, nil), nil)
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/health/", nil)

	s.handle(rec, req)

	if rec.Code != 503 {
		t.Fatalf("status = %d, want 503", rec.Code)
	}
}

func TestServer_RunStopsOnCancel(t *testing.T) {
	// Bind an ephemeral port via :0 — Run uses cfg port, so use a high one and
	// tolerate bind races by just checking clean shutdown.
	s := NewServer(0, NewChecker(fakeTCP{up: true}, nil, nil), nil)
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- s.Run(ctx) }()
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned %v", err)
		}
	case <-context.Background().Done():
	}
}
