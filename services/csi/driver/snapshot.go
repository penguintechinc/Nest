// Package driver — snapshot operations for the Nest CSI driver.
package driver

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// CreateSnapshot proxies to Rook-Ceph RBD or CephFS controller socket.
// Routes by trying RBD first (most PVCs are block), then CephFS on NotFound.
func (d *Driver) CreateSnapshot(ctx context.Context, req *csi.CreateSnapshotRequest) (*csi.CreateSnapshotResponse, error) {
	d.cfg.Logger.Info("CreateSnapshot",
		zap.String("sourceVolumeID", req.GetSourceVolumeId()),
		zap.String("snapshotName", req.GetName()),
	)
	if d.rbdConn != nil {
		resp, err := csi.NewControllerClient(d.rbdConn).CreateSnapshot(ctx, req)
		if err == nil {
			return resp, nil
		}
		d.cfg.Logger.Debug("CreateSnapshot RBD failed, trying CephFS", zap.Error(err))
	}
	if d.cephfsConn != nil {
		return csi.NewControllerClient(d.cephfsConn).CreateSnapshot(ctx, req)
	}
	return nil, fmt.Errorf("no upstream socket configured for CreateSnapshot")
}

// DeleteSnapshot tries RBD socket first, then CephFS.
func (d *Driver) DeleteSnapshot(ctx context.Context, req *csi.DeleteSnapshotRequest) (*csi.DeleteSnapshotResponse, error) {
	d.cfg.Logger.Info("DeleteSnapshot", zap.String("snapshotID", req.GetSnapshotId()))
	if d.rbdConn != nil {
		resp, err := csi.NewControllerClient(d.rbdConn).DeleteSnapshot(ctx, req)
		if err == nil {
			return resp, nil
		}
		d.cfg.Logger.Debug("DeleteSnapshot RBD failed, trying CephFS", zap.Error(err))
	}
	if d.cephfsConn != nil {
		return csi.NewControllerClient(d.cephfsConn).DeleteSnapshot(ctx, req)
	}
	return &csi.DeleteSnapshotResponse{}, nil
}

// ListSnapshots aggregates snapshots from both RBD and CephFS sockets.
func (d *Driver) ListSnapshots(ctx context.Context, req *csi.ListSnapshotsRequest) (*csi.ListSnapshotsResponse, error) {
	var entries []*csi.ListSnapshotsResponse_Entry
	for _, conn := range []*grpc.ClientConn{d.rbdConn, d.cephfsConn} {
		if conn == nil {
			continue
		}
		resp, err := csi.NewControllerClient(conn).ListSnapshots(ctx, req)
		if err != nil {
			d.cfg.Logger.Debug("ListSnapshots upstream error", zap.Error(err))
			continue
		}
		entries = append(entries, resp.GetEntries()...)
	}
	return &csi.ListSnapshotsResponse{Entries: entries}, nil
}
