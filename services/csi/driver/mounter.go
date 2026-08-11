package driver

import (
	"context"
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"syscall"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// VolumeStats represents filesystem statistics.
type VolumeStats struct {
	AvailableBytes  int64
	TotalBytes      int64
	UsedBytes       int64
	AvailableInodes int64
	TotalInodes     int64
	UsedInodes      int64
}

// Mounter defines the interface for staging and publishing volumes on nodes.
type Mounter interface {
	// StageVolume stages a volume at the staging path (mounts RBD/CephFS to staging path).
	StageVolume(ctx context.Context, volumeID string, stagingPath string, volumeContext map[string]string) error

	// UnstageVolume unstages a volume (unmounts from staging path).
	UnstageVolume(ctx context.Context, volumeID string, stagingPath string) error

	// PublishVolume publishes a staged volume to the target path (bind mount or mount from staging).
	PublishVolume(ctx context.Context, volumeID string, stagingPath, targetPath string, readOnly bool) error

	// UnpublishVolume unpublishes a volume (unmounts from target path).
	UnpublishVolume(ctx context.Context, volumeID string, targetPath string) error

	// GetVolumeStats returns filesystem statistics for a volume.
	GetVolumeStats(ctx context.Context, volumePath string) (*VolumeStats, error)

	// ExpandFilesystem expands the filesystem on a volume to use available capacity.
	ExpandFilesystem(ctx context.Context, volumePath string) error
}

// RealMounter implements Mounter using real system calls and CLI tools.
type RealMounter struct {
	logger  *zap.Logger
	timeout time.Duration
}

// NewRealMounter creates a new real mounter.
func NewRealMounter(logger *zap.Logger) *RealMounter {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RealMounter{
		logger:  logger,
		timeout: 30 * time.Second,
	}
}

// StageVolume stages an RBD or CephFS volume.
func (m *RealMounter) StageVolume(ctx context.Context, volumeID string, stagingPath string, volumeContext map[string]string) error {
	m.logger.Info("Staging volume", zap.String("volumeID", volumeID), zap.String("stagingPath", stagingPath))

	// Ensure staging path exists
	if err := os.MkdirAll(stagingPath, 0750); err != nil {
		return status.Errorf(codes.Internal, "failed to create staging path: %v", err)
	}

	volumeType := volumeContext["volumeType"]
	if volumeType == "cephfs" {
		// Mount CephFS
		fsName := volumeContext["fsName"]
		subvolumePath := volumeContext["subvolumePath"]
		if fsName == "" || subvolumePath == "" {
			return status.Errorf(codes.InvalidArgument, "fsName and subvolumePath required for CephFS volumes")
		}
		return m.mountCephFS(ctx, fsName, subvolumePath, stagingPath)
	}

	// Default: RBD
	return m.mountRBD(ctx, volumeID, stagingPath)
}

// mountRBD maps and mounts an RBD image.
func (m *RealMounter) mountRBD(ctx context.Context, rbdImageName string, mountPath string) error {
	// rbd map <image>
	device, err := m.rbdMap(ctx, rbdImageName)
	if err != nil {
		return err
	}

	// mkfs.ext4 if needed (idempotent check)
	if err := m.mkfsIfNeeded(ctx, device); err != nil {
		_ = m.rbdUnmap(ctx, device)
		return err
	}

	// mount <device> <mountPath>
	if err := m.mount(ctx, device, mountPath, "ext4", ""); err != nil {
		_ = m.rbdUnmap(ctx, device)
		return err
	}

	return nil
}

// rbdMap maps an RBD image to a block device.
func (m *RealMounter) rbdMap(ctx context.Context, imageName string) (string, error) {
	cmd := exec.CommandContext(ctx, "rbd", "map", imageName)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return "", status.Errorf(codes.Internal, "rbd map failed: %v", err)
	}
	device := strings.TrimSpace(string(output))
	m.logger.Info("Mapped RBD image", zap.String("image", imageName), zap.String("device", device))
	return device, nil
}

// rbdUnmap unmaps an RBD device.
func (m *RealMounter) rbdUnmap(ctx context.Context, device string) error {
	cmd := exec.CommandContext(ctx, "rbd", "unmap", device)
	_, err := cmd.CombinedOutput()
	if err != nil {
		m.logger.Warn("rbd unmap failed", zap.String("device", device), zap.Error(err))
		// Continue anyway; error may be transient
	}
	return nil
}

// mkfsIfNeeded formats a device if it hasn't been formatted yet.
func (m *RealMounter) mkfsIfNeeded(ctx context.Context, device string) error {
	// Try to detect filesystem; if none exists, format it
	cmd := exec.CommandContext(ctx, "blkid", device)
	err := cmd.Run()
	if err == nil {
		// Filesystem already exists
		return nil
	}

	m.logger.Info("Formatting device", zap.String("device", device))
	cmd = exec.CommandContext(ctx, "mkfs.ext4", "-F", device)
	if _, err := cmd.CombinedOutput(); err != nil {
		return status.Errorf(codes.Internal, "mkfs.ext4 failed: %v", err)
	}
	return nil
}

// mount mounts a filesystem.
func (m *RealMounter) mount(ctx context.Context, source, target, fstype, options string) error {
	m.logger.Info("Mounting filesystem", zap.String("source", source), zap.String("target", target), zap.String("fstype", fstype))

	cmd := exec.CommandContext(ctx, "mount")
	if fstype != "" {
		cmd.Args = append(cmd.Args, "-t", fstype)
	}
	if options != "" {
		cmd.Args = append(cmd.Args, "-o", options)
	}
	cmd.Args = append(cmd.Args, source, target)

	if _, err := cmd.CombinedOutput(); err != nil {
		return status.Errorf(codes.Internal, "mount failed: %v", err)
	}
	return nil
}

// mountCephFS mounts a CephFS subvolume.
func (m *RealMounter) mountCephFS(ctx context.Context, fsName, subvolumePath, mountPath string) error {
	m.logger.Info("Mounting CephFS", zap.String("fs", fsName), zap.String("subvolume", subvolumePath), zap.String("mountPath", mountPath))

	// mount -t ceph -o name=admin,secret=<secret> <mon1>:<mon2>:/<subvolume> <mountPath>
	// For now, use a simplified mount with name=admin (requires cluster config)
	cmd := exec.CommandContext(ctx, "mount", "-t", "ceph", "-o", "name=admin", "mon1:/"+subvolumePath, mountPath)
	if _, err := cmd.CombinedOutput(); err != nil {
		return status.Errorf(codes.Internal, "ceph mount failed: %v", err)
	}
	return nil
}

// UnstageVolume unstages a volume (unmounts from staging path).
func (m *RealMounter) UnstageVolume(ctx context.Context, volumeID string, stagingPath string) error {
	m.logger.Info("Unstaging volume", zap.String("volumeID", volumeID), zap.String("stagingPath", stagingPath))

	if err := m.unmount(ctx, stagingPath); err != nil {
		return err
	}

	// Clean up staging path (best-effort)
	_ = os.RemoveAll(stagingPath)
	return nil
}

// unmount unmounts a filesystem.
func (m *RealMounter) unmount(ctx context.Context, path string) error {
	m.logger.Info("Unmounting path", zap.String("path", path))

	cmd := exec.CommandContext(ctx, "umount", path)
	if _, err := cmd.CombinedOutput(); err != nil {
		// Ignore "not mounted" errors
		if !strings.Contains(err.Error(), "not mounted") {
			return status.Errorf(codes.Internal, "umount failed: %v", err)
		}
	}
	return nil
}

// PublishVolume publishes a volume to a target path (bind mount or mount from staging).
func (m *RealMounter) PublishVolume(ctx context.Context, volumeID string, stagingPath, targetPath string, readOnly bool) error {
	m.logger.Info("Publishing volume", zap.String("volumeID", volumeID), zap.String("stagingPath", stagingPath), zap.String("targetPath", targetPath), zap.Bool("readOnly", readOnly))

	// Ensure target path parent exists
	if err := os.MkdirAll(filepath.Dir(targetPath), 0750); err != nil {
		return status.Errorf(codes.Internal, "failed to create target path parent: %v", err)
	}

	// Create target as file or directory depending on staging path
	if fi, err := os.Stat(stagingPath); err == nil && fi.IsDir() {
		if err := os.MkdirAll(targetPath, 0750); err != nil {
			return status.Errorf(codes.Internal, "failed to create target directory: %v", err)
		}
	} else {
		if err := os.MkdirAll(filepath.Dir(targetPath), 0750); err != nil {
			return status.Errorf(codes.Internal, "failed to create target parent: %v", err)
		}
	}

	// Bind mount staging to target
	options := "bind"
	if readOnly {
		options = "bind,ro"
	}

	cmd := exec.CommandContext(ctx, "mount", "-o", options, stagingPath, targetPath)
	if _, err := cmd.CombinedOutput(); err != nil {
		return status.Errorf(codes.Internal, "bind mount failed: %v", err)
	}
	return nil
}

// UnpublishVolume unpublishes a volume (unmounts from target path).
func (m *RealMounter) UnpublishVolume(ctx context.Context, volumeID string, targetPath string) error {
	m.logger.Info("Unpublishing volume", zap.String("volumeID", volumeID), zap.String("targetPath", targetPath))

	if err := m.unmount(ctx, targetPath); err != nil {
		return err
	}

	// Clean up target path (best-effort)
	_ = os.RemoveAll(targetPath)
	return nil
}

// GetVolumeStats returns filesystem statistics.
func (m *RealMounter) GetVolumeStats(ctx context.Context, volumePath string) (*VolumeStats, error) {
	m.logger.Info("Getting volume stats", zap.String("volumePath", volumePath))

	var stat syscall.Statfs_t
	if err := syscall.Statfs(volumePath, &stat); err != nil {
		return nil, status.Errorf(codes.Internal, "statfs failed: %v", err)
	}

	bsize := int64(stat.Bsize)
	return &VolumeStats{
		AvailableBytes:  clampInt64(stat.Bavail) * bsize,
		TotalBytes:      clampInt64(stat.Blocks) * bsize,
		UsedBytes:       (clampInt64(stat.Blocks) - clampInt64(stat.Bfree)) * bsize,
		AvailableInodes: clampInt64(stat.Ffree),
		TotalInodes:     clampInt64(stat.Files),
		UsedInodes:      clampInt64(stat.Files) - clampInt64(stat.Ffree),
	}, nil
}

// clampInt64 converts a uint64 to int64, clamping to math.MaxInt64 to avoid
// signed integer overflow (gosec G115) on pathological statfs values.
func clampInt64(u uint64) int64 {
	if u > uint64(math.MaxInt64) {
		return math.MaxInt64
	}
	return int64(u)
}

// ExpandFilesystem expands the filesystem on a volume.
func (m *RealMounter) ExpandFilesystem(ctx context.Context, volumePath string) error {
	m.logger.Info("Expanding filesystem", zap.String("volumePath", volumePath))

	// Detect filesystem type and expand accordingly
	cmd := exec.CommandContext(ctx, "df", "-T", volumePath)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return status.Errorf(codes.Internal, "df -T failed: %v", err)
	}

	lines := strings.Split(string(output), "\n")
	if len(lines) < 2 {
		return status.Errorf(codes.Internal, "unexpected df output")
	}

	fields := strings.Fields(lines[1])
	if len(fields) < 2 {
		return status.Errorf(codes.Internal, "unexpected df field count")
	}

	fsType := fields[1]
	m.logger.Info("Detected filesystem type", zap.String("fsType", fsType))

	// Expand based on filesystem type
	switch fsType {
	case "ext4":
		return m.expandExt4(ctx, volumePath)
	case "xfs":
		return m.expandXFS(ctx, volumePath)
	case "ceph":
		// CephFS doesn't need explicit expansion
		return nil
	default:
		return status.Errorf(codes.Internal, "unsupported filesystem type: %s", fsType)
	}
}

// expandExt4 expands an ext4 filesystem.
func (m *RealMounter) expandExt4(ctx context.Context, volumePath string) error {
	cmd := exec.CommandContext(ctx, "resize2fs", volumePath)
	if _, err := cmd.CombinedOutput(); err != nil {
		return status.Errorf(codes.Internal, "resize2fs failed: %v", err)
	}
	return nil
}

// expandXFS expands an XFS filesystem.
func (m *RealMounter) expandXFS(ctx context.Context, volumePath string) error {
	cmd := exec.CommandContext(ctx, "xfs_growfs", volumePath)
	if _, err := cmd.CombinedOutput(); err != nil {
		return status.Errorf(codes.Internal, "xfs_growfs failed: %v", err)
	}
	return nil
}

// FakeMounter implements Mounter with in-memory tracking for testing.
type FakeMounter struct {
	mu         sync.Mutex
	stagedVols map[string]string // volumeID -> stagingPath
	pubVols    map[string]string // targetPath -> volumeID
	stats      map[string]*VolumeStats
	logger     *zap.Logger
}

// NewFakeMounter creates a new fake mounter.
func NewFakeMounter() *FakeMounter {
	return &FakeMounter{
		stagedVols: make(map[string]string),
		pubVols:    make(map[string]string),
		stats:      make(map[string]*VolumeStats),
		logger:     zap.NewNop(),
	}
}

// StageVolume stages a volume (fake).
func (f *FakeMounter) StageVolume(ctx context.Context, volumeID string, stagingPath string, volumeContext map[string]string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if existing, exists := f.stagedVols[volumeID]; exists && existing != stagingPath {
		return status.Errorf(codes.AlreadyExists, "volume %s already staged at %s", volumeID, existing)
	}

	f.stagedVols[volumeID] = stagingPath

	// Record stats for this volume (10 GiB default)
	f.stats[stagingPath] = &VolumeStats{
		AvailableBytes:  10 * 1024 * 1024 * 1024,
		TotalBytes:      10 * 1024 * 1024 * 1024,
		UsedBytes:       0,
		AvailableInodes: 1000000,
		TotalInodes:     1000000,
		UsedInodes:      0,
	}

	return nil
}

// UnstageVolume unstages a volume (fake).
func (f *FakeMounter) UnstageVolume(ctx context.Context, volumeID string, stagingPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if existing, exists := f.stagedVols[volumeID]; !exists || existing != stagingPath {
		return status.Errorf(codes.NotFound, "volume %s not staged at %s", volumeID, stagingPath)
	}

	delete(f.stagedVols, volumeID)
	delete(f.stats, stagingPath)
	return nil
}

// PublishVolume publishes a volume to a target (fake).
func (f *FakeMounter) PublishVolume(ctx context.Context, volumeID string, stagingPath, targetPath string, readOnly bool) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	// Verify volume is staged
	if staged, exists := f.stagedVols[volumeID]; !exists || staged != stagingPath {
		return status.Errorf(codes.FailedPrecondition, "volume %s not staged at %s", volumeID, stagingPath)
	}

	if existing, exists := f.pubVols[targetPath]; exists {
		if existing != volumeID {
			return status.Errorf(codes.AlreadyExists, "target %s already published with volume %s", targetPath, existing)
		}
		// Idempotent: already published
		return nil
	}

	f.pubVols[targetPath] = volumeID
	// Inherit stats from staging
	if stats, exists := f.stats[stagingPath]; exists {
		f.stats[targetPath] = &VolumeStats{
			AvailableBytes:  stats.AvailableBytes,
			TotalBytes:      stats.TotalBytes,
			UsedBytes:       stats.UsedBytes,
			AvailableInodes: stats.AvailableInodes,
			TotalInodes:     stats.TotalInodes,
			UsedInodes:      stats.UsedInodes,
		}
	}

	return nil
}

// UnpublishVolume unpublishes a volume (fake).
func (f *FakeMounter) UnpublishVolume(ctx context.Context, volumeID string, targetPath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if existing, exists := f.pubVols[targetPath]; !exists || existing != volumeID {
		return status.Errorf(codes.NotFound, "target %s not published with volume %s", targetPath, volumeID)
	}

	delete(f.pubVols, targetPath)
	delete(f.stats, targetPath)
	return nil
}

// GetVolumeStats returns fake stats.
func (f *FakeMounter) GetVolumeStats(ctx context.Context, volumePath string) (*VolumeStats, error) {
	f.mu.Lock()
	defer f.mu.Unlock()

	if stats, exists := f.stats[volumePath]; exists {
		return stats, nil
	}
	return nil, status.Errorf(codes.NotFound, "volume stats not found for %s", volumePath)
}

// ExpandFilesystem expands a fake filesystem (just updates stats).
func (f *FakeMounter) ExpandFilesystem(ctx context.Context, volumePath string) error {
	f.mu.Lock()
	defer f.mu.Unlock()

	if stats, exists := f.stats[volumePath]; exists {
		// Double the available bytes (simulate expansion)
		stats.TotalBytes *= 2
		stats.AvailableBytes = stats.TotalBytes - stats.UsedBytes
		return nil
	}
	return status.Errorf(codes.NotFound, "volume stats not found for %s", volumePath)
}
