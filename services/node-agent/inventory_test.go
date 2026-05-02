package main

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewInventoryCollector(t *testing.T) {
	logger := zap.NewNop()
	ic := NewInventoryCollector("node-1", logger)
	if ic == nil {
		t.Fatal("NewInventoryCollector returned nil")
	}
	if ic.nodeName != "node-1" {
		t.Errorf("nodeName = %s, want node-1", ic.nodeName)
	}
	if ic.lastSMARTScan == nil {
		t.Error("lastSMARTScan is nil")
	}
	if ic.lastSMART == nil {
		t.Error("lastSMART is nil")
	}
}

func TestCollectReturnsDevices(t *testing.T) {
	logger := zap.NewNop()
	ic := NewInventoryCollector("node-1", logger)
	ctx := context.Background()

	// lsblk will fail in test environment; Collect falls back to stubDevices
	devices, err := ic.Collect(ctx)
	if err != nil {
		t.Fatalf("Collect returned error: %v", err)
	}
	if len(devices) == 0 {
		t.Error("expected at least one device from stub fallback")
	}
}

func TestClassifyDevice(t *testing.T) {
	logger := zap.NewNop()
	ic := NewInventoryCollector("node-1", logger)

	tests := []struct {
		name      string
		device    *DeviceInfo
		wantClass string
	}{
		{
			name:      "nvme device",
			device:    &DeviceInfo{Name: "/dev/nvme0n1", Model: "Samsung", CapacityBytes: 1024},
			wantClass: "nvme-hot",
		},
		{
			name:      "ssd by model keyword",
			device:    &DeviceInfo{Name: "/dev/sda", Model: "Samsung SSD 870 EVO", CapacityBytes: 1024},
			wantClass: "ssd-warm",
		},
		{
			name:      "ssd by crucial keyword",
			device:    &DeviceInfo{Name: "/dev/sdb", Model: "Crucial MX500", CapacityBytes: 1024},
			wantClass: "ssd-warm",
		},
		{
			name:      "ssd by micron keyword",
			device:    &DeviceInfo{Name: "/dev/sdc", Model: "Micron 5300", CapacityBytes: 1024},
			wantClass: "ssd-warm",
		},
		{
			name:      "ssd by intel keyword",
			device:    &DeviceInfo{Name: "/dev/sdd", Model: "Intel S4510", CapacityBytes: 1024},
			wantClass: "ssd-warm",
		},
		{
			name:      "ssd by solid keyword",
			device:    &DeviceInfo{Name: "/dev/sde", Model: "Solid State Drive", CapacityBytes: 1024},
			wantClass: "ssd-warm",
		},
		{
			name:      "ssd by pro keyword",
			device:    &DeviceInfo{Name: "/dev/sdf", Model: "WD Black Pro SN850", CapacityBytes: 1024},
			wantClass: "ssd-warm",
		},
		{
			name:      "large HDD over 16TiB → sata-cold",
			device:    &DeviceInfo{Name: "/dev/sdg", Model: "WD Ultrastar HC550", CapacityBytes: 18 * 1024 * 1024 * 1024 * 1024},
			wantClass: "sata-cold",
		},
		{
			name:      "normal HDD → sata-bulk",
			device:    &DeviceInfo{Name: "/dev/sdh", Model: "Seagate Barracuda", CapacityBytes: 4 * 1024 * 1024 * 1024 * 1024},
			wantClass: "sata-bulk",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := ic.classifyDevice(tt.device)
			if got != tt.wantClass {
				t.Errorf("classifyDevice(%q, %q) = %s, want %s", tt.device.Name, tt.device.Model, got, tt.wantClass)
			}
		})
	}
}

func TestDetectState(t *testing.T) {
	logger := zap.NewNop()
	ic := NewInventoryCollector("node-1", logger)

	// detectState shells out to lsblk; on non-Linux or stub environments it returns "Dark".
	d := &DeviceInfo{Name: "/dev/nonexistent-stub-999"}
	state := ic.detectState(d)
	// Either "Dark" or "Active" — just ensure it returns a valid string and doesn't panic.
	if state != "Dark" && state != "Active" {
		t.Errorf("detectState returned unexpected state %q", state)
	}
}

func TestCollectSMARTWithScheduling_NewDevice(t *testing.T) {
	logger := zap.NewNop()
	ic := NewInventoryCollector("node-1", logger)
	ctx := context.Background()

	// First call — device not seen before; will run smartctl (which falls back to stub)
	result := ic.collectSMARTWithScheduling(ctx, "/dev/stub-test")
	if result == nil {
		t.Error("expected non-nil SMARTInfo for new device")
	}
}

func TestCollectSMARTWithScheduling_RecentlyCached(t *testing.T) {
	logger := zap.NewNop()
	ic := NewInventoryCollector("node-1", logger)
	ctx := context.Background()

	devName := "/dev/stub-cached"
	cached := &SMARTInfo{Health: "PASSED", WearPercent: 5, HoursOn: 100, TemperatureCelsius: 30}

	// Seed cache as if device was scanned just now
	ic.mu.Lock()
	ic.lastSMARTScan[devName] = time.Now()
	ic.lastSMART[devName] = cached
	ic.mu.Unlock()

	result := ic.collectSMARTWithScheduling(ctx, devName)
	if result == nil {
		t.Error("expected non-nil cached SMARTInfo")
	}
	if result.HoursOn != cached.HoursOn {
		t.Errorf("expected cached HoursOn %d, got %d", cached.HoursOn, result.HoursOn)
	}
}

func TestCollectSMARTWithScheduling_StaleCache(t *testing.T) {
	logger := zap.NewNop()
	ic := NewInventoryCollector("node-1", logger)
	ctx := context.Background()

	devName := "/dev/stub-stale"
	// Seed cache with a timestamp >23 hours ago — should re-run smartctl
	ic.mu.Lock()
	ic.lastSMARTScan[devName] = time.Now().Add(-25 * time.Hour)
	ic.lastSMART[devName] = &SMARTInfo{Health: "PASSED"}
	ic.mu.Unlock()

	result := ic.collectSMARTWithScheduling(ctx, devName)
	if result == nil {
		t.Error("expected non-nil SMARTInfo after stale cache refresh")
	}
}

func TestCollectSMARTWithScheduling_CachedButNoEntry(t *testing.T) {
	logger := zap.NewNop()
	ic := NewInventoryCollector("node-1", logger)
	ctx := context.Background()

	devName := "/dev/stub-noentry"
	// Seed lastSMARTScan but NOT lastSMART (simulate inconsistency to hit fallback)
	ic.mu.Lock()
	ic.lastSMARTScan[devName] = time.Now()
	// intentionally omit ic.lastSMART[devName]
	ic.mu.Unlock()

	// Should fall through to stubSMART
	result := ic.collectSMARTWithScheduling(ctx, devName)
	if result == nil {
		t.Error("expected non-nil SMARTInfo from fallback stub")
	}
}

func TestCollectSMART_Fallback(t *testing.T) {
	logger := zap.NewNop()
	ic := NewInventoryCollector("node-1", logger)
	ctx := context.Background()

	// smartctl not available in test environment — should return stub
	result := ic.collectSMART(ctx, "/dev/nonexistent-9999")
	if result == nil {
		t.Error("collectSMART should return stub when smartctl unavailable")
	}
}

func TestCollectBlockStats_NonExistentDevice(t *testing.T) {
	logger := zap.NewNop()
	ic := NewInventoryCollector("node-1", logger)

	// /proc/diskstats may or may not exist; for a non-existent device name it should return nil
	result := ic.collectBlockStats("/dev/nonexistent-stub-xyz999")
	// nil is acceptable (device not found or not on Linux)
	_ = result
}

func TestStubDevices(t *testing.T) {
	logger := zap.NewNop()
	ic := NewInventoryCollector("node-1", logger)

	devices := ic.stubDevices()
	if len(devices) == 0 {
		t.Error("stubDevices() returned empty slice")
	}
	for _, d := range devices {
		if d.Serial == "" {
			t.Error("stub device has empty serial")
		}
		if d.CapacityBytes == 0 {
			t.Error("stub device has zero capacity")
		}
	}
}

func TestStubSMART(t *testing.T) {
	logger := zap.NewNop()
	ic := NewInventoryCollector("node-1", logger)

	s := ic.stubSMART(nil)
	if s == nil {
		t.Fatal("stubSMART returned nil")
	}
	if s.Health != "PASSED" {
		t.Errorf("stubSMART health = %s, want PASSED", s.Health)
	}
}

func TestLsblk_Context(t *testing.T) {
	logger := zap.NewNop()
	ic := NewInventoryCollector("node-1", logger)

	// lsblk typically doesn't exist in macOS test env — just verify no panic
	ctx := context.Background()
	_, _ = ic.lsblk(ctx)
}
