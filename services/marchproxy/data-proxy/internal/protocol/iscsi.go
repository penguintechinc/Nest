package protocol

import (
	"context"
	"fmt"
	"net"
	"time"
)

func init() { Register(&iscsiProtocol{}) }

type iscsiProtocol struct{}

func (p *iscsiProtocol) Name() string        { return "iscsi" }
func (p *iscsiProtocol) Protocols() []string { return []string{"iscsi"} }

func (p *iscsiProtocol) Dial(ctx context.Context, cfg ProtocolConfig) (BackendConn, error) {
	return nil, fmt.Errorf("iscsi: active proxying not yet implemented (health probe only)")
}

func (p *iscsiProtocol) HealthCheck(ctx context.Context, cfg ProtocolConfig) (*HealthResult, error) {
	host := cfg.Extra["host"]
	if host == "" {
		host = cfg.Endpoint
	}

	addr := fmt.Sprintf("%s:3260", host)
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return &HealthResult{State: "failed", Message: fmt.Sprintf("iscsi port 3260 unreachable: %v", err)}, nil
	}
	conn.Close()

	return &HealthResult{State: "healthy", Message: "iscsi port 3260 reachable"}, nil
}

func (p *iscsiProtocol) PoolConfig(cfg ProtocolConfig) PoolOptions {
	return PoolOptions{MaxConns: 0}
}
