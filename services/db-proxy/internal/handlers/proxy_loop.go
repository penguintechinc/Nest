package handlers

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"
	"time"

	cachepkg "github.com/penguintechinc/nest/services/db-proxy/internal/cache"
	protocolpkg "github.com/penguintechinc/nest/services/db-proxy/internal/protocol"
	"github.com/penguintechinc/nest/services/db-proxy/internal/routing"
	"go.uber.org/zap"
)

// ProxyLoop implements the request-response loop for a client connection
// It reads complete protocol frames, routes them to appropriate backends, and relays responses
type ProxyLoop struct {
	clientConn   net.Conn
	clientFramer ProtocolFramer
	protocol     string
	router       *routing.Router
	routeID      string
	checker      *CheckerInterface // security checker wrapper
	logger       *zap.Logger
	session      *SessionState
	parser       protocolpkg.Parser
	cache        cachepkg.Store // query result cache
	cacheTTL     time.Duration  // cache TTL

	// Stats
	totalQueries    atomic.Int64
	blockedQueries  atomic.Int64
	multiwriteFails atomic.Int64 // secondary write failures in best-effort mode
}

// CheckerInterface is a wrapper around the security checker
type CheckerInterface struct {
	CheckQuery       func(query string) (bool, string)
	CheckParsedQuery func(query string) (bool, string)
}

// NewProxyLoop creates a new request-response proxy loop
func NewProxyLoop(
	clientConn net.Conn,
	protocol string,
	router *routing.Router,
	routeID string,
	checker *CheckerInterface,
	logger *zap.Logger,
) *ProxyLoop {
	return NewProxyLoopWithCache(clientConn, protocol, router, routeID, checker, logger, nil, 0)
}

// NewProxyLoopWithCache creates a new request-response proxy loop with cache support
func NewProxyLoopWithCache(
	clientConn net.Conn,
	protocol string,
	router *routing.Router,
	routeID string,
	checker *CheckerInterface,
	logger *zap.Logger,
	cache cachepkg.Store,
	cacheTTL time.Duration,
) *ProxyLoop {
	var framer ProtocolFramer
	switch protocol {
	case "mysql":
		framer = NewMySQLFramer(clientConn)
	case "postgresql":
		framer = NewPostgreSQLFramer(clientConn)
	default:
		framer = NewMySQLFramer(clientConn) // default to MySQL
	}

	if cache == nil {
		cache = &cachepkg.NoOpStore{}
	}

	return &ProxyLoop{
		clientConn:   clientConn,
		clientFramer: framer,
		protocol:     protocol,
		router:       router,
		routeID:      routeID,
		checker:      checker,
		logger:       logger,
		session:      NewSessionState(),
		parser:       protocolpkg.NewParser(protocol),
		cache:        cache,
		cacheTTL:     cacheTTL,
	}
}

// Run starts the request-response loop
// Processes requests one at a time until the client closes the connection
func (pl *ProxyLoop) Run(ctx context.Context) error {
	defer pl.closeBackendConnections()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
		}

		// Read one complete request frame from client
		clientRequest, err := pl.clientFramer.ReadFrame()
		if err != nil {
			if err.Error() == "EOF" {
				return nil // client closed connection
			}
			return fmt.Errorf("failed to read client frame: %w", err)
		}

		pl.totalQueries.Add(1)

		// Parse the query to determine its type and extract SQL
		parsedQuery, parseErr := pl.parser.Parse(clientRequest)
		if parseErr != nil {
			pl.logger.Debug("query parse error (continuing with security fallback)",
				zap.String("protocol", pl.protocol),
				zap.Error(parseErr),
			)
		}

		// Run security check on parsed query text if available
		if parsedQuery != nil && parsedQuery.QueryText != "" {
			blocked, reason := pl.checker.CheckParsedQuery(parsedQuery.QueryText)
			if blocked {
				pl.blockedQueries.Add(1)
				pl.logger.Warn("query blocked by security checker",
					zap.String("reason", reason),
					zap.String("query_type", parsedQuery.QueryType.String()),
				)
				// Send error to client and close connection
				return fmt.Errorf("security violation: %s", reason)
			}
		} else {
			// Fallback: check raw data if parsing failed
			blocked, reason := pl.checker.CheckQuery(string(clientRequest))
			if blocked {
				pl.blockedQueries.Add(1)
				pl.logger.Warn("query blocked by security checker (raw check)",
					zap.String("reason", reason),
				)
				return fmt.Errorf("security violation: %s", reason)
			}
		}

		// Update session state based on query type and content
		if parsedQuery != nil {
			pl.updateSessionState(parsedQuery)
		}

		// Route to appropriate backend
		backend, err := pl.selectBackend(parsedQuery)
		if err != nil {
			pl.logger.Error("failed to select backend", zap.Error(err))
			return fmt.Errorf("backend selection failed: %w", err)
		}

		// For writes, check if multi-write is enabled
		writeTargets := []*routing.BackendEndpoint{backend}
		var authoritativeBackend *routing.BackendEndpoint = backend
		if parsedQuery != nil && parsedQuery.QueryType.IsWrite() {
			route := pl.router.GetRoute(pl.routeID)
			if route != nil {
				targets := route.GetWriteTargets()
				if len(targets) > 0 {
					writeTargets = targets
					authoritativeBackend = route.GetAuthoritativeWriteTarget()
					pl.logger.Debug("multi-write enabled",
						zap.Int("target_count", len(targets)),
						zap.String("authoritative", authoritativeBackend.Name),
					)
				}
			}
		}

		// Try cache lookup for cacheable queries
		var backendResponse [][]byte
		var cacheHit bool

		if parsedQuery != nil && cachepkg.IsCacheable(int(parsedQuery.QueryType), parsedQuery.QueryText, pl.session.IsInTransaction(), pl.session.IsStateDirty()) {
			// Attempt to get from cache (use authoritative backend for cache key)
			route := pl.router.GetRoute(pl.routeID)
			if route != nil {
				cachedResponse, err := pl.cache.Get(ctx, route.Tenant, authoritativeBackend.Host, parsedQuery.QueryText)
				if err == nil && cachedResponse != nil {
					backendResponse = cachedResponse
					cacheHit = true
					pl.logger.Debug("cache hit for query",
						zap.String("query_type", parsedQuery.QueryType.String()),
					)
				}
			}
		}

		// If not from cache, fetch from backend(s)
		if !cacheHit {
			if len(writeTargets) > 1 && parsedQuery != nil && parsedQuery.QueryType.IsWrite() {
				// Multi-write path
				backendResponse, err = pl.executeMultiWrite(ctx, clientRequest, writeTargets, authoritativeBackend, parsedQuery)
				if err != nil {
					pl.logger.Error("multi-write execution failed", zap.Error(err))
					return fmt.Errorf("multi-write failed: %w", err)
				}
			} else {
				// Single-target path (reads or single-write)
				backendConn, err := pl.getBackendConnection(backend)
				if err != nil {
					pl.logger.Error("failed to get backend connection",
						zap.String("endpoint", backend.Name),
						zap.Error(err),
					)
					return fmt.Errorf("backend connection failed: %w", err)
				}

				// Forward request to backend
				if err := backendConn.framer.WriteFrame(clientRequest); err != nil {
					pl.logger.Error("failed to write to backend",
						zap.String("endpoint", backend.Name),
						zap.Error(err),
					)
					return fmt.Errorf("backend write failed: %w", err)
				}

				// Read response from backend (may be multiple frames for complex queries)
				var readErr error
				backendResponse, readErr = pl.readFullResponse(backendConn)
				if readErr != nil {
					pl.logger.Error("failed to read from backend",
						zap.String("endpoint", backend.Name),
						zap.Error(readErr),
					)
					return fmt.Errorf("backend read failed: %w", readErr)
				}
			}

			// Store in cache if cacheable
			if parsedQuery != nil && cachepkg.IsCacheable(int(parsedQuery.QueryType), parsedQuery.QueryText, pl.session.IsInTransaction(), pl.session.IsStateDirty()) {
				route := pl.router.GetRoute(pl.routeID)
				if route != nil {
					// Extract tables for invalidation tagging
					tables := cachepkg.ExtractTablesFromQuery(int(parsedQuery.QueryType), parsedQuery.QueryText)
					err := pl.cache.Set(ctx, route.Tenant, authoritativeBackend.Host, parsedQuery.QueryText, backendResponse, pl.cacheTTL, tables)
					if err != nil {
						pl.logger.Debug("failed to cache query result", zap.Error(err))
						// Continue despite cache error - caching is best-effort
					}
				}
			}
		}

		// Invalidate cache for writes
		if parsedQuery != nil && parsedQuery.QueryType.IsWrite() && !cacheHit {
			pl.invalidateCacheForWrite(ctx, parsedQuery)
		}

		// Forward response back to client
		// Response may be multiple frames; write each one
		for _, frame := range backendResponse {
			if err := pl.clientFramer.WriteFrame(frame); err != nil {
				pl.logger.Error("failed to write to client", zap.Error(err))
				return fmt.Errorf("client write failed: %w", err)
			}
		}
	}
}

// invalidateCacheForWrite invalidates cache entries affected by a write query
func (pl *ProxyLoop) invalidateCacheForWrite(ctx context.Context, parsedQuery *protocolpkg.ParsedQuery) {
	if parsedQuery == nil || !parsedQuery.QueryType.IsWrite() {
		return
	}

	// Extract tables affected by this write
	tables := cachepkg.ExtractTablesFromQuery(int(parsedQuery.QueryType), parsedQuery.QueryText)
	if len(tables) == 0 {
		return
	}

	for _, table := range tables {
		if err := pl.cache.InvalidateByTable(ctx, table); err != nil {
			pl.logger.Debug("failed to invalidate cache for table",
				zap.String("table", table),
				zap.Error(err),
			)
			// Continue despite error - invalidation is best-effort
		}
	}

	pl.logger.Debug("invalidated cache for write",
		zap.String("query_type", parsedQuery.QueryType.String()),
		zap.Strings("tables", tables),
	)
}

// updateSessionState updates session flags based on query type and content
func (pl *ProxyLoop) updateSessionState(parsedQuery *protocolpkg.ParsedQuery) {
	if parsedQuery == nil {
		return
	}

	queryType := parsedQuery.QueryType

	// Check transaction boundaries
	switch queryType {
	case protocolpkg.QueryTypeBegin:
		pl.session.SetInTransaction(true)
		pl.logger.Debug("transaction started")

	case protocolpkg.QueryTypeCommit, protocolpkg.QueryTypeRollback:
		pl.session.SetInTransaction(false)
		pl.session.MarkStateDirty() // keep dirty flag even after COMMIT (session state persists)
		pl.logger.Debug("transaction ended", zap.String("type", queryType.String()))
	}

	// Check for writes and DDL
	switch queryType {
	case protocolpkg.QueryTypeInsert, protocolpkg.QueryTypeUpdate, protocolpkg.QueryTypeDelete,
		protocolpkg.QueryTypeDDL, protocolpkg.QueryTypeCall:
		// Writes and DDL dirty the session
		pl.session.MarkStateDirty()
	}

	// Check for session-dirtying statements (SET, PREPARE, DECLARE, LISTEN, CREATE TEMP, etc.)
	if protocolpkg.IsSessionDirtyingStatement(parsedQuery.QueryText) {
		pl.session.MarkStateDirty()
		pl.logger.Debug("session marked dirty",
			zap.String("reason", "session-dirtying statement"),
			zap.String("query_type", queryType.String()),
		)
	}
}

// selectBackend determines which backend to use for this query
func (pl *ProxyLoop) selectBackend(parsedQuery *protocolpkg.ParsedQuery) (*routing.BackendEndpoint, error) {
	route := pl.router.GetRoute(pl.routeID)
	if route == nil {
		return nil, fmt.Errorf("route not found: %s", pl.routeID)
	}

	// Determine if we must use primary
	mustUsePrimary := pl.session.ShouldRouteToPrimary()

	// If session has mutable state, we're bound to active primary (respects blue/green)
	if mustUsePrimary {
		return route.GetActivePrimary(), nil
	}

	// If parsed query available, use its type to guide routing
	if parsedQuery != nil && parsedQuery.QueryType != protocolpkg.QueryTypeUnknown {
		// Use the router's logic to select backend
		backend, err := pl.router.SelectBackend(pl.routeID, parsedQuery, false)
		if err != nil {
			return nil, err
		}
		return backend, nil
	}

	// Fallback: use active primary for unknown/unparseable
	return route.GetActivePrimary(), nil
}

// getBackendConnection gets or creates a connection to the backend
// Primary connection is reused; replica connection is created as needed
func (pl *ProxyLoop) getBackendConnection(endpoint *routing.BackendEndpoint) (*BackendConnection, error) {
	if endpoint.Name == "primary" {
		// Primary connection
		if pl.session.GetPrimaryConnection() != nil {
			return pl.session.GetPrimaryConnection(), nil
		}

		// Establish new primary connection
		backendConn, err := pl.dialAndAuthBackend(endpoint)
		if err != nil {
			return nil, fmt.Errorf("failed to connect to primary: %w", err)
		}

		pl.session.SetPrimaryConnection(backendConn)
		return backendConn, nil
	}

	// Replica connection
	// Check if we already have one
	if pl.session.GetReplicaConnection() != nil {
		replicaConn := pl.session.GetReplicaConnection()
		// If it's to the same endpoint, reuse it
		if replicaConn.endpoint.Host == endpoint.Host && replicaConn.endpoint.Port == endpoint.Port {
			return replicaConn, nil
		}
		// Different replica; close old one
		// (In real implementation, would close the connection)
	}

	// Establish new replica connection
	backendConn, err := pl.dialAndAuthBackend(endpoint)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to replica: %w", err)
	}

	pl.session.SetReplicaConnection(backendConn)
	return backendConn, nil
}

// dialAndAuthBackend dials a backend endpoint and performs protocol-specific authentication
func (pl *ProxyLoop) dialAndAuthBackend(endpoint *routing.BackendEndpoint) (*BackendConnection, error) {
	addr := net.JoinHostPort(endpoint.Host, fmt.Sprintf("%d", endpoint.Port))
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("failed to dial %s: %w", addr, err)
	}

	// Perform protocol-specific authentication
	if pl.protocol == "postgresql" {
		err = pl.authPostgreSQL(conn, endpoint)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("PostgreSQL auth failed: %w", err)
		}
	} else if pl.protocol == "mysql" {
		err = pl.authMySQL(conn, endpoint)
		if err != nil {
			conn.Close()
			return nil, fmt.Errorf("MySQL auth failed: %w", err)
		}
	}

	var framer ProtocolFramer
	switch pl.protocol {
	case "mysql":
		framer = NewMySQLFramer(conn)
	case "postgresql":
		framer = NewPostgreSQLFramer(conn)
	default:
		framer = NewMySQLFramer(conn)
	}

	backendConn := &BackendConnection{
		conn:      conn,
		framer:    framer,
		endpoint:  endpoint,
		isReplica: endpoint.Name != "primary",
	}
	return backendConn, nil
}

// authPostgreSQL performs PostgreSQL authentication handshake
func (pl *ProxyLoop) authPostgreSQL(conn net.Conn, endpoint *routing.BackendEndpoint) error {
	// For now, use "postgres" as default user if not configured
	user := endpoint.User
	if user == "" {
		user = "postgres"
	}

	// Send StartupMessage
	startupMsg := protocolpkg.StartupMessage(user, "postgres")
	_, err := conn.Write(startupMsg)
	if err != nil {
		return fmt.Errorf("failed to send StartupMessage: %w", err)
	}

	// Handle auth exchange
	authHandler := protocolpkg.NewAuthHandler(user, endpoint.Password)
	err = authHandler.HandleStartup(conn)
	if err != nil {
		return fmt.Errorf("auth handshake failed: %w", err)
	}

	pl.logger.Debug("PostgreSQL auth successful",
		zap.String("endpoint", endpoint.Name),
		zap.String("user", user),
	)

	return nil
}

// authMySQL performs MySQL authentication handshake (mysql_native_password)
func (pl *ProxyLoop) authMySQL(conn net.Conn, endpoint *routing.BackendEndpoint) error {
	// Use "root" as default user if not configured
	user := endpoint.User
	if user == "" {
		user = "root"
	}

	// Create auth handler
	authHandler := protocolpkg.NewMySQLAuthHandler(endpoint.Password)

	// Read server's Handshake packet
	hs, err := authHandler.HandleHandshake(conn)
	if err != nil {
		return fmt.Errorf("failed to read handshake: %w", err)
	}

	pl.logger.Debug("MySQL handshake received",
		zap.String("server_version", hs.ServerVersion),
		zap.String("auth_plugin", hs.AuthPluginName),
	)

	// Send HandshakeResponse using mysql_native_password
	err = authHandler.SendHandshakeResponse(conn, user, "")
	if err != nil {
		return fmt.Errorf("failed to send handshake response: %w", err)
	}

	// Read auth result (OK or ERR packet)
	err = authHandler.ReadAuthResult(conn)
	if err != nil {
		return fmt.Errorf("auth result error: %w", err)
	}

	pl.logger.Debug("MySQL auth successful",
		zap.String("endpoint", endpoint.Name),
		zap.String("user", user),
	)

	return nil
}

// readFullResponse reads a complete response from the backend
// Accumulates frames until the response is complete (ReadyForQuery 'Z' for PostgreSQL, or command completion for MySQL)
func (pl *ProxyLoop) readFullResponse(backendConn *BackendConnection) ([][]byte, error) {
	var frames [][]byte

	if pl.protocol == "postgresql" {
		// PostgreSQL: read frames until we see ReadyForQuery ('Z')
		for {
			frame, err := backendConn.framer.ReadFrame()
			if err != nil {
				return nil, fmt.Errorf("failed to read backend frame: %w", err)
			}

			frames = append(frames, frame)

			// Check if this is ReadyForQuery ('Z') - indicates end of response
			if len(frame) > 0 && frame[0] == 'Z' {
				// ReadyForQuery is the final message of a response
				pl.logger.Debug("read complete PostgreSQL response",
					zap.Int("frame_count", len(frames)),
					zap.Int("total_bytes", totalFrameBytes(frames)),
				)
				return frames, nil
			}

			// Also stop on ErrorResponse ('E')
			if len(frame) > 0 && frame[0] == 'E' {
				pl.logger.Debug("received error response from backend",
					zap.Int("frame_count", len(frames)),
				)
				// Continue reading to collect the complete error response, but error responses
				// may not always be followed by ReadyForQuery in error cases
				// For safety, read one more frame to see if ReadyForQuery follows
				frame, err := backendConn.framer.ReadFrame()
				if err == nil {
					frames = append(frames, frame)
					if len(frame) > 0 && frame[0] == 'Z' {
						return frames, nil
					}
				}
				return frames, nil
			}
		}
	} else if pl.protocol == "mysql" {
		// MySQL: read frames until the response is complete
		// MySQL responses can span multiple packets for large result sets
		// We use a response accumulator to determine when a response is complete
		accumulator := protocolpkg.NewMySQLResponseAccumulator()

		for {
			frame, err := backendConn.framer.ReadFrame()
			if err != nil {
				return nil, fmt.Errorf("failed to read backend frame: %w", err)
			}

			// Add frame to accumulator
			isComplete, err := accumulator.AddPacket(frame)
			if err != nil {
				pl.logger.Debug("MySQL packet parsing error (continuing)",
					zap.Error(err),
				)
			}

			frames = append(frames, frame)

			if isComplete {
				pl.logger.Debug("read complete MySQL response",
					zap.Int("packet_count", len(frames)),
					zap.Int("total_bytes", totalFrameBytes(frames)),
				)
				return frames, nil
			}

			// Check for maximum packets to avoid infinite loops
			// Typical result sets should not exceed 1000 packets
			if len(frames) > 1000 {
				pl.logger.Warn("MySQL response exceeded 1000 packets, truncating",
					zap.Int("packet_count", len(frames)),
				)
				return frames, nil
			}
		}
	}

	// Fallback: just read one frame
	frame, err := backendConn.framer.ReadFrame()
	if err != nil {
		return nil, fmt.Errorf("failed to read backend frame: %w", err)
	}
	frames = append(frames, frame)

	return frames, nil
}

// totalFrameBytes calculates total bytes across all frames
func totalFrameBytes(frames [][]byte) int {
	total := 0
	for _, frame := range frames {
		total += len(frame)
	}
	return total
}

// executeMultiWrite sends the same write request to multiple targets
// Returns response from authoritative target; handles secondary failures per consistency policy
func (pl *ProxyLoop) executeMultiWrite(
	ctx context.Context,
	clientRequest []byte,
	targets []*routing.BackendEndpoint,
	authoritativeTarget *routing.BackendEndpoint,
	parsedQuery *protocolpkg.ParsedQuery,
) ([][]byte, error) {
	route := pl.router.GetRoute(pl.routeID)
	if route == nil || route.MultiWrite == nil {
		return nil, fmt.Errorf("multi-write not configured")
	}

	policy := route.MultiWrite.ConsistencyPolicy
	var authoritativeResponse [][]byte
	var authoritativeErr error

	// Send request to all targets concurrently
	type writeResult struct {
		target   *routing.BackendEndpoint
		response [][]byte
		err      error
	}

	results := make(chan writeResult, len(targets))
	for _, target := range targets {
		go func(t *routing.BackendEndpoint) {
			conn, err := pl.getBackendConnection(t)
			if err != nil {
				results <- writeResult{target: t, err: err}
				return
			}

			if err := conn.framer.WriteFrame(clientRequest); err != nil {
				results <- writeResult{target: t, err: err}
				return
			}

			response, err := pl.readFullResponse(conn)
			results <- writeResult{target: t, response: response, err: err}
		}(target)
	}

	// Collect results
	secondaryFailures := 0
	for range targets {
		result := <-results

		// Check if this is the authoritative target
		if result.target.Host == authoritativeTarget.Host && result.target.Port == authoritativeTarget.Port {
			authoritativeResponse = result.response
			authoritativeErr = result.err
		} else {
			// Secondary target
			if result.err != nil {
				secondaryFailures++
				pl.multiwriteFails.Add(1)
				pl.logger.Debug("secondary write target failed",
					zap.String("target", result.target.Name),
					zap.Error(result.err),
				)

				if policy == "strict" {
					return nil, fmt.Errorf("strict consistency violation: secondary target %s failed: %w",
						result.target.Name, result.err)
				}
				// best-effort: log and continue
			}
		}
	}

	if authoritativeErr != nil {
		return nil, fmt.Errorf("authoritative write target failed: %w", authoritativeErr)
	}

	if authoritativeResponse == nil {
		return nil, fmt.Errorf("no response from authoritative target")
	}

	if secondaryFailures > 0 && policy == "best-effort" {
		pl.logger.Info("multi-write completed with secondary failures",
			zap.Int("secondary_failures", secondaryFailures),
			zap.String("consistency_policy", policy),
		)
	}

	return authoritativeResponse, nil
}

// closeBackendConnections closes all backend connections
func (pl *ProxyLoop) closeBackendConnections() {
	if primaryConn := pl.session.GetPrimaryConnection(); primaryConn != nil && primaryConn.conn != nil {
		primaryConn.conn.Close()
	}
	if replicaConn := pl.session.GetReplicaConnection(); replicaConn != nil && replicaConn.conn != nil {
		replicaConn.conn.Close()
	}
}

// Stats returns statistics for this proxy loop
func (pl *ProxyLoop) Stats() map[string]interface{} {
	return map[string]interface{}{
		"total_queries":    pl.totalQueries.Load(),
		"blocked_queries":  pl.blockedQueries.Load(),
		"multiwrite_fails": pl.multiwriteFails.Load(),
	}
}
