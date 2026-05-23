package server

import (
	"net"
	"testing"

	"go.uber.org/zap"
	"google.golang.org/grpc"

	"github.com/penguintechinc/nest/services/gateway/internal/config"
)

func TestGatewayServer_Register(t *testing.T) {
	cfg := config.Config{
		OIDCIssuer:   "http://issuer",
		OIDCAudience: "test",
	}
	logger := zap.NewNop()

	s := New(cfg, logger)
	if s == nil {
		t.Fatal("New() returned nil")
	}

	// Create a real gRPC server and register
	grpcServer := grpc.NewServer()
	// Register should not panic
	s.Register(grpcServer)

	// Verify services are registered (ServiceInfo returns a map)
	info := grpcServer.GetServiceInfo()
	if len(info) == 0 {
		t.Error("Register() did not register any gRPC services")
	}

	// Check expected services
	expectedServices := []string{
		"nest.v1.QueryService",
		"nest.v1.ObjectService",
		"nest.v1.VolumeService",
	}
	for _, svc := range expectedServices {
		if _, ok := info[svc]; !ok {
			t.Errorf("Register() missing service %q", svc)
		}
	}
}

func TestGatewayServer_Register_WithListener(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	s := New(cfg, logger)

	// Create listener on ephemeral port
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	grpcServer := grpc.NewServer()
	s.Register(grpcServer)

	// Verify registration worked
	info := grpcServer.GetServiceInfo()
	if len(info) == 0 {
		t.Error("Register() should have registered services")
	}
}
