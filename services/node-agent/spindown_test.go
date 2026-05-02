package main

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewSpindownTracker(t *testing.T) {
	logger := zap.NewNop()
	st := NewSpindownTracker(30*time.Minute, logger)
	if st == nil {
		t.Fatal("NewSpindownTracker returned nil")
	}
	if st.idleThreshold != 30*time.Minute {
		t.Errorf("idleThreshold = %v, want 30m", st.idleThreshold)
	}
	if st.lastIOTime == nil || st.spunDown == nil || st.lastIOOps == nil {
		t.Error("internal maps not initialized")
	}
}

func TestSpindownUpdate_NonSataCold(t *testing.T) {
	logger := zap.NewNop()
	st := NewSpindownTracker(30*time.Minute, logger)
	ctx := context.Background()

	// Non sata-cold devices are ignored
	devices := []*DeviceInfo{
		{Name: "/dev/nvme0n1", Class: "nvme-hot", BlockStats: nil},
	}
	st.Update(ctx, devices)

	if len(st.lastIOTime) != 0 {
		t.Errorf("expected no tracking for non-sata-cold, got %d entries", len(st.lastIOTime))
	}
}

func TestSpindownUpdate_NewSataColdDevice(t *testing.T) {
	logger := zap.NewNop()
	st := NewSpindownTracker(30*time.Minute, logger)
	ctx := context.Background()

	devices := []*DeviceInfo{
		{Name: "/dev/sda", Class: "sata-cold", BlockStats: nil},
	}
	st.Update(ctx, devices)

	if _, ok := st.lastIOTime["/dev/sda"]; !ok {
		t.Error("expected /dev/sda to be tracked")
	}
	if st.spunDown["/dev/sda"] {
		t.Error("expected new device to not be spun-down")
	}
}

func TestSpindownUpdate_IOActivity(t *testing.T) {
	logger := zap.NewNop()
	st := NewSpindownTracker(30*time.Minute, logger)
	ctx := context.Background()

	devName := "/dev/sda"

	// First update — initialize tracking
	devices := []*DeviceInfo{
		{Name: devName, Class: "sata-cold", BlockStats: &BlockStats{ReadIops: 100, WriteIops: 50}},
	}
	st.Update(ctx, devices)

	// Mark as spun-down to verify IO resets it
	st.mu.Lock()
	st.spunDown[devName] = true
	st.lastIOOps[devName] = 50 // different from current 150 → IO detected
	oldTime := time.Now().Add(-5 * time.Minute)
	st.lastIOTime[devName] = oldTime
	st.mu.Unlock()

	// Second update with different IO count
	st.Update(ctx, devices)

	st.mu.Lock()
	defer st.mu.Unlock()
	if st.spunDown[devName] {
		t.Error("expected spunDown to be reset after IO activity")
	}
}

func TestSpindownUpdate_IdleSpindown(t *testing.T) {
	logger := zap.NewNop()
	// Very short idle threshold for test
	st := NewSpindownTracker(1*time.Millisecond, logger)
	ctx := context.Background()

	devName := "/dev/sda"

	// Pre-seed as known device with idle time well past threshold
	st.mu.Lock()
	st.lastIOTime[devName] = time.Now().Add(-10 * time.Minute)
	st.spunDown[devName] = false
	st.lastIOOps[devName] = 0
	st.mu.Unlock()

	devices := []*DeviceInfo{
		{Name: devName, Class: "sata-cold", BlockStats: &BlockStats{ReadIops: 0, WriteIops: 0}},
	}
	// Update should call spindown (which will fail gracefully since hdparm not available)
	st.Update(ctx, devices)

	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.spunDown[devName] {
		t.Error("expected device to be marked as spun-down after idle threshold exceeded")
	}
}

func TestSpindownUpdate_CleanupRemovedDevices(t *testing.T) {
	logger := zap.NewNop()
	st := NewSpindownTracker(30*time.Minute, logger)
	ctx := context.Background()

	// Pre-seed a device
	st.mu.Lock()
	st.lastIOTime["/dev/gone"] = time.Now()
	st.spunDown["/dev/gone"] = false
	st.lastIOOps["/dev/gone"] = 0
	st.mu.Unlock()

	// Update with empty device list — /dev/gone should be cleaned up
	st.Update(ctx, []*DeviceInfo{})

	st.mu.Lock()
	defer st.mu.Unlock()
	if _, ok := st.lastIOTime["/dev/gone"]; ok {
		t.Error("expected /dev/gone to be removed from tracking")
	}
}

func TestSpindownSpindown_HdparmNotFound(t *testing.T) {
	logger := zap.NewNop()
	st := NewSpindownTracker(30*time.Minute, logger)
	ctx := context.Background()

	// spindown is unexported but called via Update; call directly with reflection-free hack
	// by triggering the idle path
	devName := "/dev/stub-spin-test"
	st.mu.Lock()
	st.lastIOTime[devName] = time.Now().Add(-1 * time.Hour)
	st.spunDown[devName] = false
	st.lastIOOps[devName] = 0
	st.mu.Unlock()

	devices := []*DeviceInfo{
		{Name: devName, Class: "sata-cold", BlockStats: nil},
	}
	// Should not panic even if hdparm is not available
	st.Update(ctx, devices)
}
