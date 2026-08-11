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
// Returns error if device is mounted, has foreign filesystem, or is an active member (unless force=true).
// Idempotent: if device already has nest-previous signature, returns nil immediately.
func (a *DriveAdopter) FormatDrive(ctx context.Context, devName, fsType string, force bool) error {
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

	// Safety gates: refuse format unless force=true
	if !force {
		// Check if currently mounted anywhere
		if a.isCurrentlyMounted(ctx, devName) {
			return fmt.Errorf("device %s is currently mounted, refusing to format without force=true", devName)
		}

		// Check for foreign filesystem
		if strings.HasPrefix(sig, "foreign-fs:") {
			return fmt.Errorf("device %s has foreign filesystem (%s), refusing to format without force=true", devName, sig)
		}

		// Check for active LVM/mdraid/ZFS membership
		if a.isActiveMember(ctx, devName) {
			return fmt.Errorf("device %s is an active LVM/mdraid/ZFS/Ceph member, refusing to format without force=true", devName)
		}

		// Check for system mounts
		if a.hasSystemMount(ctx, devName) {
			return fmt.Errorf("device %s has system mount points, refusing to format", devName)
		}
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
		// blkid error — fail closed, treat as unknown
		return "unknown"
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

	// Check for Nest labels (btrfs/ZFS with nest label)
	if label := metadata["LABEL"]; label != "" {
		if strings.Contains(label, "nest.penguintech.io") || strings.Contains(label, "nest-") {
			return "nest-previous"
		}
	}

	// Check for Ceph BlueStore
	if fsType := metadata["TYPE"]; fsType == "ceph_bluestore" {
		return "foreign-fs:ceph_bluestore"
	}

	// Check for ZFS member — only nest-previous if it has nest label (checked above)
	// Otherwise, it's a foreign ZFS pool
	if fsType := metadata["TYPE"]; fsType == "zfs_member" {
		return "foreign-fs:zfs_member"
	}

	// Any other filesystem is foreign
	if fsType := metadata["TYPE"]; fsType != "" {
		return "foreign-fs:" + fsType
	}

	return "blank"
}

// isCurrentlyMounted checks if devName or any of its partitions are mounted anywhere
func (a *DriveAdopter) isCurrentlyMounted(ctx context.Context, devName string) bool {
	data, err := a.fs.ReadFile("/proc/mounts")
	if err != nil {
		a.logger.Debug("cannot read /proc/mounts", zap.Error(err))
		return false
	}

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		fields := strings.Fields(line)
		if len(fields) < 2 {
			continue
		}

		dev := fields[0]
		// Check direct device match
		if dev == devName {
			return true
		}

		// Check partition match
		if strings.HasPrefix(dev, devName) && len(dev) > len(devName) {
			suffix := dev[len(devName):]
			if len(suffix) > 0 && (suffix[0] == 'p' || (suffix[0] >= '0' && suffix[0] <= '9')) {
				return true
			}
		}
	}

	return false
}

// isActiveMember checks if devName is an active LVM PV, mdraid member, ZFS pool member, or Ceph BlueStore OSD
func (a *DriveAdopter) isActiveMember(ctx context.Context, devName string) bool {
	// Check if currently mounted
	if a.isCurrentlyMounted(ctx, devName) {
		return true
	}

	// Check for Ceph BlueStore
	if a.isCephBlueStore(ctx, devName) {
		return true
	}

	// Check LVM PV
	out, err := a.cmd.Output(ctx, "pvdisplay", "--noheadings", "-C", "-o", "vg_name", devName)
	if err == nil {
		vgName := strings.TrimSpace(string(out))
		if vgName != "" {
			return true
		}
	}

	// Check mdraid
	out, err = a.cmd.Output(ctx, "mdadm", "--examine", devName)
	if err == nil && len(strings.TrimSpace(string(out))) > 0 {
		return true
	}

	// Check ZFS pool
	out, err = a.cmd.Output(ctx, "zpool", "status")
	if err == nil && strings.Contains(string(out), devName) {
		return true
	}

	return false
}

// isCephBlueStore checks if devName has a Ceph BlueStore signature
func (a *DriveAdopter) isCephBlueStore(ctx context.Context, devName string) bool {
	out, err := a.cmd.Output(ctx, "blkid", "-o", "export", devName)
	if err != nil {
		return false
	}

	lines := strings.Split(string(out), "\n")
	for _, line := range lines {
		if strings.HasPrefix(line, "TYPE=ceph_bluestore") {
			return true
		}
	}

	return false
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
