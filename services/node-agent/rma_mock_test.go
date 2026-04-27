package main

import (
	"context"
	"errors"
	"os/exec"
	"testing"
	"time"

	"go.uber.org/zap"
)

// newTestRMAManager creates an RMAManager with a mock CommandRunner.
func newTestRMAManager(cmd CommandRunner) *RMAManager {
	m := NewRMAManager("node-1", zap.NewNop())
	m.cmd = cmd
	return m
}

// TestOfflineCephOSD_Success verifies the happy path: osd find + osd out succeed.
func TestOfflineCephOSD_Success(t *testing.T) {
	outCallArgs := []string{}
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "ceph" && len(args) >= 1 && args[0] == "osd" && args[1] == "find" {
				return []byte(`{"osd": 42, "info": "osd.42 up"}`), nil
			}
			return nil, exec.ErrNotFound
		},
		runFn: func(_ context.Context, name string, args ...string) error {
			if name == "ceph" {
				outCallArgs = args
				return nil
			}
			return exec.ErrNotFound
		},
	}
	m := newTestRMAManager(cmd)

	err := m.offlineCephOSD(context.Background(), "/dev/sda")
	if err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
	if len(outCallArgs) == 0 {
		t.Error("expected ceph osd out to be called")
	}
}

// TestOfflineCephOSD_CephNotFound verifies graceful handling when ceph is not installed.
func TestOfflineCephOSD_CephNotFound(t *testing.T) {
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			return nil, exec.ErrNotFound
		},
	}
	m := newTestRMAManager(cmd)

	err := m.offlineCephOSD(context.Background(), "/dev/sda")
	if err != nil {
		t.Fatalf("expected nil when ceph not found, got %v", err)
	}
}

// TestOfflineCephOSD_FindError verifies graceful handling when osd find fails.
func TestOfflineCephOSD_FindError(t *testing.T) {
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			// Not ErrNotFound — simulate a command error (ceph is present but fails)
			return nil, errors.New("connection refused")
		},
	}
	m := newTestRMAManager(cmd)

	err := m.offlineCephOSD(context.Background(), "/dev/sda")
	// Should return nil (best-effort, warning logged)
	if err != nil {
		t.Fatalf("expected nil for find error, got %v", err)
	}
}

// TestOfflineCephOSD_CannotParseOSDID verifies graceful handling when output has no osd.N.
func TestOfflineCephOSD_CannotParseOSDID(t *testing.T) {
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "ceph" {
				return []byte("no osd id in this output"), nil
			}
			return nil, exec.ErrNotFound
		},
	}
	m := newTestRMAManager(cmd)

	err := m.offlineCephOSD(context.Background(), "/dev/sda")
	if err != nil {
		t.Fatalf("expected nil when OSD ID unparseable, got %v", err)
	}
}

// TestOfflineCephOSD_OutError verifies error is returned when osd out fails.
func TestOfflineCephOSD_OutError(t *testing.T) {
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "ceph" {
				return []byte("osd.7 is up"), nil
			}
			return nil, exec.ErrNotFound
		},
		runFn: func(_ context.Context, name string, args ...string) error {
			if name == "ceph" {
				return errors.New("ceph osd out failed")
			}
			return exec.ErrNotFound
		},
	}
	m := newTestRMAManager(cmd)

	err := m.offlineCephOSD(context.Background(), "/dev/sda")
	if err == nil {
		t.Error("expected error when osd out fails")
	}
}

// TestProcessDevices_LongFailureTriggersOffline verifies that long-failed drives
// trigger offlineCephOSD via ProcessDevices.
func TestProcessDevices_LongFailureTriggersOffline(t *testing.T) {
	cephCalled := false
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			if name == "ceph" {
				cephCalled = true
				return []byte("osd.3 info"), nil
			}
			return nil, exec.ErrNotFound
		},
		runFn: func(_ context.Context, name string, args ...string) error {
			return nil // osd out succeeds
		},
	}
	m := newTestRMAManager(cmd)

	serial := "FAIL-LONG"
	m.failedDrives[serial] = time.Now().Add(-10 * time.Minute)

	devices := []*DeviceInfo{
		{Serial: serial, Name: "/dev/sda", Model: "WD", State: "Active", SMART: &SMARTInfo{Health: "FAILED"}},
	}

	m.ProcessDevices(context.Background(), devices)

	if !cephCalled {
		t.Error("expected ceph to be called for long-failed drive")
	}
	if devices[0].State != "Failed" {
		t.Errorf("expected State=Failed, got %s", devices[0].State)
	}
}
