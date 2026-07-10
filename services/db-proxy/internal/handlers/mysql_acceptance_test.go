package handlers

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/penguintechinc/nest/services/db-proxy/internal/routing"
	"github.com/penguintechinc/nest/services/db-proxy/internal/security"
	"go.uber.org/zap"
)

// TestMySQLReadWriteSplitting is the acceptance test that proves MySQL read/write splitting works end-to-end
func TestMySQLReadWriteSplitting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	logger := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup: Create two fake MySQL backends (primary and replica)
	primary, err := NewFakeMySQLBackend("primary", "")
	if err != nil {
		t.Fatalf("failed to create primary backend: %v", err)
	}
	defer primary.Close()

	replica, err := NewFakeMySQLBackend("replica", "")
	if err != nil {
		t.Fatalf("failed to create replica backend: %v", err)
	}
	defer replica.Close()

	// Setup: Create router with these backends
	router := routing.NewRouter(logger)

	primaryEndpoint := &routing.BackendEndpoint{
		Name:     "primary",
		Host:     "127.0.0.1",
		Port:     primary.Port,
		Protocol: "mysql",
		MaxConns: 10,
		User:     "root",
	}
	primaryEndpoint.Healthy.Store(true)

	replicaEndpoint := &routing.BackendEndpoint{
		Name:     "replica",
		Host:     "127.0.0.1",
		Port:     replica.Port,
		Protocol: "mysql",
		MaxConns: 10,
		User:     "root",
	}
	replicaEndpoint.Healthy.Store(true)

	route := &routing.RouteConfig{
		ID:       "test-mysql-route",
		Protocol: "mysql",
		Primary:  primaryEndpoint,
		Replicas: []*routing.BackendEndpoint{replicaEndpoint},
		Tenant:   "test",
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	// Setup: Create security checker
	checker := security.NewChecker(logger)
	checkerIface := &CheckerInterface{
		CheckQuery:       checker.CheckQuery,
		CheckParsedQuery: checker.CheckParsedQuery,
	}

	// Test Case 1: SELECT should route to replica
	t.Run("SELECT_routes_to_replica", func(t *testing.T) {
		// Clear previous data
		primary.mu.Lock()
		primary.ReceivedData = nil
		primary.mu.Unlock()
		replica.mu.Lock()
		replica.ReceivedData = nil
		replica.mu.Unlock()

		// Create a connection to the proxy
		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		if err := runMySQLProxyQueryTest(testCtx, t, router, checkerIface, logger, "SELECT * FROM users", primary, replica); err != nil {
			t.Fatalf("proxy test failed: %v", err)
		}

		// Verify: Query should be on replica
		replicaQueries := replica.GetReceivedQueries()
		primaryQueries := primary.GetReceivedQueries()

		if len(replicaQueries) == 0 {
			t.Errorf("SELECT query did not arrive at replica. Primary queries: %v, Replica queries: %v", primaryQueries, replicaQueries)
		} else {
			t.Logf("✓ SELECT correctly routed to replica. Query: %s", replicaQueries[0])
		}

		if len(primaryQueries) > 0 {
			t.Errorf("SELECT should not route to primary. Got: %v", primaryQueries)
		} else {
			t.Logf("✓ SELECT correctly bypassed primary")
		}
	})

	// Test Case 2: INSERT should route to primary
	t.Run("INSERT_routes_to_primary", func(t *testing.T) {
		// Clear previous data
		primary.mu.Lock()
		primary.ReceivedData = nil
		primary.mu.Unlock()
		replica.mu.Lock()
		replica.ReceivedData = nil
		replica.mu.Unlock()

		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		if err := runMySQLProxyQueryTest(testCtx, t, router, checkerIface, logger, "INSERT INTO users VALUES (1)", primary, replica); err != nil {
			t.Fatalf("proxy test failed: %v", err)
		}

		// Verify: Query should be on primary
		primaryQueries := primary.GetReceivedQueries()
		replicaQueries := replica.GetReceivedQueries()

		if len(primaryQueries) == 0 {
			t.Errorf("INSERT query did not arrive at primary. Primary queries: %v, Replica queries: %v", primaryQueries, replicaQueries)
		} else {
			t.Logf("✓ INSERT correctly routed to primary. Query: %s", primaryQueries[0])
		}

		if len(replicaQueries) > 0 {
			t.Errorf("INSERT should not route to replica. Got: %v", replicaQueries)
		} else {
			t.Logf("✓ INSERT correctly bypassed replica")
		}
	})

	// Test Case 3: BEGIN should route to primary (transaction start)
	t.Run("BEGIN_routes_to_primary", func(t *testing.T) {
		// Clear previous data
		primary.mu.Lock()
		primary.ReceivedData = nil
		primary.mu.Unlock()
		replica.mu.Lock()
		replica.ReceivedData = nil
		replica.mu.Unlock()

		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		if err := runMySQLProxyQueryTest(testCtx, t, router, checkerIface, logger, "BEGIN", primary, replica); err != nil {
			t.Fatalf("proxy test failed: %v", err)
		}

		// Verify: BEGIN should be on primary
		primaryQueries := primary.GetReceivedQueries()
		replicaQueries := replica.GetReceivedQueries()

		if len(primaryQueries) > 0 && contains(primaryQueries[0], "BEGIN") {
			t.Logf("✓ BEGIN correctly routed to primary")
		} else {
			t.Errorf("BEGIN did not arrive at primary. Primary: %v, Replica: %v", primaryQueries, replicaQueries)
		}

		if len(replicaQueries) > 0 {
			t.Errorf("BEGIN should not route to replica. Got: %v", replicaQueries)
		}
	})

	// Test Case 4: USE should route to primary (session-dirtying statement)
	t.Run("USE_routes_to_primary", func(t *testing.T) {
		// Clear previous data
		primary.mu.Lock()
		primary.ReceivedData = nil
		primary.mu.Unlock()
		replica.mu.Lock()
		replica.ReceivedData = nil
		replica.mu.Unlock()

		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		if err := runMySQLProxyQueryTest(testCtx, t, router, checkerIface, logger, "USE testdb", primary, replica); err != nil {
			t.Fatalf("proxy test failed: %v", err)
		}

		// Verify: USE should be on primary
		primaryQueries := primary.GetReceivedQueries()
		replicaQueries := replica.GetReceivedQueries()

		if len(primaryQueries) > 0 && contains(primaryQueries[0], "USE") {
			t.Logf("✓ USE correctly routed to primary (session-dirtying statement)")
		} else {
			t.Errorf("USE did not arrive at primary. Primary: %v, Replica: %v", primaryQueries, replicaQueries)
		}

		if len(replicaQueries) > 0 {
			t.Errorf("USE should not route to replica. Got: %v", replicaQueries)
		}
	})

	t.Logf("✓ All MySQL read/write splitting tests passed! Summary:")
	t.Logf("  - SELECT routes to replica ✓")
	t.Logf("  - INSERT routes to primary ✓")
	t.Logf("  - BEGIN (transaction start) routes to primary ✓")
	t.Logf("  - USE (session-dirtying) routes to primary ✓")
}

// TestMySQLMultiQuerySessionPinning proves that multi-query sequences on a single connection
// properly track session state and pin to primary when needed
func TestMySQLMultiQuerySessionPinning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	logger := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup: Create two fake MySQL backends (primary and replica)
	primary, err := NewFakeMySQLBackend("primary", "")
	if err != nil {
		t.Fatalf("failed to create primary backend: %v", err)
	}
	defer primary.Close()

	replica, err := NewFakeMySQLBackend("replica", "")
	if err != nil {
		t.Fatalf("failed to create replica backend: %v", err)
	}
	defer replica.Close()

	// Setup: Create router with these backends
	router := routing.NewRouter(logger)

	primaryEndpoint := &routing.BackendEndpoint{
		Name:     "primary",
		Host:     "127.0.0.1",
		Port:     primary.Port,
		Protocol: "mysql",
		MaxConns: 10,
		User:     "root",
	}
	primaryEndpoint.Healthy.Store(true)

	replicaEndpoint := &routing.BackendEndpoint{
		Name:     "replica",
		Host:     "127.0.0.1",
		Port:     replica.Port,
		Protocol: "mysql",
		MaxConns: 10,
		User:     "root",
	}
	replicaEndpoint.Healthy.Store(true)

	route := &routing.RouteConfig{
		ID:       "test-mysql-route",
		Protocol: "mysql",
		Primary:  primaryEndpoint,
		Replicas: []*routing.BackendEndpoint{replicaEndpoint},
		Tenant:   "test",
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	// Setup: Create security checker
	checker := security.NewChecker(logger)
	checkerIface := &CheckerInterface{
		CheckQuery:       checker.CheckQuery,
		CheckParsedQuery: checker.CheckParsedQuery,
	}

	// Test Case: Multi-query session pinning on ONE persistent connection
	t.Run("multi_query_session_pinning_on_persistent_connection", func(t *testing.T) {
		// Create a persistent pipe connection that we'll reuse for multiple queries
		clientConn, proxyConn := net.Pipe()

		// Start the proxy loop in a goroutine
		proxyLoop := NewProxyLoop(proxyConn, "mysql", router, "test-mysql-route", checkerIface, logger)

		done := make(chan error, 1)
		go func() {
			done <- proxyLoop.Run(ctx)
		}()

		// Start a goroutine to read responses (prevents deadlock on pipe)
		responsesDone := make(chan error, 1)
		go func() {
			defer close(responsesDone)
			responseBuf := make([]byte, 1024)
			for {
				clientConn.SetReadDeadline(time.Now().Add(500 * time.Millisecond))
				n, err := clientConn.Read(responseBuf)
				clientConn.SetReadDeadline(time.Time{})
				if err != nil {
					if err.Error() != "EOF" && err.Error() != "i/o timeout" {
						t.Logf("response read error: %v", err)
					}
					return
				}
				if n > 0 {
					t.Logf("received response: %d bytes", n)
				}
			}
		}()

		// Give the proxy a moment to start
		time.Sleep(50 * time.Millisecond)

		// Helper to send a query and verify it reaches the expected backend
		type queryStep struct {
			query         string
			expectReplica bool
			expectPrimary bool
		}

		steps := []queryStep{
			// Step (a): Clean SELECT should route to replica
			{"SELECT * FROM users", true, false},
			// Step (b): USE should route to primary and mark session as dirty
			{"USE testdb", false, true},
			// Step (c): After USE, subsequent SELECT should stay on primary (session is DIRTY)
			{"SELECT * FROM users", false, true},
			// Step (d): COMMIT should route to primary (dirty state persists across transactions)
			{"COMMIT", false, true},
			// Step (e): After COMMIT, SELECT should STAY on primary (session is still DIRTY - state is per-connection)
			{"SELECT * FROM users", false, true},
			// Step (f): BEGIN should route to primary (already dirty)
			{"BEGIN", false, true},
			// Step (g): SELECT inside transaction should route to primary
			{"SELECT * FROM users", false, true},
			// Step (h): COMMIT should route to primary
			{"COMMIT", false, true},
			// Step (i): After COMMIT, SELECT should STAY on primary (still dirty)
			{"SELECT * FROM users", false, true},
		}

		for stepIdx, step := range steps {
			// Clear backend state
			primary.mu.Lock()
			primary.ReceivedData = nil
			primary.mu.Unlock()
			replica.mu.Lock()
			replica.ReceivedData = nil
			replica.mu.Unlock()

			// Send query (no timeout since response reader is running)
			queryMsg := buildMySQLQuery(step.query)
			_, err := clientConn.Write(queryMsg)
			if err != nil {
				t.Fatalf("step %d: failed to send query: %v", stepIdx, err)
			}

			// Give backends time to process
			time.Sleep(100 * time.Millisecond)

			// Verify routing
			replicaQueries := replica.GetReceivedQueries()
			primaryQueries := primary.GetReceivedQueries()

			if step.expectReplica && len(replicaQueries) == 0 {
				t.Errorf("step %d (%q): expected query on replica, but got none. Primary queries: %v, Replica queries: %v",
					stepIdx, step.query, primaryQueries, replicaQueries)
			}
			if step.expectPrimary && len(primaryQueries) == 0 {
				t.Errorf("step %d (%q): expected query on primary, but got none. Primary queries: %v, Replica queries: %v",
					stepIdx, step.query, primaryQueries, replicaQueries)
			}
			if !step.expectReplica && len(replicaQueries) > 0 {
				t.Errorf("step %d (%q): query should NOT be on replica, but got: %v",
					stepIdx, step.query, replicaQueries)
			}
			if !step.expectPrimary && len(primaryQueries) > 0 {
				t.Errorf("step %d (%q): query should NOT be on primary, but got: %v",
					stepIdx, step.query, primaryQueries)
			}

			t.Logf("✓ step %d (%q): correctly routed (replica=%v, primary=%v)",
				stepIdx, step.query, len(replicaQueries) > 0, len(primaryQueries) > 0)
		}

		// Close client connection to trigger proxy shutdown
		clientConn.Close()

		// Wait for proxy to finish
		select {
		case err := <-done:
			if err != nil && err.Error() != "EOF" && err.Error() != "use of closed network connection" {
				t.Logf("proxy exited with error (expected): %v", err)
			}
		case <-time.After(3 * time.Second):
			t.Errorf("proxy did not exit in time")
		}

		// Wait for response reader to finish
		<-responsesDone

		t.Logf("✓ Multi-query session pinning test passed!")
	})
}

// runMySQLProxyQueryTest runs a single query through the proxy and verifies it arrives at the backend
func runMySQLProxyQueryTest(ctx context.Context, t *testing.T, router *routing.Router, checker *CheckerInterface,
	logger *zap.Logger, queryText string, primary, replica *FakeMySQLBackend) error {

	// Create a pipe to simulate the client connection
	clientConn, proxyConn := net.Pipe()
	defer clientConn.Close()
	defer proxyConn.Close()

	// Start the proxy loop in a goroutine
	proxyLoop := NewProxyLoop(proxyConn, "mysql", router, "test-mysql-route", checker, logger)

	done := make(chan error, 1)
	go func() {
		done <- proxyLoop.Run(ctx)
	}()

	// Give the proxy a moment to start
	time.Sleep(50 * time.Millisecond)

	// Send a MySQL Query message from the client
	queryMsg := buildMySQLQuery(queryText)
	_, err := clientConn.Write(queryMsg)
	if err != nil {
		return fmt.Errorf("failed to send query: %w", err)
	}

	// Read response (just make sure we get something back without error)
	responseBuf := make([]byte, 1024)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = clientConn.Read(responseBuf)
	if err != nil && err.Error() != "EOF" {
		return fmt.Errorf("failed to read response: %w", err)
	}

	// Close client connection to trigger proxy shutdown
	clientConn.Close()

	// Wait for proxy to finish
	select {
	case err := <-done:
		if err != nil && err.Error() != "EOF" && err.Error() != "use of closed network connection" {
			t.Logf("proxy exited with error (expected): %v", err)
		}
	case <-time.After(3 * time.Second):
		return fmt.Errorf("proxy did not exit in time")
	}

	// Give backends time to process
	time.Sleep(100 * time.Millisecond)

	return nil
}

// buildMySQLQuery constructs a MySQL COM_QUERY packet
// Format: 3-byte length + 1-byte sequence + 1-byte command (0x03) + query_string
func buildMySQLQuery(queryText string) []byte {
	payload := []byte{0x03} // COM_QUERY
	payload = append(payload, []byte(queryText)...)

	// Build packet with length and sequence number
	length := len(payload)
	packet := []byte{
		byte(length),
		byte(length >> 8),
		byte(length >> 16),
		0x00, // sequence number 0 (from client)
	}
	packet = append(packet, payload...)

	return packet
}
