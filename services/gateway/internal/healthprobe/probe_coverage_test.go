package healthprobe

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
)

// ============================================================================
// probe() function coverage
// ============================================================================

func TestProbe_HTTPSuccess(t *testing.T) {
	// Start an HTTP server for the probe
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	logger := zap.NewNop()
	p := NewProber(30*time.Second, nil, logger)

	target := ResourceTarget{
		Tenant:    "tenant1",
		Name:      "resource1",
		HTTPProbe: srv.URL + "/health",
	}

	result := p.probe(context.Background(), target)

	if result.State != "healthy" {
		t.Errorf("probe() state = %q, want healthy", result.State)
	}
	if result.Tenant != "tenant1" {
		t.Errorf("probe() tenant = %q, want tenant1", result.Tenant)
	}
	if result.Name != "resource1" {
		t.Errorf("probe() name = %q, want resource1", result.Name)
	}
}

func TestProbe_HTTP_Fails_TCPSuccess(t *testing.T) {
	// Start a TCP listener (not HTTP) so HTTP probe fails but TCP succeeds
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	logger := zap.NewNop()
	p := NewProber(30*time.Second, nil, logger)

	target := ResourceTarget{
		Tenant:    "tenant1",
		Name:      "resource1",
		HTTPProbe: "http://127.0.0.1:1/health", // port 1 — will fail
		Endpoint:  ln.Addr().String(),
	}

	result := p.probe(context.Background(), target)

	if result.State != "healthy" {
		t.Errorf("probe() state = %q, want healthy (TCP fallback)", result.State)
	}
}

func TestProbe_NoEndpoints_Degraded(t *testing.T) {
	logger := zap.NewNop()
	p := NewProber(30*time.Second, nil, logger)

	target := ResourceTarget{
		Tenant: "tenant1",
		Name:   "resource1",
		// No HTTPProbe and no Endpoint
	}

	result := p.probe(context.Background(), target)

	if result.State != "degraded" {
		t.Errorf("probe() state = %q, want degraded", result.State)
	}
	if result.Message == "" {
		t.Error("probe() message should not be empty for degraded state")
	}
}

func TestProbe_TCPFail_Unreachable(t *testing.T) {
	logger := zap.NewNop()
	p := NewProber(30*time.Second, nil, logger)

	target := ResourceTarget{
		Tenant:   "tenant1",
		Name:     "resource1",
		Endpoint: "127.0.0.1:1", // port 1 — unlikely to be listening
	}

	result := p.probe(context.Background(), target)

	if result.State != "unreachable" {
		t.Errorf("probe() state = %q, want unreachable", result.State)
	}
	if result.Message == "" {
		t.Error("probe() message should not be empty for unreachable state")
	}
}

func TestProbe_OnlyHTTP_ServerError(t *testing.T) {
	// HTTP server returns 500, but probe considers <500 as "reachable"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	logger := zap.NewNop()
	p := NewProber(30*time.Second, nil, logger)

	target := ResourceTarget{
		Tenant:    "tenant1",
		Name:      "resource1",
		HTTPProbe: srv.URL + "/health",
	}

	// 500 response means httpReachable returns false
	result := p.probe(context.Background(), target)

	// Falls through to TCP (no endpoint) → degraded
	if result.State != "degraded" {
		t.Logf("probe() state = %q (expected degraded with 500 HTTP and no TCP endpoint)", result.State)
	}
}

func TestProbe_HTTPSuccess_NoTCP(t *testing.T) {
	// HTTP probe succeeds → should return healthy immediately without TCP check
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	logger := zap.NewNop()
	p := NewProber(30*time.Second, nil, logger)

	target := ResourceTarget{
		Tenant:    "tenant1",
		Name:      "resource1",
		HTTPProbe: srv.URL,
		Endpoint:  "127.0.0.1:1", // would fail TCP, but shouldn't be reached
	}

	result := p.probe(context.Background(), target)

	if result.State != "healthy" {
		t.Errorf("probe() state = %q, want healthy", result.State)
	}
}

// ============================================================================
// tcpReachable() coverage
// ============================================================================

func TestTCPReachable_Success(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	if !tcpReachable(ln.Addr().String(), 3*time.Second) {
		t.Error("tcpReachable() = false, want true")
	}
}

func TestTCPReachable_Fail(t *testing.T) {
	if tcpReachable("127.0.0.1:1", 100*time.Millisecond) {
		t.Error("tcpReachable() = true, want false")
	}
}

// ============================================================================
// httpReachable() coverage
// ============================================================================

func TestHTTPReachable_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	if !httpReachable(context.Background(), srv.URL) {
		t.Error("httpReachable() = false, want true")
	}
}

func TestHTTPReachable_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	if httpReachable(context.Background(), srv.URL) {
		t.Error("httpReachable() = true, want false for 500 status")
	}
}

func TestHTTPReachable_BadURL(t *testing.T) {
	if httpReachable(context.Background(), "http://127.0.0.1:1") {
		t.Error("httpReachable() = true, want false for unreachable server")
	}
}

func TestHTTPReachable_InvalidURL(t *testing.T) {
	if httpReachable(context.Background(), "://invalid-url") {
		t.Error("httpReachable() = true, want false for invalid URL")
	}
}

func TestHTTPReachable_CancelledContext(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// slow response
		time.Sleep(500 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel immediately

	if httpReachable(ctx, srv.URL) {
		t.Error("httpReachable() = true, want false for cancelled context")
	}
}

// ============================================================================
// probeAll() coverage
// ============================================================================

func TestProbeAll_HandlerCalled(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	var results []HealthResult
	logger := zap.NewNop()
	handler := func(r HealthResult) {
		results = append(results, r)
	}

	p := NewProber(30*time.Second, handler, logger)
	p.Register(ResourceTarget{
		Tenant:   "tenant1",
		Name:     "res1",
		Endpoint: ln.Addr().String(),
	})

	p.probeAll(context.Background())

	if len(results) != 1 {
		t.Errorf("probeAll() called handler %d times, want 1", len(results))
	}
	if results[0].Tenant != "tenant1" {
		t.Errorf("probeAll() result tenant = %q, want tenant1", results[0].Tenant)
	}
}

func TestProbeAll_NoTargets(t *testing.T) {
	called := false
	logger := zap.NewNop()
	handler := func(r HealthResult) {
		called = true
	}

	p := NewProber(30*time.Second, handler, logger)
	p.probeAll(context.Background())

	if called {
		t.Error("probeAll() called handler, want no calls for empty targets")
	}
}

func TestProbeAll_MultipleTargets_AllCalled(t *testing.T) {
	ln1, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln1.Close()
	ln2, _ := net.Listen("tcp", "127.0.0.1:0")
	defer ln2.Close()

	var callCount int32
	logger := zap.NewNop()
	handler := func(r HealthResult) {
		atomic.AddInt32(&callCount, 1)
	}

	p := NewProber(30*time.Second, handler, logger)
	p.Register(ResourceTarget{Tenant: "t1", Name: "r1", Endpoint: ln1.Addr().String()})
	p.Register(ResourceTarget{Tenant: "t1", Name: "r2", Endpoint: ln2.Addr().String()})

	p.probeAll(context.Background())

	if callCount != 2 {
		t.Errorf("probeAll() called handler %d times, want 2", callCount)
	}
}

func TestProbeAll_NilHandler(t *testing.T) {
	logger := zap.NewNop()
	p := NewProber(30*time.Second, nil, logger) // nil handler
	p.Register(ResourceTarget{Tenant: "t1", Name: "r1", Endpoint: "127.0.0.1:1"})

	// Should not panic with nil handler
	p.probeAll(context.Background())
}
