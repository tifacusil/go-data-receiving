package health

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// stubHandler marks that it was hit.
type stubHandler struct{ hit bool }

func (s *stubHandler) ServeHTTP(w http.ResponseWriter, _ *http.Request) {
	s.hit = true
	w.WriteHeader(200)
	_, _ = w.Write([]byte("# metrics"))
}

func TestServer_MetricsRouteServed(t *testing.T) {
	stub := &stubHandler{}
	s := NewServer(0, NewChecker(fakeTCP{up: true}, nil, nil), nil).WithMetrics(stub)

	// Build the same mux Run builds and exercise /metrics.
	mux := http.NewServeMux()
	mux.HandleFunc("/health/", s.handle)
	mux.Handle("/metrics", s.metrics)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))

	if !stub.hit {
		t.Fatal("/metrics handler not invoked")
	}
	if rec.Code != 200 {
		t.Fatalf("status = %d", rec.Code)
	}
}

func TestServer_NoMetricsWhenUnset(t *testing.T) {
	s := NewServer(0, NewChecker(fakeTCP{up: true}, nil, nil), nil)
	if s.metrics != nil {
		t.Fatal("metrics handler should be nil by default")
	}
}
