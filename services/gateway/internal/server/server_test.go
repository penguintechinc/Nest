package server

import (
	"testing"

	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/gateway/internal/config"
)

func TestNewGatewayServer(t *testing.T) {
	cfg := config.Config{
		OIDCIssuer:   "http://issuer",
		OIDCAudience: "test",
	}
	logger := zap.NewNop()

	server := New(cfg, logger)

	if server == nil {
		t.Error("New() returned nil")
	}

	if server.cfg != cfg {
		t.Error("New() did not set config")
	}

	if server.logger != logger {
		t.Error("New() did not set logger")
	}
}

func TestGatewayServer_Fields(t *testing.T) {
	cfg := config.Config{OIDCIssuer: "http://test"}
	logger := zap.NewNop()

	server := New(cfg, logger)

	if server.cfg.OIDCIssuer != "http://test" {
		t.Errorf("cfg not set correctly")
	}
}
