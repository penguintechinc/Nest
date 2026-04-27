package main

import (
	"context"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// AgentConfig holds node-agent configuration
type AgentConfig struct {
	NodeName     string
	ScanInterval time.Duration
	MetricsAddr  string
	Logger       *zap.Logger
}

// Agent is the main node-agent struct
type Agent struct {
	cfg            AgentConfig
	inventory      *InventoryCollector
	metrics        *AgentMetrics
	publisher      *CRPublisher
	spindown       *SpindownTracker
	rma            *RMAManager
	prevBlockStats map[string]*BlockStats // keyed by device serial; tracks delta for counters
}

// AgentMetrics holds Prometheus metrics exported by the node-agent
type AgentMetrics struct {
	driveHealth        *prometheus.GaugeVec
	driveWearPercent   *prometheus.GaugeVec
	driveTemperature   *prometheus.GaugeVec
	driveCapacityBytes *prometheus.GaugeVec
	darkDriveCount     prometheus.Gauge
	scanDuration       prometheus.Histogram

	// Block-layer I/O counters (cumulative)
	driveReadBytesTotal  *prometheus.CounterVec // nest_drive_read_bytes_total
	driveWriteBytesTotal *prometheus.CounterVec // nest_drive_write_bytes_total

	// Block-layer I/O gauges (point-in-time / 15s avg approximation)
	driveReadIops   *prometheus.GaugeVec // nest_drive_read_iops
	driveWriteIops  *prometheus.GaugeVec // nest_drive_write_iops
	driveQueueDepth *prometheus.GaugeVec // nest_drive_queue_depth

	// SMART gauges
	driveHoursOn        *prometheus.GaugeVec // nest_drive_smart_hours_on
	driveReallocSectors *prometheus.GaugeVec // nest_drive_smart_reallocated_sectors_total

	// Hardware pool capacity (placeholder until HardwarePool CRDs are populated)
	hwPoolFreeBytes *prometheus.GaugeVec // nest_hardware_pool_free_bytes  labels: pool, class
	hwPoolUsedBytes *prometheus.GaugeVec // nest_hardware_pool_used_bytes  labels: pool, class
}

func newAgentMetrics() *AgentMetrics {
	return newAgentMetricsWithRegistry(prometheus.DefaultRegisterer)
}

func newAgentMetricsWithRegistry(reg prometheus.Registerer) *AgentMetrics {
	m := &AgentMetrics{
		driveHealth: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nest_drive_health",
			Help: "Drive health state: 0=healthy 1=warn 2=degraded 3=failed",
		}, []string{"node", "serial", "class", "model"}),
		driveWearPercent: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nest_drive_smart_wear_percent",
			Help: "Drive media wear percentage (0-100)",
		}, []string{"node", "serial"}),
		driveTemperature: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nest_drive_smart_temperature_celsius",
			Help: "Drive temperature in Celsius",
		}, []string{"node", "serial"}),
		driveCapacityBytes: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name: "nest_drive_capacity_bytes",
			Help: "Drive raw capacity in bytes",
		}, []string{"node", "serial"}),
		darkDriveCount: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "nest_dark_drive_discovered_total",
			Help: "Number of dark (unadopted) drives on this node",
		}),
		scanDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "nest_node_agent_scan_duration_seconds",
			Help:    "Time taken for a full drive inventory scan",
			Buckets: prometheus.DefBuckets,
		}),
	}
	m.driveReadBytesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "nest_drive_read_bytes_total",
		Help: "Total bytes read from the drive (cumulative)",
	}, []string{"node", "serial"})
	m.driveWriteBytesTotal = prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: "nest_drive_write_bytes_total",
		Help: "Total bytes written to the drive (cumulative)",
	}, []string{"node", "serial"})
	m.driveReadIops = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nest_drive_read_iops",
		Help: "Drive read IOPS (reads completed, from /proc/diskstats)",
	}, []string{"node", "serial"})
	m.driveWriteIops = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nest_drive_write_iops",
		Help: "Drive write IOPS (writes completed, from /proc/diskstats)",
	}, []string{"node", "serial"})
	m.driveQueueDepth = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nest_drive_queue_depth",
		Help: "Drive I/O queue depth (io_in_progress from /proc/diskstats)",
	}, []string{"node", "serial"})
	m.driveHoursOn = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nest_drive_smart_hours_on",
		Help: "Drive power-on hours reported by SMART",
	}, []string{"node", "serial"})
	m.driveReallocSectors = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nest_drive_smart_reallocated_sectors_total",
		Help: "Reallocated sector count from SMART attribute ID 5",
	}, []string{"node", "serial"})
	m.hwPoolFreeBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nest_hardware_pool_free_bytes",
		Help: "Free bytes in a hardware storage pool (placeholder until HardwarePool CRDs)",
	}, []string{"pool", "class"})
	m.hwPoolUsedBytes = prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: "nest_hardware_pool_used_bytes",
		Help: "Used bytes in a hardware storage pool (placeholder until HardwarePool CRDs)",
	}, []string{"pool", "class"})

	reg.MustRegister(
		m.driveHealth,
		m.driveWearPercent,
		m.driveTemperature,
		m.driveCapacityBytes,
		m.darkDriveCount,
		m.scanDuration,
		m.driveReadBytesTotal,
		m.driveWriteBytesTotal,
		m.driveReadIops,
		m.driveWriteIops,
		m.driveQueueDepth,
		m.driveHoursOn,
		m.driveReallocSectors,
		m.hwPoolFreeBytes,
		m.hwPoolUsedBytes,
	)
	return m
}

func NewAgent(cfg AgentConfig) *Agent {
	pub, err := NewCRPublisher(cfg.NodeName, cfg.Logger)
	if err != nil {
		cfg.Logger.Warn("CR publisher init failed, continuing without it", zap.Error(err))
	}
	return &Agent{
		cfg:            cfg,
		inventory:      NewInventoryCollector(cfg.NodeName, cfg.Logger),
		metrics:        newAgentMetrics(),
		publisher:      pub,
		spindown:       NewSpindownTracker(30*time.Minute, cfg.Logger),
		rma:            NewRMAManager(cfg.NodeName, cfg.Logger),
		prevBlockStats: make(map[string]*BlockStats),
	}
}

func (a *Agent) Run(ctx context.Context) error {
	// Start metrics HTTP server
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.Handler())
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
	srv := &http.Server{Addr: a.cfg.MetricsAddr, Handler: mux}
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			a.cfg.Logger.Error("metrics server error", zap.Error(err))
		}
	}()
	defer srv.Shutdown(context.Background())

	// Initial scan on startup
	a.scan(ctx)

	ticker := time.NewTicker(a.cfg.ScanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			a.scan(ctx)
		}
	}
}

func (a *Agent) scan(ctx context.Context) {
	timer := prometheus.NewTimer(a.metrics.scanDuration)
	defer timer.ObserveDuration()

	a.cfg.Logger.Info("starting drive inventory scan", zap.String("node", a.cfg.NodeName))

	devices, err := a.inventory.Collect(ctx)
	if err != nil {
		a.cfg.Logger.Error("inventory collection failed", zap.Error(err))
		return
	}

	// Process RMA state for failed drives (mutates devices in place)
	_ = a.rma.ProcessDevices(ctx, devices)

	// Update spindown tracking for sata-cold drives
	a.spindown.Update(ctx, devices)

	// Publish HardwareInventory CR (no-op when publisher is nil / out-of-cluster)
	if a.publisher != nil {
		if err := a.publisher.UpsertHardwareInventory(ctx, devices); err != nil {
			a.cfg.Logger.Warn("failed to upsert HardwareInventory CR", zap.Error(err))
		}
	}

	darkCount := 0
	// Aggregate capacity per class for hardware pool placeholders.
	poolUsed := make(map[string]int64) // class → bytes used (Active devices)

	for _, d := range devices {
		if d.State == "Dark" {
			darkCount++
			if a.publisher != nil {
				if err := a.publisher.EnsureDarkDriveCR(ctx, d); err != nil {
					a.cfg.Logger.Warn("failed to ensure DarkDrive CR",
						zap.String("device", d.Name), zap.Error(err))
				}
			}
		}

		// Update drive health metric
		healthVal := float64(0) // healthy
		if d.SMART != nil {
			if d.SMART.WearPercent > 80 {
				healthVal = 1 // warn
			}
			if d.SMART.ReallocatedSectors > 10 {
				healthVal = 2 // degraded
			}
		}

		a.metrics.driveHealth.WithLabelValues(
			a.cfg.NodeName, d.Serial, d.Class, d.Model,
		).Set(healthVal)

		a.metrics.driveCapacityBytes.WithLabelValues(
			a.cfg.NodeName, d.Serial,
		).Set(float64(d.CapacityBytes))

		if d.SMART != nil {
			a.metrics.driveWearPercent.WithLabelValues(
				a.cfg.NodeName, d.Serial,
			).Set(float64(d.SMART.WearPercent))

			a.metrics.driveTemperature.WithLabelValues(
				a.cfg.NodeName, d.Serial,
			).Set(float64(d.SMART.TemperatureCelsius))

			a.metrics.driveHoursOn.WithLabelValues(
				a.cfg.NodeName, d.Serial,
			).Set(float64(d.SMART.HoursOn))

			a.metrics.driveReallocSectors.WithLabelValues(
				a.cfg.NodeName, d.Serial,
			).Set(float64(d.SMART.ReallocatedSectors))
		}

		// Block-layer metrics from /proc/diskstats.
		if d.BlockStats != nil {
			bs := d.BlockStats

			a.metrics.driveReadIops.WithLabelValues(
				a.cfg.NodeName, d.Serial,
			).Set(float64(bs.ReadIops))

			a.metrics.driveWriteIops.WithLabelValues(
				a.cfg.NodeName, d.Serial,
			).Set(float64(bs.WriteIops))

			a.metrics.driveQueueDepth.WithLabelValues(
				a.cfg.NodeName, d.Serial,
			).Set(float64(bs.InFlight))

			// Counter deltas: only add the positive difference since the last scan.
			if prev, ok := a.prevBlockStats[d.Serial]; ok {
				if delta := bs.ReadBytes - prev.ReadBytes; delta > 0 {
					a.metrics.driveReadBytesTotal.WithLabelValues(
						a.cfg.NodeName, d.Serial,
					).Add(float64(delta))
				}
				if delta := bs.WriteBytes - prev.WriteBytes; delta > 0 {
					a.metrics.driveWriteBytesTotal.WithLabelValues(
						a.cfg.NodeName, d.Serial,
					).Add(float64(delta))
				}
			}
			// Snapshot current stats for next scan.
			snapshot := *bs
			a.prevBlockStats[d.Serial] = &snapshot
		}

		// Accumulate capacity by class for pool placeholders.
		if d.State == "Active" {
			poolUsed[d.Class] += d.CapacityBytes
		}
	}

	// Publish hardware pool placeholder metrics.
	classes := []string{"nvme-hot", "ssd-warm", "sata-bulk", "sata-cold"}
	for _, class := range classes {
		used := poolUsed[class]
		a.metrics.hwPoolUsedBytes.WithLabelValues(class, class).Set(float64(used))
		a.metrics.hwPoolFreeBytes.WithLabelValues(class, class).Set(0)
	}

	a.metrics.darkDriveCount.Set(float64(darkCount))
	a.cfg.Logger.Info("scan complete",
		zap.Int("devices", len(devices)),
		zap.Int("dark", darkCount),
	)
}
