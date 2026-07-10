package routing

import (
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/penguintechinc/nest/services/db-proxy/internal/protocol"
	"go.uber.org/zap"
)

// BackendEndpoint represents a database backend (primary or replica)
type BackendEndpoint struct {
	Name      string // "primary", "replica-1", etc.
	Host      string // hostname or IP
	Port      int    // port number
	Protocol  string // "mysql", "postgresql", "redis"
	MaxConns  int
	Healthy   atomic.Bool
	CheckTime atomic.Int64 // Unix nanoseconds of last health check
	FailCount atomic.Int32
	User      string // database user (for PostgreSQL auth)
	Password  string // database password (for PostgreSQL auth, if needed)
}

// RouteConfig represents the configuration for a single route
type RouteConfig struct {
	ID       string
	Protocol string
	Primary  *BackendEndpoint
	Replicas []*BackendEndpoint // slice of replica endpoints
	Tenant   string
}

// Router manages backend selection for read/write routing
type Router struct {
	routes map[string]*RouteConfig // route ID → route config
	logger *zap.Logger
	mu     sync.RWMutex

	// Health check
	healthCheckInterval time.Duration
	healthCheckTicker   *time.Ticker
	stopChan            chan struct{}
}

// NewRouter creates a new router
func NewRouter(logger *zap.Logger) *Router {
	if logger == nil {
		logger = zap.NewNop()
	}

	return &Router{
		routes:              make(map[string]*RouteConfig),
		logger:              logger,
		healthCheckInterval: 5 * time.Second,
		stopChan:            make(chan struct{}),
	}
}

// AddRoute registers a new route
func (r *Router) AddRoute(route *RouteConfig) error {
	if route == nil || route.ID == "" {
		return fmt.Errorf("invalid route: ID is required")
	}
	if route.Primary == nil {
		return fmt.Errorf("route %s: primary endpoint is required", route.ID)
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	if _, exists := r.routes[route.ID]; exists {
		return fmt.Errorf("route %s already exists", route.ID)
	}

	r.routes[route.ID] = route

	// Note: Health status is initialized by the health check goroutine.
	// Tests can set custom health status before adding this route.

	r.logger.Info("route added",
		zap.String("route_id", route.ID),
		zap.String("primary", fmt.Sprintf("%s:%d", route.Primary.Host, route.Primary.Port)),
		zap.Int("replicas", len(route.Replicas)),
	)

	return nil
}

// GetRoute returns a route by ID
func (r *Router) GetRoute(routeID string) *RouteConfig {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.routes[routeID]
}

// SelectBackend selects the appropriate backend (primary or replica) for a query
// txnState indicates if we're in an explicit transaction
// For queries in a transaction, always route to primary (consistency)
func (r *Router) SelectBackend(
	routeID string,
	query *protocol.ParsedQuery,
	txnState bool,
) (*BackendEndpoint, error) {
	route := r.GetRoute(routeID)
	if route == nil {
		return nil, fmt.Errorf("route not found: %s", routeID)
	}

	// Transaction-aware routing: BEGIN/COMMIT/ROLLBACK or inside a txn → PRIMARY
	if txnState || query.QueryType.IsTxnControl() {
		return route.Primary, nil
	}

	// Query-type routing
	if query.QueryType.IsRead() {
		// Route to a healthy replica if available
		if len(route.Replicas) > 0 {
			replica := r.selectHealthyReplica(route)
			if replica != nil {
				return replica, nil
			}
		}
		// No healthy replica; fall back to primary
		r.logger.Debug("no healthy replica available for route",
			zap.String("route_id", routeID),
			zap.String("query_type", query.QueryType.String()),
		)
		return route.Primary, nil
	}

	// Write query or unknown type → PRIMARY
	return route.Primary, nil
}

// selectHealthyReplica selects a healthy replica from the route
// Uses simple round-robin with health awareness
func (r *Router) selectHealthyReplica(route *RouteConfig) *BackendEndpoint {
	healthyReplicas := []*BackendEndpoint{}
	for _, replica := range route.Replicas {
		if replica.Healthy.Load() {
			healthyReplicas = append(healthyReplicas, replica)
		}
	}

	if len(healthyReplicas) == 0 {
		return nil
	}

	// Simple selection: return first healthy replica
	// TODO: Implement proper load balancing (round-robin, weighted)
	return healthyReplicas[0]
}

// StartHealthChecks starts periodic health checks on all backends
func (r *Router) StartHealthChecks(ctx context.Context) {
	r.healthCheckTicker = time.NewTicker(r.healthCheckInterval)

	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			case <-r.stopChan:
				return
			case <-r.healthCheckTicker.C:
				r.checkAllBackends()
			}
		}
	}()

	r.logger.Info("health checks started",
		zap.Duration("interval", r.healthCheckInterval),
	)
}

// StopHealthChecks stops the health check goroutine
func (r *Router) StopHealthChecks() {
	if r.healthCheckTicker != nil {
		r.healthCheckTicker.Stop()
	}
	close(r.stopChan)
}

// checkAllBackends performs health checks on all backends
func (r *Router) checkAllBackends() {
	r.mu.RLock()
	routes := make([]*RouteConfig, 0, len(r.routes))
	for _, route := range r.routes {
		routes = append(routes, route)
	}
	r.mu.RUnlock()

	for _, route := range routes {
		// Check primary
		r.checkBackendHealth(route.Primary)

		// Check replicas
		for _, replica := range route.Replicas {
			r.checkBackendHealth(replica)
		}
	}
}

// checkBackendHealth performs a TCP dial to check if a backend is reachable
func (r *Router) checkBackendHealth(endpoint *BackendEndpoint) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	addr := fmt.Sprintf("%s:%d", endpoint.Host, endpoint.Port)
	dialer := &net.Dialer{}
	conn, err := dialer.DialContext(ctx, "tcp", addr)
	if err != nil {
		endpoint.Healthy.Store(false)
		endpoint.FailCount.Add(1)
		r.logger.Debug("health check failed",
			zap.String("endpoint", endpoint.Name),
			zap.String("addr", addr),
			zap.Error(err),
		)
		return
	}

	_ = conn.Close()
	endpoint.Healthy.Store(true)
	endpoint.FailCount.Store(0)
	endpoint.CheckTime.Store(time.Now().UnixNano())

	r.logger.Debug("health check passed",
		zap.String("endpoint", endpoint.Name),
		zap.String("addr", addr),
	)
}

// GetStats returns routing statistics
func (r *Router) GetStats() map[string]interface{} {
	r.mu.RLock()
	defer r.mu.RUnlock()

	stats := make(map[string]interface{})
	for routeID, route := range r.routes {
		routeStats := map[string]interface{}{
			"protocol": route.Protocol,
			"primary": map[string]interface{}{
				"addr":    fmt.Sprintf("%s:%d", route.Primary.Host, route.Primary.Port),
				"healthy": route.Primary.Healthy.Load(),
			},
			"replicas": len(route.Replicas),
		}

		replicaStats := []map[string]interface{}{}
		for _, replica := range route.Replicas {
			replicaStats = append(replicaStats, map[string]interface{}{
				"name":    replica.Name,
				"addr":    fmt.Sprintf("%s:%d", replica.Host, replica.Port),
				"healthy": replica.Healthy.Load(),
			})
		}
		routeStats["replica_details"] = replicaStats

		stats[routeID] = routeStats
	}

	return stats
}
