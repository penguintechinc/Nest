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

func TestRouterGetStats(t *testing.T) {
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
	replica1.Healthy.Store(true)

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

	stats := router.GetStats()
	if len(stats) != 1 {
		t.Errorf("expected 1 route in stats, got %d", len(stats))
	}

	routeStats, ok := stats["tenant-1-main"].(map[string]interface{})
	if !ok {
		t.Fatalf("failed to cast route stats")
	}

	if routeStats["protocol"] != "mysql" {
		t.Errorf("protocol mismatch")
	}

	if routeStats["replicas"].(int) != 1 {
		t.Errorf("expected 1 replica in stats")
	}
}
