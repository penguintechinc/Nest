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

// TestPostgreSQLReadWriteSplitting is the acceptance test that proves read/write splitting works end-to-end
func TestPostgreSQLReadWriteSplitting(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	logger := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup: Create two fake PostgreSQL backends (primary and replica)
	primary, err := NewFakePostgreSQLBackend("primary")
	if err != nil {
		t.Fatalf("failed to create primary backend: %v", err)
	}
	defer primary.Close()

	replica, err := NewFakePostgreSQLBackend("replica")
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

		if err := runProxyQueryTest(testCtx, t, router, checkerIface, logger, "SELECT * FROM users", primary, replica); err != nil {
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

		if err := runProxyQueryTest(testCtx, t, router, checkerIface, logger, "INSERT INTO users VALUES (1)", primary, replica); err != nil {
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

		if err := runProxyQueryTest(testCtx, t, router, checkerIface, logger, "BEGIN", primary, replica); err != nil {
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

	// Test Case 4: SET should route to primary (session-dirtying statement)
	t.Run("SET_routes_to_primary", func(t *testing.T) {
		// Clear previous data
		primary.mu.Lock()
		primary.ReceivedData = nil
		primary.mu.Unlock()
		replica.mu.Lock()
		replica.ReceivedData = nil
		replica.mu.Unlock()

		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		if err := runProxyQueryTest(testCtx, t, router, checkerIface, logger, "SET search_path TO public", primary, replica); err != nil {
			t.Fatalf("proxy test failed: %v", err)
		}

		// Verify: SET should be on primary
		primaryQueries := primary.GetReceivedQueries()
		replicaQueries := replica.GetReceivedQueries()

		if len(primaryQueries) > 0 && contains(primaryQueries[0], "SET") {
			t.Logf("✓ SET correctly routed to primary (session-dirtying statement)")
		} else {
			t.Errorf("SET did not arrive at primary. Primary: %v, Replica: %v", primaryQueries, replicaQueries)
		}

		if len(replicaQueries) > 0 {
			t.Errorf("SET should not route to replica. Got: %v", replicaQueries)
		}
	})

	t.Logf("✓ All PostgreSQL read/write splitting tests passed! Summary:")
	t.Logf("  - SELECT routes to replica ✓")
	t.Logf("  - INSERT routes to primary ✓")
	t.Logf("  - BEGIN (transaction start) routes to primary ✓")
	t.Logf("  - SET (session-dirtying) routes to primary ✓")
}

// TestMultiQuerySessionPinning proves that multi-query sequences on a single connection
// properly track session state and pin to primary when needed
func TestMultiQuerySessionPinning(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	logger := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Setup: Create two fake PostgreSQL backends (primary and replica)
	primary, err := NewFakePostgreSQLBackend("primary")
	if err != nil {
		t.Fatalf("failed to create primary backend: %v", err)
	}
	defer primary.Close()

	replica, err := NewFakePostgreSQLBackend("replica")
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
		proxyLoop := NewProxyLoop(proxyConn, "postgresql", router, "test-pg-route", checkerIface, logger)

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
			// Step (b): SET should route to primary and mark session as dirty
			{"SET search_path TO public", false, true},
			// Step (c): After SET, subsequent SELECT should stay on primary (session is DIRTY)
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
			queryMsg := buildPostgreSQLQuery(step.query)
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

	t.Run("separate_connection_isolation", func(t *testing.T) {
		// Prove that dirty state is PER-CONNECTION, not global

		// First connection: execute SET (dirtying statement)
		clientConn1, proxyConn1 := net.Pipe()
		proxyLoop1 := NewProxyLoop(proxyConn1, "postgresql", router, "test-pg-route", checkerIface, logger)

		done1 := make(chan error, 1)
		go func() {
			done1 <- proxyLoop1.Run(ctx)
		}()
		time.Sleep(50 * time.Millisecond)

		// Send SET (dirtying)
		primary.mu.Lock()
		primary.ReceivedData = nil
		primary.mu.Unlock()
		replica.mu.Lock()
		replica.ReceivedData = nil
		replica.mu.Unlock()

		setMsg := buildPostgreSQLQuery("SET search_path TO public")
		clientConn1.SetWriteDeadline(time.Now().Add(2 * time.Second))
		clientConn1.Write(setMsg)
		clientConn1.SetWriteDeadline(time.Time{})

		responseBuf := make([]byte, 1024)
		clientConn1.SetReadDeadline(time.Now().Add(2 * time.Second))
		clientConn1.Read(responseBuf)
		clientConn1.SetReadDeadline(time.Time{})

		time.Sleep(50 * time.Millisecond)

		if len(primary.GetReceivedQueries()) == 0 {
			t.Errorf("connection 1: SET should route to primary")
		}

		// Second connection: clean SELECT should still go to replica (not affected by connection 1's dirty state)
		clientConn2, proxyConn2 := net.Pipe()
		proxyLoop2 := NewProxyLoop(proxyConn2, "postgresql", router, "test-pg-route", checkerIface, logger)

		done2 := make(chan error, 1)
		go func() {
			done2 <- proxyLoop2.Run(ctx)
		}()
		time.Sleep(50 * time.Millisecond)

		primary.mu.Lock()
		primary.ReceivedData = nil
		primary.mu.Unlock()
		replica.mu.Lock()
		replica.ReceivedData = nil
		replica.mu.Unlock()

		selectMsg := buildPostgreSQLQuery("SELECT * FROM users")
		clientConn2.SetWriteDeadline(time.Now().Add(2 * time.Second))
		clientConn2.Write(selectMsg)
		clientConn2.SetWriteDeadline(time.Time{})

		responseBuf = make([]byte, 1024)
		clientConn2.SetReadDeadline(time.Now().Add(2 * time.Second))
		clientConn2.Read(responseBuf)
		clientConn2.SetReadDeadline(time.Time{})

		time.Sleep(50 * time.Millisecond)

		if len(replica.GetReceivedQueries()) == 0 {
			t.Errorf("connection 2: SELECT should route to replica (dirty state is per-connection, not global)")
		}

		// Cleanup
		clientConn1.Close()
		clientConn2.Close()

		select {
		case <-done1:
		case <-time.After(3 * time.Second):
			t.Logf("connection 1 proxy did not exit in time")
		}

		select {
		case <-done2:
		case <-time.After(3 * time.Second):
			t.Logf("connection 2 proxy did not exit in time")
		}

		t.Logf("✓ Separate connection isolation test passed!")
	})
}

// runProxyQueryTest runs a single query through the proxy and verifies it arrives at the backend
func runProxyQueryTest(ctx context.Context, t *testing.T, router *routing.Router, checker *CheckerInterface,
	logger *zap.Logger, queryText string, primary, replica *FakePostgreSQLBackend) error {

	// Create a pipe to simulate the client connection
	clientConn, proxyConn := net.Pipe()
	defer clientConn.Close()
	defer proxyConn.Close()

	// Start the proxy loop in a goroutine
	proxyLoop := NewProxyLoop(proxyConn, "postgresql", router, "test-pg-route", checker, logger)

	done := make(chan error, 1)
	go func() {
		done <- proxyLoop.Run(ctx)
	}()

	// Give the proxy a moment to start
	time.Sleep(50 * time.Millisecond)

	// Send a PostgreSQL Query message from the client
	queryMsg := buildPostgreSQLQuery(queryText)
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

// buildPostgreSQLQuery constructs a PostgreSQL Query message
// Format: 'Q' + length (4 bytes, big-endian) + query_string (null-terminated)
func buildPostgreSQLQuery(queryText string) []byte {
	msg := []byte{'Q'}

	// Build body
	body := []byte(queryText)
	body = append(body, 0) // null terminator

	// Calculate length (includes 4-byte length field)
	length := len(body) + 4
	msg = append(msg, byte(length>>24), byte(length>>16), byte(length>>8), byte(length))
	msg = append(msg, body...)

	return msg
}

// contains checks if a string contains a substring (case-insensitive)
func contains(s, substr string) bool {
	for i := 0; i < len(s); i++ {
		if i+len(substr) <= len(s) {
			match := true
			for j := 0; j < len(substr); j++ {
				if toLower(s[i+j]) != toLower(substr[j]) {
					match = false
					break
				}
			}
			if match {
				return true
			}
		}
	}
	return false
}

func toLower(b byte) byte {
	if b >= 'A' && b <= 'Z' {
		return b + ('a' - 'A')
	}
	return b
}
