package service

import (
	"context"

	"go.uber.org/zap"
)

// ProxyService provides gRPC-based proxy routing for wire-protocol backends.
// This is the Phase 1 stub; full proxying is implemented in Phase 2.
type ProxyService struct {
	logger *zap.Logger
}

// NewProxyService creates a new ProxyService.
func NewProxyService(logger *zap.Logger) *ProxyService {
	return &ProxyService{logger: logger}
}

// RouteRequest returns the backend endpoint for a given resource.
func (s *ProxyService) RouteRequest(ctx context.Context, resource, protocol string) (string, error) {
	s.logger.Info("proxy route request",
		zap.String("resource", resource),
		zap.String("protocol", protocol),
	)
	return "", nil
}
