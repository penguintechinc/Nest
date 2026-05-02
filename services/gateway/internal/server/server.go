package server

import (
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/penguintechinc/nest/services/gateway/internal/config"
	"github.com/penguintechinc/nest/services/gateway/internal/service"
)

// GatewayServer holds all gRPC service implementations.
type GatewayServer struct {
	cfg    config.Config
	logger *zap.Logger
}

func New(cfg config.Config, logger *zap.Logger) *GatewayServer {
	return &GatewayServer{cfg: cfg, logger: logger}
}

// Register mounts all gRPC service implementations on the server.
// Services are registered via grpc.ServiceDesc to avoid proto compilation.
func (s *GatewayServer) Register(srv *grpc.Server) {
	service.RegisterQueryService(srv, service.NewQueryService(s.cfg, s.logger))
	service.RegisterObjectService(srv, service.NewObjectService(s.cfg, s.logger))
	service.RegisterVolumeService(srv, service.NewVolumeService(s.cfg, s.logger))
}
