package handlers

import (
	"context"
	"database/sql"
	"fmt"
	"net"
	"sync"
	"sync/atomic"

	"github.com/penguintechinc/articdbm/proxy/internal/auth"
	"github.com/penguintechinc/articdbm/proxy/internal/config"
	"github.com/penguintechinc/articdbm/proxy/internal/metrics"
	"github.com/penguintechinc/articdbm/proxy/internal/pool"
	"github.com/penguintechinc/articdbm/proxy/internal/security"
	"github.com/go-redis/redis/v8"
	_ "github.com/denisenkom/go-mssqldb"
	"go.uber.org/zap"
)

type MSSQLHandler struct {
	cfg          *config.Config
	redis        *redis.Client
	logger       *zap.Logger
	pools        map[string]*pool.ConnectionPool
	poolMu       sync.RWMutex
	roundRobin   uint64
	authManager  *auth.Manager
	secChecker   *security.SQLChecker
}

func NewMSSQLHandler(cfg *config.Config, redis *redis.Client, logger *zap.Logger) *MSSQLHandler {
	handler := &MSSQLHandler{
		cfg:         cfg,
		redis:       redis,
		logger:      logger,
		pools:       make(map[string]*pool.ConnectionPool),
		authManager: auth.NewManager(cfg, redis, logger),
		secChecker:  security.NewSQLChecker(cfg.SQLInjectionDetection, redis),
	}
	
	// Seed default blocked resources if enabled
	if cfg.SeedDefaultBlocked {
		ctx := context.Background()
		if err := handler.secChecker.SeedDefaultBlockedResources(ctx); err != nil {
			logger.Warn("Failed to seed default blocked resources", zap.Error(err))
		}
	}
	
	return handler
}

func (h *MSSQLHandler) Start(ctx context.Context, listener net.Listener) {
	h.initPools()

	for {
		select {
		case <-ctx.Done():
			h.closePools()
			return
		default:
			conn, err := listener.Accept()
			if err != nil {
				select {
				case <-ctx.Done():
					return
				default:
					h.logger.Error("Failed to accept connection", zap.Error(err))
					continue
				}
			}

			go h.handleConnection(ctx, conn)
		}
	}
}

func (h *MSSQLHandler) initPools() {
	h.poolMu.Lock()
	defer h.poolMu.Unlock()

	for _, backend := range h.cfg.MSSQLBackends {
		var dsn string
		if backend.TLS {
			dsn = fmt.Sprintf("server=%s;port=%d;user id=%s;password=%s;database=%s;encrypt=true",
				backend.Host, backend.Port, backend.User, backend.Password, backend.Database)
		} else {
			dsn = fmt.Sprintf("server=%s;port=%d;user id=%s;password=%s;database=%s;encrypt=false",
				backend.Host, backend.Port, backend.User, backend.Password, backend.Database)
		}

		p := pool.NewConnectionPool("mssql", dsn, h.cfg.MaxConnections/len(h.cfg.MSSQLBackends))
		key := fmt.Sprintf("%s:%d", backend.Host, backend.Port)
		h.pools[key] = p
	}
}

func (h *MSSQLHandler) closePools() {
	h.poolMu.Lock()
	defer h.poolMu.Unlock()

	for _, p := range h.pools {
		p.Close()
	}
}

func (h *MSSQLHandler) handleConnection(ctx context.Context, clientConn net.Conn) {
	defer clientConn.Close()

	metrics.IncConnection("mssql")
	defer metrics.DecConnection("mssql")

	username, database, err := h.performHandshake(clientConn)
	if err != nil {
		h.logger.Error("Handshake failed", zap.Error(err))
		return
	}

	// Check for blocked databases/users/tables if blocking is enabled
	if h.cfg.BlockingEnabled {
		if blocked, reason := h.secChecker.IsBlockedConnection(ctx, database, "", username); blocked {
			h.logger.Warn("Blocked resource access attempt",
				zap.String("user", username),
				zap.String("database", database),
				zap.String("reason", reason),
				zap.String("type", "mssql_connection"))
			h.sendError(clientConn, "Access to this resource is blocked: "+reason)
			return
		}
	}

	if !h.authManager.Authenticate(ctx, username, database, "mssql") {
		h.logger.Warn("Authentication failed", 
			zap.String("user", username),
			zap.String("database", database))
		h.sendError(clientConn, "Access denied")
		return
	}

	backend := h.selectBackend(false)
	if backend == nil {
		h.logger.Error("No backend available")
		h.sendError(clientConn, "No backend available")
		return
	}

	backendConn, err := h.getBackendConnection(backend)
	if err != nil {
		h.logger.Error("Failed to connect to backend", zap.Error(err))
		h.sendError(clientConn, "Backend connection failed")
		return
	}
	defer backendConn.Close()

	h.proxyTraffic(ctx, clientConn, backendConn, username, database)
}

func (h *MSSQLHandler) performHandshake(conn net.Conn) (string, string, error) {
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return "", "", err
	}

	if n < 8 {
		return "", "", fmt.Errorf("invalid handshake packet")
	}

	// Simplified MSSQL handshake parsing
	username := "unknown"
	database := "unknown"
	
	// In a real implementation, this would properly parse the TDS protocol
	// For now, we'll extract what we can from the login packet
	if n > 100 {
		// Look for username in the login packet
		for i := 50; i < n-20; i++ {
			if buf[i] != 0 && buf[i] < 128 {
				potential := string(buf[i:min(i+20, n)])
				if len(potential) > 2 && len(potential) < 50 {
					username = potential
					break
				}
			}
		}
		
		// Look for database name
		for i := 80; i < n-20; i++ {
			if buf[i] != 0 && buf[i] < 128 {
				potential := string(buf[i:min(i+20, n)])
				if len(potential) > 2 && len(potential) < 50 && potential != username {
					database = potential
					break
				}
			}
		}
	}

	// Send login acknowledgment
	ackResponse := []byte{
		0x04, 0x01, 0x00, 0x25, 0x00, 0x00, 0x01, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00,
	}
	conn.Write(ackResponse)

	return username, database, nil
}

func (h *MSSQLHandler) sendError(conn net.Conn, message string) {
	// MSSQL error packet format
	errorPacket := []byte{0x04, 0x01} // TDS response type
	errorMsg := fmt.Sprintf("Error: %s", message)
	
	// Length of packet
	length := len(errorMsg) + 10
	errorPacket = append(errorPacket, byte(length>>8), byte(length&0xFF))
	
	// Append error message
	errorPacket = append(errorPacket, []byte(errorMsg)...)
	
	conn.Write(errorPacket)
}

func (h *MSSQLHandler) selectBackend(isWrite bool) *config.Backend {
	var backends []config.Backend
	if isWrite {
		backends = h.cfg.GetWriteBackends("mssql")
	} else {
		backends = h.cfg.GetReadBackends("mssql")
	}

	if len(backends) == 0 {
		return nil
	}

	idx := atomic.AddUint64(&h.roundRobin, 1) % uint64(len(backends))
	return &backends[idx]
}

func (h *MSSQLHandler) getBackendConnection(backend *config.Backend) (*sql.Conn, error) {
	key := fmt.Sprintf("%s:%d", backend.Host, backend.Port)
	
	h.poolMu.RLock()
	p, ok := h.pools[key]
	h.poolMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("pool not found for backend %s", key)
	}

	return p.Get()
}

func (h *MSSQLHandler) proxyTraffic(ctx context.Context, client net.Conn, backend *sql.Conn, username, database string) {
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				n, err := client.Read(buf)
				if err != nil {
					return
				}

				query := string(buf[:n])
				
				// Check for blocked databases/users/tables if blocking is enabled
				if h.cfg.BlockingEnabled {
					if blocked, reason := h.secChecker.IsBlockedConnection(ctx, database, "", username); blocked {
						h.logger.Warn("Blocked resource access attempt",
							zap.String("user", username),
							zap.String("database", database),
							zap.String("reason", reason),
							zap.String("type", "mssql_query"))
						h.sendError(client, "Access to this resource is blocked: "+reason)
						return
					}
				}
				
				// Enhanced security check with details
				if isMalicious, attackType, description := h.secChecker.IsSQLInjectionWithDetails(query); isMalicious {
					h.logger.Warn("Security threat detected",
						zap.String("user", username),
						zap.String("database", database),
						zap.String("attack_type", attackType),
						zap.String("description", description),
						zap.String("query", query[:min(100, len(query))]))
					metrics.IncSQLInjection("mssql")
					h.sendError(client, "Query blocked by security policy: "+attackType)
					return
				}

				// Check threat intelligence indicators
				sourceIP := client.RemoteAddr().String()
				if matched, indicator, reason := h.secChecker.CheckThreatIntel(ctx, database, sourceIP, query, username); matched {
					h.logger.Warn("Threat intelligence match",
						zap.String("user", username),
						zap.String("database", database),
						zap.String("source_ip", sourceIP),
						zap.String("threat_level", indicator.ThreatLevel),
						zap.String("reason", reason),
						zap.String("query", query[:min(100, len(query))]))
					metrics.IncSQLInjection("mssql") // Use same metric for now
					h.sendError(client, "Query blocked by threat intelligence: "+reason)
					return
				}

				if !h.authManager.Authorize(ctx, username, database, "", h.isWriteQuery(query)) {
					h.logger.Warn("Unauthorized query",
						zap.String("user", username),
						zap.String("database", database))
					h.sendError(client, "Unauthorized")
					return
				}

				metrics.IncQuery("mssql", h.isWriteQuery(query))
			}
		}
	}()

	go func() {
		defer wg.Done()
		// Backend to client proxy would go here
	}()

	wg.Wait()
}

func (h *MSSQLHandler) isWriteQuery(query string) bool {
	return security.IsWriteQuery(query)
}

