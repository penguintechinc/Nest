// Package driver implements the Nest CSI driver.
// It is a thin admission/policy shim in front of Rook-Ceph's CSI driver.
// It injects tenant CephX credentials, enforces quotas, and rewrites StorageClass names.
package driver

import (
	"context"
	"fmt"
	"net"
	"os"
	"regexp"
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

// volumeNameRegex: volume names must start with alphanumeric, then alphanumeric/._-
// Max 253 chars (RFC 952 hostname limit), no leading dash, no /, .., or whitespace.
var volumeNameRegex = regexp.MustCompile(`^[a-zA-Z0-9][a-zA-Z0-9._-]{0,251}$`)

// validateVolumeName checks if a volume name is safe for use in CLI arguments.
// Rejects names with leading dash, slashes, .., or invalid characters.
func validateVolumeName(name string) error {
	if name == "" {
		return status.Error(codes.InvalidArgument, "volume name cannot be empty")
	}
	if strings.HasPrefix(name, "-") {
		return status.Error(codes.InvalidArgument, "volume name cannot start with dash")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		return status.Error(codes.InvalidArgument, "volume name cannot contain / or ..")
	}
	if strings.ContainsAny(name, " \t\n\r") {
		return status.Error(codes.InvalidArgument, "volume name cannot contain whitespace")
	}
	if !volumeNameRegex.MatchString(name) {
		return status.Errorf(codes.InvalidArgument, "volume name %q does not match pattern: must start with alphanumeric, then alphanumeric/._-, max 253 chars", name)
	}
	return nil
}

// validateSnapshotName applies same validation as volume names (snapshots use same constraints).
func validateSnapshotName(name string) error {
	if name == "" {
		return status.Error(codes.InvalidArgument, "snapshot name cannot be empty")
	}
	if strings.HasPrefix(name, "-") {
		return status.Error(codes.InvalidArgument, "snapshot name cannot start with dash")
	}
	if strings.Contains(name, "/") || strings.Contains(name, "..") {
		return status.Error(codes.InvalidArgument, "snapshot name cannot contain / or ..")
	}
	if strings.ContainsAny(name, " \t\n\r") {
		return status.Error(codes.InvalidArgument, "snapshot name cannot contain whitespace")
	}
	if !volumeNameRegex.MatchString(name) {
		return status.Errorf(codes.InvalidArgument, "snapshot name %q does not match pattern: must start with alphanumeric, then alphanumeric/._-, max 253 chars", name)
	}
	return nil
}

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
	cfg         Config
	serverMu    sync.Mutex // Protects server field from data races
	server      *grpc.Server
	mu          sync.RWMutex
	snapshots   map[string]*csi.Snapshot // snapshots by (name, sourceVolumeID) tuple for idempotency
	provisioner CephProvisioner          // Provisioner for volumes/snapshots (injected for testing)
	mounter     Mounter                  // Mounter for staging/publishing (injected for testing)
}

func New(cfg Config) *Driver {
	return &Driver{
		cfg:         cfg,
		snapshots:   make(map[string]*csi.Snapshot),
		provisioner: NewRealCephProvisioner(cfg.Logger),
		mounter:     NewRealMounter(cfg.Logger),
	}
}

// NewWithMocks creates a Driver with injected provisioner and mounter (for testing).
func NewWithMocks(cfg Config, provisioner CephProvisioner, mounter Mounter) *Driver {
	return &Driver{
		cfg:         cfg,
		snapshots:   make(map[string]*csi.Snapshot),
		provisioner: provisioner,
		mounter:     mounter,
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

	d.serverMu.Lock()
	d.server = grpc.NewServer()
	csi.RegisterIdentityServer(d.server, d)
	csi.RegisterControllerServer(d.server, d)
	csi.RegisterNodeServer(d.server, d)
	d.cfg.Logger.Info("CSI gRPC server listening", zap.String("endpoint", d.cfg.Endpoint))
	srv := d.server
	d.serverMu.Unlock()

	return srv.Serve(listener)
}

// IsReady returns true if the gRPC server is ready.
// Safe to call concurrently with Run().
func (d *Driver) IsReady() bool {
	d.serverMu.Lock()
	defer d.serverMu.Unlock()
	return d.server != nil
}

// Stop gracefully shuts down the gRPC server if it's running.
// Safe to call concurrently with Run().
func (d *Driver) Stop() {
	d.serverMu.Lock()
	defer d.serverMu.Unlock()

	if d.server != nil {
		d.server.GracefulStop()
	}
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

	// INVALID_ARGUMENT: strict validation to prevent CLI argument injection
	if err := validateVolumeName(req.GetName()); err != nil {
		return nil, err
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

	// Extract size from capacity range
	sizeMB := capacityFromRequest(req) / (1024 * 1024)
	if sizeMB <= 0 {
		sizeMB = 10 * 1024 // 10 GiB default
	}

	d.cfg.Logger.Info("Creating volume", zap.String("name", req.GetName()), zap.Int64("sizeMB", sizeMB), zap.Bool("isCephFS", isCephFS))

	var volInfo *VolumeInfo
	var err error

	if isCephFS {
		// Create CephFS subvolume
		fsName := req.GetParameters()["fsName"]
		if fsName == "" {
			fsName = "cephfs" // default filesystem name
		}
		volInfo, err = d.provisioner.CreateCephFSSubvolume(ctx, fsName, req.GetName(), sizeMB)
	} else {
		// Create RBD image
		pool := req.GetParameters()["pool"]
		if pool == "" {
			pool = "rbd" // default pool name
		}
		volInfo, err = d.provisioner.CreateRBDImage(ctx, pool, req.GetName(), sizeMB)
	}

	if err != nil {
		return nil, err
	}

	// Return volume info with context
	volCtx := map[string]string{
		"volumeType": "rbd",
		"pool":       volInfo.Pool,
	}
	if isCephFS {
		volCtx["volumeType"] = "cephfs"
		volCtx["fsName"] = volInfo.Pool // reuse Pool field for fsName in CephFS case
		volCtx["subvolumePath"] = req.GetName()
	}

	return &csi.CreateVolumeResponse{
		Volume: &csi.Volume{
			VolumeId:      req.GetName(),
			CapacityBytes: int64(volInfo.Size),
			VolumeContext: volCtx,
		},
	}, nil
}

func (d *Driver) DeleteVolume(ctx context.Context, req *csi.DeleteVolumeRequest) (*csi.DeleteVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID required")
	}

	// Strict validation to prevent CLI argument injection
	if err := validateVolumeName(req.GetVolumeId()); err != nil {
		return nil, err
	}

	volumeID := req.GetVolumeId()
	d.cfg.Logger.Info("Deleting volume", zap.String("volumeID", volumeID))

	// Try to determine if this is an RBD or CephFS volume; for now, try RBD first, then CephFS
	// In a real scenario, we'd track this in a metadata store
	pool := "rbd" // default
	err := d.provisioner.DeleteRBDImage(ctx, pool, volumeID)
	if err != nil && status.Code(err) == codes.NotFound {
		// Try CephFS
		fsName := "cephfs" // default
		err = d.provisioner.DeleteCephFSSubvolume(ctx, fsName, volumeID)
		if err != nil && status.Code(err) != codes.NotFound {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	return &csi.DeleteVolumeResponse{}, nil
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
	// Advertise capabilities that are actually implemented:
	// - CREATE_DELETE_VOLUME: implemented (CreateVolume, DeleteVolume)
	// - EXPAND_VOLUME: implemented (ControllerExpandVolume)
	// - CREATE_DELETE_SNAPSHOT: implemented (CreateSnapshot, DeleteSnapshot)
	// - LIST_SNAPSHOTS: implemented (ListSnapshots)
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
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID required")
	}

	// Strict validation to prevent CLI argument injection
	if err := validateVolumeName(req.GetVolumeId()); err != nil {
		return nil, err
	}

	if req.GetCapacityRange() == nil || req.GetCapacityRange().GetRequiredBytes() <= 0 {
		return nil, status.Error(codes.InvalidArgument, "required capacity range required")
	}

	volumeID := req.GetVolumeId()
	newSizeMB := req.GetCapacityRange().GetRequiredBytes() / (1024 * 1024)
	d.cfg.Logger.Info("Expanding volume", zap.String("volumeID", volumeID), zap.Int64("newSizeMB", newSizeMB))

	// Try RBD first
	pool := "rbd" // default
	err := d.provisioner.ResizeRBDImage(ctx, pool, volumeID, newSizeMB)
	if err != nil && status.Code(err) == codes.NotFound {
		// Try CephFS
		fsName := "cephfs" // default
		err = d.provisioner.ResizeCephFSSubvolume(ctx, fsName, volumeID, newSizeMB)
		if err != nil && status.Code(err) != codes.NotFound {
			return nil, err
		}
	} else if err != nil {
		return nil, err
	}

	return &csi.ControllerExpandVolumeResponse{
		CapacityBytes: req.GetCapacityRange().GetRequiredBytes(),
	}, nil
}

func (d *Driver) ControllerGetVolume(ctx context.Context, req *csi.ControllerGetVolumeRequest) (*csi.ControllerGetVolumeResponse, error) {
	return &csi.ControllerGetVolumeResponse{}, nil
}

func (d *Driver) ControllerModifyVolume(ctx context.Context, req *csi.ControllerModifyVolumeRequest) (*csi.ControllerModifyVolumeResponse, error) {
	return &csi.ControllerModifyVolumeResponse{}, nil
}

// --- Node Service ---

func (d *Driver) NodeStageVolume(ctx context.Context, req *csi.NodeStageVolumeRequest) (*csi.NodeStageVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID required")
	}
	if req.GetStagingTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "staging target path required")
	}

	volumeID := req.GetVolumeId()
	stagingPath := req.GetStagingTargetPath()
	d.cfg.Logger.Info("Staging volume", zap.String("volumeID", volumeID), zap.String("stagingPath", stagingPath))

	if err := d.mounter.StageVolume(ctx, volumeID, stagingPath, req.GetVolumeContext()); err != nil {
		return nil, err
	}

	return &csi.NodeStageVolumeResponse{}, nil
}

func (d *Driver) NodeUnstageVolume(ctx context.Context, req *csi.NodeUnstageVolumeRequest) (*csi.NodeUnstageVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID required")
	}
	if req.GetStagingTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "staging target path required")
	}

	volumeID := req.GetVolumeId()
	stagingPath := req.GetStagingTargetPath()
	d.cfg.Logger.Info("Unstaging volume", zap.String("volumeID", volumeID), zap.String("stagingPath", stagingPath))

	if err := d.mounter.UnstageVolume(ctx, volumeID, stagingPath); err != nil {
		return nil, err
	}

	return &csi.NodeUnstageVolumeResponse{}, nil
}

func (d *Driver) NodePublishVolume(ctx context.Context, req *csi.NodePublishVolumeRequest) (*csi.NodePublishVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID required")
	}
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "target path required")
	}
	if req.GetVolumeCapability() == nil {
		return nil, status.Error(codes.InvalidArgument, "volume capability required")
	}

	volumeID := req.GetVolumeId()
	targetPath := req.GetTargetPath()
	readOnly := req.GetReadonly()

	d.cfg.Logger.Info("Publishing volume", zap.String("volumeID", volumeID), zap.String("targetPath", targetPath), zap.Bool("readOnly", readOnly))

	// For block volumes, use the staging path directly
	stagingPath := req.GetStagingTargetPath()
	if stagingPath == "" {
		// If no staging path, this is a raw block device publish (not supported yet)
		return nil, status.Error(codes.InvalidArgument, "staging target path required")
	}

	if err := d.mounter.PublishVolume(ctx, volumeID, stagingPath, targetPath, readOnly); err != nil {
		return nil, err
	}

	return &csi.NodePublishVolumeResponse{}, nil
}

func (d *Driver) NodeUnpublishVolume(ctx context.Context, req *csi.NodeUnpublishVolumeRequest) (*csi.NodeUnpublishVolumeResponse, error) {
	if req.GetVolumeId() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume ID required")
	}
	if req.GetTargetPath() == "" {
		return nil, status.Error(codes.InvalidArgument, "target path required")
	}

	volumeID := req.GetVolumeId()
	targetPath := req.GetTargetPath()
	d.cfg.Logger.Info("Unpublishing volume", zap.String("volumeID", volumeID), zap.String("targetPath", targetPath))

	if err := d.mounter.UnpublishVolume(ctx, volumeID, targetPath); err != nil {
		return nil, err
	}

	return &csi.NodeUnpublishVolumeResponse{}, nil
}

func (d *Driver) NodeGetVolumeStats(ctx context.Context, req *csi.NodeGetVolumeStatsRequest) (*csi.NodeGetVolumeStatsResponse, error) {
	if req.GetVolumePath() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume path required")
	}

	volumePath := req.GetVolumePath()
	d.cfg.Logger.Info("Getting volume stats", zap.String("volumePath", volumePath))

	stats, err := d.mounter.GetVolumeStats(ctx, volumePath)
	if err != nil {
		return nil, err
	}

	return &csi.NodeGetVolumeStatsResponse{
		Usage: []*csi.VolumeUsage{
			{
				Available: stats.AvailableBytes,
				Total:     stats.TotalBytes,
				Used:      stats.UsedBytes,
				Unit:      csi.VolumeUsage_BYTES,
			},
			{
				Available: stats.AvailableInodes,
				Total:     stats.TotalInodes,
				Used:      stats.UsedInodes,
				Unit:      csi.VolumeUsage_INODES,
			},
		},
	}, nil
}

func (d *Driver) NodeExpandVolume(ctx context.Context, req *csi.NodeExpandVolumeRequest) (*csi.NodeExpandVolumeResponse, error) {
	if req.GetVolumePath() == "" {
		return nil, status.Error(codes.InvalidArgument, "volume path required")
	}

	volumePath := req.GetVolumePath()
	d.cfg.Logger.Info("Expanding volume filesystem", zap.String("volumePath", volumePath))

	if err := d.mounter.ExpandFilesystem(ctx, volumePath); err != nil {
		return nil, err
	}

	// Get updated stats
	stats, err := d.mounter.GetVolumeStats(ctx, volumePath)
	if err != nil {
		return nil, err
	}

	return &csi.NodeExpandVolumeResponse{
		CapacityBytes: stats.TotalBytes,
	}, nil
}

func (d *Driver) NodeGetCapabilities(ctx context.Context, req *csi.NodeGetCapabilitiesRequest) (*csi.NodeGetCapabilitiesResponse, error) {
	// Advertise capabilities that are actually implemented:
	// - STAGE_UNSTAGE_VOLUME: implemented (NodeStageVolume, NodeUnstageVolume)
	// - GET_VOLUME_STATS: implemented (NodeGetVolumeStats)
	// - EXPAND_VOLUME: implemented (NodeExpandVolume)
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
