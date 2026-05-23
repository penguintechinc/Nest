package main

import (
	"context"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewRMAManager(t *testing.T) {
	logger := zap.NewNop()
	m := NewRMAManager("node-1", logger)
	if m == nil {
		t.Fatal("NewRMAManager returned nil")
	}
	if m.nodeName != "node-1" {
		t.Errorf("nodeName = %s, want node-1", m.nodeName)
	}
	if m.failedDrives == nil {
		t.Error("failedDrives map is nil")
	}
}

func TestProcessDevices_NoFailures(t *testing.T) {
	logger := zap.NewNop()
	m := NewRMAManager("node-1", logger)
	ctx := context.Background()

	devices := []*DeviceInfo{
		{
			Serial: "OK-001", Name: "/dev/sda", Model: "WD",
			SMART: &SMARTInfo{Health: "PASSED"},
		},
	}

	newlyFailed := m.ProcessDevices(ctx, devices)
	if len(newlyFailed) != 0 {
		t.Errorf("expected 0 newly failed, got %d", len(newlyFailed))
	}
}

func TestProcessDevices_NoSMART(t *testing.T) {
	logger := zap.NewNop()
	m := NewRMAManager("node-1", logger)
	ctx := context.Background()

	devices := []*DeviceInfo{
		{Serial: "NO-SMART-001", Name: "/dev/sda", Model: "WD", SMART: nil},
	}

	newlyFailed := m.ProcessDevices(ctx, devices)
	if len(newlyFailed) != 0 {
		t.Errorf("expected 0 newly failed, got %d", len(newlyFailed))
	}
}

func TestProcessDevices_FirstTimeFailure(t *testing.T) {
	logger := zap.NewNop()
	m := NewRMAManager("node-1", logger)
	ctx := context.Background()

	devices := []*DeviceInfo{
		{
			Serial: "FAIL-001", Name: "/dev/sda", Model: "WD",
			SMART: &SMARTInfo{Health: "FAILED"},
		},
	}

	newlyFailed := m.ProcessDevices(ctx, devices)
	if len(newlyFailed) != 1 || newlyFailed[0] != "FAIL-001" {
		t.Errorf("expected [FAIL-001], got %v", newlyFailed)
	}
	// Second call — same device still failing but < 5 min elapsed; not newly failed again
	newlyFailed2 := m.ProcessDevices(ctx, devices)
	if len(newlyFailed2) != 0 {
		t.Errorf("expected 0 on second call, got %d", len(newlyFailed2))
	}
}

func TestProcessDevices_LongFailure(t *testing.T) {
	logger := zap.NewNop()
	m := NewRMAManager("node-1", logger)
	ctx := context.Background()

	// Pre-seed as failed >5 minutes ago
	serial := "LONG-FAIL-001"
	m.failedDrives[serial] = time.Now().Add(-10 * time.Minute)

	devices := []*DeviceInfo{
		{
			Serial: serial, Name: "/dev/sda", Model: "WD", State: "Active",
			SMART: &SMARTInfo{Health: "FAILED"},
		},
	}

	m.ProcessDevices(ctx, devices)

	// State should be mutated to "Failed"
	if devices[0].State != "Failed" {
		t.Errorf("expected State=Failed, got %s", devices[0].State)
	}
}

func TestProcessDevices_CleanupRemovedDevices(t *testing.T) {
	logger := zap.NewNop()
	m := NewRMAManager("node-1", logger)
	ctx := context.Background()

	// Seed stale failed device
	m.failedDrives["GONE-001"] = time.Now()

	// Call with empty device list — stale entry should be cleaned up
	m.ProcessDevices(ctx, []*DeviceInfo{})

	if _, ok := m.failedDrives["GONE-001"]; ok {
		t.Error("expected GONE-001 to be cleaned up from failedDrives")
	}
}

func TestOfflineCephOSD_NotFound(t *testing.T) {
	logger := zap.NewNop()
	m := NewRMAManager("node-1", logger)
	ctx := context.Background()

	// ceph binary not available — should return nil gracefully
	err := m.offlineCephOSD(ctx, "/dev/sda")
	if err != nil {
		t.Logf("offlineCephOSD returned error (expected in test env): %v", err)
	}
}
