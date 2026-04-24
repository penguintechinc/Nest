// Package driver — snapshot operations for the Nest CSI driver.
package driver

import (
	"context"
	"time"

	"go.uber.org/zap"
	"google.golang.org/protobuf/types/known/timestamppb"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// CreateSnapshot logs the request and returns a basic snapshot response.
// P2: pass-through; real Rook-CephFS/RBD snapshot delegation in P3.
func (d *Driver) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	d.cfg.Logger.Info("CreateSnapshot",
		zap.String("sourceVolumeID", req.GetSourceVolumeId()),
		zap.String("snapshotName", req.GetName()),
	)
	now := time.Now()
	return &csi.CreateSnapshotResponse{
		Snapshot: &csi.Snapshot{
			SnapshotId:     req.GetName(),
			SourceVolumeId: req.GetSourceVolumeId(),
			CreationTime:   timestamppb.New(now),
			ReadyToUse:     true,
		},
	}, nil
}

// DeleteSnapshot logs and returns an empty response.
func (d *Driver) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	d.cfg.Logger.Info("DeleteSnapshot", zap.String("snapshotID", req.GetSnapshotId()))
	return &csi.DeleteSnapshotResponse{}, nil
}

// ListSnapshots returns an empty list (P2 stub).
func (d *Driver) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	return &csi.ListSnapshotsResponse{}, nil
}
