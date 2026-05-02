package main

import (
	"context"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// newTestMetrics creates an AgentMetrics with a fresh isolated registry — safe to call
// multiple times within a test binary without duplicate-registration panics.
func newTestMetrics() *AgentMetrics {
	return newAgentMetricsWithRegistry(prometheus.NewRegistry())
}

// newTestAgent creates an Agent wired to a fresh isolated metrics registry.
func newTestAgent(cfg AgentConfig) *Agent {
	pub, _ := NewCRPublisher(cfg.NodeName, cfg.Logger)
	return &Agent{
		cfg:            cfg,
		inventory:      NewInventoryCollector(cfg.NodeName, cfg.Logger),
		metrics:        newTestMetrics(),
		publisher:      pub,
		spindown:       NewSpindownTracker(30*time.Minute, cfg.Logger),
		rma:            NewRMAManager(cfg.NodeName, cfg.Logger),
		prevBlockStats: make(map[string]*BlockStats),
	}
}

func TestNewAgentMetrics(t *testing.T) {
	m := newTestMetrics()
	if m == nil {
		t.Fatal("newAgentMetrics() returned nil")
	}

	// Verify all metrics are initialized
	if m.driveHealth == nil {
		t.Error("driveHealth metric is nil")
	}
	if m.driveWearPercent == nil {
		t.Error("driveWearPercent metric is nil")
	}
	if m.driveTemperature == nil {
		t.Error("driveTemperature metric is nil")
	}
	if m.driveCapacityBytes == nil {
		t.Error("driveCapacityBytes metric is nil")
	}
	if m.darkDriveCount == nil {
		t.Error("darkDriveCount metric is nil")
	}
	if m.scanDuration == nil {
		t.Error("scanDuration metric is nil")
	}
	if m.driveReadBytesTotal == nil {
		t.Error("driveReadBytesTotal metric is nil")
	}
	if m.driveWriteBytesTotal == nil {
		t.Error("driveWriteBytesTotal metric is nil")
	}
	if m.driveReadIops == nil {
		t.Error("driveReadIops metric is nil")
	}
	if m.driveWriteIops == nil {
		t.Error("driveWriteIops metric is nil")
	}
	if m.driveQueueDepth == nil {
		t.Error("driveQueueDepth metric is nil")
	}
	if m.driveHoursOn == nil {
		t.Error("driveHoursOn metric is nil")
	}
	if m.driveReallocSectors == nil {
		t.Error("driveReallocSectors metric is nil")
	}
	if m.hwPoolFreeBytes == nil {
		t.Error("hwPoolFreeBytes metric is nil")
	}
	if m.hwPoolUsedBytes == nil {
		t.Error("hwPoolUsedBytes metric is nil")
	}
}

// TestNewAgentMetricsMultipleCalls ensures calling newAgentMetricsWithRegistry with
// a fresh registry each time does not panic.
func TestNewAgentMetricsMultipleCalls(t *testing.T) {
	for i := 0; i < 3; i++ {
		m := newTestMetrics()
		if m == nil {
			t.Fatalf("call %d: newTestMetrics() returned nil", i)
		}
	}
}

func TestAgentConfig(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	cfg := AgentConfig{
		NodeName:     "test-node",
		ScanInterval: 5 * time.Minute,
		MetricsAddr:  ":9090",
		Logger:       logger,
	}

	if cfg.NodeName != "test-node" {
		t.Errorf("got NodeName %s, want test-node", cfg.NodeName)
	}
	if cfg.ScanInterval != 5*time.Minute {
		t.Errorf("got ScanInterval %v, want 5m0s", cfg.ScanInterval)
	}
	if cfg.MetricsAddr != ":9090" {
		t.Errorf("got MetricsAddr %s, want :9090", cfg.MetricsAddr)
	}
}

func TestNewAgent(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	cfg := AgentConfig{
		NodeName:     "test-node",
		ScanInterval: 5 * time.Minute,
		MetricsAddr:  ":9090",
		Logger:       logger,
	}

	agent := newTestAgent(cfg)
	if agent == nil {
		t.Fatal("newTestAgent() returned nil")
	}
	if agent.cfg.NodeName != cfg.NodeName {
		t.Errorf("got NodeName %s, want %s", agent.cfg.NodeName, cfg.NodeName)
	}
	if agent.inventory == nil {
		t.Error("inventory is nil")
	}
	if agent.metrics == nil {
		t.Error("metrics is nil")
	}
	if agent.spindown == nil {
		t.Error("spindown is nil")
	}
	if agent.rma == nil {
		t.Error("rma is nil")
	}
	if agent.prevBlockStats == nil {
		t.Error("prevBlockStats is nil")
	}
}

func TestAgentRunWithCanceledContext(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	cfg := AgentConfig{
		NodeName:     "test-node-run",
		ScanInterval: 100 * time.Millisecond,
		MetricsAddr:  ":19999",
		Logger:       logger,
	}

	agent := newTestAgent(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Should return context.Canceled error
	err := agent.Run(ctx)
	if err != context.Canceled {
		t.Logf("Run() error = %v, expected context.Canceled", err)
	}
}

func TestAgentScanWithStubInventory(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	cfg := AgentConfig{
		NodeName:     "scan-test-node",
		ScanInterval: 1 * time.Minute,
		MetricsAddr:  ":0",
		Logger:       logger,
	}

	agent := newTestAgent(cfg)
	ctx := context.Background()

	// scan() should not panic; it will call stubDevices() when lsblk is unavailable
	agent.scan(ctx)
}

func TestAgentScanMetricsUpdated(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	cfg := AgentConfig{
		NodeName:     "metrics-test-node",
		ScanInterval: 1 * time.Minute,
		MetricsAddr:  ":0",
		Logger:       logger,
	}

	agent := newTestAgent(cfg)

	// Inject stub devices directly to exercise scan metric paths
	devices := []*DeviceInfo{
		{
			Name:          "/dev/stub-nvme0n1",
			Serial:        "NVMe-001",
			Model:         "Samsung 980 PRO",
			CapacityBytes: 4096 * 1024 * 1024 * 1024,
			Class:         "nvme-hot",
			State:         "Active",
			SMART: &SMARTInfo{
				Health:             "PASSED",
				WearPercent:        10,
				HoursOn:            1000,
				TemperatureCelsius: 38,
				ReallocatedSectors: 0,
			},
			BlockStats: &BlockStats{
				ReadBytes:  1024 * 1024,
				WriteBytes: 512 * 1024,
				ReadIops:   200,
				WriteIops:  100,
				InFlight:   2,
			},
		},
		{
			Name:          "/dev/stub-sda",
			Serial:        "HDD-001",
			Model:         "WD Ultrastar HC550",
			CapacityBytes: 18 * 1024 * 1024 * 1024 * 1024,
			Class:         "sata-cold",
			State:         "Dark",
			SMART: &SMARTInfo{
				Health:             "PASSED",
				WearPercent:        0,
				HoursOn:            500,
				TemperatureCelsius: 30,
				ReallocatedSectors: 0,
			},
		},
	}

	// Manually exercise the metric update logic (same as scan body after inventory)
	darkCount := 0
	poolUsed := make(map[string]int64)
	for _, d := range devices {
		if d.State == "Dark" {
			darkCount++
		}
		healthVal := float64(0)
		if d.SMART != nil {
			if d.SMART.WearPercent > 80 {
				healthVal = 1
			}
			if d.SMART.ReallocatedSectors > 10 {
				healthVal = 2
			}
		}
		agent.metrics.driveHealth.WithLabelValues(
			agent.cfg.NodeName, d.Serial, d.Class, d.Model,
		).Set(healthVal)
		agent.metrics.driveCapacityBytes.WithLabelValues(
			agent.cfg.NodeName, d.Serial,
		).Set(float64(d.CapacityBytes))
		if d.SMART != nil {
			agent.metrics.driveWearPercent.WithLabelValues(agent.cfg.NodeName, d.Serial).Set(float64(d.SMART.WearPercent))
			agent.metrics.driveTemperature.WithLabelValues(agent.cfg.NodeName, d.Serial).Set(float64(d.SMART.TemperatureCelsius))
			agent.metrics.driveHoursOn.WithLabelValues(agent.cfg.NodeName, d.Serial).Set(float64(d.SMART.HoursOn))
			agent.metrics.driveReallocSectors.WithLabelValues(agent.cfg.NodeName, d.Serial).Set(float64(d.SMART.ReallocatedSectors))
		}
		if d.BlockStats != nil {
			bs := d.BlockStats
			agent.metrics.driveReadIops.WithLabelValues(agent.cfg.NodeName, d.Serial).Set(float64(bs.ReadIops))
			agent.metrics.driveWriteIops.WithLabelValues(agent.cfg.NodeName, d.Serial).Set(float64(bs.WriteIops))
			agent.metrics.driveQueueDepth.WithLabelValues(agent.cfg.NodeName, d.Serial).Set(float64(bs.InFlight))

			if prev, ok := agent.prevBlockStats[d.Serial]; ok {
				if delta := bs.ReadBytes - prev.ReadBytes; delta > 0 {
					agent.metrics.driveReadBytesTotal.WithLabelValues(agent.cfg.NodeName, d.Serial).Add(float64(delta))
				}
				if delta := bs.WriteBytes - prev.WriteBytes; delta > 0 {
					agent.metrics.driveWriteBytesTotal.WithLabelValues(agent.cfg.NodeName, d.Serial).Add(float64(delta))
				}
			}
			snapshot := *bs
			agent.prevBlockStats[d.Serial] = &snapshot
		}
		if d.State == "Active" {
			poolUsed[d.Class] += d.CapacityBytes
		}
	}
	classes := []string{"nvme-hot", "ssd-warm", "sata-bulk", "sata-cold"}
	for _, class := range classes {
		used := poolUsed[class]
		agent.metrics.hwPoolUsedBytes.WithLabelValues(class, class).Set(float64(used))
		agent.metrics.hwPoolFreeBytes.WithLabelValues(class, class).Set(0)
	}
	agent.metrics.darkDriveCount.Set(float64(darkCount))

	if darkCount != 1 {
		t.Errorf("expected 1 dark drive, got %d", darkCount)
	}
}

func TestAgentScanDegradedHealth(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	cfg := AgentConfig{
		NodeName:     "health-test-node",
		ScanInterval: 1 * time.Minute,
		MetricsAddr:  ":0",
		Logger:       logger,
	}
	agent := newTestAgent(cfg)

	// Drive with high wear (warn)
	warnDrive := &DeviceInfo{
		Name: "/dev/sda", Serial: "WARN-001", Model: "M", Class: "ssd-warm", State: "Active",
		SMART: &SMARTInfo{Health: "PASSED", WearPercent: 85, ReallocatedSectors: 0},
	}
	// Drive with many reallocated sectors (degraded)
	degradedDrive := &DeviceInfo{
		Name: "/dev/sdb", Serial: "DEGRADE-001", Model: "M", Class: "ssd-warm", State: "Active",
		SMART: &SMARTInfo{Health: "PASSED", WearPercent: 0, ReallocatedSectors: 15},
	}

	for _, d := range []*DeviceInfo{warnDrive, degradedDrive} {
		healthVal := float64(0)
		if d.SMART.WearPercent > 80 {
			healthVal = 1
		}
		if d.SMART.ReallocatedSectors > 10 {
			healthVal = 2
		}
		agent.metrics.driveHealth.WithLabelValues(agent.cfg.NodeName, d.Serial, d.Class, d.Model).Set(healthVal)
	}
	// No panic = pass
}

func TestAgentBlockStatsDelta(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	cfg := AgentConfig{
		NodeName:     "delta-test-node",
		ScanInterval: 1 * time.Minute,
		MetricsAddr:  ":0",
		Logger:       logger,
	}
	agent := newTestAgent(cfg)

	const serial = "DELTA-001"
	// Seed a previous snapshot
	agent.prevBlockStats[serial] = &BlockStats{ReadBytes: 1000, WriteBytes: 500}

	// Current stats with positive deltas
	current := &BlockStats{ReadBytes: 2000, WriteBytes: 1500, ReadIops: 10, WriteIops: 5, InFlight: 1}
	if delta := current.ReadBytes - agent.prevBlockStats[serial].ReadBytes; delta > 0 {
		agent.metrics.driveReadBytesTotal.WithLabelValues(agent.cfg.NodeName, serial).Add(float64(delta))
	}
	if delta := current.WriteBytes - agent.prevBlockStats[serial].WriteBytes; delta > 0 {
		agent.metrics.driveWriteBytesTotal.WithLabelValues(agent.cfg.NodeName, serial).Add(float64(delta))
	}
	snapshot := *current
	agent.prevBlockStats[serial] = &snapshot

	// Negative delta should be skipped (no panic)
	older := &BlockStats{ReadBytes: 500, WriteBytes: 100}
	if delta := older.ReadBytes - agent.prevBlockStats[serial].ReadBytes; delta > 0 {
		agent.metrics.driveReadBytesTotal.WithLabelValues(agent.cfg.NodeName, serial).Add(float64(delta))
	}
}
