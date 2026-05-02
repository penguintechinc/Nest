package server

import (
	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/penguintechinc/nest/services/marchproxy/data-proxy/internal/config"
	"github.com/penguintechinc/nest/services/marchproxy/data-proxy/internal/service"
)

// DataProxyServer holds all gRPC service implementations.
type DataProxyServer struct {
	cfg    config.Config
	logger *zap.Logger
}

func New(cfg config.Config, logger *zap.Logger) *DataProxyServer {
	return &DataProxyServer{cfg: cfg, logger: logger}
}

// Register mounts all gRPC service implementations on the server.
func (s *DataProxyServer) Register(srv *grpc.Server) {
	// Register proxy service
	_ = service.NewProxyService(s.logger)
	// Additional services can be registered here
}
