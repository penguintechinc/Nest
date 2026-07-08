package driver

import (
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// VolumeInfo contains details about a provisioned volume.
type VolumeInfo struct {
	Name     string // RBD image name or CephFS subvolume name
	Pool     string // Ceph pool name (RBD only)
	Type     string // "rbd" or "cephfs"
	Size     int64  // Size in bytes
	Tenant   string // Tenant ID for scoping
	Created  time.Time
	ReadOnly bool
}

// SnapshotInfo contains details about a snapshot.
type SnapshotInfo struct {
	Name       string // Snapshot name
	VolumeID   string // Source volume ID
	Type       string // "rbd" or "cephfs"
	Pool       string // Ceph pool (RBD only)
	Size       int64  // Size in bytes
	Created    time.Time
	ParentName string // Parent volume name
}

// CephProvisioner defines the interface for provisioning Ceph volumes and snapshots.
type CephProvisioner interface {
	// CreateRBDImage creates a new RBD image in the specified pool.
	CreateRBDImage(ctx context.Context, pool, name string, sizeMB int64) (*VolumeInfo, error)

	// DeleteRBDImage removes an RBD image from the specified pool.
	DeleteRBDImage(ctx context.Context, pool, name string) error

	// ResizeRBDImage resizes an RBD image to the new size.
	ResizeRBDImage(ctx context.Context, pool, name string, sizeMB int64) error

	// CreateRBDSnapshot creates a snapshot of an RBD image.
	CreateRBDSnapshot(ctx context.Context, pool, imageName, snapshotName string) (*SnapshotInfo, error)

	// DeleteRBDSnapshot removes an RBD snapshot.
	DeleteRBDSnapshot(ctx context.Context, pool, imageName, snapshotName string) error

	// ListRBDSnapshots lists all snapshots for an RBD image.
	ListRBDSnapshots(ctx context.Context, pool, imageName string) ([]*SnapshotInfo, error)

	// CreateCephFSSubvolume creates a new CephFS subvolume.
	CreateCephFSSubvolume(ctx context.Context, fsName, subvolumeName string, sizeMB int64) (*VolumeInfo, error)

	// DeleteCephFSSubvolume removes a CephFS subvolume.
	DeleteCephFSSubvolume(ctx context.Context, fsName, subvolumeName string) error

	// ResizeCephFSSubvolume resizes a CephFS subvolume.
	ResizeCephFSSubvolume(ctx context.Context, fsName, subvolumeName string, sizeMB int64) error

	// CreateCephFSSnapshot creates a CephFS subvolume snapshot.
	CreateCephFSSnapshot(ctx context.Context, fsName, subvolumeName, snapshotName string) (*SnapshotInfo, error)

	// DeleteCephFSSnapshot removes a CephFS snapshot.
	DeleteCephFSSnapshot(ctx context.Context, fsName, subvolumeName, snapshotName string) error

	// ListCephFSSnapshots lists snapshots for a CephFS subvolume.
	ListCephFSSnapshots(ctx context.Context, fsName, subvolumeName string) ([]*SnapshotInfo, error)
}

// RealCephProvisioner implements CephProvisioner using CLI tools (rbd, ceph commands).
type RealCephProvisioner struct {
	logger  *zap.Logger
	timeout time.Duration
}

// NewRealCephProvisioner creates a new real Ceph provisioner backed by CLI tools.
func NewRealCephProvisioner(logger *zap.Logger) *RealCephProvisioner {
	if logger == nil {
		logger = zap.NewNop()
	}
	return &RealCephProvisioner{
		logger:  logger,
		timeout: 30 * time.Second,
	}
}

// execWithContext runs a command with context timeout and captures output.
func (p *RealCephProvisioner) execWithContext(ctx context.Context, name string, args ...string) (string, error) {
	// Create a context with timeout if not already set
	if _, hasDeadline := ctx.Deadline(); !hasDeadline {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, p.timeout)
		defer cancel()
	}

	cmd := exec.CommandContext(ctx, name, args...)
	output, err := cmd.CombinedOutput()

	if ctx.Err() == context.DeadlineExceeded {
		return "", status.Errorf(codes.DeadlineExceeded, "command %s timed out", name)
	}

	if err != nil {
		errMsg := strings.TrimSpace(string(output))
		// Map common Ceph errors to gRPC codes
		if strings.Contains(errMsg, "No such file or directory") || strings.Contains(errMsg, "not found") {
			return "", status.Errorf(codes.NotFound, "%s: %s", name, errMsg)
		}
		if strings.Contains(errMsg, "already exists") || strings.Contains(errMsg, "File exists") {
			return "", status.Errorf(codes.AlreadyExists, "%s: %s", name, errMsg)
		}
		if strings.Contains(errMsg, "No space left") || strings.Contains(errMsg, "ENOSPC") {
			return "", status.Errorf(codes.ResourceExhausted, "%s: %s", name, errMsg)
		}
		return "", status.Errorf(codes.Internal, "%s failed: %s", name, errMsg)
	}

	return strings.TrimSpace(string(output)), nil
}

// CreateRBDImage creates a new RBD image.
func (p *RealCephProvisioner) CreateRBDImage(ctx context.Context, pool, name string, sizeMB int64) (*VolumeInfo, error) {
	p.logger.Info("Creating RBD image", zap.String("pool", pool), zap.String("name", name), zap.Int64("sizeMB", sizeMB))

	// rbd create -p <pool> --size <size>M <name>
	_, err := p.execWithContext(ctx, "rbd", "create", "-p", pool, "--size", fmt.Sprintf("%dM", sizeMB), name)
	if err != nil {
		return nil, err
	}

	return &VolumeInfo{
		Name:    name,
		Pool:    pool,
		Type:    "rbd",
		Size:    sizeMB * 1024 * 1024,
		Created: time.Now(),
	}, nil
}

// DeleteRBDImage removes an RBD image.
func (p *RealCephProvisioner) DeleteRBDImage(ctx context.Context, pool, name string) error {
	p.logger.Info("Deleting RBD image", zap.String("pool", pool), zap.String("name", name))

	// rbd remove -p <pool> --force <name>
	_, err := p.execWithContext(ctx, "rbd", "remove", "-p", pool, "--force", name)
	return err
}

// ResizeRBDImage resizes an RBD image.
func (p *RealCephProvisioner) ResizeRBDImage(ctx context.Context, pool, name string, sizeMB int64) error {
	p.logger.Info("Resizing RBD image", zap.String("pool", pool), zap.String("name", name), zap.Int64("sizeMB", sizeMB))

	// rbd resize -p <pool> --size <size>M <name>
	_, err := p.execWithContext(ctx, "rbd", "resize", "-p", pool, "--size", fmt.Sprintf("%dM", sizeMB), name)
	return err
}

// CreateRBDSnapshot creates an RBD snapshot.
func (p *RealCephProvisioner) CreateRBDSnapshot(ctx context.Context, pool, imageName, snapshotName string) (*SnapshotInfo, error) {
	p.logger.Info("Creating RBD snapshot", zap.String("pool", pool), zap.String("image", imageName), zap.String("snapshot", snapshotName))

	// rbd snap create -p <pool> <image>@<snapshot>
	_, err := p.execWithContext(ctx, "rbd", "snap", "create", "-p", pool, imageName+"@"+snapshotName)
	if err != nil {
		return nil, err
	}

	return &SnapshotInfo{
		Name:       snapshotName,
		Type:       "rbd",
		Pool:       pool,
		Created:    time.Now(),
		ParentName: imageName,
	}, nil
}

// DeleteRBDSnapshot removes an RBD snapshot.
func (p *RealCephProvisioner) DeleteRBDSnapshot(ctx context.Context, pool, imageName, snapshotName string) error {
	p.logger.Info("Deleting RBD snapshot", zap.String("pool", pool), zap.String("image", imageName), zap.String("snapshot", snapshotName))

	// rbd snap remove -p <pool> <image>@<snapshot>
	_, err := p.execWithContext(ctx, "rbd", "snap", "remove", "-p", pool, imageName+"@"+snapshotName)
	return err
}

// ListRBDSnapshots lists RBD snapshots.
func (p *RealCephProvisioner) ListRBDSnapshots(ctx context.Context, pool, imageName string) ([]*SnapshotInfo, error) {
	p.logger.Info("Listing RBD snapshots", zap.String("pool", pool), zap.String("image", imageName))

	// rbd snap ls -p <pool> --format json <image>
	output, err := p.execWithContext(ctx, "rbd", "snap", "ls", "-p", pool, "--format", "json", imageName)
	if err != nil {
		return nil, err
	}

	var snapshots []*SnapshotInfo
	var data []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to parse snapshot list: %v", err)
	}

	for _, snap := range data {
		snapshots = append(snapshots, &SnapshotInfo{
			Name:       snap["name"].(string),
			Type:       "rbd",
			Pool:       pool,
			ParentName: imageName,
		})
	}

	return snapshots, nil
}

// CreateCephFSSubvolume creates a CephFS subvolume.
func (p *RealCephProvisioner) CreateCephFSSubvolume(ctx context.Context, fsName, subvolumeName string, sizeMB int64) (*VolumeInfo, error) {
	p.logger.Info("Creating CephFS subvolume", zap.String("fs", fsName), zap.String("subvolume", subvolumeName), zap.Int64("sizeMB", sizeMB))

	// ceph fs subvolume create <fs> <subvolume> --size <size>M
	_, err := p.execWithContext(ctx, "ceph", "fs", "subvolume", "create", fsName, subvolumeName, "--size", fmt.Sprintf("%dM", sizeMB))
	if err != nil {
		return nil, err
	}

	return &VolumeInfo{
		Name:    subvolumeName,
		Type:    "cephfs",
		Size:    sizeMB * 1024 * 1024,
		Created: time.Now(),
	}, nil
}

// DeleteCephFSSubvolume removes a CephFS subvolume.
func (p *RealCephProvisioner) DeleteCephFSSubvolume(ctx context.Context, fsName, subvolumeName string) error {
	p.logger.Info("Deleting CephFS subvolume", zap.String("fs", fsName), zap.String("subvolume", subvolumeName))

	// ceph fs subvolume rm <fs> <subvolume> --force
	_, err := p.execWithContext(ctx, "ceph", "fs", "subvolume", "rm", fsName, subvolumeName, "--force")
	return err
}

// ResizeCephFSSubvolume resizes a CephFS subvolume.
func (p *RealCephProvisioner) ResizeCephFSSubvolume(ctx context.Context, fsName, subvolumeName string, sizeMB int64) error {
	p.logger.Info("Resizing CephFS subvolume", zap.String("fs", fsName), zap.String("subvolume", subvolumeName), zap.Int64("sizeMB", sizeMB))

	// ceph fs subvolume resize <fs> <subvolume> <size>M
	_, err := p.execWithContext(ctx, "ceph", "fs", "subvolume", "resize", fsName, subvolumeName, fmt.Sprintf("%dM", sizeMB))
	return err
}

// CreateCephFSSnapshot creates a CephFS snapshot.
func (p *RealCephProvisioner) CreateCephFSSnapshot(ctx context.Context, fsName, subvolumeName, snapshotName string) (*SnapshotInfo, error) {
	p.logger.Info("Creating CephFS snapshot", zap.String("fs", fsName), zap.String("subvolume", subvolumeName), zap.String("snapshot", snapshotName))

	// ceph fs subvolume snapshot create <fs> <subvolume> <snapshot>
	_, err := p.execWithContext(ctx, "ceph", "fs", "subvolume", "snapshot", "create", fsName, subvolumeName, snapshotName)
	if err != nil {
		return nil, err
	}

	return &SnapshotInfo{
		Name:       snapshotName,
		Type:       "cephfs",
		Created:    time.Now(),
		ParentName: subvolumeName,
	}, nil
}

// DeleteCephFSSnapshot removes a CephFS snapshot.
func (p *RealCephProvisioner) DeleteCephFSSnapshot(ctx context.Context, fsName, subvolumeName, snapshotName string) error {
	p.logger.Info("Deleting CephFS snapshot", zap.String("fs", fsName), zap.String("subvolume", subvolumeName), zap.String("snapshot", snapshotName))

	// ceph fs subvolume snapshot rm <fs> <subvolume> <snapshot>
	_, err := p.execWithContext(ctx, "ceph", "fs", "subvolume", "snapshot", "rm", fsName, subvolumeName, snapshotName)
	return err
}

// ListCephFSSnapshots lists CephFS snapshots.
func (p *RealCephProvisioner) ListCephFSSnapshots(ctx context.Context, fsName, subvolumeName string) ([]*SnapshotInfo, error) {
	p.logger.Info("Listing CephFS snapshots", zap.String("fs", fsName), zap.String("subvolume", subvolumeName))

	// ceph fs subvolume snapshot ls <fs> <subvolume> --format json
	output, err := p.execWithContext(ctx, "ceph", "fs", "subvolume", "snapshot", "ls", fsName, subvolumeName, "--format", "json")
	if err != nil {
		return nil, err
	}

	var snapshots []*SnapshotInfo
	var data []map[string]interface{}
	if err := json.Unmarshal([]byte(output), &data); err != nil {
		return nil, status.Errorf(codes.Internal, "failed to parse snapshot list: %v", err)
	}

	for _, snap := range data {
		snapshots = append(snapshots, &SnapshotInfo{
			Name:       snap["name"].(string),
			Type:       "cephfs",
			Created:    time.Now(),
			ParentName: subvolumeName,
		})
	}

	return snapshots, nil
}

// FakeCephProvisioner implements CephProvisioner with in-memory storage for testing.
type FakeCephProvisioner struct {
	volumes   map[string]*VolumeInfo
	snapshots map[string]*SnapshotInfo
	logger    *zap.Logger
}

// NewFakeCephProvisioner creates a new fake provisioner for testing.
func NewFakeCephProvisioner() *FakeCephProvisioner {
	return &FakeCephProvisioner{
		volumes:   make(map[string]*VolumeInfo),
		snapshots: make(map[string]*SnapshotInfo),
		logger:    zap.NewNop(),
	}
}

// CreateRBDImage creates a fake RBD image.
func (f *FakeCephProvisioner) CreateRBDImage(ctx context.Context, pool, name string, sizeMB int64) (*VolumeInfo, error) {
	if _, exists := f.volumes[pool+"/"+name]; exists {
		return nil, status.Errorf(codes.AlreadyExists, "image %s already exists in pool %s", name, pool)
	}
	vol := &VolumeInfo{
		Name:    name,
		Pool:    pool,
		Type:    "rbd",
		Size:    sizeMB * 1024 * 1024,
		Created: time.Now(),
	}
	f.volumes[pool+"/"+name] = vol
	return vol, nil
}

// DeleteRBDImage deletes a fake RBD image.
func (f *FakeCephProvisioner) DeleteRBDImage(ctx context.Context, pool, name string) error {
	key := pool + "/" + name
	if _, exists := f.volumes[key]; !exists {
		return status.Errorf(codes.NotFound, "image %s not found in pool %s", name, pool)
	}
	delete(f.volumes, key)
	return nil
}

// ResizeRBDImage resizes a fake RBD image.
func (f *FakeCephProvisioner) ResizeRBDImage(ctx context.Context, pool, name string, sizeMB int64) error {
	key := pool + "/" + name
	vol, exists := f.volumes[key]
	if !exists {
		return status.Errorf(codes.NotFound, "image %s not found in pool %s", name, pool)
	}
	vol.Size = sizeMB * 1024 * 1024
	return nil
}

// CreateRBDSnapshot creates a fake RBD snapshot.
func (f *FakeCephProvisioner) CreateRBDSnapshot(ctx context.Context, pool, imageName, snapshotName string) (*SnapshotInfo, error) {
	key := pool + "/" + imageName
	if _, exists := f.volumes[key]; !exists {
		return nil, status.Errorf(codes.NotFound, "image %s not found in pool %s", imageName, pool)
	}

	snapKey := pool + "/" + imageName + "@" + snapshotName
	if _, exists := f.snapshots[snapKey]; exists {
		return nil, status.Errorf(codes.AlreadyExists, "snapshot %s already exists", snapshotName)
	}

	snap := &SnapshotInfo{
		Name:       snapshotName,
		VolumeID:   key,
		Type:       "rbd",
		Pool:       pool,
		Created:    time.Now(),
		ParentName: imageName,
	}
	f.snapshots[snapKey] = snap
	return snap, nil
}

// DeleteRBDSnapshot deletes a fake RBD snapshot.
func (f *FakeCephProvisioner) DeleteRBDSnapshot(ctx context.Context, pool, imageName, snapshotName string) error {
	snapKey := pool + "/" + imageName + "@" + snapshotName
	if _, exists := f.snapshots[snapKey]; !exists {
		return status.Errorf(codes.NotFound, "snapshot %s not found", snapshotName)
	}
	delete(f.snapshots, snapKey)
	return nil
}

// ListRBDSnapshots lists fake RBD snapshots.
func (f *FakeCephProvisioner) ListRBDSnapshots(ctx context.Context, pool, imageName string) ([]*SnapshotInfo, error) {
	key := pool + "/" + imageName
	if _, exists := f.volumes[key]; !exists {
		return nil, status.Errorf(codes.NotFound, "image %s not found in pool %s", imageName, pool)
	}

	var result []*SnapshotInfo
	prefix := pool + "/" + imageName + "@"
	for k, snap := range f.snapshots {
		if strings.HasPrefix(k, prefix) {
			result = append(result, snap)
		}
	}
	return result, nil
}

// CreateCephFSSubvolume creates a fake CephFS subvolume.
func (f *FakeCephProvisioner) CreateCephFSSubvolume(ctx context.Context, fsName, subvolumeName string, sizeMB int64) (*VolumeInfo, error) {
	key := fsName + "/" + subvolumeName
	if _, exists := f.volumes[key]; exists {
		return nil, status.Errorf(codes.AlreadyExists, "subvolume %s already exists in fs %s", subvolumeName, fsName)
	}
	vol := &VolumeInfo{
		Name:    subvolumeName,
		Type:    "cephfs",
		Size:    sizeMB * 1024 * 1024,
		Created: time.Now(),
	}
	f.volumes[key] = vol
	return vol, nil
}

// DeleteCephFSSubvolume deletes a fake CephFS subvolume.
func (f *FakeCephProvisioner) DeleteCephFSSubvolume(ctx context.Context, fsName, subvolumeName string) error {
	key := fsName + "/" + subvolumeName
	if _, exists := f.volumes[key]; !exists {
		return status.Errorf(codes.NotFound, "subvolume %s not found in fs %s", subvolumeName, fsName)
	}
	delete(f.volumes, key)
	return nil
}

// ResizeCephFSSubvolume resizes a fake CephFS subvolume.
func (f *FakeCephProvisioner) ResizeCephFSSubvolume(ctx context.Context, fsName, subvolumeName string, sizeMB int64) error {
	key := fsName + "/" + subvolumeName
	vol, exists := f.volumes[key]
	if !exists {
		return status.Errorf(codes.NotFound, "subvolume %s not found in fs %s", subvolumeName, fsName)
	}
	vol.Size = sizeMB * 1024 * 1024
	return nil
}

// CreateCephFSSnapshot creates a fake CephFS snapshot.
func (f *FakeCephProvisioner) CreateCephFSSnapshot(ctx context.Context, fsName, subvolumeName, snapshotName string) (*SnapshotInfo, error) {
	key := fsName + "/" + subvolumeName
	if _, exists := f.volumes[key]; !exists {
		return nil, status.Errorf(codes.NotFound, "subvolume %s not found in fs %s", subvolumeName, fsName)
	}

	snapKey := key + "@" + snapshotName
	if _, exists := f.snapshots[snapKey]; exists {
		return nil, status.Errorf(codes.AlreadyExists, "snapshot %s already exists", snapshotName)
	}

	snap := &SnapshotInfo{
		Name:       snapshotName,
		VolumeID:   key,
		Type:       "cephfs",
		Created:    time.Now(),
		ParentName: subvolumeName,
	}
	f.snapshots[snapKey] = snap
	return snap, nil
}

// DeleteCephFSSnapshot deletes a fake CephFS snapshot.
func (f *FakeCephProvisioner) DeleteCephFSSnapshot(ctx context.Context, fsName, subvolumeName, snapshotName string) error {
	snapKey := fsName + "/" + subvolumeName + "@" + snapshotName
	if _, exists := f.snapshots[snapKey]; !exists {
		return status.Errorf(codes.NotFound, "snapshot %s not found", snapshotName)
	}
	delete(f.snapshots, snapKey)
	return nil
}

// ListCephFSSnapshots lists fake CephFS snapshots.
func (f *FakeCephProvisioner) ListCephFSSnapshots(ctx context.Context, fsName, subvolumeName string) ([]*SnapshotInfo, error) {
	key := fsName + "/" + subvolumeName
	if _, exists := f.volumes[key]; !exists {
		return nil, status.Errorf(codes.NotFound, "subvolume %s not found in fs %s", subvolumeName, fsName)
	}

	var result []*SnapshotInfo
	prefix := key + "@"
	for k, snap := range f.snapshots {
		if strings.HasPrefix(k, prefix) {
			result = append(result, snap)
		}
	}
	return result, nil
}
