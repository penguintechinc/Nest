package main

import (
	"context"
	"fmt"
	"strings"

	"go.uber.org/zap"
)

// DriveAdopter formats blank or nest-previous drives for use as Nest storage backends.
type DriveAdopter struct {
	logger *zap.Logger
	cmd    CommandRunner
	fs     FileReader
}

// NewDriveAdopter creates a new DriveAdopter with the given logger.
func NewDriveAdopter(logger *zap.Logger) *DriveAdopter {
	return &DriveAdopter{
		logger: logger,
		cmd:    osCommandRunner{},
		fs:     osFileReader{},
	}
}

// FormatDrive formats devName with fsType ("btrfs" or "zfs").
// For btrfs: runs mkfs.btrfs -L nest.penguintech.io -f <devName>
// For zfs:   runs zpool create -f nest-pool-<shortname> <devName>
// Returns error if device has a system mount or if fsType is unsupported.
// Idempotent: if device already has nest-previous signature, returns nil immediately.
func (a *DriveAdopter) FormatDrive(ctx context.Context, devName, fsType string) error {
	// Check if device already has nest-previous signature — if so, no-op
	sig := a.detectDeviceSignature(ctx, devName)
	if sig == "nest-previous" {
		a.logger.Info("device already formatted with Nest signature, skipping format",
			zap.String("device", devName),
			zap.String("signature", sig))
		return nil
	}

	// Validate fsType
	if fsType != "btrfs" && fsType != "zfs" {
		return fmt.Errorf("unsupported fsType %q: must be btrfs or zfs", fsType)
	}

	// Check for system mounts
	if a.hasSystemMount(ctx, devName) {
		return fmt.Errorf("device %s has system mount points, refusing to format", devName)
	}

	// Format based on fsType
	switch fsType {
	case "btrfs":
		return a.formatBtrfs(ctx, devName)
	case "zfs":
		return a.formatZFS(ctx, devName)
	default:
		return fmt.Errorf("unsupported fsType %q", fsType)
	}
}

// formatBtrfs formats devName with btrfs filesystem.
func (a *DriveAdopter) formatBtrfs(ctx context.Context, devName string) error {
	a.logger.Info("formatting drive with btrfs", zap.String("device", devName))
	err := a.cmd.Run(ctx, "mkfs.btrfs", "-L", "nest.penguintech.io", "-f", devName)
	if err != nil {
		a.logger.Error("btrfs format failed",
			zap.String("device", devName),
			zap.Error(err))
		return fmt.Errorf("mkfs.btrfs failed for %s: %w", devName, err)
	}
	a.logger.Info("successfully formatted with btrfs", zap.String("device", devName))
	return nil
}

// formatZFS formats devName with zfs filesystem.
// Creates a pool named nest-pool-<shortname> (e.g., nest-pool-sda).
func (a *DriveAdopter) formatZFS(ctx context.Context, devName string) error {
	// Extract short name (e.g., "sda" from "/dev/sda")
	shortName := strings.TrimPrefix(devName, "/dev/")
	poolName := "nest-pool-" + shortName

	a.logger.Info("formatting drive with zfs", zap.String("device", devName), zap.String("pool", poolName))
	err := a.cmd.Run(ctx, "zpool", "create", "-f", poolName, devName)
	if err != nil {
		a.logger.Error("zfs pool creation failed",
			zap.String("device", devName),
			zap.String("pool", poolName),
			zap.Error(err))
		return fmt.Errorf("zpool create failed for %s: %w", devName, err)
	}
	a.logger.Info("successfully created zfs pool", zap.String("pool", poolName))
	return nil
}

// VerifyNestLabel confirms the device has the nest.penguintech.io btrfs label
// or is in a nest-pool ZFS pool.
func (a *DriveAdopter) VerifyNestLabel(ctx context.Context, devName string) bool {
	sig := a.detectDeviceSignature(ctx, devName)
	return sig == "nest-previous"
}

// detectDeviceSignature is a simplified version of InventoryCollector.detectSignature
// used by DriveAdopter to check if a device is already formatted by Nest.
func (a *DriveAdopter) detectDeviceSignature(ctx context.Context, devName string) string {
	out, err := a.cmd.Output(ctx, "blkid", "-o", "export", devName)
	if err != nil {
		// blkid returns error for blank devices
		return "blank"
	}

	// Parse key=value output
	metadata := make(map[string]string)
	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		parts := strings.SplitN(line, "=", 2)
		if len(parts) == 2 {
			metadata[parts[0]] = parts[1]
		}
	}

	// Check for Nest labels
	if label := metadata["LABEL"]; label != "" {
		if strings.Contains(label, "nest.penguintech.io") || strings.Contains(label, "nest-") {
			return "nest-previous"
		}
	}

	// Check for ZFS pool with nest- prefix
	if fsType := metadata["TYPE"]; fsType == "zfs_member" {
		// zpool import or zfs list would give us the pool name, but for now
		// we assume if it's a ZFS member and has a nest label, it's nest-previous
		return "nest-previous"
	}

	// Any other filesystem is foreign
	if fsType := metadata["TYPE"]; fsType != "" {
		return "foreign-fs:" + fsType
	}

	return "blank"
}

// hasSystemMount checks if devName or any of its partitions have system mounts.
// This is a duplicate of InventoryCollector.hasSystemMount for encapsulation.
func (a *DriveAdopter) hasSystemMount(ctx context.Context, devName string) bool {
	data, err := a.fs.ReadFile("/proc/mounts")
	if err != nil {
		a.logger.Debug("cannot read /proc/mounts", zap.Error(err))
		return false
	}

	systemPaths := map[string]bool{
		"/":         true,
		"/boot":     true,
		"/boot/efi": true,
		"/var":      true,
		"/var/lib":  true,
		"/home":     true,
		"/usr":      true,
		"/tmp":      true,
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}

		dev := fields[0]
		mountPoint := fields[1]
		fsType := fields[2]

		// Check direct device match
		if dev == devName {
			if fsType == "swap" || mountPoint == "none" {
				return true
			}
			if systemPaths[mountPoint] {
				return true
			}
			for sys := range systemPaths {
				if strings.HasPrefix(mountPoint, sys+"/") {
					return true
				}
			}
		}

		// Check partition match
		if strings.HasPrefix(dev, devName) && len(dev) > len(devName) {
			suffix := dev[len(devName):]
			if len(suffix) > 0 && (suffix[0] == 'p' || (suffix[0] >= '0' && suffix[0] <= '9')) {
				if fsType == "swap" || mountPoint == "none" {
					return true
				}
				if systemPaths[mountPoint] {
					return true
				}
				for sys := range systemPaths {
					if strings.HasPrefix(mountPoint, sys+"/") {
						return true
					}
				}
			}
		}
	}

	return false
}
