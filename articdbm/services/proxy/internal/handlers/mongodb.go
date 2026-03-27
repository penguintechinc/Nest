package handlers

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/penguintechinc/articdbm/proxy/internal/auth"
	"github.com/penguintechinc/articdbm/proxy/internal/config"
	"github.com/penguintechinc/articdbm/proxy/internal/metrics"
	"github.com/penguintechinc/articdbm/proxy/internal/security"
	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"
)

type MongoDBHandler struct {
	cfg          *config.Config
	redis        *redis.Client
	logger       *zap.Logger
	roundRobin   uint64
	authManager  *auth.Manager
	secChecker   *security.SQLChecker
}

func NewMongoDBHandler(cfg *config.Config, redis *redis.Client, logger *zap.Logger) *MongoDBHandler {
	handler := &MongoDBHandler{
		cfg:         cfg,
		redis:       redis,
		logger:      logger,
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

func (h *MongoDBHandler) Start(ctx context.Context, listener net.Listener) {
	for {
		select {
		case <-ctx.Done():
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

func (h *MongoDBHandler) handleConnection(ctx context.Context, clientConn net.Conn) {
	defer clientConn.Close()

	metrics.IncConnection("mongodb")
	defer metrics.DecConnection("mongodb")

	username, database, err := h.performHandshake(clientConn)
	if err != nil {
		h.logger.Error("Handshake failed", zap.Error(err))
		return
	}

	// Check for blocked databases/users if blocking is enabled
	if h.cfg.BlockingEnabled {
		if blocked, reason := h.secChecker.IsBlockedConnection(ctx, database, "", username); blocked {
			h.logger.Warn("Blocked resource access attempt",
				zap.String("user", username),
				zap.String("database", database),
				zap.String("reason", reason),
				zap.String("type", "mongodb_connection"))
			h.sendError(clientConn, "Access to this resource is blocked: "+reason)
			return
		}
	}

	if !h.authManager.Authenticate(ctx, username, database, "mongodb") {
		h.logger.Warn("Authentication failed", 
			zap.String("user", username),
			zap.String("database", database))
		h.sendError(clientConn, "Access denied")
		return
	}

	backend := h.selectBackend()
	if backend == nil {
		h.logger.Error("No backend available")
		h.sendError(clientConn, "No backend available")
		return
	}

	backendConn, err := h.connectToBackend(backend)
	if err != nil {
		h.logger.Error("Failed to connect to backend", zap.Error(err))
		h.sendError(clientConn, "Backend connection failed")
		return
	}
	defer backendConn.Close()

	h.proxyTraffic(ctx, clientConn, backendConn, username, database)
}

func (h *MongoDBHandler) performHandshake(conn net.Conn) (string, string, error) {
	buf := make([]byte, 4096)
	n, err := conn.Read(buf)
	if err != nil {
		return "", "", err
	}

	if n < 16 {
		return "", "", fmt.Errorf("invalid handshake packet")
	}

	username := "unknown"
	database := "unknown"
	
	// Simplified MongoDB wire protocol parsing
	// In practice, this would need to properly parse BSON documents
	message := string(buf[16:n])
	
	// Look for authentication information in the message
	if len(message) > 0 {
		// Try to extract username and database from the message
		// This is a simplified approach - real implementation would parse BSON
		for i := 0; i < len(message)-10; i++ {
			if message[i:i+4] == "user" && i+10 < len(message) {
				start := i + 4
				end := start
				for end < len(message) && message[end] != 0 && end-start < 50 {
					end++
				}
				if end > start {
					username = message[start:end]
				}
			}
			if message[i:i+2] == "db" && i+8 < len(message) {
				start := i + 2
				end := start
				for end < len(message) && message[end] != 0 && end-start < 50 {
					end++
				}
				if end > start {
					database = message[start:end]
				}
			}
		}
	}

	// Send a simplified OK response
	response := []byte{
		0x10, 0x00, 0x00, 0x00, // Message length
		0x01, 0x00, 0x00, 0x00, // Request ID
		0x00, 0x00, 0x00, 0x00, // Response to
		0xD4, 0x07, 0x00, 0x00, // OpCode (OP_REPLY)
	}
	conn.Write(response)

	return username, database, nil
}

func (h *MongoDBHandler) sendError(conn net.Conn, message string) {
	// MongoDB error response format
	errorDoc := map[string]interface{}{
		"ok":           0,
		"errmsg":       message,
		"code":         18,
		"codeName":     "AuthenticationFailed",
	}
	
	errorJSON, _ := json.Marshal(errorDoc)
	
	// Create a simple response packet
	response := []byte{
		0x20, 0x00, 0x00, 0x00, // Message length (will be updated)
		0x02, 0x00, 0x00, 0x00, // Request ID
		0x00, 0x00, 0x00, 0x00, // Response to
		0xD4, 0x07, 0x00, 0x00, // OpCode (OP_REPLY)
		0x00, 0x00, 0x00, 0x00, // Response flags
		0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, // Cursor ID
		0x00, 0x00, 0x00, 0x00, // Starting from
		0x01, 0x00, 0x00, 0x00, // Number returned
	}
	
	response = append(response, errorJSON...)
	
	// Update message length
	length := len(response)
	response[0] = byte(length & 0xFF)
	response[1] = byte((length >> 8) & 0xFF)
	response[2] = byte((length >> 16) & 0xFF)
	response[3] = byte((length >> 24) & 0xFF)
	
	conn.Write(response)
}

func (h *MongoDBHandler) selectBackend() *config.Backend {
	backends := h.cfg.MongoDBBackends
	if len(backends) == 0 {
		return nil
	}

	idx := atomic.AddUint64(&h.roundRobin, 1) % uint64(len(backends))
	return &backends[idx]
}

func (h *MongoDBHandler) connectToBackend(backend *config.Backend) (net.Conn, error) {
	address := fmt.Sprintf("%s:%d", backend.Host, backend.Port)
	return net.Dial("tcp", address)
}

func (h *MongoDBHandler) proxyTraffic(ctx context.Context, client net.Conn, backend net.Conn, username, database string) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Client to backend
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

				// Parse MongoDB command for security checks
				command := h.parseMongoCommand(buf[:n])
				
				// Check for blocked collections/operations
				if h.cfg.BlockingEnabled {
					if blocked, reason := h.secChecker.IsBlockedConnection(ctx, database, command.Collection, username); blocked {
						h.logger.Warn("Blocked resource access attempt",
							zap.String("user", username),
							zap.String("database", database),
							zap.String("collection", command.Collection),
							zap.String("reason", reason),
							zap.String("type", "mongodb_query"))
						h.sendError(client, "Access to this resource is blocked: "+reason)
						return
					}
				}
				
				// Check for malicious operations
				if h.isBlockedMongoOperation(command) {
					h.logger.Warn("Blocked MongoDB operation",
						zap.String("user", username),
						zap.String("database", database),
						zap.String("operation", command.Operation),
						zap.String("collection", command.Collection))
					metrics.IncSQLInjection("mongodb")
					h.sendError(client, "Operation blocked by security policy")
					return
				}

				if !h.authManager.Authorize(ctx, username, database, command.Collection, h.isWriteOperation(command.Operation)) {
					h.logger.Warn("Unauthorized operation",
						zap.String("user", username),
						zap.String("database", database),
						zap.String("operation", command.Operation))
					h.sendError(client, "Unauthorized")
					return
				}

				// Forward to backend
				backend.Write(buf[:n])
				metrics.IncQuery("mongodb", h.isWriteOperation(command.Operation))
			}
		}
	}()

	// Backend to client
	go func() {
		defer wg.Done()
		buf := make([]byte, 32*1024)
		for {
			select {
			case <-ctx.Done():
				return
			default:
				n, err := backend.Read(buf)
				if err != nil {
					return
				}
				client.Write(buf[:n])
			}
		}
	}()

	wg.Wait()
}

type MongoCommand struct {
	Operation  string
	Database   string
	Collection string
	Command    map[string]interface{}
}

func (h *MongoDBHandler) parseMongoCommand(data []byte) MongoCommand {
	// Simplified MongoDB command parsing
	// In practice, this would need proper BSON parsing
	
	cmd := MongoCommand{
		Operation:  "unknown",
		Database:   "unknown",
		Collection: "unknown",
		Command:    make(map[string]interface{}),
	}
	
	// Basic pattern matching for common operations
	message := string(data)
	
	if containsIgnoreCase(message, "find") {
		cmd.Operation = "find"
	} else if containsIgnoreCase(message, "insert") {
		cmd.Operation = "insert"
	} else if containsIgnoreCase(message, "update") {
		cmd.Operation = "update"
	} else if containsIgnoreCase(message, "delete") {
		cmd.Operation = "delete"
	} else if containsIgnoreCase(message, "drop") {
		cmd.Operation = "drop"
	} else if containsIgnoreCase(message, "eval") {
		cmd.Operation = "eval"
	} else if containsIgnoreCase(message, "mapreduce") {
		cmd.Operation = "mapreduce"
	}
	
	return cmd
}

func (h *MongoDBHandler) isBlockedMongoOperation(cmd MongoCommand) bool {
	// Block dangerous operations
	dangerousOps := []string{
		"eval",           // Server-side JavaScript execution
		"mapreduce",      // Can execute arbitrary JavaScript
		"group",          // Can execute arbitrary JavaScript
		"where",          // JavaScript evaluation in queries
		"copydb",         // Database copying
		"clone",          // Database cloning
		"shutdown",       // Server shutdown
		"killop",         // Kill operations
		"fsync",          // Force filesystem sync
		"dropDatabase",   // Drop entire database
	}
	
	for _, dangerous := range dangerousOps {
		if cmd.Operation == dangerous {
			return true
		}
	}
	
	// Block operations on system collections
	systemCollections := []string{
		"system.users",
		"system.roles", 
		"system.version",
		"system.replset",
		"system.indexBuilds",
	}
	
	for _, sysCol := range systemCollections {
		if cmd.Collection == sysCol {
			return true
		}
	}
	
	return false
}

func (h *MongoDBHandler) isWriteOperation(operation string) bool {
	writeOps := []string{
		"insert", "update", "delete", "remove", "save", "drop", 
		"dropDatabase", "createIndex", "dropIndex", "dropIndexes",
		"create", "convertToCapped", "emptycapped", "renameCollection",
	}
	
	for _, writeOp := range writeOps {
		if operation == writeOp {
			return true
		}
	}
	
	return false
}

func containsIgnoreCase(s, substr string) bool {
	s = strings.ToLower(s)
	substr = strings.ToLower(substr)
	return strings.Contains(s, substr)
}