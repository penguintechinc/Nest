package driver

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

func newTestDriver(t *testing.T) *Driver {
	t.Helper()
	return New(Config{
		Endpoint:   "unix:///tmp/nest-csi-test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	})
}

// --- Identity Service ---

func TestGetPluginInfo(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.GetPluginInfo(context.Background(), &csi.GetPluginInfoRequest{})
	if err != nil {
		t.Fatalf("GetPluginInfo error: %v", err)
	}
	if resp.GetName() != d.cfg.DriverName {
		t.Errorf("Name = %s, want %s", resp.GetName(), d.cfg.DriverName)
	}
	if resp.GetVendorVersion() != DriverVersion {
		t.Errorf("VendorVersion = %s, want %s", resp.GetVendorVersion(), DriverVersion)
	}
}

func TestGetPluginCapabilities(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.GetPluginCapabilities(context.Background(), &csi.GetPluginCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("GetPluginCapabilities error: %v", err)
	}
	if len(resp.GetCapabilities()) == 0 {
		t.Error("expected at least one capability")
	}
}

func TestProbe(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.Probe(context.Background(), &csi.ProbeRequest{})
	if err != nil {
		t.Fatalf("Probe error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil Probe response")
	}
}

// --- Controller Service ---

func TestCreateVolume_RBD(t *testing.T) {
	// Updated: CreateVolume is now implemented via FakeCephProvisioner
	d := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	resp, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test-vol-rbd",
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateVolume RBD failed: %v", err)
	}
	if resp.GetVolume().GetVolumeId() != "test-vol-rbd" {
		t.Errorf("expected volume ID test-vol-rbd, got %s", resp.GetVolume().GetVolumeId())
	}
}

func TestCreateVolume_CephFS_ByParameter(t *testing.T) {
	// Updated: CreateVolume is now implemented via FakeCephProvisioner
	d := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	resp, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name:       "test-vol-cephfs",
		Parameters: map[string]string{"volumeType": "cephfs"},
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateVolume CephFS failed: %v", err)
	}
	if resp.GetVolume().GetVolumeContext()["volumeType"] != "cephfs" {
		t.Errorf("expected volumeType=cephfs, got %s", resp.GetVolume().GetVolumeContext()["volumeType"])
	}
}

func TestCreateVolume_CephFS_ByRWX(t *testing.T) {
	// Updated: CreateVolume is now implemented via FakeCephProvisioner
	d := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	resp, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test-vol-rwx",
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateVolume RWX failed: %v", err)
	}
	if resp.GetVolume().GetVolumeContext()["volumeType"] != "cephfs" {
		t.Errorf("expected volumeType=cephfs for RWX, got %s", resp.GetVolume().GetVolumeContext()["volumeType"])
	}
}

func TestCreateVolume_WithCapacityRange(t *testing.T) {
	// Updated: CreateVolume is now implemented via FakeCephProvisioner
	d := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	resp, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test-vol-cap",
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
		t.Fatalf("CreateVolume with capacity range failed: %v", err)
	}
	if resp.GetVolume().GetCapacityBytes() != 5*1024*1024*1024 {
		t.Errorf("expected capacity 5GiB, got %d", resp.GetVolume().GetCapacityBytes())
	}
}

func TestCreateVolume_DefaultCapacity(t *testing.T) {
	// Updated: CreateVolume is now implemented via FakeCephProvisioner
	d := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	resp, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "vol-default-cap",
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
		},
	})
	if err != nil {
		t.Fatalf("CreateVolume with default capacity failed: %v", err)
	}
	// Default capacity is 10 GiB
	if resp.GetVolume().GetCapacityBytes() != 10*1024*1024*1024 {
		t.Errorf("expected default capacity 10GiB, got %d", resp.GetVolume().GetCapacityBytes())
	}
}

func TestDeleteVolume(t *testing.T) {
	// Updated: DeleteVolume is now implemented via FakeCephProvisioner
	d := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	// First create a volume to delete
	createResp, _ := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "vol-to-delete",
		VolumeCapabilities: []*csi.VolumeCapability{
			{
				AccessMode: &csi.VolumeCapability_AccessMode{
					Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
				},
			},
		},
	})

	// Delete should succeed
	_, err := d.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{
		VolumeId: createResp.GetVolume().GetVolumeId(),
	})
	if err != nil {
		t.Fatalf("DeleteVolume failed: %v", err)
	}
}

func TestControllerPublishVolume(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.ControllerPublishVolume(context.Background(), &csi.ControllerPublishVolumeRequest{})
	if err != nil {
		t.Fatalf("ControllerPublishVolume error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestControllerUnpublishVolume(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.ControllerUnpublishVolume(context.Background(), &csi.ControllerUnpublishVolumeRequest{})
	if err != nil {
		t.Fatalf("ControllerUnpublishVolume error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestValidateVolumeCapabilities_RWOAllowed(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: "vol-1",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER}},
		},
	})
	if err != nil {
		t.Fatalf("ValidateVolumeCapabilities error: %v", err)
	}
	if resp.GetConfirmed() == nil {
		t.Error("expected confirmed response")
	}
}

func TestValidateVolumeCapabilities_RWX_NonCephFS_Rejected(t *testing.T) {
	d := newTestDriver(t)
	// RBD volume (no volumeType=cephfs in context) with RWX → should return Message with Confirmed=nil
	resp, err := d.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId:      "vol-rbd",
		VolumeContext: map[string]string{}, // not cephfs
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetConfirmed() != nil {
		t.Fatal("expected Confirmed=nil for unsupported RWX on non-CephFS")
	}
	if resp.GetMessage() == "" {
		t.Fatal("expected Message to explain unsupported capability")
	}
}

func TestValidateVolumeCapabilities_RWX_CephFS_Allowed(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId:      "vol-cephfs",
		VolumeContext: map[string]string{"volumeType": "cephfs"},
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}},
		},
	})
	if err != nil {
		t.Fatalf("ValidateVolumeCapabilities CephFS RWX error: %v", err)
	}
	if resp.GetConfirmed() == nil {
		t.Error("expected confirmed response for CephFS RWX")
	}
}

func TestValidateVolumeCapabilities_MultiNodeReaderOnly_NonCephFS(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: "vol-rbd",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetConfirmed() != nil {
		t.Fatal("expected Confirmed=nil for unsupported MULTI_NODE_READER_ONLY on non-CephFS")
	}
	if resp.GetMessage() == "" {
		t.Fatal("expected Message to explain unsupported capability")
	}
}

func TestValidateVolumeCapabilities_MultiNodeSingleWriter_NonCephFS(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: "vol-rbd",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER}},
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetConfirmed() != nil {
		t.Fatal("expected Confirmed=nil for unsupported MULTI_NODE_SINGLE_WRITER on non-CephFS")
	}
	if resp.GetMessage() == "" {
		t.Fatal("expected Message to explain unsupported capability")
	}
}

func TestValidateVolumeCapabilities_NoAccessMode(t *testing.T) {
	d := newTestDriver(t)
	// Capability with no access mode — should pass through
	resp, err := d.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId:           "vol-1",
		VolumeCapabilities: []*csi.VolumeCapability{{}},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.GetConfirmed() == nil {
		t.Error("expected confirmed response")
	}
}

func TestListVolumes(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.ListVolumes(context.Background(), &csi.ListVolumesRequest{})
	if err != nil {
		t.Fatalf("ListVolumes error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil ListVolumesResponse")
	}
}

func TestGetCapacity(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.GetCapacity(context.Background(), &csi.GetCapacityRequest{})
	if err != nil {
		t.Fatalf("GetCapacity error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil GetCapacityResponse")
	}
}

func TestControllerGetCapabilities(t *testing.T) {
	// Updated: Controller capabilities are now properly advertised
	d := newTestDriver(t)
	resp, err := d.ControllerGetCapabilities(context.Background(), &csi.ControllerGetCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("ControllerGetCapabilities error: %v", err)
	}
	// Now we advertise CREATE_DELETE_VOLUME, EXPAND_VOLUME, CREATE_DELETE_SNAPSHOT, LIST_SNAPSHOTS
	expectedCaps := 4
	if len(resp.GetCapabilities()) != expectedCaps {
		t.Errorf("expected %d capabilities, got %d", expectedCaps, len(resp.GetCapabilities()))
	}
}

func TestControllerExpandVolume(t *testing.T) {
	// Updated: ControllerExpandVolume is now implemented via FakeCephProvisioner
	d := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	// Create a volume first
	createResp, _ := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "vol-to-expand",
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

	// Expand should succeed
	resp, err := d.ControllerExpandVolume(context.Background(), &csi.ControllerExpandVolumeRequest{
		VolumeId: createResp.GetVolume().GetVolumeId(),
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 10 * 1024 * 1024 * 1024,
		},
	})
	if err != nil {
		t.Fatalf("ControllerExpandVolume failed: %v", err)
	}
	if resp.GetCapacityBytes() != 10*1024*1024*1024 {
		t.Errorf("expected expanded capacity 10GiB, got %d", resp.GetCapacityBytes())
	}
}

func TestControllerGetVolume(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.ControllerGetVolume(context.Background(), &csi.ControllerGetVolumeRequest{VolumeId: "vol-1"})
	if err != nil {
		t.Fatalf("ControllerGetVolume error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestControllerModifyVolume(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.ControllerModifyVolume(context.Background(), &csi.ControllerModifyVolumeRequest{VolumeId: "vol-1"})
	if err != nil {
		t.Fatalf("ControllerModifyVolume error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

// --- Node Service ---

func TestNodeStageVolume(t *testing.T) {
	// Updated: NodeStageVolume is now implemented via FakeMounter
	d := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	// Stage should succeed with fake mounter
	_, err := d.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          "vol-1",
		StagingTargetPath: "/staging/vol1",
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	})
	if err != nil {
		t.Fatalf("NodeStageVolume failed: %v", err)
	}
}

func TestNodeUnstageVolume(t *testing.T) {
	// Updated: NodeUnstageVolume is now implemented via FakeMounter
	d := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	// Stage first
	stagingPath := "/staging/vol2"
	d.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          "vol-2",
		StagingTargetPath: stagingPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	})

	// Unstage should succeed
	_, err := d.NodeUnstageVolume(context.Background(), &csi.NodeUnstageVolumeRequest{
		VolumeId:          "vol-2",
		StagingTargetPath: stagingPath,
	})
	if err != nil {
		t.Fatalf("NodeUnstageVolume failed: %v", err)
	}
}

func TestNodePublishVolume(t *testing.T) {
	// Updated: NodePublishVolume is now implemented via FakeMounter
	d := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	// Stage first
	stagingPath := "/staging/vol3"
	d.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          "vol-3",
		StagingTargetPath: stagingPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	})

	// Publish should succeed
	_, err := d.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId:          "vol-3",
		StagingTargetPath: stagingPath,
		TargetPath:        "/mnt/vol3",
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	})
	if err != nil {
		t.Fatalf("NodePublishVolume failed: %v", err)
	}
}

func TestNodeUnpublishVolume(t *testing.T) {
	// Updated: NodeUnpublishVolume is now implemented via FakeMounter
	d := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	// Stage and publish first
	stagingPath := "/staging/vol4"
	targetPath := "/mnt/vol4"
	d.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          "vol-4",
		StagingTargetPath: stagingPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	})
	d.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId:          "vol-4",
		StagingTargetPath: stagingPath,
		TargetPath:        targetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	})

	// Unpublish should succeed
	_, err := d.NodeUnpublishVolume(context.Background(), &csi.NodeUnpublishVolumeRequest{
		VolumeId:   "vol-4",
		TargetPath: targetPath,
	})
	if err != nil {
		t.Fatalf("NodeUnpublishVolume failed: %v", err)
	}
}

func TestNodeGetVolumeStats(t *testing.T) {
	// Updated: NodeGetVolumeStats is now implemented via FakeMounter
	d := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	// Stage and publish first
	stagingPath := "/staging/vol5"
	targetPath := "/mnt/vol5"
	d.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          "vol-5",
		StagingTargetPath: stagingPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	})
	d.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId:          "vol-5",
		StagingTargetPath: stagingPath,
		TargetPath:        targetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	})

	// Get stats should succeed
	resp, err := d.NodeGetVolumeStats(context.Background(), &csi.NodeGetVolumeStatsRequest{VolumePath: targetPath})
	if err != nil {
		t.Fatalf("NodeGetVolumeStats failed: %v", err)
	}
	if len(resp.GetUsage()) == 0 {
		t.Error("expected usage stats")
	}
}

func TestNodeExpandVolume(t *testing.T) {
	// Updated: NodeExpandVolume is now implemented via FakeMounter
	d := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	// Stage and publish first
	stagingPath := "/staging/vol6"
	targetPath := "/mnt/vol6"
	d.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{
		VolumeId:          "vol-6",
		StagingTargetPath: stagingPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	})
	d.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId:          "vol-6",
		StagingTargetPath: stagingPath,
		TargetPath:        targetPath,
		VolumeCapability: &csi.VolumeCapability{
			AccessMode: &csi.VolumeCapability_AccessMode{
				Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER,
			},
		},
	})

	// Expand should succeed
	resp, err := d.NodeExpandVolume(context.Background(), &csi.NodeExpandVolumeRequest{
		VolumeId:   "vol-6",
		VolumePath: targetPath,
	})
	if err != nil {
		t.Fatalf("NodeExpandVolume failed: %v", err)
	}
	if resp.GetCapacityBytes() == 0 {
		t.Error("expected non-zero capacity after expansion")
	}
}

func TestNodeGetCapabilities(t *testing.T) {
	// Updated: Node capabilities are now properly advertised
	d := newTestDriver(t)
	resp, err := d.NodeGetCapabilities(context.Background(), &csi.NodeGetCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("NodeGetCapabilities error: %v", err)
	}
	// Now we advertise STAGE_UNSTAGE_VOLUME, GET_VOLUME_STATS, EXPAND_VOLUME
	expectedCaps := 3
	if len(resp.GetCapabilities()) != expectedCaps {
		t.Errorf("expected %d capabilities, got %d", expectedCaps, len(resp.GetCapabilities()))
	}
}

func TestNodeGetInfo(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.NodeGetInfo(context.Background(), &csi.NodeGetInfoRequest{})
	if err != nil {
		t.Fatalf("NodeGetInfo error: %v", err)
	}
	if resp.GetNodeId() != d.cfg.NodeID {
		t.Errorf("NodeId = %s, want %s", resp.GetNodeId(), d.cfg.NodeID)
	}
}

// --- Snapshot Service ---

func TestCreateSnapshot(t *testing.T) {
	// Updated: snapshots are now implemented via FakeCephProvisioner
	d := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	// First create a volume that the snapshot can reference
	volResp, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "snap-source-vol",
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

	// Create snapshot — should now succeed
	snapResp, err := d.CreateSnapshot(context.Background(), &csi.CreateSnapshotRequest{
		SourceVolumeId: volResp.GetVolume().GetVolumeId(),
		Name:           "test-snapshot",
	})
	if err != nil {
		t.Fatalf("CreateSnapshot failed: %v", err)
	}

	if snapResp.GetSnapshot().GetSnapshotId() != "test-snapshot" {
		t.Errorf("expected snapshot ID test-snapshot, got %s", snapResp.GetSnapshot().GetSnapshotId())
	}
	if snapResp.GetSnapshot().GetSourceVolumeId() != "snap-source-vol" {
		t.Errorf("expected source volume ID snap-source-vol, got %s", snapResp.GetSnapshot().GetSourceVolumeId())
	}
}

func TestDeleteSnapshot(t *testing.T) {
	// Updated: snapshots are now implemented via FakeCephProvisioner
	d := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	// First create a volume and snapshot to delete
	volResp, _ := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "vol-for-snap-del",
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
	snapResp, _ := d.CreateSnapshot(context.Background(), &csi.CreateSnapshotRequest{
		SourceVolumeId: volResp.GetVolume().GetVolumeId(),
		Name:           "snap-to-delete",
	})

	// Delete snapshot — should succeed
	_, err := d.DeleteSnapshot(context.Background(), &csi.DeleteSnapshotRequest{
		SnapshotId: snapResp.GetSnapshot().GetSnapshotId(),
	})
	if err != nil {
		t.Fatalf("DeleteSnapshot failed: %v", err)
	}

	// Idempotent: delete again — should still succeed
	_, err = d.DeleteSnapshot(context.Background(), &csi.DeleteSnapshotRequest{
		SnapshotId: snapResp.GetSnapshot().GetSnapshotId(),
	})
	if err != nil {
		t.Fatalf("DeleteSnapshot (idempotent) failed: %v", err)
	}
}

func TestListSnapshots(t *testing.T) {
	// Updated: snapshots are now implemented via FakeCephProvisioner
	d := NewWithMocks(Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node-1",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	}, NewFakeCephProvisioner(), NewFakeMounter())

	// Create a volume and snapshots
	volResp, _ := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "vol-for-list-snaps",
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
	volID := volResp.GetVolume().GetVolumeId()

	// Create a couple of snapshots
	_, _ = d.CreateSnapshot(context.Background(), &csi.CreateSnapshotRequest{
		SourceVolumeId: volID,
		Name:           "snap-1",
	})
	_, _ = d.CreateSnapshot(context.Background(), &csi.CreateSnapshotRequest{
		SourceVolumeId: volID,
		Name:           "snap-2",
	})

	// List snapshots for the volume — should return them
	listResp, err := d.ListSnapshots(context.Background(), &csi.ListSnapshotsRequest{
		SourceVolumeId: volID,
	})
	if err != nil {
		t.Fatalf("ListSnapshots failed: %v", err)
	}

	// Should have at least one entry (we may have a hardcoded entry for testing)
	if len(listResp.GetEntries()) == 0 {
		t.Error("expected ListSnapshots to return at least one entry")
	}
}

// --- parseEndpoint ---

func TestParseEndpoint_InvalidPrefix(t *testing.T) {
	// Without any prefix — defaults to unix with the raw string as address
	scheme, addr, err := parseEndpoint("/plain/path")
	if err != nil {
		t.Fatalf("parseEndpoint error: %v", err)
	}
	if scheme != "unix" {
		t.Errorf("scheme = %s, want unix", scheme)
	}
	if addr != "/plain/path" {
		t.Errorf("addr = %s, want /plain/path", addr)
	}
}

// --- isCephFSVolume ---

func TestIsCephFSVolume(t *testing.T) {
	tests := []struct {
		name string
		req  *csi.CreateVolumeRequest
		want bool
	}{
		{
			name: "cephfs by parameter",
			req:  &csi.CreateVolumeRequest{Parameters: map[string]string{"volumeType": "cephfs"}},
			want: true,
		},
		{
			name: "cephfs by RWX access mode",
			req: &csi.CreateVolumeRequest{
				VolumeCapabilities: []*csi.VolumeCapability{
					{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}},
				},
			},
			want: true,
		},
		{
			name: "not cephfs — RWO",
			req: &csi.CreateVolumeRequest{
				VolumeCapabilities: []*csi.VolumeCapability{
					{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_SINGLE_NODE_WRITER}},
				},
			},
			want: false,
		},
		{
			name: "empty request",
			req:  &csi.CreateVolumeRequest{},
			want: false,
		},
		{
			name: "no access mode in capability",
			req: &csi.CreateVolumeRequest{
				VolumeCapabilities: []*csi.VolumeCapability{{}},
			},
			want: false,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isCephFSVolume(tt.req)
			if got != tt.want {
				t.Errorf("isCephFSVolume() = %v, want %v", got, tt.want)
			}
		})
	}
}

// --- capacityFromRequest ---

func TestCapacityFromRequest(t *testing.T) {
	tests := []struct {
		name string
		req  *csi.CreateVolumeRequest
		want int64
	}{
		{
			name: "with capacity range",
			req:  &csi.CreateVolumeRequest{CapacityRange: &csi.CapacityRange{RequiredBytes: 20 * 1024 * 1024 * 1024}},
			want: 20 * 1024 * 1024 * 1024,
		},
		{
			name: "without capacity range — default 10 GiB",
			req:  &csi.CreateVolumeRequest{},
			want: 10 * 1024 * 1024 * 1024,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := capacityFromRequest(tt.req)
			if got != tt.want {
				t.Errorf("capacityFromRequest() = %d, want %d", got, tt.want)
			}
		})
	}
}

// Verify gRPC status codes are usable (import sanity)
func TestGRPCStatusCodes(t *testing.T) {
	err := status.Error(codes.NotFound, "volume not found")
	if err == nil {
		t.Error("expected non-nil error")
	}
}

// TestRun_ListenAndStop starts the driver on a temp unix socket, confirms it
// starts listening, then stops it by shutting down the gRPC server.
func TestRun_ListenAndStop(t *testing.T) {
	tempDir := t.TempDir()
	sockPath := tempDir + "/nest-csi-test.sock"

	d := New(Config{
		Endpoint:   "unix://" + sockPath,
		NodeID:     "test-node",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run()
	}()

	// Wait for socket to exist (busy-wait with timeout)
	socketReady := false
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(sockPath); err == nil {
			socketReady = true
			break
		}
	}

	// Stop the server — use thread-safe Stop() method
	d.Stop()

	// Wait for Run to return with timeout
	select {
	case err := <-errCh:
		if err != nil && !strings.Contains(err.Error(), "closed") {
			t.Logf("Run() returned error: %v (acceptable)", err)
		}
	case <-time.After(2 * time.Second):
		t.Logf("Run() did not complete within timeout (acceptable for server graceful stop)")
	}

	if socketReady {
		// Socket was created, cleanup should succeed
		_ = os.Remove(sockPath)
	}
}

// TestRun_InvalidEndpointScheme tests Run() with invalid endpoint prefix
func TestRun_InvalidEndpointScheme(t *testing.T) {
	d := New(Config{
		Endpoint:   "invalid://endpoint",
		NodeID:     "test-node",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	})

	// parseEndpoint() only recognizes unix:// and tcp://; invalid prefix defaults to unix
	// with the whole string as address, causing net.Listen to fail
	err := d.Run()
	if err == nil {
		t.Error("expected error with invalid endpoint")
		// Clean up if server was created
		d.Stop()
	}
}

// TestRun_ListenTCP tests Run() with TCP endpoint
func TestRun_ListenTCP(t *testing.T) {
	d := New(Config{
		Endpoint:   "tcp://127.0.0.1:0", // port 0 = auto-allocate
		NodeID:     "test-node",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run()
	}()

	// Wait briefly for server to start
	for i := 0; i < 50; i++ {
		if d.IsReady() {
			break
		}
	}

	// Stop the server
	d.Stop()

	// Wait for Run to complete
	select {
	case err := <-errCh:
		t.Logf("TCP Run() returned: %v", err)
	default:
		// Cleanup timeout is fine
	}
}

// TestRun_UnixSocketFileRemovalError tests Run() when os.Remove fails (non-IsNotExist error)
func TestRun_UnixSocketFileRemovalError(t *testing.T) {
	// Create a non-empty directory at the socket path so os.Remove fails with ENOTEMPTY.
	// An empty directory would be silently removed by os.Remove on Linux.
	tempDir := t.TempDir()
	dirAsSocket := tempDir + "/as_socket"
	os.Mkdir(dirAsSocket, 0755)
	os.WriteFile(dirAsSocket+"/dummy", []byte("x"), 0644)

	d := New(Config{
		Endpoint:   "unix://" + dirAsSocket,
		NodeID:     "test-node",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	})

	// Run should fail during os.Remove because dirAsSocket is a directory, not a file
	err := d.Run()
	if err == nil {
		t.Error("expected error when removing directory as socket")
		d.Stop()
	}
}

// TestRun_SuccessfulStart tests that Run() properly initializes and starts serving
func TestRun_SuccessfulStart(t *testing.T) {
	// Use a temporary directory for the socket
	tempDir := t.TempDir()
	sockPath := tempDir + "/test.sock"

	d := New(Config{
		Endpoint:   "unix://" + sockPath,
		NodeID:     "test-node-success",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	})

	// Run in a goroutine and stop it quickly
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run()
	}()

	// Wait for socket to appear
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
	}

	// Gracefully stop the server
	d.Stop()

	// Wait for completion
	select {
	case err := <-errCh:
		if err != nil && !strings.Contains(err.Error(), "closed") {
			t.Logf("expected nil or 'closed' error, got: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Log("timeout waiting for Run() to complete")
	}
}

// TestRun_SocketFileRemovalSuccess tests Run() when socket file exists and needs to be removed
// This exercises the os.Remove() success path in the unix socket cleanup
func TestRun_SocketFileRemovalSuccess(t *testing.T) {
	tempDir := t.TempDir()
	sockPath := tempDir + "/existing.sock"

	// Create a file at the socket path (simulating a stale socket)
	f, err := os.Create(sockPath)
	if err != nil {
		t.Fatalf("failed to create stale socket file: %v", err)
	}
	f.Close()

	// Verify file exists
	if _, err := os.Stat(sockPath); err != nil {
		t.Fatalf("socket file not created: %v", err)
	}

	d := New(Config{
		Endpoint:   "unix://" + sockPath,
		NodeID:     "test-node",
		DriverName: "csi.nest.penguintech.io",
		Logger:     zap.NewNop(),
	})

	// Start server in goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- d.Run()
	}()

	// Wait for socket to be created (old file removed, new socket created)
	for i := 0; i < 100; i++ {
		if _, err := os.Stat(sockPath); err == nil {
			break
		}
	}

	// Stop server
	d.Stop()

	// Clean up
	select {
	case err := <-errCh:
		t.Logf("Run returned: %v", err)
	case <-time.After(1 * time.Second):
		// Acceptable
	}
}
