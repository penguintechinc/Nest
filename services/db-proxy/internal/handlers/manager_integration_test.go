package handlers

import (
	"context"
	"net"
	"testing"
	"time"

	"strconv"

	"golang.org/x/time/rate"

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

// TestManagerAcceptLoopMySQL is the MySQL analogue — covers authMySQL and the
// MySQL request/response relay through the accept loop.
func TestManagerAcceptLoopMySQL(t *testing.T) {
	logger := zap.NewNop()

	backend, err := NewFakeMySQLBackend("primary", "")
	if err != nil {
		t.Fatalf("fake mysql: %v", err)
	}
	defer backend.Close()

	router := routing.NewRouter(logger)
	pe := &routing.BackendEndpoint{Name: "primary", Host: "127.0.0.1", Port: backend.Port, Protocol: "mysql", MaxConns: 10, User: "root"}
	pe.Healthy.Store(true)
	if err := router.AddRoute(&routing.RouteConfig{ID: "mysql-route", Protocol: "mysql", Primary: pe, Tenant: "test"}); err != nil {
		t.Fatalf("AddRoute: %v", err)
	}

	p := pool.NewPool(10, logger)
	p.RegisterProtocol("mysql")
	cfg, _ := config.NewConfig(logger)
	port := freeTCPPort(t)
	handler := NewTCPHandlerWithCache("mysql", port, p, security.NewChecker(logger), cfg, logger, router, "mysql-route", &cachepkg.NoOpStore{}, 0)

	mgr := NewManager(logger)
	_ = mgr.AddHandler(handler)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := mgr.StartAll(ctx); err != nil {
		t.Fatalf("StartAll: %v", err)
	}
	defer func() { _ = mgr.StopAll() }()

	time.Sleep(100 * time.Millisecond)
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	if _, err := conn.Write(buildMySQLQuery("SELECT 1")); err != nil {
		t.Fatalf("write: %v", err)
	}
	_, _ = conn.Read(make([]byte, 1024))
	_ = conn.Close()
	time.Sleep(150 * time.Millisecond)
}

// TestHandlerBackendUnreachable covers the dial/auth failure path: the route
// points at a port with no backend, so getBackendConnection/dialAndAuthBackend
// error out and the proxy loop tears the client connection down.
func TestHandlerBackendUnreachable(t *testing.T) {
	logger := zap.NewNop()
	deadBackend := freeTCPPort(t) // nothing listening here

	router := routing.NewRouter(logger)
	pe := &routing.BackendEndpoint{Name: "primary", Host: "127.0.0.1", Port: deadBackend, Protocol: "postgresql", MaxConns: 10, User: "postgres"}
	pe.Healthy.Store(true)
	_ = router.AddRoute(&routing.RouteConfig{ID: "dead-route", Protocol: "postgresql", Primary: pe, Tenant: "test"})

	p := pool.NewPool(10, logger)
	p.RegisterProtocol("postgresql")
	cfg, _ := config.NewConfig(logger)
	port := freeTCPPort(t)
	handler := NewTCPHandlerWithCache("postgresql", port, p, security.NewChecker(logger), cfg, logger, router, "dead-route", &cachepkg.NoOpStore{}, 0)

	mgr := NewManager(logger)
	_ = mgr.AddHandler(handler)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = mgr.StartAll(ctx)
	defer func() { _ = mgr.StopAll() }()

	time.Sleep(100 * time.Millisecond)
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.SetDeadline(time.Now().Add(3 * time.Second))
	_, _ = conn.Write(buildPostgreSQLQuery("SELECT 1"))
	_, _ = conn.Read(make([]byte, 256)) // expected to fail/EOF as backend dial fails
	_ = conn.Close()
	time.Sleep(150 * time.Millisecond)
}

// TestManagerStartAll_ListenError covers Start's listen-failure branch (and the
// StartAll error propagation) by binding a handler to an already-occupied port.
func TestManagerStartAll_ListenError(t *testing.T) {
	logger := zap.NewNop()
	occupied, err := net.Listen("tcp", "0.0.0.0:0")
	if err != nil {
		t.Fatalf("occupy: %v", err)
	}
	defer occupied.Close()
	port := occupied.Addr().(*net.TCPAddr).Port

	cfg, _ := config.NewConfig(logger)
	handler := NewTCPHandlerWithCache("postgresql", port, pool.NewPool(1, logger), security.NewChecker(logger), cfg, logger, routing.NewRouter(logger), "r", &cachepkg.NoOpStore{}, 0)
	mgr := NewManager(logger)
	_ = mgr.AddHandler(handler)
	if err := mgr.StartAll(context.Background()); err == nil {
		t.Error("StartAll should fail when the port is already bound")
		_ = mgr.StopAll()
	}
}

// TestHandler_AcceptRateLimited covers the connection rate-limit branch of
// acceptConnections: a zero-rate limiter rejects and closes the accepted conn.
func TestHandler_AcceptRateLimited(t *testing.T) {
	logger := zap.NewNop()
	cfg, _ := config.NewConfig(logger)
	port := freeTCPPort(t)
	handler := NewTCPHandlerWithCache("postgresql", port, pool.NewPool(1, logger), security.NewChecker(logger), cfg, logger, routing.NewRouter(logger), "r", &cachepkg.NoOpStore{}, 0)
	handler.connLimiter = rate.NewLimiter(0, 0) // deny every connection

	mgr := NewManager(logger)
	_ = mgr.AddHandler(handler)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := mgr.StartAll(ctx); err != nil {
		t.Fatalf("StartAll: %v", err)
	}
	defer func() { _ = mgr.StopAll() }()

	time.Sleep(100 * time.Millisecond)
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), 2*time.Second)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	_ = conn.SetReadDeadline(time.Now().Add(2 * time.Second))
	// Rate-limited connections are closed immediately -> read returns EOF/error.
	if _, err := conn.Read(make([]byte, 8)); err == nil {
		t.Error("expected the rate-limited connection to be closed")
	}
	_ = conn.Close()
}

// TestHandlerSetCache covers the SetCache nil-guard and real-store paths.
func TestHandlerSetCache(t *testing.T) {
	logger := zap.NewNop()
	router := routing.NewRouter(logger)
	cfg, _ := config.NewConfig(logger)
	h := NewTCPHandlerWithCache("postgresql", 0, pool.NewPool(1, logger), security.NewChecker(logger), cfg, logger, router, "r", &cachepkg.NoOpStore{}, 0)

	h.SetCache(nil, time.Second)                     // nil -> falls back to NoOpStore
	h.SetCache(&cachepkg.NoOpStore{}, 5*time.Second) // explicit store
}
