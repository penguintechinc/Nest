package protocol

import (
	"context"
	"fmt"
	"net"
	"time"
)

func init() { Register(&nfsProtocol{}) }

type nfsProtocol struct{}

func (p *nfsProtocol) Name() string        { return "nfs" }
func (p *nfsProtocol) Protocols() []string { return []string{"nfs"} }

func (p *nfsProtocol) Dial(ctx context.Context, cfg ProtocolConfig) (BackendConn, error) {
	return nil, fmt.Errorf("nfs: active proxying not yet implemented (health probe only)")
}

func (p *nfsProtocol) HealthCheck(ctx context.Context, cfg ProtocolConfig) (*HealthResult, error) {
	host := cfg.Extra["host"]
	if host == "" {
		host = cfg.Endpoint
	}

	addr := fmt.Sprintf("%s:2049", host)
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return &HealthResult{State: "failed", Message: fmt.Sprintf("nfs port 2049 unreachable: %v", err)}, nil
	}
	conn.Close()

	return &HealthResult{State: "healthy", Message: "nfs port 2049 reachable"}, nil
}

func (p *nfsProtocol) PoolConfig(cfg ProtocolConfig) PoolOptions {
	return PoolOptions{MaxConns: 0}
}
