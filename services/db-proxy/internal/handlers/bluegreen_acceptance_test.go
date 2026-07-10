package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/penguintechinc/nest/services/db-proxy/internal/routing"
	"github.com/penguintechinc/nest/services/db-proxy/internal/security"
	"go.uber.org/zap"
)

// TestBlueGreenPrimarySwitchingDataPath proves blue/green switching works end-to-end
// with real query delivery to the correct backend
func TestBlueGreenPrimarySwitchingDataPath(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	logger := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup: Create fake blue and green primary backends
	bluePrimary, err := NewFakePostgreSQLBackend("blue-primary")
	if err != nil {
		t.Fatalf("failed to create blue primary: %v", err)
	}
	defer bluePrimary.Close()

	greenPrimary, err := NewFakePostgreSQLBackend("green-primary")
	if err != nil {
		t.Fatalf("failed to create green primary: %v", err)
	}
	defer greenPrimary.Close()

	// Setup: Create router with blue/green config (active=blue)
	router := routing.NewRouter(logger)

	blueEndpoint := &routing.BackendEndpoint{
		Name:     "blue",
		Host:     "127.0.0.1",
		Port:     bluePrimary.Port,
		Protocol: "postgresql",
		MaxConns: 10,
		User:     "postgres",
	}
	blueEndpoint.Healthy.Store(true)

	greenEndpoint := &routing.BackendEndpoint{
		Name:     "green",
		Host:     "127.0.0.1",
		Port:     greenPrimary.Port,
		Protocol: "postgresql",
		MaxConns: 10,
		User:     "postgres",
	}
	greenEndpoint.Healthy.Store(true)

	blueGreenConfig := &routing.BlueGreenConfig{
		Blue:   blueEndpoint,
		Green:  greenEndpoint,
		Active: "blue",
	}

	route := &routing.RouteConfig{
		ID:        "test-pg-route",
		Protocol:  "postgresql",
		Primary:   blueEndpoint, // fallback
		Replicas:  []*routing.BackendEndpoint{},
		Tenant:    "test",
		BlueGreen: blueGreenConfig,
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	checker := security.NewChecker(logger)
	checkerIface := &CheckerInterface{
		CheckQuery:       checker.CheckQuery,
		CheckParsedQuery: checker.CheckParsedQuery,
	}

	// TEST 1: Blue is active - INSERT should go to blue
	t.Run("blue_active_write_to_blue", func(t *testing.T) {
		bluePrimary.mu.Lock()
		bluePrimary.ReceivedData = nil
		bluePrimary.mu.Unlock()
		greenPrimary.mu.Lock()
		greenPrimary.ReceivedData = nil
		greenPrimary.mu.Unlock()

		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		if err := runProxyQueryTest(testCtx, t, router, checkerIface, logger, "INSERT INTO test_table VALUES (1)", bluePrimary, nil); err != nil {
			t.Fatalf("proxy test failed: %v", err)
		}

		blueQueries := bluePrimary.GetReceivedQueries()
		greenQueries := greenPrimary.GetReceivedQueries()

		if len(blueQueries) == 0 {
			t.Fatalf("expected INSERT to reach blue primary, got none. Blue: %v, Green: %v", blueQueries, greenQueries)
		}
		if len(greenQueries) > 0 {
			t.Fatalf("INSERT should not reach green when blue is active. Green queries: %v", greenQueries)
		}
		t.Logf("✓ Blue active: INSERT correctly routed to blue primary. Query: %s", blueQueries[0])
	})

	// TEST 2: Flip to green - next INSERT should go to green
	// NOTE: Each query test creates a fresh ProxyLoop (new client connection)
	// so it picks up the updated active color
	t.Run("flip_to_green_write_to_green", func(t *testing.T) {
		// Flip active color
		if err := route.SetActiveColor("green"); err != nil {
			t.Fatalf("failed to flip to green: %v", err)
		}

		bluePrimary.mu.Lock()
		bluePrimary.ReceivedData = nil
		bluePrimary.mu.Unlock()
		greenPrimary.mu.Lock()
		greenPrimary.ReceivedData = nil
		greenPrimary.mu.Unlock()

		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		// Create a fresh client connection (new ProxyLoop) which will read the new active color
		if err := runProxyQueryTest(testCtx, t, router, checkerIface, logger, "INSERT INTO test_table VALUES (2)", greenPrimary, nil); err != nil {
			t.Fatalf("proxy test failed: %v", err)
		}

		blueQueries := bluePrimary.GetReceivedQueries()
		greenQueries := greenPrimary.GetReceivedQueries()

		if len(greenQueries) == 0 {
			t.Fatalf("expected INSERT to reach green primary after flip, got none. Blue: %v, Green: %v", blueQueries, greenQueries)
		}
		if len(blueQueries) > 0 {
			t.Fatalf("INSERT should not reach blue after flip to green. Blue queries: %v", blueQueries)
		}
		t.Logf("✓ Green active: INSERT correctly routed to green primary. Query: %s", greenQueries[0])
	})

	// TEST 3: Flip back to blue - verify blue receives again
	t.Run("flip_back_to_blue", func(t *testing.T) {
		if err := route.SetActiveColor("blue"); err != nil {
			t.Fatalf("failed to flip back to blue: %v", err)
		}

		bluePrimary.mu.Lock()
		bluePrimary.ReceivedData = nil
		bluePrimary.mu.Unlock()
		greenPrimary.mu.Lock()
		greenPrimary.ReceivedData = nil
		greenPrimary.mu.Unlock()

		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		if err := runProxyQueryTest(testCtx, t, router, checkerIface, logger, "INSERT INTO test_table VALUES (3)", bluePrimary, nil); err != nil {
			t.Fatalf("proxy test failed: %v", err)
		}

		blueQueries := bluePrimary.GetReceivedQueries()
		greenQueries := greenPrimary.GetReceivedQueries()

		if len(blueQueries) == 0 {
			t.Fatalf("expected INSERT to reach blue after flip back, got none. Blue: %v, Green: %v", blueQueries, greenQueries)
		}
		if len(greenQueries) > 0 {
			t.Fatalf("INSERT should not reach green after flip back to blue. Green queries: %v", greenQueries)
		}
		t.Logf("✓ Blue active again: INSERT correctly routed to blue. Query: %s", blueQueries[0])
	})
}

// TestBlueGreenRegressionSinglePrimary proves legacy single-primary routing still works
func TestBlueGreenRegressionSinglePrimary(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	logger := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup: Create fake primary (no blue/green)
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

	// Setup: Create router WITHOUT blue/green (legacy single primary)
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
		// NO BlueGreen config
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	checker := security.NewChecker(logger)
	checkerIface := &CheckerInterface{
		CheckQuery:       checker.CheckQuery,
		CheckParsedQuery: checker.CheckParsedQuery,
	}

	// INSERT should go to primary
	t.Run("insert_to_primary", func(t *testing.T) {
		primary.mu.Lock()
		primary.ReceivedData = nil
		primary.mu.Unlock()

		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		if err := runProxyQueryTest(testCtx, t, router, checkerIface, logger, "INSERT INTO users VALUES (1)", primary, nil); err != nil {
			t.Fatalf("proxy test failed: %v", err)
		}

		primaryQueries := primary.GetReceivedQueries()
		if len(primaryQueries) == 0 {
			t.Fatalf("expected INSERT to reach primary in legacy mode")
		}
		t.Logf("✓ Legacy mode: INSERT to primary works. Query: %s", primaryQueries[0])
	})

	// SELECT should go to replica
	t.Run("select_to_replica", func(t *testing.T) {
		replica.mu.Lock()
		replica.ReceivedData = nil
		replica.mu.Unlock()
		primary.mu.Lock()
		primary.ReceivedData = nil
		primary.mu.Unlock()

		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		if err := runProxyQueryTest(testCtx, t, router, checkerIface, logger, "SELECT * FROM users", primary, nil); err != nil {
			t.Fatalf("proxy test failed: %v", err)
		}

		replicaQueries := replica.GetReceivedQueries()
		if len(replicaQueries) == 0 {
			t.Fatalf("expected SELECT to reach replica in legacy mode")
		}
		t.Logf("✓ Legacy mode: SELECT to replica works. Query: %s", replicaQueries[0])
	})
}
