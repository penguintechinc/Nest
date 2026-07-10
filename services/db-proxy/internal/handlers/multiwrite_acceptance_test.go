package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/penguintechinc/nest/services/db-proxy/internal/routing"
	"github.com/penguintechinc/nest/services/db-proxy/internal/security"
	"go.uber.org/zap"
)

// TestMultiWriteDuplicationDataPath proves multi-write duplicates writes to all targets
// and returns the authoritative response
func TestMultiWriteDuplicationDataPath(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	logger := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup: Create two fake primary backends (write targets)
	primary1, err := NewFakePostgreSQLBackend("primary-1")
	if err != nil {
		t.Fatalf("failed to create primary-1: %v", err)
	}
	defer primary1.Close()

	primary2, err := NewFakePostgreSQLBackend("primary-2")
	if err != nil {
		t.Fatalf("failed to create primary-2: %v", err)
	}
	defer primary2.Close()

	// Setup: Create router with multi-write enabled (best-effort)
	router := routing.NewRouter(logger)

	endpoint1 := &routing.BackendEndpoint{
		Name:          "primary-1",
		Host:          "127.0.0.1",
		Port:          primary1.Port,
		Protocol:      "postgresql",
		MaxConns:      10,
		User:          "postgres",
		Authoritative: true,
	}
	endpoint1.Healthy.Store(true)

	endpoint2 := &routing.BackendEndpoint{
		Name:          "primary-2",
		Host:          "127.0.0.1",
		Port:          primary2.Port,
		Protocol:      "postgresql",
		MaxConns:      10,
		User:          "postgres",
		Authoritative: false,
	}
	endpoint2.Healthy.Store(true)

	multiWriteConfig := &routing.MultiWriteConfig{
		Enabled:           true,
		ConsistencyPolicy: "best-effort",
		Targets: []*routing.MultiWriteTarget{
			{
				Endpoint:      endpoint1,
				Authoritative: true,
			},
			{
				Endpoint:      endpoint2,
				Authoritative: false,
			},
		},
	}

	route := &routing.RouteConfig{
		ID:         "test-pg-route",
		Protocol:   "postgresql",
		Primary:    endpoint1,
		Replicas:   []*routing.BackendEndpoint{},
		Tenant:     "test",
		MultiWrite: multiWriteConfig,
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	checker := security.NewChecker(logger)
	checkerIface := &CheckerInterface{
		CheckQuery:       checker.CheckQuery,
		CheckParsedQuery: checker.CheckParsedQuery,
	}

	// TEST: Single INSERT should be duplicated to both targets
	t.Run("multi_write_duplication", func(t *testing.T) {
		primary1.mu.Lock()
		primary1.ReceivedData = nil
		primary1.mu.Unlock()
		primary2.mu.Lock()
		primary2.ReceivedData = nil
		primary2.mu.Unlock()

		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		if err := runProxyQueryTest(testCtx, t, router, checkerIface, logger, "INSERT INTO users VALUES (1)", primary1, nil); err != nil {
			t.Fatalf("proxy test failed: %v", err)
		}

		p1Queries := primary1.GetReceivedQueries()
		p2Queries := primary2.GetReceivedQueries()

		if len(p1Queries) == 0 {
			t.Fatalf("expected INSERT to reach primary-1, got none. P1: %v, P2: %v", p1Queries, p2Queries)
		}
		if len(p2Queries) == 0 {
			t.Fatalf("expected INSERT to reach primary-2 (duplication), got none. P1: %v, P2: %v", p1Queries, p2Queries)
		}
		if len(p1Queries) != 1 || len(p2Queries) != 1 {
			t.Fatalf("expected exactly 1 query on each target. P1: %d, P2: %d", len(p1Queries), len(p2Queries))
		}

		// Verify they received the same query
		if p1Queries[0] != p2Queries[0] {
			t.Fatalf("queries should be identical. P1: %s, P2: %s", p1Queries[0], p2Queries[0])
		}

		t.Logf("✓ Multi-write duplication: both targets received query '%s'", p1Queries[0])
	})

	// TEST: Client should get valid response (from authoritative target)
	t.Run("multi_write_client_response", func(t *testing.T) {
		primary1.mu.Lock()
		primary1.ReceivedData = nil
		primary1.mu.Unlock()
		primary2.mu.Lock()
		primary2.ReceivedData = nil
		primary2.mu.Unlock()

		// Client should receive a valid response without error
		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		err := runProxyQueryTest(testCtx, t, router, checkerIface, logger, "INSERT INTO users VALUES (2)", primary1, nil)
		if err != nil {
			t.Fatalf("proxy test failed (should succeed in best-effort): %v", err)
		}

		p1Queries := primary1.GetReceivedQueries()
		p2Queries := primary2.GetReceivedQueries()

		if len(p1Queries) == 0 || len(p2Queries) == 0 {
			t.Fatalf("both targets should have received query. P1: %d, P2: %d", len(p1Queries), len(p2Queries))
		}

		t.Logf("✓ Multi-write: client received response successfully")
	})
}

// TestMultiWriteRegressionSingleTarget proves single-target writes still work
func TestMultiWriteRegressionSingleTarget(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	logger := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup: Create single primary (no multi-write)
	primary, err := NewFakePostgreSQLBackend("primary")
	if err != nil {
		t.Fatalf("failed to create primary: %v", err)
	}
	defer primary.Close()

	replica, err := NewFakePostgreSQLBackend("replica")
	if err != nil {
		t.Fatalf("failed to create replica: %v", err)
	}
	defer replica.Close()

	// Setup: Create router WITHOUT multi-write (legacy single target)
	router := routing.NewRouter(logger)

	primaryEndpoint := &routing.BackendEndpoint{
		Name:     "primary",
		Host:     "127.0.0.1",
		Port:     primary.Port,
		Protocol: "postgresql",
		MaxConns: 10,
		User:     "postgres",
	}
	primaryEndpoint.Healthy.Store(true)

	replicaEndpoint := &routing.BackendEndpoint{
		Name:     "replica",
		Host:     "127.0.0.1",
		Port:     replica.Port,
		Protocol: "postgresql",
		MaxConns: 10,
		User:     "postgres",
	}
	replicaEndpoint.Healthy.Store(true)

	route := &routing.RouteConfig{
		ID:       "test-pg-route",
		Protocol: "postgresql",
		Primary:  primaryEndpoint,
		Replicas: []*routing.BackendEndpoint{replicaEndpoint},
		Tenant:   "test",
		// NO MultiWrite config
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	checker := security.NewChecker(logger)
	checkerIface := &CheckerInterface{
		CheckQuery:       checker.CheckQuery,
		CheckParsedQuery: checker.CheckParsedQuery,
	}

	// TEST: INSERT should go only to primary (no duplication)
	t.Run("single_target_write", func(t *testing.T) {
		primary.mu.Lock()
		primary.ReceivedData = nil
		primary.mu.Unlock()
		replica.mu.Lock()
		replica.ReceivedData = nil
		replica.mu.Unlock()

		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		if err := runProxyQueryTest(testCtx, t, router, checkerIface, logger, "INSERT INTO users VALUES (1)", primary, nil); err != nil {
			t.Fatalf("proxy test failed: %v", err)
		}

		primaryQueries := primary.GetReceivedQueries()
		replicaQueries := replica.GetReceivedQueries()

		if len(primaryQueries) == 0 {
			t.Fatalf("expected INSERT to reach primary")
		}
		if len(replicaQueries) > 0 {
			t.Fatalf("INSERT should not reach replica in single-target mode")
		}

		t.Logf("✓ Single-target write: INSERT correctly sent only to primary. Query: %s", primaryQueries[0])
	})

	// TEST: SELECT should go to replica
	t.Run("single_target_read", func(t *testing.T) {
		primary.mu.Lock()
		primary.ReceivedData = nil
		primary.mu.Unlock()
		replica.mu.Lock()
		replica.ReceivedData = nil
		replica.mu.Unlock()

		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		if err := runProxyQueryTest(testCtx, t, router, checkerIface, logger, "SELECT * FROM users", primary, nil); err != nil {
			t.Fatalf("proxy test failed: %v", err)
		}

		primaryQueries := primary.GetReceivedQueries()
		replicaQueries := replica.GetReceivedQueries()

		if len(replicaQueries) == 0 {
			t.Fatalf("expected SELECT to reach replica")
		}
		if len(primaryQueries) > 0 {
			t.Fatalf("SELECT should not route to primary when replica available")
		}

		t.Logf("✓ Single-target read: SELECT correctly routed to replica. Query: %s", replicaQueries[0])
	})
}

// TestMultiWriteStrictConsistencyConfiguration verifies strict mode is properly configured
func TestMultiWriteStrictConsistencyConfiguration(t *testing.T) {
	endpoint1 := &routing.BackendEndpoint{
		Name:          "primary-1",
		Host:          "127.0.0.1",
		Port:          3306,
		Protocol:      "postgresql",
		Authoritative: true,
	}

	endpoint2 := &routing.BackendEndpoint{
		Name:          "primary-2",
		Host:          "127.0.0.1",
		Port:          3307,
		Protocol:      "postgresql",
		Authoritative: false,
	}

	// Test strict mode configuration
	multiWriteConfig := &routing.MultiWriteConfig{
		Enabled:           true,
		ConsistencyPolicy: "strict",
		Targets: []*routing.MultiWriteTarget{
			{
				Endpoint:      endpoint1,
				Authoritative: true,
			},
			{
				Endpoint:      endpoint2,
				Authoritative: false,
			},
		},
	}

	route := &routing.RouteConfig{
		ID:         "test-pg-route",
		Protocol:   "postgresql",
		Primary:    endpoint1,
		MultiWrite: multiWriteConfig,
	}

	// Verify consistency policy is strict
	if route.MultiWrite.ConsistencyPolicy != "strict" {
		t.Fatalf("expected strict policy, got %s", route.MultiWrite.ConsistencyPolicy)
	}

	// Verify write targets
	targets := route.GetWriteTargets()
	if len(targets) != 2 {
		t.Fatalf("expected 2 write targets, got %d", len(targets))
	}

	// Verify authoritative target
	authTarget := route.GetAuthoritativeWriteTarget()
	if authTarget.Name != "primary-1" {
		t.Fatalf("expected authoritative target primary-1, got %s", authTarget.Name)
	}

	t.Logf("✓ Multi-write strict mode configuration verified")
}
