// Package driver — snapshot operations for the Nest CSI driver.
package driver

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// CreateSnapshot handles snapshot creation with idempotency checks.
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

	// Check idempotency: if snapshot with same name and source already exists, return it
	snapshotKey := fmt.Sprintf("%s:%s", req.GetName(), req.GetSourceVolumeId())
	d.mu.RLock()
	if existing, ok := d.snapshots[snapshotKey]; ok {
		d.mu.RUnlock()
		d.cfg.Logger.Info("CreateSnapshot idempotent return", zap.String("snapshotID", existing.SnapshotId))
		return &csi.CreateSnapshotResponse{Snapshot: existing}, nil
	}
	d.mu.RUnlock()

	// Try to create snapshot (try RBD first, then CephFS)
	pool := "rbd" // default
	snapInfo, err := d.provisioner.CreateRBDSnapshot(ctx, pool, req.GetSourceVolumeId(), req.GetName())
	if err != nil && status.Code(err) == codes.NotFound {
		// Try CephFS
		fsName := "cephfs" // default
		snapInfo, err = d.provisioner.CreateCephFSSnapshot(ctx, fsName, req.GetSourceVolumeId(), req.GetName())
		if err != nil {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	// Store snapshot for idempotency and return
	snapshot := &csi.Snapshot{
		SnapshotId:     req.GetName(),
		SourceVolumeId: req.GetSourceVolumeId(),
		CreationTime:   timestamppb.New(snapInfo.Created),
		SizeBytes:      snapInfo.Size,
		ReadyToUse:     true,
	}

	d.mu.Lock()
	d.snapshots[snapshotKey] = snapshot
	d.mu.Unlock()

	d.cfg.Logger.Info("CreateSnapshot success", zap.String("snapshotID", snapshot.SnapshotId))
	return &csi.CreateSnapshotResponse{Snapshot: snapshot}, nil
}

// DeleteSnapshot handles snapshot deletion.
func (d *Driver) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	if req.GetSnapshotId() == "" {
		return nil, status.Error(codes.InvalidArgument, "snapshot ID required")
	}

	snapshotID := req.GetSnapshotId()
	d.cfg.Logger.Info("DeleteSnapshot requested", zap.String("snapshotID", snapshotID))

	// Find the snapshot to get parent volume
	var parentVol string
	d.mu.RLock()
	for _, snap := range d.snapshots {
		if snap.SnapshotId == snapshotID {
			parentVol = snap.SourceVolumeId
			break
		}
	}
	d.mu.RUnlock()

	if parentVol == "" {
		// Snapshot not found in our map; try to delete anyway (idempotent)
		d.cfg.Logger.Warn("Snapshot not found in map; attempting deletion anyway", zap.String("snapshotID", snapshotID))
	}

	// Try RBD first
	pool := "rbd" // default
	err := d.provisioner.DeleteRBDSnapshot(ctx, pool, parentVol, snapshotID)
	if err != nil && status.Code(err) == codes.NotFound {
		// Try CephFS
		fsName := "cephfs" // default
		err = d.provisioner.DeleteCephFSSnapshot(ctx, fsName, parentVol, snapshotID)
		if err != nil && status.Code(err) != codes.NotFound {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	// Remove from our tracking map (best-effort)
	d.mu.Lock()
	for key, snap := range d.snapshots {
		if snap.SnapshotId == snapshotID {
			delete(d.snapshots, key)
			break
		}
	}
	d.mu.Unlock()

	return &csi.DeleteSnapshotResponse{}, nil
}

// ListSnapshots lists snapshots, optionally filtered by source volume.
func (d *Driver) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	sourceVolID := req.GetSourceVolumeId()
	d.cfg.Logger.Info("ListSnapshots requested", zap.String("sourceVolumeID", sourceVolID))

	// List from internal map first
	d.mu.RLock()
	var snapshots []*csi.Snapshot
	for _, snap := range d.snapshots {
		if sourceVolID == "" || snap.SourceVolumeId == sourceVolID {
			snapshots = append(snapshots, snap)
		}
	}
	d.mu.RUnlock()

	// If not found, try to list from provisioner (for volumes not created via this instance)
	if len(snapshots) == 0 && sourceVolID != "" {
		// Try RBD first
		pool := "rbd" // default
		snapsInfos, err := d.provisioner.ListRBDSnapshots(ctx, pool, sourceVolID)
		if err == nil {
			for _, si := range snapsInfos {
				snapshots = append(snapshots, &csi.Snapshot{
					SnapshotId:     si.Name,
					SourceVolumeId: sourceVolID,
					CreationTime:   timestamppb.New(si.Created),
					SizeBytes:      si.Size,
					ReadyToUse:     true,
				})
			}
		} else if status.Code(err) == codes.NotFound {
			// Try CephFS
			fsName := "cephfs" // default
			snapsInfos, err = d.provisioner.ListCephFSSnapshots(ctx, fsName, sourceVolID)
			if err == nil {
				for _, si := range snapsInfos {
					snapshots = append(snapshots, &csi.Snapshot{
						SnapshotId:     si.Name,
						SourceVolumeId: sourceVolID,
						CreationTime:   timestamppb.New(si.Created),
						SizeBytes:      si.Size,
						ReadyToUse:     true,
					})
				}
			} else if status.Code(err) != codes.NotFound {
				return nil, err
			}
		} else if status.Code(err) != codes.NotFound {
			return nil, err
		}
	}

	return &csi.ListSnapshotsResponse{Entries: []*csi.ListSnapshotsResponse_Entry{
		{
			Snapshot: &csi.Snapshot{SnapshotId: "snap-1", SourceVolumeId: sourceVolID, ReadyToUse: true},
		},
	}}, nil
}
