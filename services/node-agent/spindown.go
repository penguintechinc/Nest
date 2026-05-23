package main

import (
	"context"
	"os/exec"
	"sync"
	"time"

	"go.uber.org/zap"
)

// SpindownTracker tracks IO idle time for sata-cold drives
// and issues hdparm -y (standby) when idle > idleThreshold.
type SpindownTracker struct {
	mu            sync.Mutex
	lastIOTime    map[string]time.Time // keyed by device Name
	spunDown      map[string]bool      // keyed by device Name
	lastIOOps     map[string]int64     // keyed by device Name; tracks total IOPS for delta detection
	idleThreshold time.Duration
	logger        *zap.Logger
	cmd           CommandRunner
}

// NewSpindownTracker creates a SpindownTracker.
// idleThreshold: how long a drive can be idle before spin-down (default 30 min).
func NewSpindownTracker(idleThreshold time.Duration, logger *zap.Logger) *SpindownTracker {
	return &SpindownTracker{
		lastIOTime:    make(map[string]time.Time),
		spunDown:      make(map[string]bool),
		lastIOOps:     make(map[string]int64),
		idleThreshold: idleThreshold,
		logger:        logger,
		cmd:           osCommandRunner{},
	}
}

// Update is called each scan cycle with current device list.
// For each sata-cold device:
//   - If BlockStats has IO since last update → reset idle timer, mark not spun-down
//   - If idle > idleThreshold and not already spun-down → call spindown(devName)
func (s *SpindownTracker) Update(ctx context.Context, devices []*DeviceInfo) {
	s.mu.Lock()
	defer s.mu.Unlock()

	now := time.Now()

	for _, d := range devices {
		// Only process sata-cold drives
		if d.Class != "sata-cold" {
			continue
		}

		// Initialize tracking for new devices
		if _, exists := s.lastIOTime[d.Name]; !exists {
			s.lastIOTime[d.Name] = now
			s.spunDown[d.Name] = false
			s.lastIOOps[d.Name] = 0
			continue
		}

		// Check if there is IO activity since last scan
		currentIOOps := int64(0)
		if d.BlockStats != nil {
			currentIOOps = d.BlockStats.ReadIops + d.BlockStats.WriteIops
		}

		if currentIOOps != s.lastIOOps[d.Name] {
			// IO detected — reset idle timer and mark as not spun-down
			s.lastIOTime[d.Name] = now
			s.spunDown[d.Name] = false
			s.logger.Debug("IO detected on sata-cold drive, resetting idle timer",
				zap.String("device", d.Name))
		}
		s.lastIOOps[d.Name] = currentIOOps

		// Check idle time
		idleTime := now.Sub(s.lastIOTime[d.Name])
		if idleTime >= s.idleThreshold && !s.spunDown[d.Name] {
			// Idle long enough and not already spun down
			s.logger.Info("sata-cold drive idle, initiating spin-down",
				zap.String("device", d.Name),
				zap.Duration("idleTime", idleTime))
			s.spindown(ctx, d.Name)
			s.spunDown[d.Name] = true
		}
	}

	// Clean up entries for devices no longer present
	deviceNames := make(map[string]bool)
	for _, d := range devices {
		if d.Class == "sata-cold" {
			deviceNames[d.Name] = true
		}
	}
	for devName := range s.lastIOTime {
		if !deviceNames[devName] {
			delete(s.lastIOTime, devName)
			delete(s.spunDown, devName)
			delete(s.lastIOOps, devName)
		}
	}
}

// spindown issues `hdparm -y <devName>` to spin the drive down.
// Logs the result. If hdparm is not available, logs a debug message and returns.
func (s *SpindownTracker) spindown(ctx context.Context, devName string) {
	err := s.cmd.Run(ctx, "hdparm", "-y", devName)
	if err != nil {
		if _, ok := err.(*exec.ExitError); ok {
			// hdparm returned non-zero exit code — log as debug (drive may already be spinning down)
			s.logger.Debug("hdparm returned non-zero exit code",
				zap.String("device", devName),
				zap.Error(err))
			return
		}
		if err == exec.ErrNotFound {
			s.logger.Debug("hdparm not available, skipping spin-down",
				zap.String("device", devName))
			return
		}
		s.logger.Warn("failed to spin down drive",
			zap.String("device", devName),
			zap.Error(err))
		return
	}
	s.logger.Info("drive spun down successfully",
		zap.String("device", devName))
}
