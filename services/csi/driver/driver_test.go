package driver

import (
	"testing"

	"go.uber.org/zap"
)

func TestNew(t *testing.T) {
	logger := zap.NewNop()
	defer logger.Sync()

	cfg := Config{
		Endpoint:   "unix:///tmp/test.sock",
		NodeID:     "test-node",
		DriverName: "test-driver",
		Logger:     logger,
	}

	d := New(cfg)
	if d == nil {
		t.Fatal("New() returned nil")
	}
	if d.cfg.Endpoint != cfg.Endpoint {
		t.Errorf("got Endpoint %s, want %s", d.cfg.Endpoint, cfg.Endpoint)
	}
	if d.cfg.NodeID != cfg.NodeID {
		t.Errorf("got NodeID %s, want %s", d.cfg.NodeID, cfg.NodeID)
	}
	if d.cfg.DriverName != cfg.DriverName {
		t.Errorf("got DriverName %s, want %s", d.cfg.DriverName, cfg.DriverName)
	}
}

func TestParseEndpoint(t *testing.T) {
	tests := []struct {
		name       string
		endpoint   string
		wantScheme string
		wantAddr   string
	}{
		{
			name:       "unix socket",
			endpoint:   "unix:///var/lib/kubelet/plugins/csi.sock",
			wantScheme: "unix",
			wantAddr:   "/var/lib/kubelet/plugins/csi.sock",
		},
		{
			name:       "tcp endpoint",
			endpoint:   "tcp://localhost:50051",
			wantScheme: "tcp",
			wantAddr:   "localhost:50051",
		},
		{
			name:       "plain path defaults to unix",
			endpoint:   "/var/lib/kubelet/plugins/csi.sock",
			wantScheme: "unix",
			wantAddr:   "/var/lib/kubelet/plugins/csi.sock",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			scheme, addr, err := parseEndpoint(tt.endpoint)
			if err != nil {
				t.Errorf("parseEndpoint() unexpected error = %v", err)
				return
			}
			if scheme != tt.wantScheme {
				t.Errorf("got scheme %s, want %s", scheme, tt.wantScheme)
			}
			if addr != tt.wantAddr {
				t.Errorf("got addr %s, want %s", addr, tt.wantAddr)
			}
		})
	}
}
