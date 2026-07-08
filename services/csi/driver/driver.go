// Package driver implements the Nest CSI driver.
// It is a thin admission/policy shim in front of Rook-Ceph's CSI driver.
// It injects tenant CephX credentials, enforces quotas, and rewrites StorageClass names.
package driver

import (
	"context"
	"fmt"
	"net"
	"os"
	"strings"
	"sync"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

const (
	DriverVersion = "1.0.0"
)

// Config holds CSI driver configuration
type Config struct {
	Endpoint   string
	NodeID     string
	DriverName string
	Logger     *zap.Logger
}

// Driver implements the CSI Identity, Controller, and Node services
type Driver struct {
	csi.UnimplementedIdentityServer
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer
	cfg       Config
	server    *grpc.Server
	mu        sync.RWMutex
	snapshots map[string]*csi.Snapshot // snapshots by (name, sourceVolumeID) tuple for idempotency
}

func New(cfg Config) *Driver {
	return &Driver{
		cfg:       cfg,
		snapshots: make(map[string]*csi.Snapshot),
	}
}

// Run starts the gRPC server on the configured endpoint
func (d *Driver) Run() error {
	scheme, addr, err := parseEndpoint(d.cfg.Endpoint)
	if err != nil {
		return err
	}

	if scheme == "unix" {
		// Remove only stale socket files, don't kill an active listener
		if _, err := os.Stat(addr); err == nil {
			// File exists; only remove it if it's not currently in use
			// Try to connect to it; if connection fails, it's stale and safe to remove
			conn, err := net.Dial("unix", addr)
			if err != nil {
				// Connection failed; socket is stale, safe to remove
				if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
					return err
				}
			} else {
				// Connection succeeded; socket is active, don't remove it
				conn.Close()
				return fmt.Errorf("unix socket %s already in use by another process", addr)
			}
		}
	}

	listener, err := net.Listen(scheme, addr)
	if err != nil {
		return err
	}

	d.server = grpc.NewServer()
	csi.RegisterIdentityServer(d.server, d)
	csi.RegisterControllerServer(d.server, d)
	csi.RegisterNodeServer(d.server, d)

	d.cfg.Logger.Info("CSI gRPC server listening", zap.String("endpoint", d.cfg.Endpoint))
	return d.server.Serve(listener)
}

// --- Identity Service ---

func (d *Driver) GetPluginInfo(ctx context.Context, req *csi.GetPluginInfoRequest) (*csi.GetPluginInfoResponse, error) {
	return &csi.GetPluginInfoResponse{
		Name:          d.cfg.DriverName,
		VendorVersion: DriverVersion,
	}, nil
}

func (d *Driver) GetPluginCapabilities(ctx context.Context, req *csi.GetPluginCapabilitiesRequest) (*csi.GetPluginCapabilitiesResponse, error) {
	return &csi.GetPluginCapabilitiesResponse{
		Capabilities: []*csi.PluginCapability{
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_CONTROLLER_SERVICE,
					},
				},
			},
			{
				Type: &csi.PluginCapability_Service_{
					Service: &csi.PluginCapability_Service{
						Type: csi.PluginCapability_Service_VOLUME_ACCESSIBILITY_CONSTRAINTS,
					},
				},
			},
		},
	}, nil
}

func (d *Driver) Probe(ctx context.Context, req *csi.ProbeRequest) (*csi.ProbeResponse, error) {
	return &csi.ProbeResponse{}, nil
}

// --- Controller Service (stubs for P1 — delegates to Rook-Ceph in P2) ---

func (d *Driver) CreateVolume(ctx context.Context, req *csi.CreateVolumeRequest) (*csi.CreateVolumeResponse, error) {
	// INVALID_ARGUMENT: validate name
	if req.GetName() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume name required")
	}

	// INVALID_ARGUMENT: validate VolumeCapabilities
	if len(req.GetVolumeCapabilities()) == 0 {
		return nil, status.Error(codes.InvalidArgument, "VolumeCapabilities required")
	}

	isCephFS := isCephFSVolume(req)

	// INVALID_ARGUMENT: RWX not supported on RBD
	for _, cap := range req.GetVolumeCapabilities() {
		if am := cap.GetAccessMode(); am != nil {
			mode := am.GetMode()
			isRWX := mode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER ||
				mode == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY ||
				mode == csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER
			if isRWX && !isCephFS {
				return nil, status.Error(codes.InvalidArgument, "RWX access mode only supported for CephFS volumes (volumeType=cephfs); RBD volumes support RWO only")
			}
		}
	}

	// P2: real volume creation not yet implemented.
	// Return Unimplemented to prevent external-provisioner from marking volumes as provisioned falsely.
	return nil, status.Error(codes.Unimplemented, "CreateVolume requires real Ceph volume implementation; P2 feature")
}

func (d *Driver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	// P2: real volume deletion not yet implemented.
	// Return Unimplemented to prevent external-provisioner from marking volumes as deleted falsely.
	return nil, status.Error(codes.Unimplemented, "DeleteVolume requires real Ceph volume implementation; P2 feature")
}

func (d *Driver) ControllerPublishVolume(ctx context.Context, req *csi.ControllerPublishVolumeRequest) (*csi.ControllerPublishVolumeResponse, error) {
	return &csi.ControllerPublishVolumeResponse{}, nil
}

func (d *Driver) ControllerUnpublishVolume(ctx context.Context, req *csi.ControllerUnpublishVolumeRequest) (*csi.ControllerUnpublishVolumeResponse, error) {
	return &csi.ControllerUnpublishVolumeResponse{}, nil
}

func (d *Driver) ValidateVolumeCapabilities(ctx context.Context, req *csi.ValidateVolumeCapabilitiesRequest) (*csi.ValidateVolumeCapabilitiesResponse, error) {
	volumeCtx := req.GetVolumeContext()
	isCephFS := volumeCtx["volumeType"] == "cephfs"

	for _, cap := range req.GetVolumeCapabilities() {
		if am := cap.GetAccessMode(); am != nil {
			mode := am.GetMode()
			isRWX := mode == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER ||
				mode == csi.VolumeCapability_AccessMode_MULTI_NODE_READER_ONLY ||
				mode == csi.VolumeCapability_AccessMode_MULTI_NODE_SINGLE_WRITER
			if isRWX && !isCephFS {
				// Unsupported capability: return Confirmed=nil with message (don't error out with codes.Unknown)
				return &csi.ValidateVolumeCapabilitiesResponse{
					Message: "RWX access mode is only supported for CephFS volumes (volumeType=cephfs); RBD volumes support RWO only",
				}, nil
			}
		}
	}

	return &csi.ValidateVolumeCapabilitiesResponse{
		Confirmed: &csi.ValidateVolumeCapabilitiesResponse_Confirmed{
			VolumeCapabilities: req.GetVolumeCapabilities(),
		},
	}, nil
}

func (d *Driver) ListVolumes(ctx context.Context, req *csi.ListVolumesRequest) (*csi.ListVolumesResponse, error) {
	return &csi.ListVolumesResponse{}, nil
}

func (d *Driver) GetCapacity(ctx context.Context, req *csi.GetCapacityRequest) (*csi.GetCapacityResponse, error) {
	return &csi.GetCapacityResponse{}, nil
}

func (d *Driver) ControllerGetCapabilities(ctx context.Context, req *csi.ControllerGetCapabilitiesRequest) (*csi.ControllerGetCapabilitiesResponse, error) {
	// Only advertise capabilities that are actually implemented.
	// CREATE_DELETE_VOLUME, CREATE_DELETE_SNAPSHOT, and LIST_SNAPSHOTS are P2 stubs not yet implemented.
	// EXPAND_VOLUME not implemented; don't advertise to prevent external-resizer false expansion.
	// Return empty capabilities list — all volume/snapshot operations return Unimplemented.
	capTypes := []csi.ControllerServiceCapability_RPC_Type{}
	caps := make([]*csi.ControllerServiceCapability, 0, len(capTypes))
	for _, t := range capTypes {
		caps = append(caps, &csi.ControllerServiceCapability{
			Type: &csi.ControllerServiceCapability_Rpc{
				Rpc: &csi.ControllerServiceCapability_RPC{Type: t},
			},
		})
	}
	return &csi.ControllerGetCapabilitiesResponse{Capabilities: caps}, nil
}

func (d *Driver) ControllerExpandVolume(ctx context.Context, req *csi.ControllerExpandVolumeRequest) (*csi.ControllerExpandVolumeResponse, error) {
	// P2: real volume expansion implementation required.
	// Not implemented yet — return Unimplemented to prevent external-resizer from marking PVCs expanded falsely.
	return nil, status.Error(codes.Unimplemented, "ControllerExpandVolume requires real expansion implementation; P2 feature")
}

func (d *Driver) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return &csi.ControllerGetVolumeResponse{}, nil
}

func (d *Driver) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	return &csi.ControllerModifyVolumeResponse{}, nil
}

// --- Node Service ---

func (d *Driver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	// P2: real mount implementation required to avoid silent data loss (writes to node root disk).
	// Not implemented yet — return Unimplemented to prevent callers from being misled.
	return nil, status.Error(codes.Unimplemented, "NodeStageVolume requires real mount implementation; P2 feature")
}

func (d *Driver) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	// Paired with NodeStageVolume; P2.
	return nil, status.Error(codes.Unimplemented, "NodeUnstageVolume requires real unmount implementation; P2 feature")
}

func (d *Driver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	// P2: real mount implementation required to avoid silent data loss (writes to node root disk).
	// Not implemented yet — return Unimplemented to prevent callers from being misled.
	d.cfg.Logger.Info("NodePublishVolume not yet implemented",
		zap.String("volumeID", req.GetVolumeId()),
		zap.String("targetPath", req.GetTargetPath()),
	)
	return nil, status.Error(codes.Unimplemented, "NodePublishVolume requires real mount implementation; P2 feature")
}

func (d *Driver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	// Paired with NodePublishVolume; P2.
	return nil, status.Error(codes.Unimplemented, "NodeUnpublishVolume requires real unmount implementation; P2 feature")
}

func (d *Driver) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	// P2: real statfs-based stats implementation required.
	// Not implemented yet — return Unimplemented.
	return nil, status.Error(codes.Unimplemented, "NodeGetVolumeStats requires real statfs implementation; P2 feature")
}

func (d *Driver) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	// P2: real volume expansion not yet implemented.
	// Return Unimplemented to prevent callers from assuming expansion succeeded.
	return nil, status.Error(codes.Unimplemented, "NodeExpandVolume requires real expansion implementation; P2 feature")
}

func (d *Driver) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	// Only advertise capabilities that are actually implemented.
	// STAGE_UNSTAGE_VOLUME, GET_VOLUME_STATS, and EXPAND_VOLUME are not yet implemented (P2).
	capTypes := []csi.NodeServiceCapability_RPC_Type{
		// P2: add RPC_STAGE_UNSTAGE_VOLUME when real mount is implemented
		// P2: add RPC_GET_VOLUME_STATS when real statfs is implemented
		// P2: add RPC_EXPAND_VOLUME when real expansion is implemented
	}
	caps := make([]*csi.NodeServiceCapability, 0, len(capTypes))
	for _, t := range capTypes {
		caps = append(caps, &csi.NodeServiceCapability{
			Type: &csi.NodeServiceCapability_Rpc{
				Rpc: &csi.NodeServiceCapability_RPC{Type: t},
			},
		})
	}
	return &csi.NodeGetCapabilitiesResponse{Capabilities: caps}, nil
}

func (d *Driver) NodeGetInfo(ctx context.Context, req *csi.NodeGetInfoRequest) (*csi.NodeGetInfoResponse, error) {
	return &csi.NodeGetInfoResponse{
		NodeId: d.cfg.NodeID,
	}, nil
}

// helpers

func parseEndpoint(ep string) (string, string, error) {
	if strings.HasPrefix(ep, "unix://") {
		return "unix", strings.TrimPrefix(ep, "unix://"), nil
	}
	if strings.HasPrefix(ep, "tcp://") {
		return "tcp", strings.TrimPrefix(ep, "tcp://"), nil
	}
	return "unix", ep, nil
}

func capacityFromRequest(req *csi.CreateVolumeRequest) int64 {
	if req.GetCapacityRange() != nil {
		capacityRange := req.GetCapacityRange()
		// Honor RequiredBytes if set
		if capacityRange.GetRequiredBytes() > 0 {
			return capacityRange.GetRequiredBytes()
		}
		// Fall back to LimitBytes if RequiredBytes is unset but LimitBytes is set
		if capacityRange.GetLimitBytes() > 0 {
			return capacityRange.GetLimitBytes()
		}
	}
	return 10 * 1024 * 1024 * 1024 // 10 GiB default
}

// isCephFSVolume returns true if the CreateVolumeRequest targets CephFS.
// Detection is based on volumeContext["volumeType"] == "cephfs" or if any
// access mode is MULTI_NODE_MULTI_WRITER (RWX implies CephFS in this driver).
func isCephFSVolume(req *csi.CreateVolumeRequest) bool {
	if req.GetParameters()["volumeType"] == "cephfs" {
		return true
	}
	for _, cap := range req.GetVolumeCapabilities() {
		if am := cap.GetAccessMode(); am != nil {
			if am.GetMode() == csi.VolumeCapability_AccessMode_MULTI_NODE_MULTI_WRITER {
				return true
			}
		}
	}
	return false
}
