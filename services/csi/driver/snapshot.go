// Package driver — snapshot operations for the Nest CSI driver.
package driver

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// CreateSnapshot handles snapshot creation with idempotency checks.
// P2: real Rook-CephFS/RBD snapshot delegation in P3.
// Returns Unimplemented since real backend snapshot creation is not yet implemented.
func (d *Driver) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	// Validate request
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot name required")
	}
	if req.GetSourceVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "source volume ID required")
	}

	d.cfg.Logger.Info("CreateSnapshot requested",
		zap.String("sourceVolumeID", req.GetSourceVolumeId()),
		zap.String("snapshotName", req.GetName()),
	)

	// Check idempotency: if snapshot with same name and source already exists, return it (ALREADY_EXISTS semantics)
	snapshotKey := fmt.Sprintf("%s:%s", req.GetName(), req.GetSourceVolumeId())
	d.mu.RLock()
	if existing, ok := d.snapshots[snapshotKey]; ok {
		d.mu.RUnlock()
		d.cfg.Logger.Info("CreateSnapshot idempotent return", zap.String("snapshotID", existing.SnapshotId))
		return &csi.CreateSnapshotResponse{Snapshot: existing}, nil
	}
	d.mu.RUnlock()

	// P2: real snapshot creation not yet implemented.
	// Return Unimplemented to prevent callers from assuming snapshot was created.
	return nil, status.Error(codes.Unimplemented, "CreateSnapshot requires real Ceph snapshot implementation; P2 feature")
}

// DeleteSnapshot handles snapshot deletion.
// P2: real Ceph snapshot deletion not yet implemented.
func (d *Driver) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	if req.GetSnapshotId() == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot ID required")
	}

	d.cfg.Logger.Info("DeleteSnapshot requested", zap.String("snapshotID", req.GetSnapshotId()))

	// P2: real snapshot deletion not yet implemented.
	// Return Unimplemented to prevent callers from assuming snapshot was deleted.
	return nil, status.Error(codes.Unimplemented, "DeleteSnapshot requires real Ceph snapshot implementation; P2 feature")
}

// ListSnapshots returns an error since snapshot listing is not implemented.
func (d *Driver) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	// P2: real snapshot listing not yet implemented.
	// Return Unimplemented to prevent external-snapshotter from assuming snapshots exist.
	return nil, status.Error(codes.Unimplemented, "ListSnapshots requires real Ceph snapshot implementation; P2 feature")
}
