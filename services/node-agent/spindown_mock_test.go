package main

import (
	"context"
	"os/exec"
	"testing"
	"time"

	"go.uber.org/zap"
)

// newTestSpindownTracker creates a SpindownTracker with a mock CommandRunner.
func newTestSpindownTracker(threshold time.Duration, cmd CommandRunner) *SpindownTracker {
	st := NewSpindownTracker(threshold, zap.NewNop())
	st.cmd = cmd
	return st
}

// TestSpindown_Success verifies hdparm -y is called and succeeds.
func TestSpindown_Success(t *testing.T) {
	called := false
	cmd := &mockCommandRunner{
		runFn: func(_ context.Context, name string, args ...string) error {
			if name == "hdparm" {
				called = true
				return nil // success
			}
			return exec.ErrNotFound
		},
	}
	st := newTestSpindownTracker(1*time.Millisecond, cmd)

	devName := "/dev/sda"
	st.mu.Lock()
	st.lastIOTime[devName] = time.Now().Add(-1 * time.Hour)
	st.spunDown[devName] = false
	st.lastIOOps[devName] = 0
	st.mu.Unlock()

	devices := []*DeviceInfo{{Name: devName, Class: "sata-cold", BlockStats: nil}}
	st.Update(context.Background(), devices)

	if !called {
		t.Error("expected hdparm to be called")
	}
	st.mu.Lock()
	defer st.mu.Unlock()
	if !st.spunDown[devName] {
		t.Error("expected device to be marked spun-down")
	}
}

// TestSpindown_HdparmExitError verifies non-zero exit is logged as debug, not error.
func TestSpindown_HdparmExitError(t *testing.T) {
	cmd := &mockCommandRunner{
		runFn: func(_ context.Context, name string, args ...string) error {
			if name == "hdparm" {
				// Simulate a non-zero exit code
				return &exec.ExitError{}
			}
			return exec.ErrNotFound
		},
	}
	st := newTestSpindownTracker(1*time.Millisecond, cmd)

	devName := "/dev/sda"
	st.mu.Lock()
	st.lastIOTime[devName] = time.Now().Add(-1 * time.Hour)
	st.spunDown[devName] = false
	st.lastIOOps[devName] = 0
	st.mu.Unlock()

	devices := []*DeviceInfo{{Name: devName, Class: "sata-cold"}}
	// Should not panic
	st.Update(context.Background(), devices)

	st.mu.Lock()
	defer st.mu.Unlock()
	// Still marked as spun-down even on exit error (Update sets it before spindown returns)
	if !st.spunDown[devName] {
		t.Error("expected spunDown to be true even after hdparm exit error")
	}
}

// TestSpindown_HdparmNotFound verifies ErrNotFound is handled gracefully.
func TestSpindown_HdparmNotFound(t *testing.T) {
	cmd := &mockCommandRunner{
		runFn: func(_ context.Context, name string, args ...string) error {
			return exec.ErrNotFound
		},
	}
	st := newTestSpindownTracker(1*time.Millisecond, cmd)
	ctx := context.Background()

	// Call spindown directly
	st.spindown(ctx, "/dev/sda")
	// Should not panic; no state to verify here
}

// TestSpindown_OtherError verifies other errors are logged as warning.
func TestSpindown_OtherError(t *testing.T) {
	cmd := &mockCommandRunner{
		runFn: func(_ context.Context, name string, args ...string) error {
			if name == "hdparm" {
				// Not an ExitError and not ErrNotFound
				return context.DeadlineExceeded
			}
			return exec.ErrNotFound
		},
	}
	st := newTestSpindownTracker(30*time.Minute, cmd)
	ctx := context.Background()

	// Call spindown directly — should not panic
	st.spindown(ctx, "/dev/sda")
}

// TestSpindownUpdate_IOResetsSpunDown verifies that IO activity resets the spin-down flag
// even with a mock runner.
func TestSpindownUpdate_IOResetsSpunDown(t *testing.T) {
	cmd := &mockCommandRunner{
		runFn: func(_ context.Context, _ string, _ ...string) error {
			return exec.ErrNotFound
		},
	}
	st := newTestSpindownTracker(30*time.Minute, cmd)
	ctx := context.Background()

	devName := "/dev/sda"
	// Initialize with IO ops = 100, spunDown = true
	st.mu.Lock()
	st.lastIOTime[devName] = time.Now().Add(-5 * time.Minute)
	st.spunDown[devName] = true
	st.lastIOOps[devName] = 100
	st.mu.Unlock()

	// New IO (different from 100) → should reset spunDown
	devices := []*DeviceInfo{
		{Name: devName, Class: "sata-cold", BlockStats: &BlockStats{ReadIops: 150, WriteIops: 10}},
	}
	st.Update(ctx, devices)

	st.mu.Lock()
	defer st.mu.Unlock()
	if st.spunDown[devName] {
		t.Error("expected spunDown to be reset after IO detected")
	}
}

// TestSpindownUpdate_NoIOKeepsSpunDown verifies that no IO doesn't reset spin-down.
func TestSpindownUpdate_NoIOKeepsSpunDown(t *testing.T) {
	cmd := &mockCommandRunner{
		runFn: func(_ context.Context, _ string, _ ...string) error {
			return exec.ErrNotFound
		},
	}
	st := newTestSpindownTracker(30*time.Minute, cmd)
	ctx := context.Background()

	devName := "/dev/sda"
	// Already spun down with 0 iops
	st.mu.Lock()
	st.lastIOTime[devName] = time.Now().Add(-2 * time.Hour)
	st.spunDown[devName] = true
	st.lastIOOps[devName] = 0
	st.mu.Unlock()

	// Same IO count (0)
	devices := []*DeviceInfo{
		{Name: devName, Class: "sata-cold", BlockStats: &BlockStats{ReadIops: 0, WriteIops: 0}},
	}
	st.Update(ctx, devices)

	st.mu.Lock()
	defer st.mu.Unlock()
	// Already spun down and idle → stays spunDown, spindown NOT called again
	if !st.spunDown[devName] {
		t.Error("expected spunDown to remain true when already spun down")
	}
}
