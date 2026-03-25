package handlers

import (
	"bufio"
	"context"
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

type RedisProxyHandler struct {
	cfg          *config.Config
	redis        *redis.Client
	logger       *zap.Logger
	roundRobin   uint64
	authManager  *auth.Manager
	secChecker   *security.SQLChecker
}

func NewRedisProxyHandler(cfg *config.Config, redis *redis.Client, logger *zap.Logger) *RedisProxyHandler {
	handler := &RedisProxyHandler{
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

func (h *RedisProxyHandler) Start(ctx context.Context, listener net.Listener) {
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

func (h *RedisProxyHandler) handleConnection(ctx context.Context, clientConn net.Conn) {
	defer clientConn.Close()

	metrics.IncConnection("redis")
	defer metrics.DecConnection("redis")

	// Redis doesn't have traditional databases like SQL, but it has numbered databases (0-15)
	// and users can be authenticated via AUTH command
	username := "default"
	database := "0" // Default Redis database

	// Check for blocked users if blocking is enabled
	if h.cfg.BlockingEnabled {
		if blocked, reason := h.secChecker.IsBlockedConnection(ctx, database, "", username); blocked {
			h.logger.Warn("Blocked resource access attempt",
				zap.String("user", username),
				zap.String("database", database),
				zap.String("reason", reason),
				zap.String("type", "redis_connection"))
			h.sendError(clientConn, "Access to this resource is blocked: "+reason)
			return
		}
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

func (h *RedisProxyHandler) sendError(conn net.Conn, message string) {
	// Redis error response format
	errorResponse := fmt.Sprintf("-ERR %s\r\n", message)
	conn.Write([]byte(errorResponse))
}

func (h *RedisProxyHandler) selectBackend() *config.Backend {
	backends := h.cfg.RedisBackends
	if len(backends) == 0 {
		return nil
	}

	idx := atomic.AddUint64(&h.roundRobin, 1) % uint64(len(backends))
	return &backends[idx]
}

func (h *RedisProxyHandler) connectToBackend(backend *config.Backend) (net.Conn, error) {
	address := fmt.Sprintf("%s:%d", backend.Host, backend.Port)
	return net.Dial("tcp", address)
}

func (h *RedisProxyHandler) proxyTraffic(ctx context.Context, client net.Conn, backend net.Conn, username, database string) {
	var wg sync.WaitGroup
	wg.Add(2)

	// Client to backend
	go func() {
		defer wg.Done()
		scanner := bufio.NewScanner(client)
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return
			default:
				line := scanner.Text()
				command := h.parseSimpleRedisCommand(line)
				
				// Update current database if SELECT command
				if strings.ToUpper(command.Command) == "SELECT" && len(command.Args) > 0 {
					database = command.Args[0]
				}
				
				// Update username if AUTH command
				if strings.ToUpper(command.Command) == "AUTH" && len(command.Args) > 0 {
					if len(command.Args) == 1 {
						// AUTH password (Redis < 6.0)
						username = "default"
					} else if len(command.Args) == 2 {
						// AUTH username password (Redis >= 6.0)
						username = command.Args[0]
					}
				}
				
				// Check for blocked databases/users if blocking is enabled
				if h.cfg.BlockingEnabled {
					if blocked, reason := h.secChecker.IsBlockedConnection(ctx, database, "", username); blocked {
						h.logger.Warn("Blocked resource access attempt",
							zap.String("user", username),
							zap.String("database", database),
							zap.String("reason", reason),
							zap.String("command", command.Command),
							zap.String("type", "redis_command"))
						h.sendError(client, "Access to this resource is blocked: "+reason)
						return
					}
				}
				
				// Check for dangerous Redis commands
				if h.isBlockedSimpleRedisCommand(command) {
					h.logger.Warn("Blocked Redis command",
						zap.String("user", username),
						zap.String("database", database),
						zap.String("command", command.Command))
					metrics.IncSQLInjection("redis")
					h.sendError(client, "Command blocked by security policy")
					return
				}

				// Check authorization
				if !h.authManager.Authorize(ctx, username, database, "", h.isWriteCommand(command.Command)) {
					h.logger.Warn("Unauthorized command",
						zap.String("user", username),
						zap.String("database", database),
						zap.String("command", command.Command))
					h.sendError(client, "Unauthorized")
					return
				}

				// Forward to backend
				backend.Write([]byte(line + "\r\n"))
				metrics.IncQuery("redis", h.isWriteCommand(command.Command))
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

type SimpleRedisCommand struct {
	Command string
	Args    []string
}

func (h *RedisProxyHandler) parseSimpleRedisCommand(line string) SimpleRedisCommand {
	cmd := SimpleRedisCommand{}
	
	// Simple Redis RESP protocol parsing
	if strings.HasPrefix(line, "*") {
		// Array format
		parts := strings.Fields(line)
		if len(parts) > 0 {
			cmd.Command = strings.TrimPrefix(parts[0], "*")
			cmd.Args = parts[1:]
		}
	} else {
		// Simple string format
		parts := strings.Fields(line)
		if len(parts) > 0 {
			cmd.Command = parts[0]
			cmd.Args = parts[1:]
		}
	}
	
	return cmd
}

func (h *RedisProxyHandler) isBlockedSimpleRedisCommand(cmd SimpleRedisCommand) bool {
	// Block dangerous Redis commands
	dangerousCommands := []string{
		// System/server management
		"FLUSHDB", "FLUSHALL", "SHUTDOWN", "DEBUG", "CONFIG",
		"CLIENT", "MONITOR", "SYNC", "PSYNC", "REPLCONF",
		
		// Script execution
		"EVAL", "EVALSHA", "SCRIPT",
		
		// Dangerous data operations
		"MIGRATE", "RESTORE", "DUMP",
		
		// Cluster management
		"CLUSTER",
		
		// Module management
		"MODULE",
		
		// ACL management (if blocking admin operations)
		"ACL",
		
		// Potentially dangerous info commands
		"INFO", "SLOWLOG", "LATENCY",
	}
	
	cmdUpper := strings.ToUpper(cmd.Command)
	for _, dangerous := range dangerousCommands {
		if cmdUpper == dangerous {
			return true
		}
	}
	
	return false
}

func (h *RedisProxyHandler) isWriteCommand(command string) bool {
	writeCommands := []string{
		// String operations
		"SET", "SETNX", "SETEX", "PSETEX", "MSET", "MSETNX", "APPEND",
		"INCR", "INCRBY", "INCRBYFLOAT", "DECR", "DECRBY",
		"DEL", "UNLINK", "EXPIRE", "EXPIREAT", "PEXPIRE", "PEXPIREAT",
		"PERSIST", "RENAME", "RENAMENX",
		
		// Hash operations
		"HSET", "HSETNX", "HMSET", "HINCRBY", "HINCRBYFLOAT", "HDEL",
		
		// List operations
		"LPUSH", "LPUSHX", "RPUSH", "RPUSHX", "LPOP", "RPOP",
		"LREM", "LSET", "LTRIM", "LINSERT", "RPOPLPUSH", "BRPOPLPUSH",
		
		// Set operations
		"SADD", "SREM", "SPOP", "SMOVE",
		
		// Sorted set operations
		"ZADD", "ZREM", "ZINCRBY", "ZREMRANGEBYSCORE", "ZREMRANGEBYRANK",
		"ZREMRANGEBYLEX",
		
		// Stream operations
		"XADD", "XDEL", "XTRIM",
		
		// Database operations
		"FLUSHDB", "FLUSHALL", "SELECT", "SWAPDB",
		
		// Transaction operations
		"MULTI", "EXEC", "DISCARD", "WATCH", "UNWATCH",
		
		// Pub/Sub operations
		"PUBLISH",
		
		// Scripting
		"EVAL", "EVALSHA",
	}
	
	cmdUpper := strings.ToUpper(command)
	for _, writeCmd := range writeCommands {
		if cmdUpper == writeCmd {
			return true
		}
	}
	
	return false
}