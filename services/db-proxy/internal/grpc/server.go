package grpc

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"time"

	pb "github.com/penguintechinc/nest/services/db-proxy/proto/dbproxyv1"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// ConfigServiceServer implements the ConfigService gRPC API
type ConfigServiceServer struct {
	pb.UnimplementedConfigServiceServer

	logger             *zap.Logger
	getConfigFunc      func(ctx context.Context, key string) (interface{}, error)
	setConfigFunc      func(ctx context.Context, key string, data []byte) error
	reloadConfigFunc   func(ctx context.Context) error
	getStatsFunc       func() map[string]interface{}
	startTime          time.Time
	activeConnsFunc    func() int64
	totalConnsFunc     func() int64
	queriesBlockedFunc func() int64
}

// NewConfigServiceServer creates a new config service server
func NewConfigServiceServer(
	logger *zap.Logger,
	getConfigFunc func(ctx context.Context, key string) (interface{}, error),
	setConfigFunc func(ctx context.Context, key string, data []byte) error,
	reloadConfigFunc func(ctx context.Context) error,
	getStatsFunc func() map[string]interface{},
	activeConnsFunc func() int64,
	totalConnsFunc func() int64,
	queriesBlockedFunc func() int64,
) *ConfigServiceServer {
	return &ConfigServiceServer{
		logger:             logger,
		getConfigFunc:      getConfigFunc,
		setConfigFunc:      setConfigFunc,
		reloadConfigFunc:   reloadConfigFunc,
		getStatsFunc:       getStatsFunc,
		startTime:          time.Now(),
		activeConnsFunc:    activeConnsFunc,
		totalConnsFunc:     totalConnsFunc,
		queriesBlockedFunc: queriesBlockedFunc,
	}
}

// GetConfig retrieves current configuration
func (s *ConfigServiceServer) GetConfig(ctx context.Context, req *pb.GetConfigRequest) (*pb.GetConfigResponse, error) {
	if req.ApiVersion != "v1" {
		return &pb.GetConfigResponse{
			ApiVersion:   "v1",
			Status:       "error",
			ErrorMessage: fmt.Sprintf("api_version %q not supported", req.ApiVersion),
		}, status.Errorf(codes.Unimplemented, "api_version %q not supported", req.ApiVersion)
	}

	cfg, err := s.getConfigFunc(ctx, req.ConfigKey)
	if err != nil {
		s.logger.Error("failed to get config", zap.Error(err), zap.String("key", req.ConfigKey))
		return &pb.GetConfigResponse{
			ApiVersion:   "v1",
			Status:       "error",
			ErrorMessage: err.Error(),
		}, nil
	}

	data, err := json.Marshal(cfg)
	if err != nil {
		s.logger.Error("failed to marshal config", zap.Error(err))
		return &pb.GetConfigResponse{
			ApiVersion:   "v1",
			Status:       "error",
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.GetConfigResponse{
		ApiVersion: "v1",
		Status:     "success",
		ConfigData: data,
	}, nil
}

// SetConfig updates configuration
func (s *ConfigServiceServer) SetConfig(ctx context.Context, req *pb.SetConfigRequest) (*pb.SetConfigResponse, error) {
	if req.ApiVersion != "v1" {
		return &pb.SetConfigResponse{
			ApiVersion:   "v1",
			Status:       "error",
			ErrorMessage: fmt.Sprintf("api_version %q not supported", req.ApiVersion),
		}, status.Errorf(codes.Unimplemented, "api_version %q not supported", req.ApiVersion)
	}

	err := s.setConfigFunc(ctx, req.ConfigKey, req.ConfigData)
	if err != nil {
		s.logger.Error("failed to set config", zap.Error(err), zap.String("key", req.ConfigKey))
		return &pb.SetConfigResponse{
			ApiVersion:   "v1",
			Status:       "error",
			ErrorMessage: err.Error(),
		}, nil
	}

	s.logger.Info("config updated via gRPC", zap.String("key", req.ConfigKey))
	return &pb.SetConfigResponse{
		ApiVersion: "v1",
		Status:     "success",
	}, nil
}

// ReloadConfig triggers a configuration reload from Redis
func (s *ConfigServiceServer) ReloadConfig(ctx context.Context, req *pb.ReloadConfigRequest) (*pb.ReloadConfigResponse, error) {
	if req.ApiVersion != "v1" {
		return &pb.ReloadConfigResponse{
			ApiVersion:   "v1",
			Status:       "error",
			ErrorMessage: fmt.Sprintf("api_version %q not supported", req.ApiVersion),
		}, status.Errorf(codes.Unimplemented, "api_version %q not supported", req.ApiVersion)
	}

	err := s.reloadConfigFunc(ctx)
	if err != nil {
		s.logger.Error("failed to reload config", zap.Error(err))
		return &pb.ReloadConfigResponse{
			ApiVersion:   "v1",
			Status:       "error",
			ErrorMessage: err.Error(),
		}, nil
	}

	return &pb.ReloadConfigResponse{
		ApiVersion:      "v1",
		Status:          "success",
		ReloadTimestamp: time.Now().Format(time.RFC3339),
	}, nil
}

// GetStatus retrieves proxy runtime status
func (s *ConfigServiceServer) GetStatus(ctx context.Context, req *pb.GetStatusRequest) (*pb.GetStatusResponse, error) {
	if req.ApiVersion != "v1" {
		return nil, status.Errorf(codes.Unimplemented, "api_version %q not supported", req.ApiVersion)
	}

	uptime := int64(time.Since(s.startTime).Seconds())

	return &pb.GetStatusResponse{
		ApiVersion:        "v1",
		Status:            "success",
		ActiveConnections: s.activeConnsFunc(),
		TotalConnections:  s.totalConnsFunc(),
		QueriesProcessed:  0, // Not currently tracked; deferred
		QueriesBlocked:    s.queriesBlockedFunc(),
		UptimeSeconds:     fmt.Sprintf("%d", uptime),
	}, nil
}

// Server wraps the gRPC server
type Server struct {
	addr    string
	port    int
	service *ConfigServiceServer
	logger  *zap.Logger

	mu     sync.Mutex // guards server (Start writes it, Stop reads it)
	server *grpc.Server
}

// NewServer creates a new gRPC server
func NewServer(addr string, port int, service *ConfigServiceServer, logger *zap.Logger) *Server {
	return &Server{
		addr:    addr,
		port:    port,
		service: service,
		logger:  logger,
	}
}

// Start starts the gRPC server
func (s *Server) Start() error {
	lis, err := net.Listen("tcp", fmt.Sprintf("%s:%d", s.addr, s.port))
	if err != nil {
		return fmt.Errorf("failed to listen on %s:%d: %w", s.addr, s.port, err)
	}

	srv := grpc.NewServer()
	pb.RegisterConfigServiceServer(srv, s.service)

	s.mu.Lock()
	s.server = srv
	s.mu.Unlock()

	s.logger.Info("gRPC server starting",
		zap.String("addr", s.addr),
		zap.Int("port", s.port),
	)

	// Serve blocks until GracefulStop; use the local to avoid racing the field.
	return srv.Serve(lis)
}

// Stop stops the gRPC server
func (s *Server) Stop() error {
	s.mu.Lock()
	srv := s.server
	s.mu.Unlock()
	if srv != nil {
		srv.GracefulStop()
		s.logger.Info("gRPC server stopped")
	}
	return nil
}
