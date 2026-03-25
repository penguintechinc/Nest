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
	_ "github.com/lib/pq"
	"go.uber.org/zap"
)

type PostgreSQLHandler struct {
	cfg          *config.Config
	redis        *redis.Client
	logger       *zap.Logger
	pools        map[string]*pool.ConnectionPool
	poolMu       sync.RWMutex
	roundRobin   uint64
	authManager  *auth.Manager
	secChecker   *security.SQLChecker
}

func NewPostgreSQLHandler(cfg *config.Config, redis *redis.Client, logger *zap.Logger) *PostgreSQLHandler {
	handler := &PostgreSQLHandler{
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

func (h *PostgreSQLHandler) Start(ctx context.Context, listener net.Listener) {
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

func (h *PostgreSQLHandler) initPools() {
	h.poolMu.Lock()
	defer h.poolMu.Unlock()

	for _, backend := range h.cfg.PostgreSQLBackends {
		dsn := fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=disable",
			backend.Host, backend.Port, backend.User, backend.Password, backend.Database)
		
		if backend.TLS {
			dsn = fmt.Sprintf("host=%s port=%d user=%s password=%s dbname=%s sslmode=require",
				backend.Host, backend.Port, backend.User, backend.Password, backend.Database)
		}

		p := pool.NewConnectionPool("postgres", dsn, h.cfg.MaxConnections/len(h.cfg.PostgreSQLBackends))
		key := fmt.Sprintf("%s:%d", backend.Host, backend.Port)
		h.pools[key] = p
	}
}

func (h *PostgreSQLHandler) closePools() {
	h.poolMu.Lock()
	defer h.poolMu.Unlock()

	for _, p := range h.pools {
		p.Close()
	}
}

func (h *PostgreSQLHandler) handleConnection(ctx context.Context, clientConn net.Conn) {
	defer clientConn.Close()

	metrics.IncConnection("postgresql")
	defer metrics.DecConnection("postgresql")

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
				zap.String("type", "postgresql_connection"))
			h.sendError(clientConn, "Access to this resource is blocked: "+reason)
			return
		}
	}

	if !h.authManager.Authenticate(ctx, username, database, "postgresql") {
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

func (h *PostgreSQLHandler) performHandshake(conn net.Conn) (string, string, error) {
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return "", "", err
	}

	if n < 8 {
		return "", "", fmt.Errorf("invalid handshake packet")
	}

	// Simplified PostgreSQL handshake parsing
	username := "unknown"
	database := "unknown"
	
	// Parse the startup message for username and database
	message := string(buf[8:n])
	params := make(map[string]string)
	
	for i := 0; i < len(message); i++ {
		if message[i] == 0 {
			continue
		}
		
		// Find key
		keyStart := i
		for i < len(message) && message[i] != 0 {
			i++
		}
		if i >= len(message) {
			break
		}
		key := message[keyStart:i]
		i++
		
		// Find value
		valueStart := i
		for i < len(message) && message[i] != 0 {
			i++
		}
		if i <= len(message) {
			value := message[valueStart:i]
			params[key] = value
		}
	}
	
	if user, ok := params["user"]; ok {
		username = user
	}
	if db, ok := params["database"]; ok {
		database = db
	}

	// Send authentication OK response
	authResponse := []byte{0x52, 0x00, 0x00, 0x00, 0x08, 0x00, 0x00, 0x00, 0x00}
	conn.Write(authResponse)

	return username, database, nil
}

func (h *PostgreSQLHandler) sendError(conn net.Conn, message string) {
	errorPacket := []byte{0x45} // Error response
	errorMsg := fmt.Sprintf("SERROR\x00CFATAL\x00M%s\x00\x00", message)
	length := make([]byte, 4)
	length[0] = byte((len(errorMsg) + 4) >> 24)
	length[1] = byte((len(errorMsg) + 4) >> 16)
	length[2] = byte((len(errorMsg) + 4) >> 8)
	length[3] = byte(len(errorMsg) + 4)
	
	errorPacket = append(errorPacket, length...)
	errorPacket = append(errorPacket, []byte(errorMsg)...)
	conn.Write(errorPacket)
}

func (h *PostgreSQLHandler) selectBackend(isWrite bool) *config.Backend {
	var backends []config.Backend
	if isWrite {
		backends = h.cfg.GetWriteBackends("postgresql")
	} else {
		backends = h.cfg.GetReadBackends("postgresql")
	}

	if len(backends) == 0 {
		return nil
	}

	idx := atomic.AddUint64(&h.roundRobin, 1) % uint64(len(backends))
	return &backends[idx]
}

func (h *PostgreSQLHandler) getBackendConnection(backend *config.Backend) (*sql.Conn, error) {
	key := fmt.Sprintf("%s:%d", backend.Host, backend.Port)
	
	h.poolMu.RLock()
	p, ok := h.pools[key]
	h.poolMu.RUnlock()

	if !ok {
		return nil, fmt.Errorf("pool not found for backend %s", key)
	}

	return p.Get()
}

func (h *PostgreSQLHandler) proxyTraffic(ctx context.Context, client net.Conn, backend *sql.Conn, username, database string) {
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
							zap.String("type", "postgresql_query"))
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
					metrics.IncSQLInjection("postgresql")
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
					metrics.IncSQLInjection("postgresql") // Use same metric for now
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

				metrics.IncQuery("postgresql", h.isWriteQuery(query))
			}
		}
	}()

	go func() {
		defer wg.Done()
		// Backend to client proxy would go here
	}()

	wg.Wait()
}

func (h *PostgreSQLHandler) isWriteQuery(query string) bool {
	return security.IsWriteQuery(query)
}

