package pool

import (
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Pool manages database connection pooling by protocol
type Pool struct {
	pools       map[string]*ProtocolPool
	maxConns    int
	idleTimeout time.Duration
	logger      *zap.Logger
	mu          sync.RWMutex
}

// ProtocolPool manages connections for a specific protocol
type ProtocolPool struct {
	protocol    string
	connections chan net.Conn
	maxConns    int
	activeConns atomic.Int32
	totalConns  atomic.Int64
	mu          sync.RWMutex
}

// NewPool creates a new connection pool
func NewPool(maxConns int, logger *zap.Logger) *Pool {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &Pool{
		pools:       make(map[string]*ProtocolPool),
		maxConns:    maxConns,
		idleTimeout: 5 * time.Minute,
		logger:      logger,
	}
}

// RegisterProtocol registers a protocol pool
func (p *Pool) RegisterProtocol(protocol string) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.pools[protocol]; exists {
		return
	}

	p.pools[protocol] = &ProtocolPool{
		protocol:    protocol,
		connections: make(chan net.Conn, p.maxConns),
		maxConns:    p.maxConns,
	}
	p.logger.Debug("protocol pool registered", zap.String("protocol", protocol))
}

// Get retrieves a connection from the pool for the specified protocol
func (p *Pool) Get(protocol string) (net.Conn, error) {
	p.mu.RLock()
	protocolPool, exists := p.pools[protocol]
	p.mu.RUnlock()

	if !exists {
		return nil, fmt.Errorf("no pool for protocol: %s", protocol)
	}

	// Try to get connection from pool
	select {
	case conn := <-protocolPool.connections:
		if conn != nil {
			return conn, nil
		}
	default:
	}

	// Check if we can create a new connection
	current := protocolPool.activeConns.Load()
	if current >= int32(protocolPool.maxConns) { // G115: validated cast - maxConns is always positive
		return nil, fmt.Errorf("connection pool exhausted for protocol: %s (active: %d, max: %d)",
			protocol, current, protocolPool.maxConns)
	}

	// Increment active connections
	if !protocolPool.activeConns.CompareAndSwap(current, current+1) {
		// Lost race; try again
		return p.Get(protocol)
	}

	protocolPool.totalConns.Add(1)

	// Create placeholder connection (real implementation would connect to backend)
	conn := &mockConn{
		protocol: protocol,
		pool:     protocolPool,
	}

	return conn, nil
}

// Put returns a connection to the pool
func (p *Pool) Put(protocol string, conn net.Conn) error {
	p.mu.RLock()
	protocolPool, exists := p.pools[protocol]
	p.mu.RUnlock()

	if !exists {
		if conn != nil {
			if err := conn.Close(); err != nil {
				p.logger.Error("failed to close connection",
					zap.String("protocol", protocol),
					zap.Error(err),
				)
			}
		}
		return fmt.Errorf("no pool for protocol: %s", protocol)
	}

	// Try to return to pool
	select {
	case protocolPool.connections <- conn:
		return nil
	default:
		// Pool is full, close the connection
		if conn != nil {
			if err := conn.Close(); err != nil {
				p.logger.Error("failed to close connection",
					zap.String("protocol", protocol),
					zap.Error(err),
				)
			}
		}
		protocolPool.activeConns.Add(-1)
		return nil
	}
}

// Close closes all pooled connections
func (p *Pool) Close() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	for protocol, protocolPool := range p.pools {
		close(protocolPool.connections)
		// Drain remaining connections
		for conn := range protocolPool.connections {
			if conn != nil {
				if err := conn.Close(); err != nil {
					p.logger.Error("failed to close connection",
						zap.String("protocol", protocol),
						zap.Error(err),
					)
				}
			}
		}
		p.logger.Debug("protocol pool closed", zap.String("protocol", protocol))
	}

	return nil
}

// GetStats returns pool statistics
func (p *Pool) GetStats() map[string]interface{} {
	p.mu.RLock()
	defer p.mu.RUnlock()

	stats := make(map[string]interface{})
	for protocol, protocolPool := range p.pools {
		stats[protocol] = map[string]interface{}{
			"active_connections": protocolPool.activeConns.Load(),
			"total_connections":  protocolPool.totalConns.Load(),
			"max_connections":    protocolPool.maxConns,
		}
	}
	return stats
}

// mockConn is a placeholder connection for testing; real implementation would use actual DB connections
type mockConn struct {
	protocol string
	pool     *ProtocolPool
	closed   bool
	mu       sync.Mutex
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, fmt.Errorf("connection closed")
	}
	return 0, nil
}

func (m *mockConn) Write(b []byte) (n int, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.closed {
		return 0, fmt.Errorf("connection closed")
	}
	return len(b), nil
}

func (m *mockConn) Close() error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.closed {
		m.closed = true
		m.pool.activeConns.Add(-1)
	}
	return nil
}

func (m *mockConn) LocalAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
}

func (m *mockConn) RemoteAddr() net.Addr {
	return &net.TCPAddr{IP: net.ParseIP("127.0.0.1"), Port: 0}
}

func (m *mockConn) SetDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetReadDeadline(t time.Time) error {
	return nil
}

func (m *mockConn) SetWriteDeadline(t time.Time) error {
	return nil
}
