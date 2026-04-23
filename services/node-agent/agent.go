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
	cfg       AgentConfig
	inventory *InventoryCollector
	metrics   *AgentMetrics
}

// AgentMetrics holds Prometheus metrics exported by the node-agent
type AgentMetrics struct {
	driveHealth       *prometheus.GaugeVec
	driveWearPercent  *prometheus.GaugeVec
	driveTemperature  *prometheus.GaugeVec
	driveCapacityBytes *prometheus.GaugeVec
	darkDriveCount    prometheus.Gauge
	scanDuration      prometheus.Histogram
}

func newAgentMetrics() *AgentMetrics {
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
	prometheus.MustRegister(
		m.driveHealth,
		m.driveWearPercent,
		m.driveTemperature,
		m.driveCapacityBytes,
		m.darkDriveCount,
		m.scanDuration,
	)
	return m
}

func NewAgent(cfg AgentConfig) *Agent {
	return &Agent{
		cfg:       cfg,
		inventory: NewInventoryCollector(cfg.NodeName, cfg.Logger),
		metrics:   newAgentMetrics(),
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

	darkCount := 0
	for _, d := range devices {
		if d.State == "Dark" {
			darkCount++
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
		}
	}

	a.metrics.darkDriveCount.Set(float64(darkCount))
	a.cfg.Logger.Info("scan complete",
		zap.Int("devices", len(devices)),
		zap.Int("dark", darkCount),
	)
}
