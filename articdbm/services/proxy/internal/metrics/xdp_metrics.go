package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"go.uber.org/zap"

	"github.com/penguintechinc/articdbm/proxy/internal/numa"
	"github.com/penguintechinc/articdbm/proxy/internal/xdp"
)

// XDPMetricsExporter exports XDP statistics to Prometheus
type XDPMetricsExporter struct {
	xdpController *xdp.Controller
	topology      *numa.TopologyInfo
	logger        *zap.Logger

	// XDP Packet Processing Metrics
	xdpPacketsProcessed *prometheus.CounterVec
	xdpPacketsDropped   *prometheus.CounterVec
	xdpPacketsRedirect  *prometheus.CounterVec
	xdpPacketsAborted   *prometheus.CounterVec
	xdpPacketsPass      *prometheus.CounterVec

	// XDP Cache Metrics
	xdpCacheHits         *prometheus.CounterVec
	xdpCacheMisses       *prometheus.CounterVec
	xdpCacheSize         *prometheus.GaugeVec
	xdpCacheUtilization  *prometheus.GaugeVec
	xdpCacheEvictions    *prometheus.CounterVec
	xdpCacheLatency      *prometheus.HistogramVec

	// XDP Rate Limiting Metrics
	xdpRateLimitHits     *prometheus.CounterVec
	xdpRateLimitAllowed  *prometheus.CounterVec
	xdpRateLimitDropped  *prometheus.CounterVec
	xdpTokenBucketLevel  *prometheus.GaugeVec
	xdpBurstDetected     *prometheus.CounterVec

	// XDP IP Blocking Metrics
	xdpBlockedIPs        *prometheus.CounterVec
	xdpBlockedPackets    *prometheus.CounterVec
	xdpBlocklistSize     *prometheus.GaugeVec
	xdpBlocklistUpdates  *prometheus.CounterVec
	xdpEmergencyMode     *prometheus.GaugeVec

	// AF_XDP Zero-Copy Metrics
	afxdpRxPackets       *prometheus.CounterVec
	afxdpTxPackets       *prometheus.CounterVec
	afxdpRxBytes         *prometheus.CounterVec
	afxdpTxBytes         *prometheus.CounterVec
	afxdpRxDropped       *prometheus.CounterVec
	afxdpTxDropped       *prometheus.CounterVec
	afxdpRingUtilization *prometheus.GaugeVec
	afxdpBatchSize       *prometheus.HistogramVec

	// NUMA Optimization Metrics
	numaLocalMemory      *prometheus.GaugeVec
	numaRemoteMemory     *prometheus.GaugeVec
	numaCPUUtilization   *prometheus.GaugeVec
	numaInterconnectBW   *prometheus.GaugeVec
	numaWorkerDistrib    *prometheus.GaugeVec

	// Performance Metrics
	xdpProcessingLatency *prometheus.HistogramVec
	xdpCPUUsage         *prometheus.GaugeVec
	xdpMemoryUsage      *prometheus.GaugeVec
	xdpPPS              *prometheus.GaugeVec
	xdpBandwidth        *prometheus.GaugeVec

	// Health and Status Metrics
	xdpProgramStatus     *prometheus.GaugeVec
	xdpMapSize          *prometheus.GaugeVec
	xdpVerifierStats    *prometheus.GaugeVec
	xdpReloadCount      *prometheus.CounterVec
	xdpErrors           *prometheus.CounterVec

	mu            sync.RWMutex
	ctx           context.Context
	cancel        context.CancelFunc
	collectInterval time.Duration
}

func NewXDPMetricsExporter(xdpController *xdp.Controller, topology *numa.TopologyInfo, logger *zap.Logger) *XDPMetricsExporter {
	ctx, cancel := context.WithCancel(context.Background())

	exporter := &XDPMetricsExporter{
		xdpController:   xdpController,
		topology:       topology,
		logger:         logger,
		ctx:            ctx,
		cancel:         cancel,
		collectInterval: 10 * time.Second,
	}

	exporter.initMetrics()
	go exporter.collectLoop()

	logger.Info("XDP metrics exporter initialized")
	return exporter
}

func (e *XDPMetricsExporter) initMetrics() {
	// XDP Packet Processing Metrics
	e.xdpPacketsProcessed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_packets_processed_total",
			Help: "Total number of packets processed by XDP programs",
		},
		[]string{"interface", "program", "numa_node"},
	)

	e.xdpPacketsDropped = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_packets_dropped_total",
			Help: "Total number of packets dropped by XDP programs",
		},
		[]string{"interface", "program", "reason", "numa_node"},
	)

	e.xdpPacketsRedirect = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_packets_redirect_total",
			Help: "Total number of packets redirected by XDP programs",
		},
		[]string{"interface", "program", "target", "numa_node"},
	)

	e.xdpPacketsAborted = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_packets_aborted_total",
			Help: "Total number of packets aborted by XDP programs",
		},
		[]string{"interface", "program", "error", "numa_node"},
	)

	e.xdpPacketsPass = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_packets_pass_total",
			Help: "Total number of packets passed through by XDP programs",
		},
		[]string{"interface", "program", "numa_node"},
	)

	// XDP Cache Metrics
	e.xdpCacheHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_cache_hits_total",
			Help: "Total number of XDP cache hits",
		},
		[]string{"interface", "cache_type", "numa_node"},
	)

	e.xdpCacheMisses = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_cache_misses_total",
			Help: "Total number of XDP cache misses",
		},
		[]string{"interface", "cache_type", "numa_node"},
	)

	e.xdpCacheSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_xdp_cache_size_bytes",
			Help: "Current size of XDP cache in bytes",
		},
		[]string{"interface", "cache_type", "numa_node"},
	)

	e.xdpCacheUtilization = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_xdp_cache_utilization_ratio",
			Help: "XDP cache utilization ratio (0.0-1.0)",
		},
		[]string{"interface", "cache_type", "numa_node"},
	)

	e.xdpCacheEvictions = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_cache_evictions_total",
			Help: "Total number of XDP cache evictions",
		},
		[]string{"interface", "cache_type", "reason", "numa_node"},
	)

	e.xdpCacheLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "articdbm_xdp_cache_lookup_duration_seconds",
			Help:    "XDP cache lookup latency in seconds",
			Buckets: prometheus.ExponentialBuckets(0.000001, 2, 15), // 1µs to 16ms
		},
		[]string{"interface", "cache_type", "result", "numa_node"},
	)

	// XDP Rate Limiting Metrics
	e.xdpRateLimitHits = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_rate_limit_hits_total",
			Help: "Total number of rate limit hits",
		},
		[]string{"interface", "limit_type", "numa_node"},
	)

	e.xdpRateLimitAllowed = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_rate_limit_allowed_total",
			Help: "Total number of rate limit allowed requests",
		},
		[]string{"interface", "limit_type", "numa_node"},
	)

	e.xdpRateLimitDropped = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_rate_limit_dropped_total",
			Help: "Total number of rate limit dropped requests",
		},
		[]string{"interface", "limit_type", "numa_node"},
	)

	e.xdpTokenBucketLevel = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_xdp_token_bucket_level",
			Help: "Current token bucket level for rate limiting",
		},
		[]string{"interface", "bucket_id", "numa_node"},
	)

	e.xdpBurstDetected = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_burst_detected_total",
			Help: "Total number of burst patterns detected",
		},
		[]string{"interface", "burst_type", "numa_node"},
	)

	// XDP IP Blocking Metrics
	e.xdpBlockedIPs = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_blocked_ips_total",
			Help: "Total number of unique IPs blocked",
		},
		[]string{"interface", "block_type", "numa_node"},
	)

	e.xdpBlockedPackets = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_blocked_packets_total",
			Help: "Total number of packets blocked by IP filtering",
		},
		[]string{"interface", "block_reason", "numa_node"},
	)

	e.xdpBlocklistSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_xdp_blocklist_size",
			Help: "Current size of IP blocklist",
		},
		[]string{"interface", "list_type", "numa_node"},
	)

	e.xdpBlocklistUpdates = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_blocklist_updates_total",
			Help: "Total number of blocklist updates",
		},
		[]string{"interface", "update_type", "numa_node"},
	)

	e.xdpEmergencyMode = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_xdp_emergency_mode_active",
			Help: "Whether XDP emergency mode is active (1=active, 0=inactive)",
		},
		[]string{"interface", "mode_type", "numa_node"},
	)

	// AF_XDP Zero-Copy Metrics
	e.afxdpRxPackets = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_afxdp_rx_packets_total",
			Help: "Total number of packets received via AF_XDP",
		},
		[]string{"interface", "queue", "numa_node"},
	)

	e.afxdpTxPackets = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_afxdp_tx_packets_total",
			Help: "Total number of packets transmitted via AF_XDP",
		},
		[]string{"interface", "queue", "numa_node"},
	)

	e.afxdpRxBytes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_afxdp_rx_bytes_total",
			Help: "Total number of bytes received via AF_XDP",
		},
		[]string{"interface", "queue", "numa_node"},
	)

	e.afxdpTxBytes = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_afxdp_tx_bytes_total",
			Help: "Total number of bytes transmitted via AF_XDP",
		},
		[]string{"interface", "queue", "numa_node"},
	)

	e.afxdpRxDropped = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_afxdp_rx_dropped_total",
			Help: "Total number of packets dropped on RX ring",
		},
		[]string{"interface", "queue", "reason", "numa_node"},
	)

	e.afxdpTxDropped = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_afxdp_tx_dropped_total",
			Help: "Total number of packets dropped on TX ring",
		},
		[]string{"interface", "queue", "reason", "numa_node"},
	)

	e.afxdpRingUtilization = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_afxdp_ring_utilization_ratio",
			Help: "AF_XDP ring buffer utilization ratio (0.0-1.0)",
		},
		[]string{"interface", "queue", "ring_type", "numa_node"},
	)

	e.afxdpBatchSize = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "articdbm_afxdp_batch_size",
			Help:    "AF_XDP packet batch size distribution",
			Buckets: []float64{1, 2, 4, 8, 16, 32, 64, 128, 256, 512},
		},
		[]string{"interface", "queue", "direction", "numa_node"},
	)

	// NUMA Optimization Metrics
	e.numaLocalMemory = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_numa_local_memory_bytes",
			Help: "Local NUMA node memory usage in bytes",
		},
		[]string{"numa_node", "memory_type"},
	)

	e.numaRemoteMemory = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_numa_remote_memory_bytes",
			Help: "Remote NUMA node memory usage in bytes",
		},
		[]string{"numa_node", "remote_node", "memory_type"},
	)

	e.numaCPUUtilization = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_numa_cpu_utilization_ratio",
			Help: "NUMA node CPU utilization ratio (0.0-1.0)",
		},
		[]string{"numa_node", "cpu_type"},
	)

	e.numaInterconnectBW = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_numa_interconnect_bandwidth_bps",
			Help: "NUMA interconnect bandwidth usage in bytes per second",
		},
		[]string{"src_node", "dst_node"},
	)

	e.numaWorkerDistrib = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_numa_worker_distribution",
			Help: "Number of workers per NUMA node",
		},
		[]string{"numa_node", "worker_type"},
	)

	// Performance Metrics
	e.xdpProcessingLatency = promauto.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "articdbm_xdp_processing_duration_seconds",
			Help:    "XDP packet processing latency in seconds",
			Buckets: prometheus.ExponentialBuckets(0.000001, 2, 15), // 1µs to 16ms
		},
		[]string{"interface", "program", "numa_node"},
	)

	e.xdpCPUUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_xdp_cpu_usage_ratio",
			Help: "XDP CPU usage ratio (0.0-1.0)",
		},
		[]string{"interface", "cpu_id", "numa_node"},
	)

	e.xdpMemoryUsage = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_xdp_memory_usage_bytes",
			Help: "XDP memory usage in bytes",
		},
		[]string{"interface", "memory_type", "numa_node"},
	)

	e.xdpPPS = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_xdp_packets_per_second",
			Help: "XDP packets processed per second",
		},
		[]string{"interface", "program", "numa_node"},
	)

	e.xdpBandwidth = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_xdp_bandwidth_bps",
			Help: "XDP bandwidth processed in bytes per second",
		},
		[]string{"interface", "direction", "numa_node"},
	)

	// Health and Status Metrics
	e.xdpProgramStatus = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_xdp_program_status",
			Help: "XDP program status (1=loaded, 0=not loaded)",
		},
		[]string{"interface", "program", "numa_node"},
	)

	e.xdpMapSize = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_xdp_map_size_entries",
			Help: "Number of entries in XDP maps",
		},
		[]string{"interface", "map_name", "numa_node"},
	)

	e.xdpVerifierStats = promauto.NewGaugeVec(
		prometheus.GaugeOpts{
			Name: "articdbm_xdp_verifier_stats",
			Help: "XDP verifier statistics",
		},
		[]string{"interface", "stat_type"},
	)

	e.xdpReloadCount = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_reload_total",
			Help: "Total number of XDP program reloads",
		},
		[]string{"interface", "program", "reason"},
	)

	e.xdpErrors = promauto.NewCounterVec(
		prometheus.CounterOpts{
			Name: "articdbm_xdp_errors_total",
			Help: "Total number of XDP errors",
		},
		[]string{"interface", "error_type", "numa_node"},
	)
}

func (e *XDPMetricsExporter) collectLoop() {
	ticker := time.NewTicker(e.collectInterval)
	defer ticker.Stop()

	for {
		select {
		case <-e.ctx.Done():
			return
		case <-ticker.C:
			e.collectMetrics()
		}
	}
}

func (e *XDPMetricsExporter) collectMetrics() {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.xdpController == nil {
		return
	}

	// Collect XDP statistics
	stats, err := e.xdpController.GetStatistics()
	if err != nil {
		e.logger.Error("Failed to collect XDP statistics", zap.Error(err))
		return
	}

	e.updatePacketProcessingMetrics(stats)
	e.updateCacheMetrics(stats)
	e.updateRateLimitingMetrics(stats)
	e.updateIPBlockingMetrics(stats)
	e.updateAFXDPMetrics(stats)
	e.updateNUMAMetrics(stats)
	e.updatePerformanceMetrics(stats)
	e.updateHealthMetrics(stats)
}

func (e *XDPMetricsExporter) updatePacketProcessingMetrics(stats *xdp.Statistics) {
	for iface, ifaceStats := range stats.Interfaces {
		numaNode := e.getInterfaceNUMANode(iface)

		for program, progStats := range ifaceStats.Programs {
			labels := []string{iface, program, numaNode}

			e.xdpPacketsProcessed.WithLabelValues(labels...).Add(float64(progStats.PacketsProcessed))
			e.xdpPacketsDropped.WithLabelValues(append(labels, "security")...).Add(float64(progStats.PacketsDropped))
			e.xdpPacketsRedirect.WithLabelValues(append(labels, "userspace")...).Add(float64(progStats.PacketsRedirect))
			e.xdpPacketsAborted.WithLabelValues(append(labels, "error")...).Add(float64(progStats.PacketsAborted))
			e.xdpPacketsPass.WithLabelValues(labels...).Add(float64(progStats.PacketsPass))

			// Calculate PPS
			if progStats.LastUpdateTime.Unix() > 0 {
				duration := time.Since(progStats.LastUpdateTime).Seconds()
				if duration > 0 {
					pps := float64(progStats.PacketsProcessed) / duration
					e.xdpPPS.WithLabelValues(labels...).Set(pps)
				}
			}
		}
	}
}

func (e *XDPMetricsExporter) updateCacheMetrics(stats *xdp.Statistics) {
	for iface, ifaceStats := range stats.Interfaces {
		numaNode := e.getInterfaceNUMANode(iface)

		if cacheStats := ifaceStats.Cache; cacheStats != nil {
			labels := []string{iface, "query", numaNode}

			e.xdpCacheHits.WithLabelValues(labels...).Add(float64(cacheStats.Hits))
			e.xdpCacheMisses.WithLabelValues(labels...).Add(float64(cacheStats.Misses))
			e.xdpCacheSize.WithLabelValues(labels...).Set(float64(cacheStats.SizeBytes))
			e.xdpCacheEvictions.WithLabelValues(append(labels, "lru")...).Add(float64(cacheStats.Evictions))

			// Calculate cache utilization
			if cacheStats.MaxSize > 0 {
				utilization := float64(cacheStats.SizeBytes) / float64(cacheStats.MaxSize)
				e.xdpCacheUtilization.WithLabelValues(labels...).Set(utilization)
			}

			// Cache latency histogram
			if cacheStats.AvgLatencyNs > 0 {
				latencySeconds := float64(cacheStats.AvgLatencyNs) / 1e9
				e.xdpCacheLatency.WithLabelValues(append(labels, "hit")...).Observe(latencySeconds)
			}
		}
	}
}

func (e *XDPMetricsExporter) updateRateLimitingMetrics(stats *xdp.Statistics) {
	for iface, ifaceStats := range stats.Interfaces {
		numaNode := e.getInterfaceNUMANode(iface)

		if rlStats := ifaceStats.RateLimit; rlStats != nil {
			labels := []string{iface, "global", numaNode}

			e.xdpRateLimitHits.WithLabelValues(labels...).Add(float64(rlStats.Hits))
			e.xdpRateLimitAllowed.WithLabelValues(labels...).Add(float64(rlStats.Allowed))
			e.xdpRateLimitDropped.WithLabelValues(labels...).Add(float64(rlStats.Dropped))
			e.xdpBurstDetected.WithLabelValues(append(labels[:2], "traffic", numaNode)...).Add(float64(rlStats.BurstsDetected))

			// Token bucket levels
			for bucketID, level := range rlStats.TokenBuckets {
				e.xdpTokenBucketLevel.WithLabelValues(iface, bucketID, numaNode).Set(float64(level))
			}
		}
	}
}

func (e *XDPMetricsExporter) updateIPBlockingMetrics(stats *xdp.Statistics) {
	for iface, ifaceStats := range stats.Interfaces {
		numaNode := e.getInterfaceNUMANode(iface)

		if blockStats := ifaceStats.IPBlocking; blockStats != nil {
			labels := []string{iface, "ipv4", numaNode}

			e.xdpBlockedPackets.WithLabelValues(append(labels[:1], "security", numaNode)...).Add(float64(blockStats.BlockedPackets))
			e.xdpBlocklistSize.WithLabelValues(append(labels[:1], "ipv4", numaNode)...).Set(float64(blockStats.BlocklistSize))
			e.xdpBlocklistUpdates.WithLabelValues(append(labels[:1], "add", numaNode)...).Add(float64(blockStats.Updates))

			// Emergency mode status
			emergencyMode := float64(0)
			if blockStats.EmergencyMode {
				emergencyMode = 1
			}
			e.xdpEmergencyMode.WithLabelValues(iface, "ddos", numaNode).Set(emergencyMode)
		}
	}
}

func (e *XDPMetricsExporter) updateAFXDPMetrics(stats *xdp.Statistics) {
	for iface, ifaceStats := range stats.Interfaces {
		numaNode := e.getInterfaceNUMANode(iface)

		if afxdpStats := ifaceStats.AFXDP; afxdpStats != nil {
			for queueID, queueStats := range afxdpStats.Queues {
				queueStr := fmt.Sprintf("%d", queueID)
				labels := []string{iface, queueStr, numaNode}

				e.afxdpRxPackets.WithLabelValues(labels...).Add(float64(queueStats.RxPackets))
				e.afxdpTxPackets.WithLabelValues(labels...).Add(float64(queueStats.TxPackets))
				e.afxdpRxBytes.WithLabelValues(labels...).Add(float64(queueStats.RxBytes))
				e.afxdpTxBytes.WithLabelValues(labels...).Add(float64(queueStats.TxBytes))
				e.afxdpRxDropped.WithLabelValues(append(labels, "overflow")...).Add(float64(queueStats.RxDropped))
				e.afxdpTxDropped.WithLabelValues(append(labels, "overflow")...).Add(float64(queueStats.TxDropped))

				// Ring utilization
				if queueStats.RxRingSize > 0 {
					rxUtil := float64(queueStats.RxRingUsed) / float64(queueStats.RxRingSize)
					e.afxdpRingUtilization.WithLabelValues(append(labels, "rx")...).Set(rxUtil)
				}

				if queueStats.TxRingSize > 0 {
					txUtil := float64(queueStats.TxRingUsed) / float64(queueStats.TxRingSize)
					e.afxdpRingUtilization.WithLabelValues(append(labels, "tx")...).Set(txUtil)
				}

				// Batch size distribution
				if queueStats.AvgBatchSize > 0 {
					e.afxdpBatchSize.WithLabelValues(append(labels, "rx")...).Observe(float64(queueStats.AvgBatchSize))
				}
			}
		}
	}
}

func (e *XDPMetricsExporter) updateNUMAMetrics(stats *xdp.Statistics) {
	if e.topology == nil {
		return
	}

	for nodeID, node := range e.topology.Nodes {
		nodeStr := fmt.Sprintf("%d", nodeID)

		// Memory usage
		e.numaLocalMemory.WithLabelValues(nodeStr, "used").Set(float64(node.MemoryUsed))
		e.numaLocalMemory.WithLabelValues(nodeStr, "free").Set(float64(node.MemoryFree))

		// CPU utilization
		for cpuID, cpuUsage := range node.CPUUsage {
			e.numaCPUUtilization.WithLabelValues(nodeStr, fmt.Sprintf("cpu%d", cpuID)).Set(cpuUsage)
		}

		// Worker distribution
		if stats.NUMA != nil {
			if workerCount, ok := stats.NUMA.WorkerDistribution[nodeID]; ok {
				e.numaWorkerDistrib.WithLabelValues(nodeStr, "xdp").Set(float64(workerCount))
			}
		}
	}

	// Interconnect bandwidth
	if stats.NUMA != nil {
		for srcNode, dstNodes := range stats.NUMA.InterconnectBandwidth {
			for dstNode, bandwidth := range dstNodes {
				e.numaInterconnectBW.WithLabelValues(
					fmt.Sprintf("%d", srcNode),
					fmt.Sprintf("%d", dstNode),
				).Set(float64(bandwidth))
			}
		}
	}
}

func (e *XDPMetricsExporter) updatePerformanceMetrics(stats *xdp.Statistics) {
	for iface, ifaceStats := range stats.Interfaces {
		numaNode := e.getInterfaceNUMANode(iface)

		if perfStats := ifaceStats.Performance; perfStats != nil {
			// Processing latency
			if perfStats.AvgLatencyNs > 0 {
				latencySeconds := float64(perfStats.AvgLatencyNs) / 1e9
				e.xdpProcessingLatency.WithLabelValues(iface, "total", numaNode).Observe(latencySeconds)
			}

			// CPU usage
			for cpuID, usage := range perfStats.CPUUsage {
				e.xdpCPUUsage.WithLabelValues(iface, fmt.Sprintf("cpu%d", cpuID), numaNode).Set(usage)
			}

			// Memory usage
			e.xdpMemoryUsage.WithLabelValues(iface, "heap", numaNode).Set(float64(perfStats.MemoryUsage.Heap))
			e.xdpMemoryUsage.WithLabelValues(iface, "stack", numaNode).Set(float64(perfStats.MemoryUsage.Stack))

			// Bandwidth
			e.xdpBandwidth.WithLabelValues(iface, "rx", numaNode).Set(float64(perfStats.RxBandwidth))
			e.xdpBandwidth.WithLabelValues(iface, "tx", numaNode).Set(float64(perfStats.TxBandwidth))
		}
	}
}

func (e *XDPMetricsExporter) updateHealthMetrics(stats *xdp.Statistics) {
	for iface, ifaceStats := range stats.Interfaces {
		numaNode := e.getInterfaceNUMANode(iface)

		// Program status
		for program, progStats := range ifaceStats.Programs {
			status := float64(0)
			if progStats.Loaded {
				status = 1
			}
			e.xdpProgramStatus.WithLabelValues(iface, program, numaNode).Set(status)
		}

		// Map sizes
		if mapStats := ifaceStats.Maps; mapStats != nil {
			for mapName, mapInfo := range mapStats {
				e.xdpMapSize.WithLabelValues(iface, mapName, numaNode).Set(float64(mapInfo.Entries))
			}
		}

		// Error counts
		if errorStats := ifaceStats.Errors; errorStats != nil {
			for errorType, count := range errorStats {
				e.xdpErrors.WithLabelValues(iface, errorType, numaNode).Add(float64(count))
			}
		}
	}
}

func (e *XDPMetricsExporter) getInterfaceNUMANode(iface string) string {
	if e.topology == nil {
		return "unknown"
	}

	// Find NUMA node for interface
	for nodeID, node := range e.topology.Nodes {
		for _, nic := range node.NICs {
			if nic.Name == iface {
				return fmt.Sprintf("%d", nodeID)
			}
		}
	}

	return "unknown"
}

// SetCollectInterval changes the metrics collection interval
func (e *XDPMetricsExporter) SetCollectInterval(interval time.Duration) {
	e.collectInterval = interval
}

// GetMetrics returns current metrics as JSON
func (e *XDPMetricsExporter) GetMetrics() ([]byte, error) {
	if e.xdpController == nil {
		return nil, fmt.Errorf("XDP controller not available")
	}

	stats, err := e.xdpController.GetStatistics()
	if err != nil {
		return nil, err
	}

	return json.Marshal(stats)
}

// Close shuts down the metrics exporter
func (e *XDPMetricsExporter) Close() error {
	e.cancel()
	e.logger.Info("XDP metrics exporter stopped")
	return nil
}