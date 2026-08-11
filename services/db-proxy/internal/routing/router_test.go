package routing

import (
	"testing"

	"github.com/penguintechinc/nest/services/db-proxy/internal/protocol"
	"go.uber.org/zap"
)

func TestRouterSelectBackend(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger)

	// Create a route with primary and replicas
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

	tests := []struct {
		name            string
		queryType       protocol.QueryType
		txnState        bool
		expectPrimary   bool
		expectReplicaOK bool
	}{
		{
			name:            "SELECT query routes to replica",
			queryType:       protocol.QueryTypeSelect,
			txnState:        false,
			expectPrimary:   false,
			expectReplicaOK: true,
		},
		{
			name:          "INSERT query routes to primary",
			queryType:     protocol.QueryTypeInsert,
			txnState:      false,
			expectPrimary: true,
		},
		{
			name:          "UPDATE query routes to primary",
			queryType:     protocol.QueryTypeUpdate,
			txnState:      false,
			expectPrimary: true,
		},
		{
			name:          "DELETE query routes to primary",
			queryType:     protocol.QueryTypeDelete,
			txnState:      false,
			expectPrimary: true,
		},
		{
			name:          "DDL routes to primary",
			queryType:     protocol.QueryTypeDDL,
			txnState:      false,
			expectPrimary: true,
		},
		{
			name:          "Query in transaction routes to primary",
			queryType:     protocol.QueryTypeSelect,
			txnState:      true,
			expectPrimary: true,
		},
		{
			name:          "BEGIN routes to primary",
			queryType:     protocol.QueryTypeBegin,
			txnState:      false,
			expectPrimary: true,
		},
		{
			name:          "COMMIT routes to primary",
			queryType:     protocol.QueryTypeCommit,
			txnState:      false,
			expectPrimary: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			query := &protocol.ParsedQuery{
				QueryType: tt.queryType,
			}

			backend, err := router.SelectBackend(route.ID, query, tt.txnState)
			if err != nil {
				t.Fatalf("SelectBackend failed: %v", err)
			}

			if tt.expectPrimary {
				if backend != primary {
					t.Errorf("expected primary backend, got %s", backend.Name)
				}
			} else if tt.expectReplicaOK {
				if backend == primary {
					t.Errorf("expected replica, got primary")
				}
				isReplica := backend == replica1 || backend == replica2
				if !isReplica {
					t.Errorf("expected one of the replicas, got %s", backend.Name)
				}
			}
		})
	}
}

func TestRouterSelectBackendUnhealthyReplica(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger)

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
	replica1.Healthy.Store(false) // Mark as unhealthy

	route := &RouteConfig{
		ID:       "tenant-1-main",
		Protocol: "mysql",
		Primary:  primary,
		Replicas: []*BackendEndpoint{replica1},
		Tenant:   "tenant-1",
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	query := &protocol.ParsedQuery{
		QueryType: protocol.QueryTypeSelect,
	}

	backend, err := router.SelectBackend(route.ID, query, false)
	if err != nil {
		t.Fatalf("SelectBackend failed: %v", err)
	}

	if backend != primary {
		t.Errorf("expected fallback to primary when replica is unhealthy, got %s", backend.Name)
	}
}

func TestRouterSelectBackendNoReplicas(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger)

	primary := &BackendEndpoint{
		Name:     "primary",
		Host:     "db-primary.internal",
		Port:     3306,
		Protocol: "mysql",
		MaxConns: 20,
	}
	primary.Healthy.Store(true)

	route := &RouteConfig{
		ID:       "tenant-1-main",
		Protocol: "mysql",
		Primary:  primary,
		Replicas: []*BackendEndpoint{}, // No replicas
		Tenant:   "tenant-1",
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	query := &protocol.ParsedQuery{
		QueryType: protocol.QueryTypeSelect,
	}

	backend, err := router.SelectBackend(route.ID, query, false)
	if err != nil {
		t.Fatalf("SelectBackend failed: %v", err)
	}

	if backend != primary {
		t.Errorf("expected primary when no replicas, got %s", backend.Name)
	}
}

func TestRouterAddRoute(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger)

	primary := &BackendEndpoint{
		Name: "primary",
		Host: "db.internal",
		Port: 3306,
	}

	route := &RouteConfig{
		ID:       "test-route",
		Protocol: "mysql",
		Primary:  primary,
		Tenant:   "tenant-1",
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	// Verify route was added
	retrievedRoute := router.GetRoute("test-route")
	if retrievedRoute == nil {
		t.Fatalf("route not found after adding")
	}

	if retrievedRoute.ID != route.ID {
		t.Errorf("route ID mismatch: got %s, expected %s", retrievedRoute.ID, route.ID)
	}
}

func TestRouterAddRouteDuplicate(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger)

	primary := &BackendEndpoint{
		Name: "primary",
		Host: "db.internal",
		Port: 3306,
	}

	route := &RouteConfig{
		ID:       "test-route",
		Protocol: "mysql",
		Primary:  primary,
		Tenant:   "tenant-1",
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	// Try to add the same route again
	if err := router.AddRoute(route); err == nil {
		t.Errorf("expected error when adding duplicate route, got nil")
	}
}

func TestRouterAddRouteNilPrimary(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger)

	route := &RouteConfig{
		ID:       "test-route",
		Protocol: "mysql",
		Primary:  nil, // Missing primary
		Tenant:   "tenant-1",
	}

	if err := router.AddRoute(route); err == nil {
		t.Errorf("expected error when primary is nil, got nil")
	}
}

func TestRouterGetRoute(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger)

	primary := &BackendEndpoint{
		Name:     "primary",
		Host:     "db.internal",
		Port:     3306,
		Protocol: "mysql",
		MaxConns: 10,
	}

	route := &RouteConfig{
		ID:       "route-1",
		Protocol: "mysql",
		Primary:  primary,
		Tenant:   "tenant-1",
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	retrieved := router.GetRoute("route-1")
	if retrieved == nil {
		t.Error("route not found")
	}

	if retrieved.ID != "route-1" {
		t.Errorf("route ID mismatch: expected route-1, got %s", retrieved.ID)
	}

	notFound := router.GetRoute("nonexistent")
	if notFound != nil {
		t.Error("expected nil for nonexistent route")
	}
}

func TestRouterSelectHealthyReplica(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger)

	primary := &BackendEndpoint{
		Name:     "primary",
		Host:     "db-primary.internal",
		Port:     3306,
		Protocol: "mysql",
		MaxConns: 10,
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
	replica2.Healthy.Store(false)

	route := &RouteConfig{
		ID:       "route-1",
		Protocol: "mysql",
		Primary:  primary,
		Replicas: []*BackendEndpoint{replica1, replica2},
		Tenant:   "tenant-1",
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	healthy := router.selectHealthyReplica(route)
	if healthy == nil {
		t.Error("no healthy replica found")
	}

	if healthy.Name != "replica-1" {
		t.Errorf("expected replica-1, got %s", healthy.Name)
	}
}

func TestRouterGetStats(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger)

	primary := &BackendEndpoint{
		Name:     "primary",
		Host:     "db.internal",
		Port:     3306,
		Protocol: "mysql",
		MaxConns: 10,
	}
	primary.Healthy.Store(true)

	route := &RouteConfig{
		ID:       "route-1",
		Protocol: "mysql",
		Primary:  primary,
		Tenant:   "tenant-1",
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	stats := router.GetStats()
	if stats == nil {
		t.Error("stats is nil")
	}

	if len(stats) == 0 {
		t.Logf("stats: %v", stats)
	}
}

func TestRouterRoundRobinReplicaSelection(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger)

	primary := &BackendEndpoint{
		Name:     "primary",
		Host:     "db-primary.internal",
		Port:     3306,
		Protocol: "mysql",
		MaxConns: 20,
	}
	primary.Healthy.Store(true)

	// Create 3 healthy replicas
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

	replica3 := &BackendEndpoint{
		Name:     "replica-3",
		Host:     "db-replica-3.internal",
		Port:     3306,
		Protocol: "mysql",
		MaxConns: 10,
	}
	replica3.Healthy.Store(true)

	route := &RouteConfig{
		ID:       "tenant-1-main",
		Protocol: "mysql",
		Primary:  primary,
		Replicas: []*BackendEndpoint{replica1, replica2, replica3},
		Tenant:   "tenant-1",
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	// Make 9 selections and verify round-robin distribution
	expectedPattern := []string{"replica-1", "replica-2", "replica-3", "replica-1", "replica-2", "replica-3", "replica-1", "replica-2", "replica-3"}
	selectionCounts := make(map[string]int)

	for i, expected := range expectedPattern {
		selected := router.selectHealthyReplica(route)
		if selected == nil {
			t.Fatalf("selection %d: got nil replica", i)
		}
		if selected.Name != expected {
			t.Errorf("selection %d: expected %s, got %s", i, expected, selected.Name)
		}
		selectionCounts[selected.Name]++
	}

	// Verify each replica was selected 3 times (evenly distributed)
	for _, replica := range []*BackendEndpoint{replica1, replica2, replica3} {
		if count := selectionCounts[replica.Name]; count != 3 {
			t.Errorf("replica %s: expected 3 selections, got %d", replica.Name, count)
		}
	}
}

func TestRouterRoundRobinSkipsUnhealthyReplicas(t *testing.T) {
	logger := zap.NewNop()
	router := NewRouter(logger)

	primary := &BackendEndpoint{
		Name:     "primary",
		Host:     "db-primary.internal",
		Port:     3306,
		Protocol: "mysql",
		MaxConns: 20,
	}
	primary.Healthy.Store(true)

	// Create 3 replicas: 2 healthy, 1 unhealthy
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
	replica2.Healthy.Store(false) // Unhealthy — should be skipped

	replica3 := &BackendEndpoint{
		Name:     "replica-3",
		Host:     "db-replica-3.internal",
		Port:     3306,
		Protocol: "mysql",
		MaxConns: 10,
	}
	replica3.Healthy.Store(true)

	route := &RouteConfig{
		ID:       "tenant-1-main",
		Protocol: "mysql",
		Primary:  primary,
		Replicas: []*BackendEndpoint{replica1, replica2, replica3},
		Tenant:   "tenant-1",
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	// Make 6 selections and verify round-robin only uses healthy replicas (1 and 3)
	expectedPattern := []string{"replica-1", "replica-3", "replica-1", "replica-3", "replica-1", "replica-3"}
	selectionCounts := make(map[string]int)

	for i, expected := range expectedPattern {
		selected := router.selectHealthyReplica(route)
		if selected == nil {
			t.Fatalf("selection %d: got nil replica", i)
		}
		if selected.Name != expected {
			t.Errorf("selection %d: expected %s, got %s", i, expected, selected.Name)
		}
		selectionCounts[selected.Name]++
	}

	// Verify replica2 (unhealthy) was never selected
	if count := selectionCounts["replica-2"]; count != 0 {
		t.Errorf("unhealthy replica-2: expected 0 selections, got %d", count)
	}

	// Verify replica1 and replica3 each got 3 selections
	if count := selectionCounts["replica-1"]; count != 3 {
		t.Errorf("replica-1: expected 3 selections, got %d", count)
	}
	if count := selectionCounts["replica-3"]; count != 3 {
		t.Errorf("replica-3: expected 3 selections, got %d", count)
	}
}
