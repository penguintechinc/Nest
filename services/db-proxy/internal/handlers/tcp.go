package handlers

import (
	"context"
	"fmt"
	"net"
	"sync/atomic"

	"github.com/penguintechinc/nest/services/db-proxy/internal/config"
	"github.com/penguintechinc/nest/services/db-proxy/internal/pool"
	protocolpkg "github.com/penguintechinc/nest/services/db-proxy/internal/protocol"
	"github.com/penguintechinc/nest/services/db-proxy/internal/routing"
	"github.com/penguintechinc/nest/services/db-proxy/internal/security"
	"go.uber.org/zap"
	"golang.org/x/time/rate"
)

// TCPHandler implements a TCP proxy handler for database protocols
// It enforces security checks on the live path before forwarding
// and routes queries to primary (write) or replica (read) backends
type TCPHandler struct {
	protocol        string
	port            int
	pool            *pool.Pool
	securityChecker *security.Checker
	config          *config.Config
	logger          *zap.Logger
	router          *routing.Router    // Route queries to primary/replica
	parser          protocolpkg.Parser // Parse protocol messages and extract queries
	routeID         string             // Default route ID for this handler

	listener     net.Listener
	connLimiter  *rate.Limiter
	queryLimiter *rate.Limiter

	activeConns  atomic.Int64
	totalConns   atomic.Int64
	totalBlocked atomic.Int64
	running      atomic.Bool
	ctx          context.Context
	cancel       context.CancelFunc
}

// NewTCPHandler creates a new TCP handler for a database protocol
func NewTCPHandler(
	protocol string,
	port int,
	pool *pool.Pool,
	securityChecker *security.Checker,
	cfg *config.Config,
	logger *zap.Logger,
	router *routing.Router,
	routeID string,
) *TCPHandler {
	return &TCPHandler{
		protocol:        protocol,
		port:            port,
		pool:            pool,
		securityChecker: securityChecker,
		config:          cfg,
		logger:          logger,
		router:          router,
		parser:          protocolpkg.NewParser(protocol),
		routeID:         routeID,
		connLimiter:     rate.NewLimiter(rate.Limit(cfg.DefaultConnectionRate), cfg.DefaultConnectionRate),
		queryLimiter:    rate.NewLimiter(rate.Limit(cfg.DefaultQueryRate), cfg.DefaultQueryRate),
	}
}

// Start starts the TCP handler
func (h *TCPHandler) Start(ctx context.Context) error {
	listener, err := net.Listen("tcp", fmt.Sprintf(":%d", h.port))
	if err != nil {
		return fmt.Errorf("failed to listen on port %d: %w", h.port, err)
	}

	h.listener = listener
	h.ctx, h.cancel = context.WithCancel(ctx)
	h.running.Store(true)

	go h.acceptConnections()

	h.logger.Info("TCP handler started",
		zap.String("protocol", h.protocol),
		zap.Int("port", h.port),
	)

	return nil
}

// Stop stops the TCP handler
func (h *TCPHandler) Stop() error {
	h.logger.Info("stopping TCP handler", zap.String("protocol", h.protocol))

	if h.cancel != nil {
		h.cancel()
	}

	if h.listener != nil {
		if err := h.listener.Close(); err != nil {
			h.logger.Error("failed to close listener", zap.Error(err))
		}
	}

	h.running.Store(false)
	return nil
}

// GetStats returns handler statistics
func (h *TCPHandler) GetStats() map[string]interface{} {
	return map[string]interface{}{
		"protocol":      h.protocol,
		"port":          h.port,
		"active_conns":  h.activeConns.Load(),
		"total_conns":   h.totalConns.Load(),
		"total_blocked": h.totalBlocked.Load(),
		"running":       h.running.Load(),
	}
}

// acceptConnections accepts incoming connections
func (h *TCPHandler) acceptConnections() {
	for {
		select {
		case <-h.ctx.Done():
			return
		default:
		}

		conn, err := h.listener.Accept()
		if err != nil {
			if !h.running.Load() {
				return
			}
			h.logger.Error("failed to accept connection", zap.Error(err))
			continue
		}

		// Apply connection rate limiting
		if !h.connLimiter.Allow() {
			h.logger.Warn("connection rate limit exceeded")
			if err := conn.Close(); err != nil {
				h.logger.Error("failed to close connection", zap.Error(err))
			}
			continue
		}

		go h.handleConnection(conn)
	}
}

// handleConnection handles a single database connection using request-response proxy loop
// This replaces the old dual-goroutine io.Copy pattern to enable per-query routing
func (h *TCPHandler) handleConnection(clientConn net.Conn) {
	defer clientConn.Close()

	h.activeConns.Add(1)
	defer h.activeConns.Add(-1)

	h.totalConns.Add(1)

	// Create a proxy loop for this connection
	// This implements the request-response pattern with per-query routing
	checker := &CheckerInterface{
		CheckQuery:       h.securityChecker.CheckQuery,
		CheckParsedQuery: h.securityChecker.CheckParsedQuery,
	}

	proxyLoop := NewProxyLoop(
		clientConn,
		h.protocol,
		h.router,
		h.routeID,
		checker,
		h.logger,
	)

	ctx, cancel := context.WithCancel(h.ctx)
	defer cancel()

	// Run the request-response loop
	if err := proxyLoop.Run(ctx); err != nil {
		if err.Error() != "EOF" && err.Error() != "context canceled" {
			h.logger.Debug("proxy loop error",
				zap.String("protocol", h.protocol),
				zap.Error(err),
			)
		}
	}

	// Update stats from proxy loop
	loopStats := proxyLoop.Stats()
	if blocked, ok := loopStats["blocked_queries"].(int64); ok {
		h.totalBlocked.Add(blocked)
	}
}

// Manager manages all TCP handlers for different protocols
type Manager struct {
	handlers map[string]*TCPHandler
	logger   *zap.Logger
}

// NewManager creates a new handler manager
func NewManager(logger *zap.Logger) *Manager {
	return &Manager{
		handlers: make(map[string]*TCPHandler),
		logger:   logger,
	}
}

// AddHandler adds a TCP handler for a protocol
func (m *Manager) AddHandler(handler *TCPHandler) error {
	if _, exists := m.handlers[handler.protocol]; exists {
		return fmt.Errorf("handler for protocol %s already exists", handler.protocol)
	}

	m.handlers[handler.protocol] = handler
	m.logger.Info("handler added", zap.String("protocol", handler.protocol))
	return nil
}

// StartAll starts all registered handlers
func (m *Manager) StartAll(ctx context.Context) error {
	for protocol, handler := range m.handlers {
		if err := handler.Start(ctx); err != nil {
			return fmt.Errorf("failed to start handler for %s: %w", protocol, err)
		}
	}
	return nil
}

// StopAll stops all registered handlers
func (m *Manager) StopAll() error {
	var lastErr error
	for protocol, handler := range m.handlers {
		if err := handler.Stop(); err != nil {
			m.logger.Error("failed to stop handler",
				zap.String("protocol", protocol),
				zap.Error(err),
			)
			lastErr = err
		}
	}
	return lastErr
}

// GetStats returns statistics for all handlers
func (m *Manager) GetStats() map[string]interface{} {
	stats := make(map[string]interface{})
	for protocol, handler := range m.handlers {
		stats[protocol] = handler.GetStats()
	}
	return stats
}
