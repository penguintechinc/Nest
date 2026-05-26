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

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/container-storage-interface/spec/lib/go/csi"
)

const (
	DriverVersion = "1.0.0"
)

// Config holds CSI driver configuration
type Config struct {
	Endpoint         string
	NodeID           string
	DriverName       string
	Logger           *zap.Logger
	RookRBDSocket    string
	RookCephFSSocket string
}

// Driver implements the CSI Identity, Controller, and Node services
type Driver struct {
	csi.UnimplementedIdentityServer
	csi.UnimplementedControllerServer
	csi.UnimplementedNodeServer
	cfg       Config
	server    *grpc.Server
	rbdConn   *grpc.ClientConn
	cephfsConn *grpc.ClientConn
}

func New(cfg Config) *Driver {
	return &Driver{cfg: cfg}
}

// Run starts the gRPC server on the configured endpoint
func (d *Driver) Run() error {
	defer func() {
		if d.rbdConn != nil {
			_ = d.rbdConn.Close()
		}
		if d.cephfsConn != nil {
			_ = d.cephfsConn.Close()
		}
	}()

	var err error
	if d.cfg.RookRBDSocket != "" {
		d.rbdConn, err = dialUnixSocket(d.cfg.RookRBDSocket)
		if err != nil {
			d.cfg.Logger.Warn("failed to dial Rook RBD socket", zap.String("socket", d.cfg.RookRBDSocket), zap.Error(err))
		}
	}

	if d.cfg.RookCephFSSocket != "" {
		d.cephfsConn, err = dialUnixSocket(d.cfg.RookCephFSSocket)
		if err != nil {
			d.cfg.Logger.Warn("failed to dial Rook CephFS socket", zap.String("socket", d.cfg.RookCephFSSocket), zap.Error(err))
		}
	}

	scheme, addr, err := parseEndpoint(d.cfg.Endpoint)
	if err != nil {
		return err
	}

	if scheme == "unix" {
		if err := os.Remove(addr); err != nil && !os.IsNotExist(err) {
			return err
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
	isCephFS := isCephFSVolume(req)
	if isCephFS {
		d.cfg.Logger.Info("CreateVolume RWX (CephFS path)",
			zap.String("name", req.GetName()),
			zap.String("volumeType", "cephfs"),
		)
	} else {
		d.cfg.Logger.Info("CreateVolume RWO (RBD path)",
			zap.String("name", req.GetName()),
			zap.String("volumeType", "rbd"),
		)
	}

	conn := d.rbdConn
	if isCephFS {
		conn = d.cephfsConn
	}

	if conn == nil {
		d.cfg.Logger.Warn("upstream socket not configured, returning stub response",
			zap.String("name", req.GetName()),
			zap.Bool("isCephFS", isCephFS),
		)
		return &csi.CreateVolumeResponse{
			Volume: &csi.Volume{
				VolumeId:      req.GetName(),
				CapacityBytes: capacityFromRequest(req),
				VolumeContext: map[string]string{
					"volumeType": func() string {
						if isCephFS {
							return "cephfs"
						}
						return "rbd"
					}(),
				},
			},
		}, nil
	}

	client := csi.NewControllerClient(conn)
	return client.CreateVolume(ctx, req)
}

func (d *Driver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	d.cfg.Logger.Info("DeleteVolume", zap.String("id", req.GetVolumeId()))

	// Try RBD first, fall back to CephFS if not found or error occurs
	if d.rbdConn != nil {
		client := csi.NewControllerClient(d.rbdConn)
		resp, err := client.DeleteVolume(ctx, req)
		if err == nil {
			return resp, nil
		}
		d.cfg.Logger.Debug("DeleteVolume failed on RBD socket, trying CephFS", zap.Error(err))
	}

	if d.cephfsConn != nil {
		client := csi.NewControllerClient(d.cephfsConn)
		resp, err := client.DeleteVolume(ctx, req)
		if err == nil {
			return resp, nil
		}
		d.cfg.Logger.Debug("DeleteVolume failed on CephFS socket", zap.Error(err))
	}

	if d.rbdConn == nil && d.cephfsConn == nil {
		d.cfg.Logger.Warn("no upstream sockets configured, returning stub response", zap.String("id", req.GetVolumeId()))
		return &csi.DeleteVolumeResponse{}, nil
	}

	return nil, fmt.Errorf("DeleteVolume failed on all upstream sockets for volume %s", req.GetVolumeId())
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
				return nil, fmt.Errorf("RWX access mode is only supported for CephFS volumes (volumeType=cephfs); RBD volumes support RWO only")
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
	capTypes := []csi.ControllerServiceCapability_RPC_Type{
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_VOLUME,
		csi.ControllerServiceCapability_RPC_EXPAND_VOLUME,
		csi.ControllerServiceCapability_RPC_CREATE_DELETE_SNAPSHOT,
		csi.ControllerServiceCapability_RPC_LIST_SNAPSHOTS,
	}
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
	// Try RBD first, fall back to CephFS if not found or error occurs
	if d.rbdConn != nil {
		client := csi.NewControllerClient(d.rbdConn)
		resp, err := client.ControllerExpandVolume(ctx, req)
		if err == nil {
			return resp, nil
		}
		d.cfg.Logger.Debug("ControllerExpandVolume failed on RBD socket, trying CephFS", zap.Error(err))
	}

	if d.cephfsConn != nil {
		client := csi.NewControllerClient(d.cephfsConn)
		resp, err := client.ControllerExpandVolume(ctx, req)
		if err == nil {
			return resp, nil
		}
		d.cfg.Logger.Debug("ControllerExpandVolume failed on CephFS socket", zap.Error(err))
	}

	if d.rbdConn == nil && d.cephfsConn == nil {
		d.cfg.Logger.Warn("no upstream sockets configured, returning stub response", zap.String("volumeId", req.GetVolumeId()))
		return &csi.ControllerExpandVolumeResponse{}, nil
	}

	return nil, fmt.Errorf("ControllerExpandVolume failed on all upstream sockets for volume %s", req.GetVolumeId())
}

func (d *Driver) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return &csi.ControllerGetVolumeResponse{}, nil
}

func (d *Driver) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	return &csi.ControllerModifyVolumeResponse{}, nil
}

// --- Node Service ---

func (d *Driver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	volumeCtx := req.GetVolumeContext()
	isCephFS := volumeCtx["volumeType"] == "cephfs"

	conn := d.rbdConn
	if isCephFS {
		conn = d.cephfsConn
	}

	if conn == nil {
		d.cfg.Logger.Warn("upstream socket not configured for NodeStageVolume",
			zap.String("volumeId", req.GetVolumeId()),
			zap.Bool("isCephFS", isCephFS),
		)
		return &csi.NodeStageVolumeResponse{}, nil
	}

	client := csi.NewNodeClient(conn)
	return client.NodeStageVolume(ctx, req)
}

func (d *Driver) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	// NodeUnstageVolume doesn't have VolumeContext; try both sockets
	if d.rbdConn != nil {
		client := csi.NewNodeClient(d.rbdConn)
		resp, err := client.NodeUnstageVolume(ctx, req)
		if err == nil {
			return resp, nil
		}
		d.cfg.Logger.Debug("NodeUnstageVolume failed on RBD socket, trying CephFS", zap.Error(err))
	}

	if d.cephfsConn != nil {
		client := csi.NewNodeClient(d.cephfsConn)
		resp, err := client.NodeUnstageVolume(ctx, req)
		if err == nil {
			return resp, nil
		}
		d.cfg.Logger.Debug("NodeUnstageVolume failed on CephFS socket", zap.Error(err))
	}

	if d.rbdConn == nil && d.cephfsConn == nil {
		d.cfg.Logger.Warn("no upstream sockets configured for NodeUnstageVolume", zap.String("volumeId", req.GetVolumeId()))
		return &csi.NodeUnstageVolumeResponse{}, nil
	}

	return nil, fmt.Errorf("NodeUnstageVolume failed on all upstream sockets for volume %s", req.GetVolumeId())
}

func (d *Driver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	d.cfg.Logger.Info("NodePublishVolume", zap.String("targetPath", req.GetTargetPath()))

	volumeCtx := req.GetVolumeContext()
	isCephFS := volumeCtx["volumeType"] == "cephfs"

	conn := d.rbdConn
	if isCephFS {
		conn = d.cephfsConn
	}

	if conn == nil {
		d.cfg.Logger.Warn("upstream socket not configured for NodePublishVolume",
			zap.String("volumeId", req.GetVolumeId()),
			zap.Bool("isCephFS", isCephFS),
		)
		return &csi.NodePublishVolumeResponse{}, nil
	}

	client := csi.NewNodeClient(conn)
	return client.NodePublishVolume(ctx, req)
}

func (d *Driver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	// NodeUnpublishVolume doesn't have VolumeContext; try both sockets
	if d.rbdConn != nil {
		client := csi.NewNodeClient(d.rbdConn)
		resp, err := client.NodeUnpublishVolume(ctx, req)
		if err == nil {
			return resp, nil
		}
		d.cfg.Logger.Debug("NodeUnpublishVolume failed on RBD socket, trying CephFS", zap.Error(err))
	}

	if d.cephfsConn != nil {
		client := csi.NewNodeClient(d.cephfsConn)
		resp, err := client.NodeUnpublishVolume(ctx, req)
		if err == nil {
			return resp, nil
		}
		d.cfg.Logger.Debug("NodeUnpublishVolume failed on CephFS socket", zap.Error(err))
	}

	if d.rbdConn == nil && d.cephfsConn == nil {
		d.cfg.Logger.Warn("no upstream sockets configured for NodeUnpublishVolume", zap.String("volumeId", req.GetVolumeId()))
		return &csi.NodeUnpublishVolumeResponse{}, nil
	}

	return nil, fmt.Errorf("NodeUnpublishVolume failed on all upstream sockets for volume %s", req.GetVolumeId())
}

func (d *Driver) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	return &csi.NodeGetVolumeStatsResponse{}, nil
}

func (d *Driver) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	// NodeExpandVolume doesn't have VolumeContext; try both sockets
	if d.rbdConn != nil {
		client := csi.NewNodeClient(d.rbdConn)
		resp, err := client.NodeExpandVolume(ctx, req)
		if err == nil {
			return resp, nil
		}
		d.cfg.Logger.Debug("NodeExpandVolume failed on RBD socket, trying CephFS", zap.Error(err))
	}

	if d.cephfsConn != nil {
		client := csi.NewNodeClient(d.cephfsConn)
		resp, err := client.NodeExpandVolume(ctx, req)
		if err == nil {
			return resp, nil
		}
		d.cfg.Logger.Debug("NodeExpandVolume failed on CephFS socket", zap.Error(err))
	}

	if d.rbdConn == nil && d.cephfsConn == nil {
		d.cfg.Logger.Warn("no upstream sockets configured for NodeExpandVolume", zap.String("volumeId", req.GetVolumeId()))
		return &csi.NodeExpandVolumeResponse{}, nil
	}

	return nil, fmt.Errorf("NodeExpandVolume failed on all upstream sockets for volume %s", req.GetVolumeId())
}

func (d *Driver) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	capTypes := []csi.NodeServiceCapability_RPC_Type{
		csi.NodeServiceCapability_RPC_STAGE_UNSTAGE_VOLUME,
		csi.NodeServiceCapability_RPC_GET_VOLUME_STATS,
		csi.NodeServiceCapability_RPC_EXPAND_VOLUME,
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
		return req.GetCapacityRange().GetRequiredBytes()
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

// dialUnixSocket connects to a unix socket via gRPC
func dialUnixSocket(socketPath string) (*grpc.ClientConn, error) {
	addr := strings.TrimPrefix(socketPath, "unix://")
	return grpc.NewClient(
		"unix://"+addr,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
}
