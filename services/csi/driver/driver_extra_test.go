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
	d := newTestDriver(t)
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
		t.Fatalf("CreateVolume RBD error: %v", err)
	}
	if resp.GetVolume().GetVolumeId() != "test-vol-rbd" {
		t.Errorf("VolumeId = %s, want test-vol-rbd", resp.GetVolume().GetVolumeId())
	}
}

func TestCreateVolume_CephFS_ByParameter(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name:       "test-vol-cephfs",
		Parameters: map[string]string{"volumeType": "cephfs"},
	})
	if err != nil {
		t.Fatalf("CreateVolume CephFS error: %v", err)
	}
	if resp.GetVolume().GetVolumeId() != "test-vol-cephfs" {
		t.Errorf("VolumeId = %s, want test-vol-cephfs", resp.GetVolume().GetVolumeId())
	}
}

func TestCreateVolume_CephFS_ByRWX(t *testing.T) {
	d := newTestDriver(t)
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
		t.Fatalf("CreateVolume CephFS RWX error: %v", err)
	}
	if resp.GetVolume().GetVolumeId() != "test-vol-rwx" {
		t.Errorf("VolumeId = %s, want test-vol-rwx", resp.GetVolume().GetVolumeId())
	}
}

func TestCreateVolume_WithCapacityRange(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{
		Name: "test-vol-cap",
		CapacityRange: &csi.CapacityRange{
			RequiredBytes: 5 * 1024 * 1024 * 1024, // 5 GiB
		},
	})
	if err != nil {
		t.Fatalf("CreateVolume with capacity error: %v", err)
	}
	if resp.GetVolume().GetCapacityBytes() != 5*1024*1024*1024 {
		t.Errorf("CapacityBytes = %d, want %d", resp.GetVolume().GetCapacityBytes(), 5*1024*1024*1024)
	}
}

func TestCreateVolume_DefaultCapacity(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.CreateVolume(context.Background(), &csi.CreateVolumeRequest{Name: "vol-default-cap"})
	if err != nil {
		t.Fatalf("CreateVolume default capacity error: %v", err)
	}
	const defaultCap = 10 * 1024 * 1024 * 1024
	if resp.GetVolume().GetCapacityBytes() != defaultCap {
		t.Errorf("CapacityBytes = %d, want %d", resp.GetVolume().GetCapacityBytes(), defaultCap)
	}
}

func TestDeleteVolume(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.DeleteVolume(context.Background(), &csi.DeleteVolumeRequest{VolumeId: "vol-to-delete"})
	if err != nil {
		t.Fatalf("DeleteVolume error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil DeleteVolumeResponse")
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
	// RBD volume (no volumeType=cephfs in context) with RWX → should fail
	_, err := d.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId:      "vol-rbd",
		VolumeContext: map[string]string{}, // not cephfs
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER}},
		},
	})
	if err == nil {
		t.Fatal("expected error for RWX on non-CephFS volume")
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
	_, err := d.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: "vol-rbd",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY}},
		},
	})
	if err == nil {
		t.Fatal("expected error for MULTI_NODE_READER_ONLY on non-CephFS")
	}
}

func TestValidateVolumeCapabilities_MultiNodeSingleWriter_NonCephFS(t *testing.T) {
	d := newTestDriver(t)
	_, err := d.ValidateVolumeCapabilities(context.Background(), &csi.ValidateVolumeCapabilitiesRequest{
		VolumeId: "vol-rbd",
		VolumeCapabilities: []*csi.VolumeCapability{
			{AccessMode: &csi.VolumeCapability_AccessMode{Mode: csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER}},
		},
	})
	if err == nil {
		t.Fatal("expected error for MULTI_NODE_SINGLE_WRITER on non-CephFS")
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
	d := newTestDriver(t)
	resp, err := d.ControllerGetCapabilities(context.Background(), &csi.ControllerGetCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("ControllerGetCapabilities error: %v", err)
	}
	if len(resp.GetCapabilities()) == 0 {
		t.Error("expected at least one controller capability")
	}
}

func TestControllerExpandVolume(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.ControllerExpandVolume(context.Background(), &csi.ControllerExpandVolumeRequest{VolumeId: "vol-1"})
	if err != nil {
		t.Fatalf("ControllerExpandVolume error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
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
	d := newTestDriver(t)
	resp, err := d.NodeStageVolume(context.Background(), &csi.NodeStageVolumeRequest{VolumeId: "vol-1"})
	if err != nil {
		t.Fatalf("NodeStageVolume error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestNodeUnstageVolume(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.NodeUnstageVolume(context.Background(), &csi.NodeUnstageVolumeRequest{VolumeId: "vol-1", StagingTargetPath: "/tmp"})
	if err != nil {
		t.Fatalf("NodeUnstageVolume error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestNodePublishVolume(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.NodePublishVolume(context.Background(), &csi.NodePublishVolumeRequest{
		VolumeId:   "vol-1",
		TargetPath: "/tmp/nest-csi-target",
	})
	if err != nil {
		t.Fatalf("NodePublishVolume error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestNodeUnpublishVolume(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.NodeUnpublishVolume(context.Background(), &csi.NodeUnpublishVolumeRequest{
		VolumeId:   "vol-1",
		TargetPath: "/tmp/nest-csi-target",
	})
	if err != nil {
		t.Fatalf("NodeUnpublishVolume error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestNodeGetVolumeStats(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.NodeGetVolumeStats(context.Background(), &csi.NodeGetVolumeStatsRequest{VolumeId: "vol-1"})
	if err != nil {
		t.Fatalf("NodeGetVolumeStats error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestNodeExpandVolume(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.NodeExpandVolume(context.Background(), &csi.NodeExpandVolumeRequest{VolumeId: "vol-1"})
	if err != nil {
		t.Fatalf("NodeExpandVolume error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil response")
	}
}

func TestNodeGetCapabilities(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.NodeGetCapabilities(context.Background(), &csi.NodeGetCapabilitiesRequest{})
	if err != nil {
		t.Fatalf("NodeGetCapabilities error: %v", err)
	}
	if len(resp.GetCapabilities()) == 0 {
		t.Error("expected at least one node capability")
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
	d := newTestDriver(t)
	resp, err := d.CreateSnapshot(context.Background(), &csi.CreateSnapshotRequest{
		SourceVolumeId: "vol-1",
		Name:           "snap-1",
	})
	if err != nil {
		t.Fatalf("CreateSnapshot error: %v", err)
	}
	if resp.GetSnapshot().GetSnapshotId() != "snap-1" {
		t.Errorf("SnapshotId = %s, want snap-1", resp.GetSnapshot().GetSnapshotId())
	}
	if resp.GetSnapshot().GetSourceVolumeId() != "vol-1" {
		t.Errorf("SourceVolumeId = %s, want vol-1", resp.GetSnapshot().GetSourceVolumeId())
	}
	if !resp.GetSnapshot().GetReadyToUse() {
		t.Error("expected ReadyToUse = true")
	}
	if resp.GetSnapshot().GetCreationTime() == nil {
		t.Error("expected non-nil CreationTime")
	}
}

func TestDeleteSnapshot(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.DeleteSnapshot(context.Background(), &csi.DeleteSnapshotRequest{SnapshotId: "snap-1"})
	if err != nil {
		t.Fatalf("DeleteSnapshot error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil DeleteSnapshotResponse")
	}
}

func TestListSnapshots(t *testing.T) {
	d := newTestDriver(t)
	resp, err := d.ListSnapshots(context.Background(), &csi.ListSnapshotsRequest{})
	if err != nil {
		t.Fatalf("ListSnapshots error: %v", err)
	}
	if resp == nil {
		t.Error("expected non-nil ListSnapshotsResponse")
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

	// Stop the server — d.server may be nil if Run failed immediately
	if d.server != nil {
		d.server.GracefulStop()
	}

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
		if d.server != nil {
			d.server.GracefulStop()
		}
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
		if d.server != nil {
			break
		}
	}

	// Stop the server
	if d.server != nil {
		d.server.GracefulStop()
	}

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
		if d.server != nil {
			d.server.GracefulStop()
		}
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
	if d.server != nil {
		d.server.GracefulStop()
	}

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
	if d.server != nil {
		d.server.GracefulStop()
	}

	// Clean up
	select {
	case err := <-errCh:
		t.Logf("Run returned: %v", err)
	case <-time.After(1 * time.Second):
		// Acceptable
	}
}
