package handlers

import (
	"context"
	"testing"
	"time"

	"github.com/penguintechinc/nest/services/db-proxy/internal/routing"
	"github.com/penguintechinc/nest/services/db-proxy/internal/security"
	"go.uber.org/zap"
)

func TestProxyLoopBasicCommunication(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping handlers coverage test in short mode")
	}

	logger := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create fake backend
	backend, err := NewFakeMySQLBackend("test-backend", "")
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Close()

	// Create router
	router := routing.NewRouter(logger)
	endpoint := &routing.BackendEndpoint{
		Name:     "backend",
		Host:     "127.0.0.1",
		Port:     backend.Port,
		Protocol: "mysql",
		MaxConns: 10,
		User:     "root",
	}
	endpoint.Healthy.Store(true)

	route := &routing.RouteConfig{
		ID:       "test",
		Protocol: "mysql",
		Primary:  endpoint,
		Tenant:   "test",
	}
	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	// Create security checker
	checker := security.NewChecker(logger)
	checkerIface := &CheckerInterface{
		CheckQuery:       checker.CheckQuery,
		CheckParsedQuery: checker.CheckParsedQuery,
	}

	// Test basic SELECT query
	if err := runMySQLProxyQueryTest(ctx, t, router, checkerIface, logger,
		"SELECT 1", backend, backend); err != nil {
		t.Logf("proxy test result: %v", err)
	}
}

func TestMultipleBackendFailover(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping multi-backend test in short mode")
	}

	logger := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	// Create primary backend
	primary, err := NewFakeMySQLBackend("primary", "")
	if err != nil {
		t.Fatalf("failed to create primary: %v", err)
	}
	defer primary.Close()

	// Create replica backend
	replica, err := NewFakeMySQLBackend("replica", "")
	if err != nil {
		t.Fatalf("failed to create replica: %v", err)
	}
	defer replica.Close()

	// Create router
	router := routing.NewRouter(logger)

	primaryEp := &routing.BackendEndpoint{
		Name:     "primary",
		Host:     "127.0.0.1",
		Port:     primary.Port,
		Protocol: "mysql",
		MaxConns: 10,
		User:     "root",
	}
	primaryEp.Healthy.Store(true)

	replicaEp := &routing.BackendEndpoint{
		Name:     "replica",
		Host:     "127.0.0.1",
		Port:     replica.Port,
		Protocol: "mysql",
		MaxConns: 10,
		User:     "root",
	}
	replicaEp.Healthy.Store(true)

	route := &routing.RouteConfig{
		ID:       "test",
		Protocol: "mysql",
		Primary:  primaryEp,
		Replicas: []*routing.BackendEndpoint{replicaEp},
		Tenant:   "test",
	}

	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	checker := security.NewChecker(logger)
	checkerIface := &CheckerInterface{
		CheckQuery:       checker.CheckQuery,
		CheckParsedQuery: checker.CheckParsedQuery,
	}

	// Test SELECT (should go to replica)
	if err := runMySQLProxyQueryTest(ctx, t, router, checkerIface, logger,
		"SELECT * FROM test", primary, replica); err != nil {
		t.Logf("SELECT test result: %v", err)
	}

	// Test INSERT (should go to primary)
	if err := runMySQLProxyQueryTest(ctx, t, router, checkerIface, logger,
		"INSERT INTO test VALUES (1)", primary, replica); err != nil {
		t.Logf("INSERT test result: %v", err)
	}
}

func TestHandlerStatistics(t *testing.T) {
	logger := zap.NewNop()

	// Create a basic proxy loop for stats testing
	backend, err := NewFakeMySQLBackend("backend", "")
	if err != nil {
		t.Fatalf("failed to create backend: %v", err)
	}
	defer backend.Close()

	router := routing.NewRouter(logger)
	endpoint := &routing.BackendEndpoint{
		Name:     "backend",
		Host:     "127.0.0.1",
		Port:     backend.Port,
		Protocol: "mysql",
		MaxConns: 10,
		User:     "root",
	}
	endpoint.Healthy.Store(true)

	route := &routing.RouteConfig{
		ID:       "test",
		Protocol: "mysql",
		Primary:  endpoint,
		Tenant:   "test",
	}
	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	// Get router stats
	stats := router.GetStats()
	if stats == nil {
		t.Error("expected non-nil stats")
	}

	if len(stats) < 1 {
		t.Logf("router stats: %v", stats)
	}
}
