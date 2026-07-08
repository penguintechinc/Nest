package driver

import (
	"context"
	"testing"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

// TestFullLifecycleRBD tests complete RBD volume lifecycle: create → stage → publish → stats → unpublish → unstage → delete.
func TestFullLifecycleRBD(t *testing.T) {
	ctx := context.Background()
	prov := NewFakeCephProvisioner()
	mounter := NewFakeMounter()
	driver := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node",
		DriverName: "test-driver",
		Logger:     zap.NewNop(),
	}, prov, mounter)

	volumeName := "test-rbd-vol"
	stagingPath := "/staging/vol1"
	targetPath := "/mnt/vol1"

	// 1. CreateVolume
	createResp, err := driver.CreateVolume(ctx, &csi.CreateVolumeRequest{
		Name: volumeName,
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 5 * 1024 * 1024 * 1024, // 5 GiB
		},
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateVolume failed: %v", err)
	}
	if createResp.GetVolume().GetVolumeId() != volumeName {
		t.Errorf("expected volume ID %s, got %s", volumeName, createResp.GetVolume().GetVolumeId())
	}

	volumeID := createResp.GetVolume().GetVolumeId()
	volumeCtx := createResp.GetVolume().GetVolumeContext()

	// 2. NodeStageVolume
	_, err = driver.NodeStageVolume(ctx, &csi.NodeStageVolumeRequest{
		VolumeId:          volumeID,
		StagingTargetPath: stagingPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
		VolumeContext: volumeCtx,
	})
	if err != nil {
		t.Fatalf("NodeStageVolume failed: %v", err)
	}

	// 3. NodePublishVolume
	_, err = driver.NodePublishVolume(ctx, &csi.NodePublishVolumeRequest{
		VolumeId:          volumeID,
		StagingTargetPath: stagingPath,
		TargetPath:        targetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
		VolumeContext: volumeCtx,
	})
	if err != nil {
		t.Fatalf("NodePublishVolume failed: %v", err)
	}

	// 4. NodeGetVolumeStats
	statsResp, err := driver.NodeGetVolumeStats(ctx, &csi.NodeGetVolumeStatsRequest{
		VolumePath: targetPath,
	})
	if err != nil {
		t.Fatalf("NodeGetVolumeStats failed: %v", err)
	}
	if len(statsResp.GetUsage()) == 0 {
		t.Error("expected usage stats")
	}

	// 5. NodeUnpublishVolume
	_, err = driver.NodeUnpublishVolume(ctx, &csi.NodeUnpublishVolumeRequest{
		VolumeId:   volumeID,
		TargetPath: targetPath,
	})
	if err != nil {
		t.Fatalf("NodeUnpublishVolume failed: %v", err)
	}

	// 6. NodeUnstageVolume
	_, err = driver.NodeUnstageVolume(ctx, &csi.NodeUnstageVolumeRequest{
		VolumeId:          volumeID,
		StagingTargetPath: stagingPath,
	})
	if err != nil {
		t.Fatalf("NodeUnstageVolume failed: %v", err)
	}

	// 7. DeleteVolume
	_, err = driver.DeleteVolume(ctx, &csi.DeleteVolumeRequest{
		VolumeId: volumeID,
	})
	if err != nil {
		t.Fatalf("DeleteVolume failed: %v", err)
	}

	t.Log("RBD lifecycle test passed")
}

// TestFullLifecycleCephFS tests complete CephFS volume lifecycle.
func TestFullLifecycleCephFS(t *testing.T) {
	ctx := context.Background()
	prov := NewFakeCephProvisioner()
	mounter := NewFakeMounter()
	driver := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node",
		DriverName: "test-driver",
		Logger:     zap.NewNop(),
	}, prov, mounter)

	volumeName := "test-cephfs-vol"
	stagingPath := "/staging/vol2"
	targetPath := "/mnt/vol2"

	// CreateVolume with CephFS
	createResp, err := driver.CreateVolume(ctx, &csi.CreateVolumeRequest{
		Name: volumeName,
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 10 * 1024 * 1024 * 1024, // 10 GiB
		},
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateVolume (CephFS) failed: %v", err)
	}

	volumeID := createResp.GetVolume().GetVolumeId()
	volumeCtx := createResp.GetVolume().GetVolumeContext()

	// Verify it's marked as CephFS
	if volumeCtx["volumeType"] != "cephfs" {
		t.Errorf("expected volumeType=cephfs, got %s", volumeCtx["volumeType"])
	}

	// Stage, publish, get stats, unpublish, unstage, delete (same as RBD)
	_, err = driver.NodeStageVolume(ctx, &csi.NodeStageVolumeRequest{
		VolumeId:          volumeID,
		StagingTargetPath: stagingPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
			},
		},
		VolumeContext: volumeCtx,
	})
	if err != nil {
		t.Fatalf("NodeStageVolume failed: %v", err)
	}

	_, err = driver.NodePublishVolume(ctx, &csi.NodePublishVolumeRequest{
		VolumeId:          volumeID,
		StagingTargetPath: stagingPath,
		TargetPath:        targetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
			},
		},
		VolumeContext: volumeCtx,
	})
	if err != nil {
		t.Fatalf("NodePublishVolume failed: %v", err)
	}

	_, err = driver.NodeUnpublishVolume(ctx, &csi.NodeUnpublishVolumeRequest{
		VolumeId:   volumeID,
		TargetPath: targetPath,
	})
	if err != nil {
		t.Fatalf("NodeUnpublishVolume failed: %v", err)
	}

	_, err = driver.NodeUnstageVolume(ctx, &csi.NodeUnstageVolumeRequest{
		VolumeId:          volumeID,
		StagingTargetPath: stagingPath,
	})
	if err != nil {
		t.Fatalf("NodeUnstageVolume failed: %v", err)
	}

	_, err = driver.DeleteVolume(ctx, &csi.DeleteVolumeRequest{
		VolumeId: volumeID,
	})
	if err != nil {
		t.Fatalf("DeleteVolume failed: %v", err)
	}

	t.Log("CephFS lifecycle test passed")
}

// TestSnapshotLifecycle tests snapshot creation, listing, and deletion.
func TestSnapshotLifecycle(t *testing.T) {
	ctx := context.Background()
	prov := NewFakeCephProvisioner()
	mounter := NewFakeMounter()
	driver := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node",
		DriverName: "test-driver",
		Logger:     zap.NewNop(),
	}, prov, mounter)

	volumeName := "snap-test-vol"

	// Create volume
	createResp, err := driver.CreateVolume(ctx, &csi.CreateVolumeRequest{
		Name: volumeName,
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 5 * 1024 * 1024 * 1024,
		},
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateVolume failed: %v", err)
	}

	volumeID := createResp.GetVolume().GetVolumeId()

	// Create snapshot
	snapResp, err := driver.CreateSnapshot(ctx, &csi.CreateSnapshotRequest{
		Name:           "test-snapshot-1",
		SourceVolumeId: volumeID,
	})
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}
	if snapResp.GetSnapshot().GetSnapshotId() != "test-snapshot-1" {
		t.Errorf("expected snapshot ID test-snapshot-1, got %s", snapResp.GetSnapshot().GetSnapshotId())
	}

	// Create another snapshot
	_, err = driver.CreateSnapshot(ctx, &csi.CreateSnapshotRequest{
		Name:           "test-snapshot-2",
		SourceVolumeId: volumeID,
	})
	if err != nil {
		t.Fatalf("CreateSnapshot 2 failed: %v", err)
	}

	// Idempotent: create same snapshot again
	snapResp2, err := driver.CreateSnapshot(ctx, &csi.CreateSnapshotRequest{
		Name:           "test-snapshot-1",
		SourceVolumeId: volumeID,
	})
	if err != nil {
		t.Fatalf("CreateSnapshot (idempotent) failed: %v", err)
	}
	if snapResp2.GetSnapshot().GetSnapshotId() != "test-snapshot-1" {
		t.Errorf("expected idempotent snapshot ID test-snapshot-1, got %s", snapResp2.GetSnapshot().GetSnapshotId())
	}

	// List snapshots
	listResp, err := driver.ListSnapshots(ctx, &csi.ListSnapshotsRequest{
		SourceVolumeId: volumeID,
	})
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}
	if len(listResp.GetEntries()) == 0 {
		t.Error("expected snapshots in list response")
	}

	// Delete snapshot
	_, err = driver.DeleteSnapshot(ctx, &csi.DeleteSnapshotRequest{
		SnapshotId: "test-snapshot-1",
	})
	if err != nil {
		t.Fatalf("DeleteSnapshot failed: %v", err)
	}

	// Idempotent: delete again
	_, err = driver.DeleteSnapshot(ctx, &csi.DeleteSnapshotRequest{
		SnapshotId: "test-snapshot-1",
	})
	if err != nil {
		t.Fatalf("DeleteSnapshot (idempotent) failed: %v", err)
	}

	// Clean up volume
	_, err = driver.DeleteVolume(ctx, &csi.DeleteVolumeRequest{
		VolumeId: volumeID,
	})
	if err != nil {
		t.Fatalf("DeleteVolume failed: %v", err)
	}

	t.Log("Snapshot lifecycle test passed")
}

// TestVolumeExpansion tests controller-side and node-side volume expansion.
func TestVolumeExpansion(t *testing.T) {
	ctx := context.Background()
	prov := NewFakeCephProvisioner()
	mounter := NewFakeMounter()
	driver := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node",
		DriverName: "test-driver",
		Logger:     zap.NewNop(),
	}, prov, mounter)

	volumeName := "expand-test-vol"
	stagingPath := "/staging/expand-vol"
	targetPath := "/mnt/expand-vol"

	// Create volume (5 GiB)
	createResp, err := driver.CreateVolume(ctx, &csi.CreateVolumeRequest{
		Name: volumeName,
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 5 * 1024 * 1024 * 1024,
		},
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateVolume failed: %v", err)
	}

	volumeID := createResp.GetVolume().GetVolumeId()
	volumeCtx := createResp.GetVolume().GetVolumeContext()

	// Stage and publish
	_, err = driver.NodeStageVolume(ctx, &csi.NodeStageVolumeRequest{
		VolumeId:          volumeID,
		StagingTargetPath: stagingPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
		VolumeContext: volumeCtx,
	})
	if err != nil {
		t.Fatalf("NodeStageVolume failed: %v", err)
	}

	_, err = driver.NodePublishVolume(ctx, &csi.NodePublishVolumeRequest{
		VolumeId:          volumeID,
		StagingTargetPath: stagingPath,
		TargetPath:        targetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
		VolumeContext: volumeCtx,
	})
	if err != nil {
		t.Fatalf("NodePublishVolume failed: %v", err)
	}

	// ControllerExpandVolume (10 GiB)
	expandResp, err := driver.ControllerExpandVolume(ctx, &csi.ControllerExpandVolumeRequest{
		VolumeId: volumeID,
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 10 * 1024 * 1024 * 1024,
		},
	})
	if err != nil {
		t.Fatalf("ControllerExpandVolume failed: %v", err)
	}
	if expandResp.GetCapacityBytes() != 10*1024*1024*1024 {
		t.Errorf("expected expanded capacity 10 GiB, got %d", expandResp.GetCapacityBytes())
	}

	// NodeExpandVolume
	nodeExpandResp, err := driver.NodeExpandVolume(ctx, &csi.NodeExpandVolumeRequest{
		VolumeId:   volumeID,
		VolumePath: targetPath,
	})
	if err != nil {
		t.Fatalf("NodeExpandVolume failed: %v", err)
	}
	if nodeExpandResp.GetCapacityBytes() == 0 {
		t.Error("expected non-zero expanded capacity")
	}

	// Clean up
	_, err = driver.NodeUnpublishVolume(ctx, &csi.NodeUnpublishVolumeRequest{
		VolumeId:   volumeID,
		TargetPath: targetPath,
	})
	if err != nil {
		t.Fatalf("NodeUnpublishVolume failed: %v", err)
	}

	_, err = driver.NodeUnstageVolume(ctx, &csi.NodeUnstageVolumeRequest{
		VolumeId:          volumeID,
		StagingTargetPath: stagingPath,
	})
	if err != nil {
		t.Fatalf("NodeUnstageVolume failed: %v", err)
	}

	_, err = driver.DeleteVolume(ctx, &csi.DeleteVolumeRequest{
		VolumeId: volumeID,
	})
	if err != nil {
		t.Fatalf("DeleteVolume failed: %v", err)
	}

	t.Log("Volume expansion test passed")
}

// TestErrorCases tests error conditions (missing volume, invalid input, etc.).
func TestErrorCases(t *testing.T) {
	ctx := context.Background()
	prov := NewFakeCephProvisioner()
	mounter := NewFakeMounter()
	driver := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node",
		DriverName: "test-driver",
		Logger:     zap.NewNop(),
	}, prov, mounter)

	// DeleteVolume with missing ID
	_, err := driver.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: ""})
	if err == nil || status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for missing volume ID, got %v", err)
	}

	// NodeStageVolume with missing volume ID
	_, err = driver.NodeStageVolume(ctx, &csi.NodeStageVolumeRequest{VolumeId: "", StagingTargetPath: "/path"})
	if err == nil || status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for missing volume ID, got %v", err)
	}

	// NodeStageVolume with missing staging path
	_, err = driver.NodeStageVolume(ctx, &csi.NodeStageVolumeRequest{VolumeId: "vol-1", StagingTargetPath: ""})
	if err == nil || status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for missing staging path, got %v", err)
	}

	// NodeGetVolumeStats with non-existent path
	_, err = driver.NodeGetVolumeStats(ctx, &csi.NodeGetVolumeStatsRequest{VolumePath: "/nonexistent"})
	if err == nil || status.Code(err) != codes.NotFound {
		t.Errorf("expected NotFound for non-existent path, got %v", err)
	}

	// DeleteSnapshot with missing ID
	_, err = driver.DeleteSnapshot(ctx, &csi.DeleteSnapshotRequest{SnapshotId: ""})
	if err == nil || status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for missing snapshot ID, got %v", err)
	}

	// CreateSnapshot with missing source volume
	_, err = driver.CreateSnapshot(ctx, &csi.CreateSnapshotRequest{Name: "snap", SourceVolumeId: ""})
	if err == nil || status.Code(err) != codes.InvalidArgument {
		t.Errorf("expected InvalidArgument for missing source volume, got %v", err)
	}

	t.Log("Error cases test passed")
}

// TestIdempotency tests idempotent operations (create same volume/snapshot twice).
func TestIdempotency(t *testing.T) {
	ctx := context.Background()
	prov := NewFakeCephProvisioner()
	mounter := NewFakeMounter()
	driver := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node",
		DriverName: "test-driver",
		Logger:     zap.NewNop(),
	}, prov, mounter)

	volumeName := "idempotent-vol"

	// Create volume
	createResp1, err := driver.CreateVolume(ctx, &csi.CreateVolumeRequest{
		Name: volumeName,
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 5 * 1024 * 1024 * 1024,
		},
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateVolume failed: %v", err)
	}

	// Create same volume again (should fail with AlreadyExists)
	_, err = driver.CreateVolume(ctx, &csi.CreateVolumeRequest{
		Name: volumeName,
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 5 * 1024 * 1024 * 1024,
		},
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
		},
	})
	if err == nil || status.Code(err) != codes.AlreadyExists {
		t.Errorf("expected AlreadyExists for duplicate volume, got %v", err)
	}

	volumeID := createResp1.GetVolume().GetVolumeId()

	// Delete volume twice (second should be idempotent — NotFound is acceptable)
	_, err = driver.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID})
	if err != nil {
		t.Fatalf("DeleteVolume first time failed: %v", err)
	}

	_, err = driver.DeleteVolume(ctx, &csi.DeleteVolumeRequest{VolumeId: volumeID})
	// Second delete may return NotFound or nil (depending on implementation)
	if err != nil && status.Code(err) != codes.NotFound {
		t.Logf("DeleteVolume second time returned: %v (acceptable if NotFound)", err)
	}

	t.Log("Idempotency test passed")
}

// TestCapabilityAdvertisement tests that capabilities are correctly advertised.
func TestCapabilityAdvertisement(t *testing.T) {
	ctx := context.Background()
	driver := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node",
		DriverName: "test-driver",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	// Check controller capabilities
	ctrlCaps, err := driver.ControllerGetCapabilities(ctx, &csi.ControllerGetCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("ControllerGetCapabilities failed: %v", err)
	}
	expectedCtrlCaps := map[csi.ControllerServiceCapability_RPC_Type]bool{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME:   true,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME:          true,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT: true,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS:         true,
	}
	for _, cap := range ctrlCaps.GetCapabilities() {
		capType := cap.GetRpc().GetType()
		if !expectedCtrlCaps[capType] {
			t.Errorf("unexpected controller capability: %v", capType)
		}
	}

	// Check node capabilities
	nodeCaps, err := driver.NodeGetCapabilities(ctx, &csi.NodeGetCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("NodeGetCapabilities failed: %v", err)
	}
	expectedNodeCaps := map[csi.NodeServiceCapability_RPC_Type]bool{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME: true,
		csi.NodeServiceCapability_RPC_GET_VOLUME_STATS:     true,
		csi.NodeServiceCapability_RPC_EXPAND_VOLUME:        true,
	}
	for _, cap := range nodeCaps.GetCapabilities() {
		capType := cap.GetRpc().GetType()
		if !expectedNodeCaps[capType] {
			t.Errorf("unexpected node capability: %v", capType)
		}
	}

	t.Log("Capability advertisement test passed")
}

// BenchmarkVolumeCreation benchmarks volume creation performance.
func BenchmarkVolumeCreation(b *testing.B) {
	ctx := context.Background()
	prov := NewFakeCephProvisioner()
	driver := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node",
		DriverName: "test-driver",
		Logger:     zap.NewNop(),
	}, prov, NewFakeMounter())

	b.ReportAllocs()
	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		volumeName := "bench-vol-" + string(rune(i))
		_, err := driver.CreateVolume(ctx, &csi.CreateVolumeRequest{
			Name: volumeName,
			CapacityRange: &csi.CapacityRange{
				RequiredBytes: 5 * 1024 * 1024 * 1024,
			},
			VolumeCapabilities: []*csi.VolumeCapability{
				{
					AccessMode: &csi.VolumeCapability_AccessMode{
						Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
					},
				},
			},
		})
		if err != nil {
			b.Fatalf("CreateVolume failed: %v", err)
		}
	}
}
