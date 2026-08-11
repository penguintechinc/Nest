package routing

import (
	"testing"

	"github.com/penguintechinc/nest/services/db-proxy/internal/protocol"
	"go.uber.org/zap"
)

// TestReadWriteRouting proves that SELECTs route to replicas and INSERTs route to primary
func TestReadWriteRouting(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger)

	// Setup: Create a route with one primary and two replicas
	primary := &BackendEndpoint{
		Name:     "primary",
		Host:     "db-primary.internal",
		Port:     3306,
		Protocol: "mysql",
		MaxConns: 20,
	}
	primary.Healthy.Store(true)

	replica1 := &BackendEndpoint{
		Name:     "replica-1",
		Host:     "db-replica-1.internal",
		Port:     3306,
		Protocol: "mysql",
		MaxConns: 10,
	}
	replica1.Healthy.Store(true)

	replica2 := &BackendEndpoint{
		Name:     "replica-2",
		Host:     "db-replica-2.internal",
		Port:     3306,
		Protocol: "mysql",
		MaxConns: 10,
	}
	replica2.Healthy.Store(true)

	route := &RouteConfig{
		ID:       "tenant-1-main",
		Protocol: "mysql",
		Primary:  primary,
		Replicas: []*BackendEndpoint{replica1, replica2},
		Tenant:   "tenant-1",
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	// Test 1: SELECT query routes to a replica
	selectQuery := &protocol.ParsedQuery{
		QueryType: protocol.QueryTypeSelect,
		QueryText: "SELECT * FROM users WHERE id = 1",
	}

	backend, err := router.SelectBackend(route.ID, selectQuery, false)
	if err != nil {
		t.Fatalf("SelectBackend failed for SELECT: %v", err)
	}

	if backend == primary {
		t.Error("SELECT query routed to primary, expected replica")
	}

	if backend != replica1 && backend != replica2 {
		t.Errorf("SELECT query routed to unexpected backend: %s", backend.Name)
	}

	t.Logf("✓ SELECT query routed to replica: %s", backend.Name)

	// Test 2: INSERT query routes to primary
	insertQuery := &protocol.ParsedQuery{
		QueryType: protocol.QueryTypeInsert,
		QueryText: "INSERT INTO users (name) VALUES ('Alice')",
	}

	backend, err = router.SelectBackend(route.ID, insertQuery, false)
	if err != nil {
		t.Fatalf("SelectBackend failed for INSERT: %v", err)
	}

	if backend != primary {
		t.Errorf("INSERT query routed to %s, expected primary", backend.Name)
	}

	t.Logf("✓ INSERT query routed to primary: %s", backend.Name)

	// Test 3: UPDATE query routes to primary
	updateQuery := &protocol.ParsedQuery{
		QueryType: protocol.QueryTypeUpdate,
		QueryText: "UPDATE users SET name='Bob' WHERE id=1",
	}

	backend, err = router.SelectBackend(route.ID, updateQuery, false)
	if err != nil {
		t.Fatalf("SelectBackend failed for UPDATE: %v", err)
	}

	if backend != primary {
		t.Errorf("UPDATE query routed to %s, expected primary", backend.Name)
	}

	t.Logf("✓ UPDATE query routed to primary: %s", backend.Name)

	// Test 4: DELETE query routes to primary
	deleteQuery := &protocol.ParsedQuery{
		QueryType: protocol.QueryTypeDelete,
		QueryText: "DELETE FROM users WHERE id=1",
	}

	backend, err = router.SelectBackend(route.ID, deleteQuery, false)
	if err != nil {
		t.Fatalf("SelectBackend failed for DELETE: %v", err)
	}

	if backend != primary {
		t.Errorf("DELETE query routed to %s, expected primary", backend.Name)
	}

	t.Logf("✓ DELETE query routed to primary: %s", backend.Name)

	// Test 5: SELECT within transaction routes to primary
	selectInTxnQuery := &protocol.ParsedQuery{
		QueryType: protocol.QueryTypeSelect,
		QueryText: "SELECT * FROM users WHERE id = 1",
	}

	backend, err = router.SelectBackend(route.ID, selectInTxnQuery, true) // txnState=true
	if err != nil {
		t.Fatalf("SelectBackend failed for SELECT in txn: %v", err)
	}

	if backend != primary {
		t.Errorf("SELECT within transaction routed to %s, expected primary", backend.Name)
	}

	t.Logf("✓ SELECT within transaction routed to primary: %s", backend.Name)

	// Test 6: BEGIN routes to primary
	beginQuery := &protocol.ParsedQuery{
		QueryType: protocol.QueryTypeBegin,
	}

	backend, err = router.SelectBackend(route.ID, beginQuery, false)
	if err != nil {
		t.Fatalf("SelectBackend failed for BEGIN: %v", err)
	}

	if backend != primary {
		t.Errorf("BEGIN routed to %s, expected primary", backend.Name)
	}

	t.Logf("✓ BEGIN routed to primary: %s", backend.Name)

	// Test 7: All queries show consistent routing behavior
	queryTests := []struct {
		name          string
		queryType     protocol.QueryType
		expectPrimary bool
	}{
		{"SHOW TABLES", protocol.QueryTypeSelect, false}, // read, goes to replica
		{"CREATE TABLE", protocol.QueryTypeDDL, true},    // write, goes to primary
		{"DROP TABLE", protocol.QueryTypeDDL, true},      // write, goes to primary
		{"CALL proc()", protocol.QueryTypeCall, true},    // call, goes to primary
	}

	for _, tt := range queryTests {
		query := &protocol.ParsedQuery{QueryType: tt.queryType}
		backend, err := router.SelectBackend(route.ID, query, false)
		if err != nil {
			t.Errorf("SelectBackend failed for %s: %v", tt.name, err)
			continue
		}

		if tt.expectPrimary && backend != primary {
			t.Errorf("%s: expected primary, got %s", tt.name, backend.Name)
		} else if !tt.expectPrimary && backend == primary {
			t.Errorf("%s: expected replica, got primary", tt.name)
		}
		t.Logf("✓ %s routed correctly to %s", tt.name, backend.Name)
	}
}
