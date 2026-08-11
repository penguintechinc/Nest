package gateway

import (
	"context"
	"fmt"
	"os"
	"syscall"
	"time"

	"github.com/godbus/dbus/v5"
)

// GaneshaReloader is the interface for signaling Ganesha to reload its configuration.
type GaneshaReloader interface {
	// Reload signals Ganesha to reload its export configuration.
	// Returns nil if reload was successful, error otherwise.
	Reload(ctx context.Context) error
}

// DBusGaneshaReloader implements GaneshaReloader using the Ganesha D-Bus API.
// Falls back to SIGHUP if D-Bus is unavailable.
type DBusGaneshaReloader struct {
	ganeshaDBusName     string
	ganeshaObjectPath   dbus.ObjectPath
	ganeshaInterface    string
	ganeshaReloadMethod string
	ganeshaSocketPath   string // Path to ganesha.nfsd PID file for SIGHUP fallback
	dbusSendTimeout     time.Duration
}

// NewDBusGaneshaReloader creates a new D-Bus Ganesha reloader.
// socketPath should point to the ganesha.nfsd process PID file for SIGHUP fallback.
func NewDBusGaneshaReloader(socketPath string) *DBusGaneshaReloader {
	return &DBusGaneshaReloader{
		ganeshaDBusName:     "org.ganesha.nfsd",
		ganeshaObjectPath:   dbus.ObjectPath("/org/ganesha/nfsd/ExportMgr"),
		ganeshaInterface:    "org.ganesha.nfsd.ExportMgr",
		ganeshaReloadMethod: "ReloadExports",
		ganeshaSocketPath:   socketPath,
		dbusSendTimeout:     5 * time.Second,
	}
}

// Reload attempts to reload Ganesha exports via D-Bus API.
// Falls back to SIGHUP if D-Bus is unavailable.
func (r *DBusGaneshaReloader) Reload(ctx context.Context) error {
	// Create a context with timeout if none provided
	if _, ok := ctx.Deadline(); !ok {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, r.dbusSendTimeout)
		defer cancel()
	}

	// Try D-Bus first
	conn, err := dbus.SystemBus()
	if err != nil {
		// D-Bus unavailable, fall back to SIGHUP
		return r.reloadViaSIGHUP()
	}
	defer conn.Close()

	obj := conn.Object(r.ganeshaDBusName, r.ganeshaObjectPath)
	call := obj.CallWithContext(ctx, r.ganeshaInterface+"."+r.ganeshaReloadMethod, 0)

	if call.Err != nil {
		// D-Bus call failed, try SIGHUP as fallback
		return r.reloadViaSIGHUP()
	}

	return nil
}

// reloadViaSIGHUP sends SIGHUP to the Ganesha process.
func (r *DBusGaneshaReloader) reloadViaSIGHUP() error {
	// Read PID from socket path (e.g., /var/run/ganesha/ganesha.pid)
	pidBytes, err := os.ReadFile(r.ganeshaSocketPath)
	if err != nil {
		return fmt.Errorf("failed to read Ganesha PID file (%s): %w", r.ganeshaSocketPath, err)
	}

	var pid int
	_, err = fmt.Sscanf(string(pidBytes), "%d", &pid)
	if err != nil {
		return fmt.Errorf("failed to parse Ganesha PID: %w", err)
	}

	// Find the process
	process, err := os.FindProcess(pid)
	if err != nil {
		return fmt.Errorf("Ganesha process (PID %d) not found: %w", pid, err)
	}

	// Send SIGHUP
	if err := process.Signal(syscall.SIGHUP); err != nil {
		return fmt.Errorf("failed to send SIGHUP to Ganesha (PID %d): %w", pid, err)
	}

	return nil
}

// FakeGaneshaReloader is a test implementation of GaneshaReloader.
// It records reload calls and allows injection of errors for testing.
type FakeGaneshaReloader struct {
	reloadCount     int
	reloadedExports []string
	shouldFail      bool
	failError       error
}

// NewFakeGaneshaReloader creates a new fake reloader for testing.
func NewFakeGaneshaReloader() *FakeGaneshaReloader {
	return &FakeGaneshaReloader{
		reloadCount:     0,
		reloadedExports: make([]string, 0),
	}
}

// Reload increments the reload count and optionally returns an error.
func (f *FakeGaneshaReloader) Reload(ctx context.Context) error {
	f.reloadCount++
	if f.shouldFail {
		if f.failError != nil {
			return f.failError
		}
		return fmt.Errorf("fake reloader configured to fail")
	}
	return nil
}

// ReloadCount returns the number of times Reload was called.
func (f *FakeGaneshaReloader) ReloadCount() int {
	return f.reloadCount
}

// SetShouldFail configures the fake reloader to fail on next reload.
func (f *FakeGaneshaReloader) SetShouldFail(shouldFail bool, err error) {
	f.shouldFail = shouldFail
	f.failError = err
}
