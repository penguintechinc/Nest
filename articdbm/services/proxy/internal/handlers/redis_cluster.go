package handlers

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/go-redis/redis/v8"
	"go.uber.org/zap"

	"github.com/penguintechinc/articdbm/proxy/internal/afxdp"
	"github.com/penguintechinc/articdbm/proxy/internal/auth"
	"github.com/penguintechinc/articdbm/proxy/internal/cache"
	"github.com/penguintechinc/articdbm/proxy/internal/config"
	"github.com/penguintechinc/articdbm/proxy/internal/metrics"
	"github.com/penguintechinc/articdbm/proxy/internal/numa"
	"github.com/penguintechinc/articdbm/proxy/internal/security"
)

const (
	HASH_SLOTS           = 16384
	DEFAULT_ASK_TIMEOUT  = 5 * time.Second
	CLUSTER_REFRESH_INTERVAL = 30 * time.Second
	MAX_REDIRECTIONS     = 3
)

type RedisClusterHandler struct {
	cfg             *config.Config
	redis           *redis.Client
	logger          *zap.Logger
	authManager     *auth.Manager
	secChecker      *security.SQLChecker
	cacheManager    *cache.MultiTierCache
	afxdpManager    *afxdp.SocketManager
	topology        *numa.TopologyInfo

	clusterNodes    map[string]*RedisNode
	slotMap         [HASH_SLOTS]*RedisNode
	nodeConnections map[string]*redis.Client

	ctx             context.Context
	cancel          context.CancelFunc
	mu              sync.RWMutex
	stats           *RedisClusterStats
}

type RedisNode struct {
	ID          string          `json:"id"`
	Host        string          `json:"host"`
	Port        int             `json:"port"`
	Master      bool            `json:"master"`
	Slots       []SlotRange     `json:"slots"`
	Replicas    []*RedisNode    `json:"replicas"`
	Client      *redis.Client   `json:"-"`
	LastSeen    time.Time       `json:"last_seen"`
	Healthy     bool            `json:"healthy"`
	NumaNode    int             `json:"numa_node"`
	Latency     time.Duration   `json:"latency"`
	Connections int32           `json:"connections"`
	QPS         uint64          `json:"qps"`
}

type SlotRange struct {
	Start int `json:"start"`
	End   int `json:"end"`
}

type RedisClusterStats struct {
	TotalNodes       int                   `json:"total_nodes"`
	MasterNodes      int                   `json:"master_nodes"`
	ReplicaNodes     int                   `json:"replica_nodes"`
	HealthyNodes     int                   `json:"healthy_nodes"`
	TotalRequests    uint64                `json:"total_requests"`
	RedirectedMoved  uint64                `json:"redirected_moved"`
	RedirectedAsk    uint64                `json:"redirected_ask"`
	ClusterErrors    uint64                `json:"cluster_errors"`
	NodeStats        map[string]*NodeStats `json:"node_stats"`
	AvgLatency       time.Duration         `json:"avg_latency"`
	LastRefresh      time.Time             `json:"last_refresh"`
}

type NodeStats struct {
	Requests     uint64        `json:"requests"`
	Errors       uint64        `json:"errors"`
	Latency      time.Duration `json:"latency"`
	Connections  int32         `json:"connections"`
	LastAccess   time.Time     `json:"last_access"`
}

type RedisCommand struct {
	Command string
	Args    []string
	Key     string
	Slot    int
	IsRead  bool
}

func NewRedisClusterHandler(cfg *config.Config, redisClient *redis.Client, logger *zap.Logger, topology *numa.TopologyInfo) *RedisClusterHandler {
	ctx, cancel := context.WithCancel(context.Background())

	handler := &RedisClusterHandler{
		cfg:             cfg,
		redis:           redisClient,
		logger:          logger,
		authManager:     auth.NewManager(cfg, redisClient, logger),
		secChecker:      security.NewSQLChecker(cfg.SQLInjectionDetection, redisClient),
		topology:        topology,
		clusterNodes:    make(map[string]*RedisNode),
		nodeConnections: make(map[string]*redis.Client),
		ctx:             ctx,
		cancel:          cancel,
		stats: &RedisClusterStats{
			NodeStats: make(map[string]*NodeStats),
		},
	}

	// Initialize multi-tier cache for Redis operations
	cacheConfig := &cache.MultiTierConfig{
		Enabled:    true,
		DefaultTTL: 5 * time.Minute,
		Tiers: []cache.TierConfig{
			{Name: "xdp", Enabled: true, MaxSize: 100 * 1024 * 1024, TTL: 30 * time.Second, Priority: 3},
			{Name: "redis", Enabled: true, MaxSize: 500 * 1024 * 1024, TTL: 5 * time.Minute, Priority: 2},
		},
		Policies: []cache.CachePolicy{
			{
				Name:    "redis_get_commands",
				Enabled: true,
				Priority: 1,
				Rules: []cache.PolicyRule{
					{
						Match:  cache.MatchCriteria{Operation: "GET"},
						Action: "cache",
						TTL:    30 * time.Second,
						Tier:   "xdp",
					},
				},
			},
		},
	}

	if cacheManager, err := cache.NewMultiTierCache(cacheConfig, redisClient, topology, logger); err == nil {
		handler.cacheManager = cacheManager
	}

	// Initialize AF_XDP socket manager for zero-copy Redis processing
	afxdpConfig := []string{"eth0"} // This would come from config
	if afxdpManager, err := afxdp.NewSocketManager(afxdpConfig, topology, logger); err == nil {
		handler.afxdpManager = afxdpManager
		afxdpManager.StartPacketProcessing(handler.processRedisPacket)
	}

	// Start background tasks
	go handler.clusterTopologyRefresh()
	go handler.healthMonitor()
	go handler.statsCollector()

	// Initial cluster discovery
	handler.discoverClusterTopology()

	logger.Info("Redis cluster handler initialized",
		zap.Int("nodes", len(handler.clusterNodes)))

	return handler
}

func (h *RedisClusterHandler) Start(ctx context.Context, listener net.Listener) {
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

func (h *RedisClusterHandler) handleConnection(ctx context.Context, clientConn net.Conn) {
	defer clientConn.Close()

	metrics.IncConnection("redis_cluster")
	defer metrics.DecConnection("redis_cluster")

	// Get optimal NUMA node for this connection
	var cpuAffinity int
	if h.topology != nil {
		if cpu, err := h.topology.GetOptimalCPUForNIC(""); err == nil {
			cpuAffinity = cpu
			h.topology.SetCPUAffinity(cpu)
		}
	}

	username := "default"
	database := "0"

	scanner := bufio.NewScanner(clientConn)
	for scanner.Scan() {
		select {
		case <-ctx.Done():
			return
		default:
			line := scanner.Text()
			if err := h.processRedisCommand(ctx, clientConn, line, username, database, cpuAffinity); err != nil {
				h.logger.Error("Error processing Redis command", zap.Error(err))
				return
			}
		}
	}
}

func (h *RedisClusterHandler) processRedisCommand(ctx context.Context, conn net.Conn, line string, username, database string, cpuAffinity int) error {
	atomic.AddUint64(&h.stats.TotalRequests, 1)

	// Parse Redis command
	cmd := h.parseRedisCommand(line)
	if cmd == nil {
		return fmt.Errorf("failed to parse Redis command")
	}

	// Security checks
	if h.cfg.BlockingEnabled {
		if blocked, reason := h.secChecker.IsBlockedConnection(ctx, database, "", username); blocked {
			h.logger.Warn("Blocked Redis command",
				zap.String("user", username),
				zap.String("command", cmd.Command),
				zap.String("reason", reason))
			return h.sendError(conn, "Command blocked: "+reason)
		}
	}

	// Check for dangerous commands
	if h.isBlockedRedisCommand(*cmd) {
		metrics.IncSQLInjection("redis_cluster")
		return h.sendError(conn, "Command blocked by security policy")
	}

	// Try cache first for read operations
	if cmd.IsRead && h.cacheManager != nil {
		cacheReq := &cache.CacheRequest{
			Key:       cmd.Key,
			Query:     line,
			Database:  database,
			Operation: cmd.Command,
			User:      username,
			Context:   ctx,
		}

		if response, err := h.cacheManager.Get(ctx, cacheReq); err == nil && response.Found {
			conn.Write(response.Data)
			return nil
		}
	}

	// Execute command through cluster
	response, err := h.executeClusterCommand(ctx, cmd, username, cpuAffinity)
	if err != nil {
		return h.sendError(conn, err.Error())
	}

	// Cache the response for read operations
	if cmd.IsRead && h.cacheManager != nil && response != nil {
		cacheReq := &cache.CacheRequest{
			Key:       cmd.Key,
			Query:     line,
			Database:  database,
			Operation: cmd.Command,
			User:      username,
			Context:   ctx,
		}
		h.cacheManager.Set(ctx, cacheReq, response)
	}

	// Send response to client
	if response != nil {
		conn.Write(response)
	}

	return nil
}

func (h *RedisClusterHandler) executeClusterCommand(ctx context.Context, cmd *RedisCommand, username string, cpuAffinity int) ([]byte, error) {
	// Find the appropriate node for this command
	node := h.getNodeForCommand(cmd)
	if node == nil {
		return nil, fmt.Errorf("no available node for command")
	}

	// Track redirections to avoid infinite loops
	redirections := 0

	for redirections <= MAX_REDIRECTIONS {
		// Execute command on the selected node
		result, err := h.executeOnNode(ctx, node, cmd, username)
		if err != nil {
			// Check for cluster redirections
			if moved, newNode := h.parseMovedError(err.Error()); moved {
				atomic.AddUint64(&h.stats.RedirectedMoved, 1)
				node = newNode
				redirections++
				continue
			}

			if ask, newNode := h.parseAskError(err.Error()); ask {
				atomic.AddUint64(&h.stats.RedirectedAsk, 1)
				// For ASK redirect, execute ASKING followed by the command
				if _, err := h.executeOnNode(ctx, newNode, &RedisCommand{Command: "ASKING"}, username); err != nil {
					h.logger.Warn("Failed to send ASKING command", zap.Error(err))
				}
				node = newNode
				redirections++
				continue
			}

			return nil, err
		}

		// Convert Redis result to bytes
		if result != nil {
			return []byte(fmt.Sprintf("%v\r\n", result)), nil
		}

		return []byte("+OK\r\n"), nil
	}

	return nil, fmt.Errorf("too many redirections")
}

func (h *RedisClusterHandler) executeOnNode(ctx context.Context, node *RedisNode, cmd *RedisCommand, username string) (interface{}, error) {
	if node.Client == nil {
		return nil, fmt.Errorf("no client connection to node %s", node.ID)
	}

	// Increment connection counter for this node
	atomic.AddInt32(&node.Connections, 1)
	defer atomic.AddInt32(&node.Connections, -1)

	start := time.Now()
	defer func() {
		latency := time.Since(start)
		node.Latency = latency

		if stats := h.stats.NodeStats[node.ID]; stats != nil {
			atomic.AddUint64(&stats.Requests, 1)
			stats.Latency = latency
			stats.LastAccess = time.Now()
		}
	}()

	// Prepare Redis command
	doArgs := make([]interface{}, 0, 1+len(cmd.Args))
	doArgs = append(doArgs, cmd.Command)
	for _, arg := range cmd.Args {
		doArgs = append(doArgs, arg)
	}

	// Execute command
	result := node.Client.Do(ctx, doArgs...)
	return result.Val(), result.Err()
}

func (h *RedisClusterHandler) getNodeForCommand(cmd *RedisCommand) *RedisNode {
	h.mu.RLock()
	defer h.mu.RUnlock()

	// For commands with keys, use consistent hashing
	if cmd.Key != "" {
		slot := h.calculateSlot(cmd.Key)
		cmd.Slot = slot

		if slot >= 0 && slot < HASH_SLOTS {
			node := h.slotMap[slot]
			if node != nil && node.Healthy {
				// For read commands, try replicas first if available
				if cmd.IsRead && len(node.Replicas) > 0 {
					// Use NUMA-aware replica selection
					if h.topology != nil {
						for _, replica := range node.Replicas {
							if replica.Healthy && h.isNumaLocal(replica) {
								return replica
							}
						}
					}

					// Fallback to first healthy replica
					for _, replica := range node.Replicas {
						if replica.Healthy {
							return replica
						}
					}
				}

				return node
			}
		}
	}

	// For commands without keys or when slot mapping fails, use any healthy master
	for _, node := range h.clusterNodes {
		if node.Master && node.Healthy {
			return node
		}
	}

	return nil
}

func (h *RedisClusterHandler) calculateSlot(key string) int {
	// Handle hash tags
	start := strings.Index(key, "{")
	if start != -1 {
		end := strings.Index(key[start+1:], "}")
		if end != -1 {
			key = key[start+1 : start+1+end]
		}
	}

	// CRC16 calculation
	crc := uint16(0)
	for _, b := range []byte(key) {
		crc ^= uint16(b) << 8
		for i := 0; i < 8; i++ {
			if crc&0x8000 != 0 {
				crc = (crc << 1) ^ 0x1021
			} else {
				crc = crc << 1
			}
		}
	}

	return int(crc % HASH_SLOTS)
}

func (h *RedisClusterHandler) isNumaLocal(node *RedisNode) bool {
	if h.topology == nil {
		return false
	}

	// Get current CPU's NUMA node
	currentNumaNode := h.topology.GetNumaNodeForCPU(0) // Simplified
	return node.NumaNode == currentNumaNode
}

func (h *RedisClusterHandler) discoverClusterTopology() error {
	h.mu.Lock()
	defer h.mu.Unlock()

	// Get cluster nodes from any connected Redis instance
	clusterNodes, err := h.redis.ClusterNodes(h.ctx).Result()
	if err != nil {
		// Fallback to single node operation
		h.logger.Warn("Failed to discover cluster topology, using single node", zap.Error(err))
		return h.setupSingleNode()
	}

	return h.parseClusterNodes(clusterNodes)
}

func (h *RedisClusterHandler) parseClusterNodes(nodesInfo string) error {
	lines := strings.Split(strings.TrimSpace(nodesInfo), "\n")

	for _, line := range lines {
		if line == "" {
			continue
		}

		parts := strings.Fields(line)
		if len(parts) < 8 {
			continue
		}

		nodeID := parts[0]
		endpoint := parts[1]
		flags := parts[2]
		masterID := parts[3]

		// Parse host:port
		hostPort := strings.Split(endpoint, ":")
		if len(hostPort) < 2 {
			continue
		}

		host := hostPort[0]
		port, err := strconv.Atoi(strings.Split(hostPort[1], "@")[0]) // Remove @cluster-bus-port if present
		if err != nil {
			continue
		}

		// Create node
		node := &RedisNode{
			ID:       nodeID,
			Host:     host,
			Port:     port,
			Master:   strings.Contains(flags, "master"),
			Healthy:  !strings.Contains(flags, "fail"),
			LastSeen: time.Now(),
			NumaNode: h.determineNumaNode(host, port),
		}

		// Create Redis client for this node
		nodeClient := redis.NewClient(&redis.Options{
			Addr:         fmt.Sprintf("%s:%d", host, port),
			DialTimeout:  5 * time.Second,
			ReadTimeout:  5 * time.Second,
			WriteTimeout: 5 * time.Second,
		})

		node.Client = nodeClient
		h.nodeConnections[nodeID] = nodeClient

		// Parse slots if this is a master
		if node.Master && len(parts) > 8 {
			for i := 8; i < len(parts); i++ {
				slotRange := h.parseSlotRange(parts[i])
				if slotRange != nil {
					node.Slots = append(node.Slots, *slotRange)

					// Update slot map
					for slot := slotRange.Start; slot <= slotRange.End; slot++ {
						if slot >= 0 && slot < HASH_SLOTS {
							h.slotMap[slot] = node
						}
					}
				}
			}
		}

		h.clusterNodes[nodeID] = node
		h.stats.NodeStats[nodeID] = &NodeStats{}

		// Link replicas to masters
		if !node.Master && masterID != "-" {
			if master, exists := h.clusterNodes[masterID]; exists {
				master.Replicas = append(master.Replicas, node)
			}
		}
	}

	h.updateStatsCounters()
	h.stats.LastRefresh = time.Now()

	h.logger.Info("Cluster topology discovered",
		zap.Int("total_nodes", h.stats.TotalNodes),
		zap.Int("master_nodes", h.stats.MasterNodes),
		zap.Int("replica_nodes", h.stats.ReplicaNodes))

	return nil
}

func (h *RedisClusterHandler) parseSlotRange(slotStr string) *SlotRange {
	if strings.Contains(slotStr, "-") {
		parts := strings.Split(slotStr, "-")
		if len(parts) == 2 {
			start, err1 := strconv.Atoi(parts[0])
			end, err2 := strconv.Atoi(parts[1])
			if err1 == nil && err2 == nil {
				return &SlotRange{Start: start, End: end}
			}
		}
	} else {
		slot, err := strconv.Atoi(slotStr)
		if err == nil {
			return &SlotRange{Start: slot, End: slot}
		}
	}
	return nil
}

func (h *RedisClusterHandler) determineNumaNode(host string, port int) int {
	// This would determine the NUMA node based on the network interface
	// For now, distribute evenly across NUMA nodes
	if h.topology != nil && len(h.topology.NumaNodes) > 0 {
		hash := 0
		for _, b := range []byte(fmt.Sprintf("%s:%d", host, port)) {
			hash += int(b)
		}
		return hash % len(h.topology.NumaNodes)
	}
	return 0
}

func (h *RedisClusterHandler) setupSingleNode() error {
	// Create a single node representing the connected Redis instance
	node := &RedisNode{
		ID:       "single",
		Host:     "localhost", // This would come from config
		Port:     6379,        // This would come from config
		Master:   true,
		Healthy:  true,
		LastSeen: time.Now(),
		Client:   h.redis,
	}

	// Map all slots to this node
	for i := 0; i < HASH_SLOTS; i++ {
		h.slotMap[i] = node
	}

	h.clusterNodes["single"] = node
	h.stats.NodeStats["single"] = &NodeStats{}
	h.updateStatsCounters()

	return nil
}

func (h *RedisClusterHandler) parseMovedError(errStr string) (bool, *RedisNode) {
	// MOVED 3999 127.0.0.1:7002
	if strings.HasPrefix(errStr, "MOVED") {
		parts := strings.Fields(errStr)
		if len(parts) >= 3 {
			hostPort := parts[2]
			return true, h.findNodeByAddress(hostPort)
		}
	}
	return false, nil
}

func (h *RedisClusterHandler) parseAskError(errStr string) (bool, *RedisNode) {
	// ASK 3999 127.0.0.1:7002
	if strings.HasPrefix(errStr, "ASK") {
		parts := strings.Fields(errStr)
		if len(parts) >= 3 {
			hostPort := parts[2]
			return true, h.findNodeByAddress(hostPort)
		}
	}
	return false, nil
}

func (h *RedisClusterHandler) findNodeByAddress(address string) *RedisNode {
	h.mu.RLock()
	defer h.mu.RUnlock()

	for _, node := range h.clusterNodes {
		if fmt.Sprintf("%s:%d", node.Host, node.Port) == address {
			return node
		}
	}
	return nil
}

func (h *RedisClusterHandler) clusterTopologyRefresh() {
	ticker := time.NewTicker(CLUSTER_REFRESH_INTERVAL)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			if err := h.discoverClusterTopology(); err != nil {
				h.logger.Warn("Failed to refresh cluster topology", zap.Error(err))
			}
		}
	}
}

func (h *RedisClusterHandler) healthMonitor() {
	ticker := time.NewTicker(10 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.checkNodeHealth()
		}
	}
}

func (h *RedisClusterHandler) checkNodeHealth() {
	h.mu.Lock()
	defer h.mu.Unlock()

	for nodeID, node := range h.clusterNodes {
		if node.Client == nil {
			continue
		}

		ctx, cancel := context.WithTimeout(h.ctx, 2*time.Second)
		start := time.Now()

		_, err := node.Client.Ping(ctx).Result()
		latency := time.Since(start)
		cancel()

		if err != nil {
			node.Healthy = false
			if stats := h.stats.NodeStats[nodeID]; stats != nil {
				atomic.AddUint64(&stats.Errors, 1)
			}
		} else {
			node.Healthy = true
			node.Latency = latency
			node.LastSeen = time.Now()
		}
	}

	h.updateStatsCounters()
}

func (h *RedisClusterHandler) updateStatsCounters() {
	totalNodes := len(h.clusterNodes)
	masterNodes := 0
	replicaNodes := 0
	healthyNodes := 0

	for _, node := range h.clusterNodes {
		if node.Master {
			masterNodes++
		} else {
			replicaNodes++
		}

		if node.Healthy {
			healthyNodes++
		}
	}

	h.stats.TotalNodes = totalNodes
	h.stats.MasterNodes = masterNodes
	h.stats.ReplicaNodes = replicaNodes
	h.stats.HealthyNodes = healthyNodes
}

func (h *RedisClusterHandler) statsCollector() {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-h.ctx.Done():
			return
		case <-ticker.C:
			h.collectStats()
		}
	}
}

func (h *RedisClusterHandler) collectStats() {
	h.mu.RLock()
	defer h.mu.RUnlock()

	var totalLatency time.Duration
	validNodes := 0

	for _, node := range h.clusterNodes {
		if node.Healthy && node.Latency > 0 {
			totalLatency += node.Latency
			validNodes++
		}

		atomic.StoreUint64(&node.QPS, 0) // Reset QPS counter
	}

	if validNodes > 0 {
		h.stats.AvgLatency = totalLatency / time.Duration(validNodes)
	}
}

func (h *RedisClusterHandler) processRedisPacket(ctx context.Context, packet *afxdp.Packet) error {
	// High-performance packet processing via AF_XDP
	// This would parse Redis protocol at the packet level
	// and potentially handle simple operations entirely in kernel space

	// For now, just log the packet processing
	h.logger.Debug("Processing Redis packet via AF_XDP",
		zap.Int("length", packet.Length),
		zap.Int("queue_id", packet.QueueID))

	return nil
}

func (h *RedisClusterHandler) parseRedisCommand(line string) *RedisCommand {
	parts := strings.Fields(strings.TrimSpace(line))
	if len(parts) == 0 {
		return nil
	}

	cmd := &RedisCommand{
		Command: strings.ToUpper(parts[0]),
		Args:    parts[1:],
	}

	// Extract key and determine if it's a read operation
	if len(parts) > 1 {
		cmd.Key = parts[1]
	}

	// Classify as read or write operation
	readCommands := map[string]bool{
		"GET": true, "MGET": true, "HGET": true, "HGETALL": true, "HMGET": true,
		"LLEN": true, "LRANGE": true, "SMEMBERS": true, "SCARD": true,
		"ZRANGE": true, "ZCARD": true, "EXISTS": true, "TTL": true, "TYPE": true,
	}

	cmd.IsRead = readCommands[cmd.Command]

	return cmd
}

func (h *RedisClusterHandler) isBlockedRedisCommand(cmd RedisCommand) bool {
	// Block dangerous Redis commands
	dangerousCommands := map[string]bool{
		"FLUSHDB": true, "FLUSHALL": true, "SHUTDOWN": true, "DEBUG": true,
		"CONFIG": true, "EVAL": true, "EVALSHA": true, "SCRIPT": true,
		"CLIENT": true, "MONITOR": true, "SYNC": true, "PSYNC": true,
		"CLUSTER": true, "MODULE": true, "ACL": true,
	}

	return dangerousCommands[cmd.Command]
}

func (h *RedisClusterHandler) sendError(conn net.Conn, message string) error {
	errorResponse := fmt.Sprintf("-ERR %s\r\n", message)
	_, err := conn.Write([]byte(errorResponse))
	return err
}

func (h *RedisClusterHandler) GetStats() *RedisClusterStats {
	return h.stats
}

func (h *RedisClusterHandler) Close() error {
	h.cancel()

	// Close all node connections
	for _, client := range h.nodeConnections {
		client.Close()
	}

	if h.cacheManager != nil {
		h.cacheManager.Close()
	}

	if h.afxdpManager != nil {
		h.afxdpManager.Close()
	}

	h.logger.Info("Redis cluster handler stopped")
	return nil
}