package service

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/penguintechinc/nest/services/gateway/internal/claims"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
)

// VolumeRequest is the request message for VolumeService.Info.
type VolumeRequest struct {
	Resource string
}

// VolumeResponse is the response message for VolumeService.Info.
type VolumeResponse struct {
	Resource     string
	Tenant       string
	CapacityGB   int64
	UsedGB       int64
	StorageClass string
	Endpoint     string
}

// VolumeServiceServer defines the gRPC service interface.
type VolumeServiceServer interface {
	Info(ctx context.Context, req *VolumeRequest) (*VolumeResponse, error)
}

type volumeServiceImpl struct {
	cfg    config.Config
	logger *zap.Logger
}

func NewVolumeService(cfg config.Config, logger *zap.Logger) VolumeServiceServer {
	return &volumeServiceImpl{cfg: cfg, logger: logger}
}

func (v *volumeServiceImpl) Info(ctx context.Context, req *VolumeRequest) (*VolumeResponse, error) {
	cl, ok := claims.FromContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing claims")
	}
	if req.Resource == "" {
		return nil, status.Error(codes.InvalidArgument, "resource is required")
	}

	endpoint, err := resolveEndpoint(ctx, v.cfg, cl.Tenant, req.Resource)
	if err != nil {
		return nil, status.Errorf(codes.NotFound, "resolve endpoint: %v", err)
	}

	v.logger.Info("volume.info",
		zap.String("tenant", cl.Tenant),
		zap.String("resource", req.Resource),
	)

	return &VolumeResponse{
		Resource: req.Resource,
		Tenant:   cl.Tenant,
		Endpoint: fmt.Sprintf("%s/volumes/%s", endpoint, req.Resource),
	}, nil
}

var volumeServiceDesc = grpc.ServiceDesc{
	ServiceName: "nest.v1.VolumeService",
	HandlerType: (*VolumeServiceServer)(nil),
	Methods: []grpc.MethodDesc{
		{MethodName: "Info", Handler: volumeInfoHandler},
	},
	Streams: []grpc.StreamDesc{},
}

func volumeInfoHandler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(VolumeRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(VolumeServiceServer).Info(ctx, in)
	}
	info := &grpc.UnaryServerInfo{Server: srv, FullMethod: "/nest.v1.VolumeService/Info"}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(VolumeServiceServer).Info(ctx, req.(*VolumeRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func RegisterVolumeService(s *grpc.Server, srv VolumeServiceServer) {
	s.RegisterService(&volumeServiceDesc, srv)
}
