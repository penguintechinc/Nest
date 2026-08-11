package handlers

import (
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	cachepkg "github.com/penguintechinc/nest/services/db-proxy/internal/cache"
	"github.com/penguintechinc/nest/services/db-proxy/internal/config"
	"github.com/penguintechinc/nest/services/db-proxy/internal/routing"
	"github.com/penguintechinc/nest/services/db-proxy/internal/security"
	"github.com/redis/go-redis/v9"
	"go.uber.org/zap"
)

// TestTCPHandlerCacheWiring tests that cache is properly wired into TCPHandler
func TestTCPHandlerCacheWiring(t *testing.T) {
	logger := zap.NewNop()

	// Setup: Create miniredis for cache
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	// Setup: Create cache store
	cacheStore := cachepkg.NewRedisStore(redisClient, "test:cache", 10*time.Second, 10000, logger)

	// Setup: Create config
	cfg := &config.Config{
		ListenAddr:             "127.0.0.1",
		ListenPort:             5432,
		MaxConnectionsPerRoute: 10,
		RedisPrefix:            "test:cache",
		Cache: config.CacheConfig{
			Enabled:   true,
			TTLSecs:   30,
			MaxSizeKB: 10000,
		},
	}

	// Setup: Create router
	router := routing.NewRouter(logger)

	// Create a TCPHandler with cache
	handler := NewTCPHandlerWithCache(
		"postgresql",
		5432,
		nil,
		nil,
		cfg,
		logger,
		router,
		"test-route",
		cacheStore,
		10*time.Second,
	)

	// Verify handler has cache and TTL set
	if handler.cache == nil {
		t.Fatalf("expected handler.cache to be set, got nil")
	}

	if handler.cacheTTL != 10*time.Second {
		t.Fatalf("expected cache TTL to be 10s, got %v", handler.cacheTTL)
	}

	t.Logf("✓ TCPHandler properly wired with cache store and TTL")

	// Test SetCache method updates cache
	newCacheStore := &cachepkg.NoOpStore{}
	handler.SetCache(newCacheStore, 20*time.Second)

	if handler.cache == newCacheStore {
		t.Logf("✓ SetCache method correctly updates handler cache")
	} else {
		t.Fatalf("SetCache did not update handler cache")
	}

	if handler.cacheTTL != 20*time.Second {
		t.Fatalf("expected cache TTL to be updated to 20s, got %v", handler.cacheTTL)
	}
}

// TestCacheIntegrationWithProxyLoop verifies cache integration with ProxyLoop
// Tests cacheability logic and cache usage patterns
func TestCacheIntegrationWithProxyLoop(t *testing.T) {
	logger := zap.NewNop()
	ctx := context.Background()

	// Setup: Create miniredis for cache
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	// Setup: Create cache store
	cacheStore := cachepkg.NewRedisStore(redisClient, "test:cache", 10*time.Second, 1000, logger)

	// Test Case 1: Verify cache hits and misses
	t.Run("cache_hit_miss_tracking", func(t *testing.T) {
		tenant := "test-tenant"
		targetDB := "127.0.0.1"
		query := "SELECT * FROM users"
		frames := [][]byte{[]byte("result1"), []byte("result2")}

		// First call should be a miss
		result, err := cacheStore.Get(ctx, tenant, targetDB, query)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result != nil {
			t.Fatalf("expected cache miss")
		}

		// Set in cache
		err = cacheStore.Set(ctx, tenant, targetDB, query, frames, 10*time.Second, []string{"users"})
		if err != nil {
			t.Fatalf("failed to set cache: %v", err)
		}

		// Second call should be a hit
		result, err = cacheStore.Get(ctx, tenant, targetDB, query)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result == nil {
			t.Fatalf("expected cache hit")
		}
		if len(result) != 2 {
			t.Fatalf("expected 2 frames, got %d", len(result))
		}

		// Verify stats
		stats := cacheStore.Stats()
		if stats["hits"].(int64) != 1 {
			t.Errorf("expected 1 hit, got %v", stats["hits"])
		}
		if stats["misses"].(int64) != 1 {
			t.Errorf("expected 1 miss, got %v", stats["misses"])
		}
	})

	// Test Case 2: Verify table-based invalidation
	t.Run("table_invalidation", func(t *testing.T) {
		mr.FlushDB()
		cacheStore := cachepkg.NewRedisStore(redisClient, "test:cache", 10*time.Second, 1000, logger)

		tenant := "test-tenant"
		targetDB := "127.0.0.1"

		// Cache two queries with different table tags
		selectUsersQuery := "SELECT * FROM users"
		selectOrdersQuery := "SELECT * FROM orders"

		cacheStore.Set(ctx, tenant, targetDB, selectUsersQuery, [][]byte{[]byte("users_data")}, 10*time.Second, []string{"users"})
		cacheStore.Set(ctx, tenant, targetDB, selectOrdersQuery, [][]byte{[]byte("orders_data")}, 10*time.Second, []string{"orders"})

		// Verify both are cached
		result1, _ := cacheStore.Get(ctx, tenant, targetDB, selectUsersQuery)
		result2, _ := cacheStore.Get(ctx, tenant, targetDB, selectOrdersQuery)
		if result1 == nil || result2 == nil {
			t.Fatalf("expected both queries cached")
		}

		// Invalidate users table
		cacheStore.InvalidateByTable(ctx, "users")

		// Verify users query is gone, orders still there
		result1, _ = cacheStore.Get(ctx, tenant, targetDB, selectUsersQuery)
		result2, _ = cacheStore.Get(ctx, tenant, targetDB, selectOrdersQuery)
		if result1 != nil {
			t.Fatalf("expected users query invalidated")
		}
		if result2 == nil {
			t.Fatalf("expected orders query still cached")
		}
	})

	// Test Case 3: Verify tenant isolation
	t.Run("tenant_isolation", func(t *testing.T) {
		mr.FlushDB()
		cacheStore := cachepkg.NewRedisStore(redisClient, "test:cache", 10*time.Second, 1000, logger)

		targetDB := "127.0.0.1"
		query := "SELECT * FROM accounts"

		// Cache same query for different tenants
		cacheStore.Set(ctx, "tenant-1", targetDB, query, [][]byte{[]byte("tenant1_data")}, 10*time.Second, nil)
		cacheStore.Set(ctx, "tenant-2", targetDB, query, [][]byte{[]byte("tenant2_data")}, 10*time.Second, nil)

		// Verify each tenant gets their own data
		result1, _ := cacheStore.Get(ctx, "tenant-1", targetDB, query)
		result2, _ := cacheStore.Get(ctx, "tenant-2", targetDB, query)

		if string(result1[0]) != "tenant1_data" || string(result2[0]) != "tenant2_data" {
			t.Fatalf("tenant isolation failed")
		}
	})

	// Test Case 4: Verify cacheability logic
	t.Run("cacheability_rules", func(t *testing.T) {
		tests := []struct {
			name        string
			queryType   int
			queryText   string
			inTxn       bool
			stateDirty  bool
			shouldCache bool
		}{
			{"simple SELECT", 1, "SELECT * FROM users", false, false, true},
			{"SELECT FOR UPDATE", 1, "SELECT * FROM users FOR UPDATE", false, false, false},
			{"SELECT with NOW", 1, "SELECT NOW() FROM users", false, false, false},
			{"SELECT in transaction", 1, "SELECT * FROM users", true, false, false},
			{"SELECT with dirty session", 1, "SELECT * FROM users", false, true, false},
			{"INSERT query", 2, "INSERT INTO users VALUES (1)", false, false, false},
		}

		for _, test := range tests {
			if cachepkg.IsCacheable(test.queryType, test.queryText, test.inTxn, test.stateDirty) != test.shouldCache {
				t.Errorf("%s: cacheability mismatch", test.name)
			}
		}
	})
}

// TestCacheBackendRoundTripSkipped proves the DB round-trip is skipped on cache hit
// Uses the proven fake PostgreSQL backend harness to assert on backend query counts
func TestCacheBackendRoundTripSkipped(t *testing.T) {
	if testing.Short() {
		t.Skip("Skipping acceptance test in short mode")
	}

	logger := zap.NewNop()
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	// Setup: Create miniredis for cache
	mr := miniredis.NewMiniRedis()
	if err := mr.Start(); err != nil {
		t.Fatalf("failed to start miniredis: %v", err)
	}
	defer mr.Close()

	redisClient := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	defer redisClient.Close()

	// Setup: Create fake PostgreSQL backends
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

	// Setup: Create router
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
		ID:       "test-cache-route",
		Protocol: "postgresql",
		Primary:  primaryEndpoint,
		Replicas: []*routing.BackendEndpoint{replicaEndpoint},
		Tenant:   "test-tenant",
	}
	if err := router.AddRoute(route); err != nil {
		t.Fatalf("failed to add route: %v", err)
	}

	checker := security.NewChecker(logger)
	checkerIface := &CheckerInterface{
		CheckQuery:       checker.CheckQuery,
		CheckParsedQuery: checker.CheckParsedQuery,
	}

	// Test 1: First SELECT hits backend (replica query count = 1)
	t.Run("first_select_hits_backend_count_1", func(t *testing.T) {
		mr.FlushDB()
		cacheStore := cachepkg.NewRedisStore(redisClient, "test:cache", 10*time.Second, 10000, logger)

		replica.mu.Lock()
		replica.ReceivedData = nil
		replica.mu.Unlock()

		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		if err := runProxyQueryTestWithCache(testCtx, t, router, checkerIface, logger, "SELECT * FROM users", cacheStore, "test-cache-route"); err != nil {
			t.Fatalf("proxy test failed: %v", err)
		}

		replicaQueries := replica.GetReceivedQueries()
		if len(replicaQueries) != 1 {
			t.Fatalf("FAIL: expected replica query count = 1, got %d", len(replicaQueries))
		}
		t.Logf("✓ First SELECT: backend received query, count = 1")
	})

	// Test 2: Second identical SELECT served from cache (replica count STAYS 1, DB NOT hit)
	t.Run("second_select_from_cache_count_stays_1", func(t *testing.T) {
		mr.FlushDB()
		cacheStore := cachepkg.NewRedisStore(redisClient, "test:cache", 10*time.Second, 10000, logger)

		replica.mu.Lock()
		replica.ReceivedData = nil
		replica.mu.Unlock()

		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		if err := runProxyQueryTestWithCache(testCtx, t, router, checkerIface, logger, "SELECT * FROM users", cacheStore, "test-cache-route"); err != nil {
			t.Fatalf("first query failed: %v", err)
		}

		countAfterFirst := len(replica.GetReceivedQueries())
		if countAfterFirst != 1 {
			t.Fatalf("expected count = 1 after first query, got %d", countAfterFirst)
		}

		// Second identical query on NEW connection (fresh session)
		testCtx2, testCancel2 := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel2()

		if err := runProxyQueryTestWithCache(testCtx2, t, router, checkerIface, logger, "SELECT * FROM users", cacheStore, "test-cache-route"); err != nil {
			t.Fatalf("second query failed: %v", err)
		}

		countAfterSecond := len(replica.GetReceivedQueries())
		if countAfterSecond != 1 {
			t.Fatalf("FAIL: expected count to STAY 1 (cache hit, DB NOT hit), got %d. Backend was called again!", countAfterSecond)
		}
		t.Logf("✓ BACKEND COUNT PROOF: second SELECT served from cache, count STAYED 1 — DB NOT hit")
	})

	// Test 3: Different tenant, same query = separate cache entry (count increments)
	t.Run("different_tenant_cache_miss_count_increments", func(t *testing.T) {
		mr.FlushDB()
		cacheStore := cachepkg.NewRedisStore(redisClient, "test:cache", 10*time.Second, 10000, logger)

		replica.mu.Lock()
		replica.ReceivedData = nil
		replica.mu.Unlock()

		route2 := &routing.RouteConfig{
			ID:       "test-cache-route-tenant2",
			Protocol: "postgresql",
			Primary:  primaryEndpoint,
			Replicas: []*routing.BackendEndpoint{replicaEndpoint},
			Tenant:   "tenant-2",
		}
		if err := router.AddRoute(route2); err != nil {
			t.Fatalf("failed to add route for tenant-2: %v", err)
		}

		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		if err := runProxyQueryTestWithCache(testCtx, t, router, checkerIface, logger, "SELECT * FROM users", cacheStore, "test-cache-route"); err != nil {
			t.Fatalf("tenant-1 query failed: %v", err)
		}
		count1 := len(replica.GetReceivedQueries())

		testCtx2, testCancel2 := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel2()

		if err := runProxyQueryTestWithCache(testCtx2, t, router, checkerIface, logger, "SELECT * FROM users", cacheStore, "test-cache-route-tenant2"); err != nil {
			t.Fatalf("tenant-2 query failed: %v", err)
		}
		count2 := len(replica.GetReceivedQueries())

		if count2 != count1+1 {
			t.Fatalf("FAIL: expected count increment from %d to %d (different tenant = cache miss), got %d", count1, count1+1, count2)
		}
		t.Logf("✓ Tenant isolation: different tenant triggered cache miss, count %d → %d", count1, count2)
	})

	// Test 4: Write invalidates cache
	t.Run("write_invalidates_cache_count_increments", func(t *testing.T) {
		mr.FlushDB()
		cacheStore := cachepkg.NewRedisStore(redisClient, "test:cache", 10*time.Second, 10000, logger)

		replica.mu.Lock()
		replica.ReceivedData = nil
		replica.mu.Unlock()

		testCtx, testCancel := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel()

		if err := runProxyQueryTestWithCache(testCtx, t, router, checkerIface, logger, "SELECT * FROM users", cacheStore, "test-cache-route"); err != nil {
			t.Fatalf("first SELECT failed: %v", err)
		}
		count1 := len(replica.GetReceivedQueries())

		testCtx2, testCancel2 := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel2()

		if err := runProxyQueryTestWithCache(testCtx2, t, router, checkerIface, logger, "SELECT * FROM users", cacheStore, "test-cache-route"); err != nil {
			t.Fatalf("second SELECT failed: %v", err)
		}
		count2 := len(replica.GetReceivedQueries())

		if count2 != count1 {
			t.Fatalf("expected cache hit, count should stay %d, got %d", count1, count2)
		}

		testCtx3, testCancel3 := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel3()

		if err := runProxyQueryTestWithCache(testCtx3, t, router, checkerIface, logger, "INSERT INTO users VALUES (1, 'test')", cacheStore, "test-cache-route"); err != nil {
			t.Fatalf("INSERT failed: %v", err)
		}

		testCtx4, testCancel4 := context.WithTimeout(ctx, 5*time.Second)
		defer testCancel4()

		if err := runProxyQueryTestWithCache(testCtx4, t, router, checkerIface, logger, "SELECT * FROM users", cacheStore, "test-cache-route"); err != nil {
			t.Fatalf("third SELECT failed: %v", err)
		}
		count3 := len(replica.GetReceivedQueries())

		if count3 != count2+1 {
			t.Fatalf("FAIL: expected cache invalidation to increment count from %d to %d, got %d", count2, count2+1, count3)
		}
		t.Logf("✓ Write invalidation: INSERT invalidated cache, SELECT re-hit backend, count %d → %d", count2, count3)
	})
}

// runProxyQueryTestWithCache runs a query through ProxyLoop WITH cache enabled
func runProxyQueryTestWithCache(ctx context.Context, t *testing.T, router *routing.Router, checker *CheckerInterface,
	logger *zap.Logger, queryText string, cacheStore cachepkg.Store, routeID string) error {

	clientConn, proxyConn := net.Pipe()
	defer clientConn.Close()
	defer proxyConn.Close()

	proxyLoop := NewProxyLoopWithCache(proxyConn, "postgresql", router, routeID, checker, logger, cacheStore, 10*time.Second)

	done := make(chan error, 1)
	go func() {
		done <- proxyLoop.Run(ctx)
	}()

	time.Sleep(50 * time.Millisecond)

	queryMsg := buildPostgreSQLQuery(queryText)
	_, err := clientConn.Write(queryMsg)
	if err != nil {
		return fmt.Errorf("failed to send query: %w", err)
	}

	responseBuf := make([]byte, 1024)
	clientConn.SetReadDeadline(time.Now().Add(2 * time.Second))
	_, err = clientConn.Read(responseBuf)
	if err != nil && err.Error() != "EOF" {
		return fmt.Errorf("failed to read response: %w", err)
	}

	clientConn.Close()

	select {
	case _ = <-done:
	case <-time.After(3 * time.Second):
		return fmt.Errorf("proxy did not exit in time")
	}

	time.Sleep(100 * time.Millisecond)
	return nil
}
