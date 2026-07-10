package handlers

import (
	"context"
	"errors"
	"net"
	"testing"
	"time"

	cachepkg "github.com/penguintechinc/nest/services/db-proxy/internal/cache"
	protocolpkg "github.com/penguintechinc/nest/services/db-proxy/internal/protocol"
	"github.com/penguintechinc/nest/services/db-proxy/internal/routing"
	"go.uber.org/zap"
)

// failingCache implements cachepkg.Store; InvalidateByTable always errors so the
// best-effort error branch of invalidateCacheForWrite is exercised.
type failingCache struct{ *cachepkg.NoOpStore }

func (failingCache) InvalidateByTable(ctx context.Context, table string) error {
	return errors.New("invalidate boom")
}

func newRoutedLoop(t *testing.T, protocol string, route *routing.RouteConfig, cache cachepkg.Store) *ProxyLoop {
	t.Helper()
	r := routing.NewRouter(zap.NewNop())
	if err := r.AddRoute(route); err != nil {
		t.Fatalf("AddRoute: %v", err)
	}
	return NewProxyLoopWithCache(nil, protocol, r, route.ID, nil, zap.NewNop(), cache, time.Second)
}

func TestSelectBackend(t *testing.T) {
	primary := &routing.BackendEndpoint{Name: "primary", Host: "127.0.0.1", Port: 1, Protocol: "postgresql"}
	primary.Healthy.Store(true)
	route := &routing.RouteConfig{ID: "r", Protocol: "postgresql", Primary: primary, Tenant: "t"}
	pl := newRoutedLoop(t, "postgresql", route, &cachepkg.NoOpStore{})

	// Read query (no replicas) -> falls back to primary.
	read := &protocolpkg.ParsedQuery{QueryType: protocolpkg.QueryTypeSelect, QueryText: "SELECT 1"}
	if be, err := pl.selectBackend(read); err != nil || be == nil {
		t.Fatalf("selectBackend(read) = (%v, %v)", be, err)
	}

	// Unknown query -> active primary fallback.
	if be, err := pl.selectBackend(nil); err != nil || be == nil {
		t.Fatalf("selectBackend(nil) = (%v, %v)", be, err)
	}

	// Dirty session -> must use primary path.
	pl.session.MarkStateDirty()
	if be, err := pl.selectBackend(read); err != nil || be == nil {
		t.Fatalf("selectBackend(dirty) = (%v, %v)", be, err)
	}

	// Missing route -> error.
	pl.routeID = "nope"
	if _, err := pl.selectBackend(read); err == nil {
		t.Error("expected route-not-found error")
	}
}

func TestInvalidateCacheForWrite(t *testing.T) {
	primary := &routing.BackendEndpoint{Name: "primary", Host: "127.0.0.1", Port: 1, Protocol: "postgresql"}
	route := &routing.RouteConfig{ID: "r", Protocol: "postgresql", Primary: primary, Tenant: "t"}
	ctx := context.Background()

	// nil query -> no-op.
	newRoutedLoop(t, "postgresql", route, &cachepkg.NoOpStore{}).invalidateCacheForWrite(ctx, nil)

	// Non-write query -> no-op.
	pl := newRoutedLoop(t, "postgresql", route, &cachepkg.NoOpStore{})
	pl.invalidateCacheForWrite(ctx, &protocolpkg.ParsedQuery{QueryType: protocolpkg.QueryTypeSelect, QueryText: "SELECT 1"})

	// Write with an identifiable table, cache errors -> best-effort continue.
	plFail := newRoutedLoop(t, "postgresql", route, failingCache{&cachepkg.NoOpStore{}})
	plFail.invalidateCacheForWrite(ctx, &protocolpkg.ParsedQuery{QueryType: protocolpkg.QueryTypeInsert, QueryText: "INSERT INTO users VALUES (1)"})

	// Write, cache succeeds.
	plOK := newRoutedLoop(t, "postgresql", route, &cachepkg.NoOpStore{})
	plOK.invalidateCacheForWrite(ctx, &protocolpkg.ParsedQuery{QueryType: protocolpkg.QueryTypeUpdate, QueryText: "UPDATE users SET a=1"})
}

func TestGetBackendConnection_PrimaryAndReplicaReuse(t *testing.T) {
	backend, err := NewFakePostgreSQLBackend("primary")
	if err != nil {
		t.Fatalf("fake backend: %v", err)
	}
	defer backend.Close()

	primary := &routing.BackendEndpoint{Name: "primary", Host: "127.0.0.1", Port: backend.Port, Protocol: "postgresql", User: "postgres"}
	primary.Healthy.Store(true)
	route := &routing.RouteConfig{ID: "r", Protocol: "postgresql", Primary: primary, Tenant: "t"}
	pl := newRoutedLoop(t, "postgresql", route, &cachepkg.NoOpStore{})

	// Primary dial + auth, then reuse.
	c1, err := pl.getBackendConnection(primary)
	if err != nil {
		t.Fatalf("getBackendConnection(primary): %v", err)
	}
	c2, err := pl.getBackendConnection(primary)
	if err != nil || c2 != c1 {
		t.Errorf("primary connection not reused: c1=%p c2=%p err=%v", c1, c2, err)
	}

	// Replica dial + auth against the same fake backend, then reuse.
	replica := &routing.BackendEndpoint{Name: "replica", Host: "127.0.0.1", Port: backend.Port, Protocol: "postgresql", User: "postgres"}
	replica.Healthy.Store(true)
	r1, err := pl.getBackendConnection(replica)
	if err != nil {
		t.Fatalf("getBackendConnection(replica): %v", err)
	}
	r2, err := pl.getBackendConnection(replica)
	if err != nil || r2 != r1 {
		t.Errorf("replica connection not reused: r1=%p r2=%p err=%v", r1, r2, err)
	}
	pl.closeBackendConnections()
}

func TestGetBackendConnection_DialError(t *testing.T) {
	dead := freeTCPPort(t) // nothing listening
	primary := &routing.BackendEndpoint{Name: "primary", Host: "127.0.0.1", Port: dead, Protocol: "postgresql", User: "postgres"}
	route := &routing.RouteConfig{ID: "r", Protocol: "postgresql", Primary: primary, Tenant: "t"}
	pl := newRoutedLoop(t, "postgresql", route, &cachepkg.NoOpStore{})
	if _, err := pl.getBackendConnection(primary); err == nil {
		t.Error("expected dial error to unreachable primary")
	}

	// Replica dial error too.
	replica := &routing.BackendEndpoint{Name: "replica", Host: "127.0.0.1", Port: dead, Protocol: "postgresql"}
	if _, err := pl.getBackendConnection(replica); err == nil {
		t.Error("expected dial error to unreachable replica")
	}
}

func TestProxyLoopRun_ContextCancelled(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	r := routing.NewRouter(zap.NewNop())
	pl := NewProxyLoopWithCache(server, "postgresql", r, "r", &CheckerInterface{}, zap.NewNop(), nil, 0)

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // cancel before Run reads anything
	if err := pl.Run(ctx); !errors.Is(err, context.Canceled) {
		t.Errorf("Run(cancelled) = %v, want context.Canceled", err)
	}
}

func TestProxyLoopRun_BlockedQuery(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	r := routing.NewRouter(zap.NewNop())
	blocker := &CheckerInterface{
		CheckParsedQuery: func(q string) (bool, string) { return true, "blocked-by-policy" },
		CheckQuery:       func(q string) (bool, string) { return true, "blocked-by-policy" },
	}
	pl := NewProxyLoopWithCache(server, "postgresql", r, "r", blocker, zap.NewNop(), nil, 0)

	go func() { _, _ = client.Write(buildPostgreSQLQuery("SELECT secret FROM vault")) }()

	err := pl.Run(context.Background())
	if err == nil {
		t.Fatal("expected security-violation error from blocked query")
	}
	if pl.blockedQueries.Load() != 1 {
		t.Errorf("blockedQueries = %d, want 1", pl.blockedQueries.Load())
	}
}

func TestProxyLoopRun_ClientEOF(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	r := routing.NewRouter(zap.NewNop())
	pl := NewProxyLoopWithCache(server, "postgresql", r, "r", &CheckerInterface{}, zap.NewNop(), nil, 0)

	_ = client.Close() // immediate EOF
	if err := pl.Run(context.Background()); err != nil {
		t.Errorf("Run(client EOF) = %v, want nil", err)
	}
}

// acceptThenClose starts a listener that accepts one connection and immediately
// closes it, so any backend auth handshake fails mid-exchange. Returns the port.
func acceptThenClose(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	port := l.Addr().(*net.TCPAddr).Port
	go func() {
		defer l.Close()
		c, err := l.Accept()
		if err != nil {
			return
		}
		_ = c.Close()
	}()
	return port
}

func TestDialAndAuthBackend_AuthFailure(t *testing.T) {
	// PostgreSQL: startup succeeds to write, but the closed conn fails the handshake read.
	pgPort := acceptThenClose(t)
	pgEP := &routing.BackendEndpoint{Name: "primary", Host: "127.0.0.1", Port: pgPort, Protocol: "postgresql", User: "postgres"}
	pgEP.Healthy.Store(true)
	pgRoute := &routing.RouteConfig{ID: "pg", Protocol: "postgresql", Primary: pgEP, Tenant: "t"}
	pgLoop := newRoutedLoop(t, "postgresql", pgRoute, &cachepkg.NoOpStore{})
	if _, err := pgLoop.dialAndAuthBackend(pgEP); err == nil {
		t.Error("expected PostgreSQL auth failure against closed backend")
	}

	// MySQL: reading the server handshake fails against a closed conn.
	myPort := acceptThenClose(t)
	myEP := &routing.BackendEndpoint{Name: "primary", Host: "127.0.0.1", Port: myPort, Protocol: "mysql", User: "root"}
	myEP.Healthy.Store(true)
	myRoute := &routing.RouteConfig{ID: "my", Protocol: "mysql", Primary: myEP, Tenant: "t"}
	myLoop := newRoutedLoop(t, "mysql", myRoute, &cachepkg.NoOpStore{})
	if _, err := myLoop.dialAndAuthBackend(myEP); err == nil {
		t.Error("expected MySQL auth failure against closed backend")
	}
}

func TestExecuteMultiWrite(t *testing.T) {
	a, err := NewFakePostgreSQLBackend("primary")
	if err != nil {
		t.Fatalf("fake a: %v", err)
	}
	defer a.Close()
	b, err := NewFakePostgreSQLBackend("secondary")
	if err != nil {
		t.Fatalf("fake b: %v", err)
	}
	defer b.Close()

	epA := &routing.BackendEndpoint{Name: "primary", Host: "127.0.0.1", Port: a.Port, Protocol: "postgresql", User: "postgres"}
	epB := &routing.BackendEndpoint{Name: "secondary", Host: "127.0.0.1", Port: b.Port, Protocol: "postgresql", User: "postgres"}
	epA.Healthy.Store(true)
	epB.Healthy.Store(true)

	route := &routing.RouteConfig{
		ID: "mw", Protocol: "postgresql", Primary: epA, Tenant: "t",
		MultiWrite: &routing.MultiWriteConfig{
			Enabled:           true,
			ConsistencyPolicy: "best-effort",
			Targets: []*routing.MultiWriteTarget{
				{Endpoint: epA, Authoritative: true},
				{Endpoint: epB},
			},
		},
	}
	pl := newRoutedLoop(t, "postgresql", route, &cachepkg.NoOpStore{})

	req := buildPostgreSQLQuery("INSERT INTO t VALUES (1)")
	targets := []*routing.BackendEndpoint{epA, epB}
	pq := &protocolpkg.ParsedQuery{QueryType: protocolpkg.QueryTypeInsert, QueryText: "INSERT INTO t VALUES (1)"}

	resp, err := pl.executeMultiWrite(context.Background(), req, targets, epA, pq)
	if err != nil {
		t.Fatalf("executeMultiWrite: %v", err)
	}
	if len(resp) == 0 {
		t.Error("expected authoritative response frames")
	}

	// Secondary dead + strict policy -> consistency violation.
	dead := freeTCPPort(t)
	epDead := &routing.BackendEndpoint{Name: "secondary", Host: "127.0.0.1", Port: dead, Protocol: "postgresql"}
	epDead.Healthy.Store(true)
	route.MultiWrite.ConsistencyPolicy = "strict"
	route.MultiWrite.Targets[1].Endpoint = epDead
	pl2 := newRoutedLoop(t, "postgresql", route, &cachepkg.NoOpStore{})
	if _, err := pl2.executeMultiWrite(context.Background(), req, []*routing.BackendEndpoint{epA, epDead}, epA, pq); err == nil {
		t.Error("expected strict-consistency violation from dead secondary")
	}

	// Multi-write not configured -> error.
	plain := &routing.RouteConfig{ID: "plain", Protocol: "postgresql", Primary: epA, Tenant: "t"}
	pl3 := newRoutedLoop(t, "postgresql", plain, &cachepkg.NoOpStore{})
	if _, err := pl3.executeMultiWrite(context.Background(), req, targets, epA, pq); err == nil {
		t.Error("expected multi-write-not-configured error")
	}
}
