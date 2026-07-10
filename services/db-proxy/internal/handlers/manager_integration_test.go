package handlers

import (
	"context"
	"net"
	"testing"
	"time"

	"strconv"

	cachepkg "github.com/penguintechinc/nest/services/db-proxy/internal/cache"
	"github.com/penguintechinc/nest/services/db-proxy/internal/config"
	"github.com/penguintechinc/nest/services/db-proxy/internal/pool"
	"github.com/penguintechinc/nest/services/db-proxy/internal/routing"
	"github.com/penguintechinc/nest/services/db-proxy/internal/security"
	"go.uber.org/zap"
)

// freeTCPPort returns a port with nothing bound to it.
func freeTCPPort(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	p := l.Addr().(*net.TCPAddr).Port
	_ = l.Close()
	return p
}

// TestManagerAcceptLoop drives the full TCP-handler path through the Manager:
// StartAll binds a listener whose acceptConnections -> handleConnection proxies a
// real client query to a fake backend, then StopAll tears it down. This covers
// the Manager lifecycle + accept loop that the acceptance tests (which call
// NewProxyLoop directly) bypass.
func TestManagerAcceptLoop(t *testing.T) {
	logger := zap.NewNop()

	primary, err := NewFakePostgreSQLBackend("primary")
	if err != nil {
		t.Fatalf("fake primary: %v", err)
	}
	defer primary.Close()

	router := routing.NewRouter(logger)
	pe := &routing.BackendEndpoint{Name: "primary", Host: "127.0.0.1", Port: primary.Port, Protocol: "postgresql", MaxConns: 10, User: "postgres"}
	pe.Healthy.Store(true)
	route := &routing.RouteConfig{ID: "mgr-route", Protocol: "postgresql", Primary: pe, Tenant: "test"}
	if err := router.AddRoute(route); err != nil {
		t.Fatalf("AddRoute: %v", err)
	}

	p := pool.NewPool(10, logger)
	p.RegisterProtocol("postgresql")
	checker := security.NewChecker(logger)
	cfg, err := config.NewConfig(logger)
	if err != nil {
		t.Fatalf("config: %v", err)
	}

	port := freeTCPPort(t)
	handler := NewTCPHandlerWithCache("postgresql", port, p, checker, cfg, logger, router, "mgr-route", &cachepkg.NoOpStore{}, 0)

	mgr := NewManager(logger)
	if err := mgr.AddHandler(handler); err != nil {
		t.Fatalf("AddHandler: %v", err)
	}
	// Duplicate protocol -> error.
	if err := mgr.AddHandler(handler); err == nil {
		t.Error("AddHandler duplicate protocol should error")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mgr.StartAll(ctx); err != nil {
		t.Fatalf("StartAll: %v", err)
	}
	defer func() { _ = mgr.StopAll() }()

	if got := mgr.GetHandlers(); len(got) != 1 {
		t.Errorf("GetHandlers len = %d, want 1", len(got))
	}
	if stats := mgr.GetStats(); stats["postgresql"] == nil {
		t.Error("GetStats missing postgresql handler")
	}

	// Connect a client to the handler and send a query -> exercises
	// acceptConnections -> handleConnection -> proxyLoop.Run -> Stats.
	time.Sleep(100 * time.Millisecond) // let the accept goroutine come up
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 2*time.Second)
	if err != nil {
		t.Fatalf("dial proxy: %v", err)
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write(buildPostgreSQLQuery("SELECT * FROM users")); err != nil {
		t.Fatalf("write query: %v", err)
	}
	_, _ = conn.Read(make([]byte, 1024)) // best-effort; relay path is what we're covering
	_ = conn.Close()

	// Let handleConnection finish and update stats.
	time.Sleep(150 * time.Millisecond)

	if err := mgr.StopAll(); err != nil {
		t.Errorf("StopAll: %v", err)
	}
}
