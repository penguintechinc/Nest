package main

import (
	"context"
	"os/exec"
	"regexp"
	"strings"
	"sync"
	"time"

	"go.uber.org/zap"
)

// RMAManager handles the RMA state machine for failed drives.
type RMAManager struct {
	mu           sync.Mutex
	nodeName     string
	failedDrives map[string]time.Time // serial → time when failure was first detected
	logger       *zap.Logger
	cmd          CommandRunner
}

// NewRMAManager creates a new RMAManager.
func NewRMAManager(nodeName string, logger *zap.Logger) *RMAManager {
	return &RMAManager{
		nodeName:     nodeName,
		failedDrives: make(map[string]time.Time),
		logger:       logger,
		cmd:          osCommandRunner{},
	}
}

// ProcessDevices checks each device's SMART health.
// For devices with SMART.Health == "FAILED":
//   - If first time seen as failed: log warning, record time
//   - If failed for > 5 minutes: call offlineCephOSD (best-effort) and return DeviceInfo with State="Failed"
//
// Returns a list of device serials that are newly marked failed.
func (m *RMAManager) ProcessDevices(ctx context.Context, devices []*DeviceInfo) []string {
	m.mu.Lock()
	defer m.mu.Unlock()

	now := time.Now()
	newlyFailed := []string{}

	for _, d := range devices {
		if d.SMART == nil || d.SMART.Health != "FAILED" {
			continue
		}

		if _, isKnownFailed := m.failedDrives[d.Serial]; !isKnownFailed {
			// First time seeing this device as failed
			m.failedDrives[d.Serial] = now
			m.logger.Warn("drive SMART health FAILED detected",
				zap.String("device", d.Name),
				zap.String("serial", d.Serial),
				zap.String("model", d.Model))
			newlyFailed = append(newlyFailed, d.Serial)
			continue
		}

		// Device was already recorded as failed; check if it's been failed long enough
		failureTime := m.failedDrives[d.Serial]
		failedDuration := now.Sub(failureTime)
		if failedDuration > 5*time.Minute {
			// Failed for more than 5 minutes — attempt to offline the OSD
			m.logger.Info("drive failed for >5 minutes, offlining Ceph OSD",
				zap.String("device", d.Name),
				zap.String("serial", d.Serial),
				zap.Duration("failedFor", failedDuration))
			_ = m.offlineCephOSD(ctx, d.Name)
			// Mark device as Failed in the DeviceInfo (in-place mutation)
			d.State = "Failed"
		}
	}

	// Clean up entries for devices no longer present
	deviceSerials := make(map[string]bool)
	for _, d := range devices {
		deviceSerials[d.Serial] = true
	}
	for serial := range m.failedDrives {
		if !deviceSerials[serial] {
			delete(m.failedDrives, serial)
		}
	}

	return newlyFailed
}

// offlineCephOSD runs `ceph osd find <device>` then `ceph osd out <osdID>` to
// remove the OSD from the Ceph cluster. Best-effort — if ceph CLI is not available,
// logs a warning and returns nil.
// In production this would be replaced by a Rook API call.
func (m *RMAManager) offlineCephOSD(ctx context.Context, devName string) error {
	// Extract the bare device name (e.g., "sda" from "/dev/sda")
	bare := strings.TrimPrefix(devName, "/dev/")

	// Try to find OSD ID for this device
	findOut, err := m.cmd.Output(ctx, "ceph", "osd", "find", bare)
	if err != nil {
		if err == exec.ErrNotFound {
			m.logger.Debug("ceph CLI not available, skipping OSD offline",
				zap.String("device", devName))
			return nil
		}
		m.logger.Warn("failed to find OSD for device",
			zap.String("device", devName),
			zap.Error(err))
		return nil
	}

	// Parse output to extract OSD ID
	// "ceph osd find <dev>" returns JSON or text like "osd.42 ..."
	// For simplicity, we'll look for the pattern "osd.N"
	osdIDRegex := regexp.MustCompile(`osd\.(\d+)`)
	matches := osdIDRegex.FindStringSubmatch(string(findOut))
	if len(matches) < 2 {
		m.logger.Warn("could not parse OSD ID from ceph output",
			zap.String("device", devName),
			zap.String("output", string(findOut)))
		return nil
	}
	osdID := matches[1]

	// Run `ceph osd out <osdID>`
	err = m.cmd.Run(ctx, "ceph", "osd", "out", osdID)
	if err != nil {
		m.logger.Warn("failed to offline Ceph OSD",
			zap.String("osdID", osdID),
			zap.String("device", devName),
			zap.Error(err))
		return err
	}

	m.logger.Info("Ceph OSD taken offline",
		zap.String("osdID", osdID),
		zap.String("device", devName))
	return nil
}
