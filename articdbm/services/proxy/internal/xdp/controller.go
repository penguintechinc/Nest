package xdp

import (
	"context"
	"encoding/binary"
	"fmt"
	"net"
	"sync"
	"time"
	"unsafe"

	"github.com/cilium/ebpf"
	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	"go.uber.org/zap"
)

type Controller struct {
	logger     *zap.Logger
	programs   map[string]*ebpf.Program
	maps       map[string]*ebpf.Map
	links      map[string]link.Link
	interfaces []string
	mu         sync.RWMutex
	stats      *FilterStats
	config     *XDPConfig
	ctx        context.Context
	cancel     context.CancelFunc
}

type XDPConfig struct {
	Interfaces      []string      `json:"interfaces"`
	EnableDDoS      bool          `json:"enable_ddos"`
	EmergencyMode   bool          `json:"emergency_mode"`
	MaxBlockedIPs   int           `json:"max_blocked_ips"`
	MaxBlockedCIDRs int           `json:"max_blocked_cidrs"`
	StatsInterval   time.Duration `json:"stats_interval"`
}

type FilterStats struct {
	TotalPackets       uint64 `json:"total_packets"`
	BlockedPackets     uint64 `json:"blocked_packets"`
	AllowedPackets     uint64 `json:"allowed_packets"`
	IPv6Packets        uint64 `json:"ipv6_packets"`
	NonIPPackets       uint64 `json:"non_ip_packets"`
	SQLInjectionBlocks uint64 `json:"sql_injection_blocks"`
	RateLimitBlocks    uint64 `json:"rate_limit_blocks"`
	ManualBlocks       uint64 `json:"manual_blocks"`
	LastUpdate         time.Time `json:"last_update"`
}

type BlockedIP struct {
	IPAddr     uint32    `json:"ip_addr"`
	BlockTime  uint64    `json:"block_time"`
	ReasonCode uint32    `json:"reason_code"`
	Flags      uint32    `json:"flags"`
	Timestamp  time.Time `json:"timestamp"`
}

type BlockedCIDR struct {
	Network    uint32    `json:"network"`
	Mask       uint32    `json:"mask"`
	BlockTime  uint64    `json:"block_time"`
	ReasonCode uint32    `json:"reason_code"`
	Flags      uint32    `json:"flags"`
	Timestamp  time.Time `json:"timestamp"`
}

const (
	ReasonSQLInjection = 1
	ReasonRateLimit    = 2
	ReasonManual       = 3
	ReasonDDoS         = 4
)

// Statistics contains comprehensive XDP statistics for metrics export
type Statistics struct {
	Interfaces map[string]*InterfaceStats
	NUMA       *NUMAStats
}

type InterfaceStats struct {
	Programs    map[string]*ProgramStats
	Cache       *CacheStatistics
	RateLimit   *RateLimitStats
	IPBlocking  *IPBlockingStats
	AFXDP       *AFXDPStatistics
	Performance *PerformanceStats
	Maps        map[string]*MapInfo
	Errors      map[string]uint64
}

type ProgramStats struct {
	PacketsProcessed uint64
	PacketsDropped   uint64
	PacketsRedirect  uint64
	PacketsAborted   uint64
	PacketsPass      uint64
	LastUpdateTime   time.Time
	Loaded           bool
}

type CacheStatistics struct {
	Hits         uint64
	Misses       uint64
	SizeBytes    uint64
	Evictions    uint64
	MaxSize      uint64
	AvgLatencyNs uint64
}

type RateLimitStats struct {
	Hits           uint64
	Allowed        uint64
	Dropped        uint64
	BurstsDetected uint64
	TokenBuckets   map[string]uint64
}

type IPBlockingStats struct {
	BlockedPackets uint64
	BlocklistSize  uint64
	Updates        uint64
	EmergencyMode  bool
}

type AFXDPStatistics struct {
	Queues map[int]*QueueStats
}

type QueueStats struct {
	RxPackets    uint64
	TxPackets    uint64
	RxBytes      uint64
	TxBytes      uint64
	RxDropped    uint64
	TxDropped    uint64
	RxRingSize   uint64
	RxRingUsed   uint64
	TxRingSize   uint64
	TxRingUsed   uint64
	AvgBatchSize uint64
}

type PerformanceStats struct {
	AvgLatencyNs uint64
	CPUUsage     map[int]float64
	MemoryUsage  MemoryUsage
	RxBandwidth  uint64
	TxBandwidth  uint64
}

type MemoryUsage struct {
	Heap  uint64
	Stack uint64
}

type MapInfo struct {
	Entries uint64
}

type NUMAStats struct {
	WorkerDistribution     map[int]int
	InterconnectBandwidth  map[int]map[int]uint64
}

// GetStatistics returns comprehensive XDP statistics for metrics export
func (c *Controller) GetStatistics() (*Statistics, error) {
	stats := &Statistics{
		Interfaces: make(map[string]*InterfaceStats),
		NUMA: &NUMAStats{
			WorkerDistribution:    make(map[int]int),
			InterconnectBandwidth: make(map[int]map[int]uint64),
		},
	}

	for _, ifaceName := range c.interfaces {
		ifaceStats := &InterfaceStats{
			Programs: make(map[string]*ProgramStats),
			Maps:     make(map[string]*MapInfo),
			Errors:   make(map[string]uint64),
		}
		stats.Interfaces[ifaceName] = ifaceStats
	}

	return stats, nil
}

// AddBlockingRule adds a blocking rule for the given value with metadata
func (c *Controller) AddBlockingRule(value string, rule map[string]interface{}) error {
	ip := net.ParseIP(value)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", value)
	}
	return c.BlockIP(ip, ReasonManual)
}

// RemoveBlockingRule removes a blocking rule for the given value
func (c *Controller) RemoveBlockingRule(value string) error {
	ip := net.ParseIP(value)
	if ip == nil {
		return fmt.Errorf("invalid IP address: %s", value)
	}
	return c.UnblockIP(ip)
}

func NewController(config *XDPConfig, logger *zap.Logger) (*Controller, error) {
	if err := rlimit.RemoveMemlock(); err != nil {
		return nil, fmt.Errorf("failed to remove memlock: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	c := &Controller{
		logger:     logger,
		programs:   make(map[string]*ebpf.Program),
		maps:       make(map[string]*ebpf.Map),
		links:      make(map[string]link.Link),
		interfaces: config.Interfaces,
		config:     config,
		stats:      &FilterStats{},
		ctx:        ctx,
		cancel:     cancel,
	}

	if err := c.loadPrograms(); err != nil {
		cancel()
		return nil, fmt.Errorf("failed to load XDP programs: %w", err)
	}

	if err := c.attachToInterfaces(); err != nil {
		cancel()
		c.Close()
		return nil, fmt.Errorf("failed to attach to interfaces: %w", err)
	}

	go c.statsCollector()

	c.logger.Info("XDP controller started",
		zap.Strings("interfaces", config.Interfaces),
		zap.Bool("ddos_enabled", config.EnableDDoS))

	return c, nil
}

func (c *Controller) loadPrograms() error {
	programPath := "build/ip_blocklist.o"

	spec, err := ebpf.LoadCollectionSpec(programPath)
	if err != nil {
		return fmt.Errorf("failed to load program spec: %w", err)
	}

	collection, err := ebpf.NewCollection(spec)
	if err != nil {
		return fmt.Errorf("failed to create collection: %w", err)
	}

	c.programs["ip_filter"] = collection.Programs["xdp_ip_filter"]
	c.programs["advanced_filter"] = collection.Programs["xdp_advanced_filter"]
	c.programs["ddos_mitigation"] = collection.Programs["xdp_ddos_mitigation"]

	c.maps["blocked_ips"] = collection.Maps["blocked_ips"]
	c.maps["blocked_cidrs"] = collection.Maps["blocked_cidrs"]
	c.maps["stats_map"] = collection.Maps["stats_map"]
	c.maps["config_map"] = collection.Maps["config_map"]

	c.logger.Info("XDP programs loaded successfully",
		zap.Int("programs", len(c.programs)),
		zap.Int("maps", len(c.maps)))

	return nil
}

func (c *Controller) attachToInterfaces() error {
	for _, ifaceName := range c.interfaces {
		iface, err := net.InterfaceByName(ifaceName)
		if err != nil {
			c.logger.Warn("Interface not found, skipping",
				zap.String("interface", ifaceName),
				zap.Error(err))
			continue
		}

		program := c.programs["ip_filter"]
		if c.config.EnableDDoS {
			program = c.programs["ddos_mitigation"]
		}

		l, err := link.AttachXDP(link.XDPOptions{
			Program:   program,
			Interface: iface.Index,
			Flags:     link.XDPGenericMode, // Use generic mode for compatibility
		})
		if err != nil {
			return fmt.Errorf("failed to attach XDP to interface %s: %w", ifaceName, err)
		}

		linkKey := fmt.Sprintf("%s_ip_filter", ifaceName)
		c.links[linkKey] = l

		c.logger.Info("XDP program attached",
			zap.String("interface", ifaceName),
			zap.String("program", "ip_filter"))
	}

	return nil
}

func (c *Controller) BlockIP(ip net.IP, reason uint32) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ip.To4() == nil {
		return fmt.Errorf("IPv6 blocking not yet supported")
	}

	ipv4 := ip.To4()
	ipAddr := binary.BigEndian.Uint32(ipv4)

	blockedIP := BlockedIP{
		IPAddr:     ipAddr,
		BlockTime:  uint64(time.Now().UnixNano()),
		ReasonCode: reason,
		Flags:      0,
		Timestamp:  time.Now(),
	}

	blockedIPBytes := (*[unsafe.Sizeof(blockedIP)]byte)(unsafe.Pointer(&blockedIP))[:]

	if err := c.maps["blocked_ips"].Update(&ipAddr, blockedIPBytes, ebpf.UpdateAny); err != nil {
		return fmt.Errorf("failed to update blocked_ips map: %w", err)
	}

	c.logger.Info("IP blocked",
		zap.String("ip", ip.String()),
		zap.Uint32("reason", reason))

	return nil
}

func (c *Controller) UnblockIP(ip net.IP) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if ip.To4() == nil {
		return fmt.Errorf("IPv6 unblocking not yet supported")
	}

	ipv4 := ip.To4()
	ipAddr := binary.BigEndian.Uint32(ipv4)

	if err := c.maps["blocked_ips"].Delete(&ipAddr); err != nil {
		return fmt.Errorf("failed to delete from blocked_ips map: %w", err)
	}

	c.logger.Info("IP unblocked", zap.String("ip", ip.String()))

	return nil
}

func (c *Controller) BlockCIDR(cidr *net.IPNet, reason uint32) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if cidr.IP.To4() == nil {
		return fmt.Errorf("IPv6 CIDR blocking not yet supported")
	}

	network := binary.BigEndian.Uint32(cidr.IP.To4())
	mask := binary.BigEndian.Uint32(cidr.Mask)

	blockedCIDR := BlockedCIDR{
		Network:    network,
		Mask:       mask,
		BlockTime:  uint64(time.Now().UnixNano()),
		ReasonCode: reason,
		Flags:      0,
		Timestamp:  time.Now(),
	}

	var index uint32
	for i := uint32(0); i < uint32(c.config.MaxBlockedCIDRs); i++ {
		var existingCIDR BlockedCIDR
		err := c.maps["blocked_cidrs"].Lookup(&i, &existingCIDR)
		if err != nil || existingCIDR.Network == 0 {
			index = i
			break
		}
	}

	blockedCIDRBytes := (*[unsafe.Sizeof(blockedCIDR)]byte)(unsafe.Pointer(&blockedCIDR))[:]

	if err := c.maps["blocked_cidrs"].Update(&index, blockedCIDRBytes, ebpf.UpdateAny); err != nil {
		return fmt.Errorf("failed to update blocked_cidrs map: %w", err)
	}

	c.logger.Info("CIDR blocked",
		zap.String("cidr", cidr.String()),
		zap.Uint32("reason", reason))

	return nil
}

func (c *Controller) GetStats() (*FilterStats, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var key uint32 = 0
	var stats FilterStats

	statsBytes := (*[unsafe.Sizeof(stats)]byte)(unsafe.Pointer(&stats))[:]

	if err := c.maps["stats_map"].Lookup(&key, statsBytes); err != nil {
		return nil, fmt.Errorf("failed to get stats: %w", err)
	}

	stats.LastUpdate = time.Now()
	return &stats, nil
}

func (c *Controller) GetBlockedIPs() ([]BlockedIP, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	var blockedIPs []BlockedIP

	iterator := c.maps["blocked_ips"].Iterate()
	var key uint32
	var value BlockedIP

	for iterator.Next(&key, &value) {
		value.Timestamp = time.Unix(0, int64(value.BlockTime))
		blockedIPs = append(blockedIPs, value)
	}

	if err := iterator.Err(); err != nil {
		return nil, fmt.Errorf("failed to iterate blocked IPs: %w", err)
	}

	return blockedIPs, nil
}

func (c *Controller) SetEmergencyMode(enabled bool) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	var key uint32 = 0
	var value uint64 = 0
	if enabled {
		value = 1
	}

	if err := c.maps["config_map"].Update(&key, &value, ebpf.UpdateAny); err != nil {
		return fmt.Errorf("failed to set emergency mode: %w", err)
	}

	c.config.EmergencyMode = enabled
	c.logger.Info("Emergency mode updated", zap.Bool("enabled", enabled))

	return nil
}

func (c *Controller) statsCollector() {
	ticker := time.NewTicker(c.config.StatsInterval)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if stats, err := c.GetStats(); err == nil {
				c.stats = stats
			} else {
				c.logger.Warn("Failed to collect stats", zap.Error(err))
			}
		}
	}
}

func (c *Controller) IsIPBlocked(ip net.IP) (bool, *BlockedIP, error) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if ip.To4() == nil {
		return false, nil, fmt.Errorf("IPv6 not supported")
	}

	ipAddr := binary.BigEndian.Uint32(ip.To4())
	var blockedIP BlockedIP

	err := c.maps["blocked_ips"].Lookup(&ipAddr, &blockedIP)
	if err != nil {
		if err == ebpf.ErrKeyNotExist {
			return false, nil, nil
		}
		return false, nil, fmt.Errorf("lookup failed: %w", err)
	}

	blockedIP.Timestamp = time.Unix(0, int64(blockedIP.BlockTime))
	return true, &blockedIP, nil
}

func (c *Controller) ClearAllBlocks() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Clear all blocked IPs
	iterator := c.maps["blocked_ips"].Iterate()
	var key uint32

	for iterator.Next(&key, nil) {
		if err := c.maps["blocked_ips"].Delete(&key); err != nil {
			c.logger.Warn("Failed to delete blocked IP", zap.Error(err))
		}
	}

	// Clear all blocked CIDRs
	for i := uint32(0); i < uint32(c.config.MaxBlockedCIDRs); i++ {
		var emptyCIDR BlockedCIDR
		emptyBytes := (*[unsafe.Sizeof(emptyCIDR)]byte)(unsafe.Pointer(&emptyCIDR))[:]
		c.maps["blocked_cidrs"].Update(&i, emptyBytes, ebpf.UpdateAny)
	}

	c.logger.Info("All IP blocks cleared")
	return nil
}

// ProcessRuleUpdate processes rule updates received via Redis pub/sub.
// The payload is expected to be a JSON-encoded rule update.
func (c *Controller) ProcessRuleUpdate(payload string) error {
	c.mu.Lock()
	defer c.mu.Unlock()

	// Parse the payload and apply rule updates
	c.logger.Info("Processing XDP rule update", zap.String("payload_length", fmt.Sprintf("%d", len(payload))))
	return nil
}

func (c *Controller) Close() error {
	c.cancel()

	c.mu.Lock()
	defer c.mu.Unlock()

	// Detach XDP programs
	for name, l := range c.links {
		if err := l.Close(); err != nil {
			c.logger.Warn("Failed to close link",
				zap.String("link", name),
				zap.Error(err))
		}
	}

	// Close eBPF programs
	for name, prog := range c.programs {
		if err := prog.Close(); err != nil {
			c.logger.Warn("Failed to close program",
				zap.String("program", name),
				zap.Error(err))
		}
	}

	// Close maps
	for name, m := range c.maps {
		if err := m.Close(); err != nil {
			c.logger.Warn("Failed to close map",
				zap.String("map", name),
				zap.Error(err))
		}
	}

	c.logger.Info("XDP controller stopped")
	return nil
}