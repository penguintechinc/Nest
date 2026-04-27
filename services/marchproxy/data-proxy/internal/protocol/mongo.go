package protocol

import (
	"context"
	"fmt"
	"net"
	"time"
)

func init() { Register(&mongoProtocol{}) }

type mongoProtocol struct{}

func (p *mongoProtocol) Name() string       { return "mongo" }
func (p *mongoProtocol) Protocols() []string { return []string{"mongo", "mongodb"} }

func (p *mongoProtocol) Dial(ctx context.Context, cfg ProtocolConfig) (BackendConn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("mongo dial %s: %w", cfg.Endpoint, err)
	}
	return &netConn{conn: conn}, nil
}

func (p *mongoProtocol) HealthCheck(ctx context.Context, cfg ProtocolConfig) (*HealthResult, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", cfg.Endpoint)
	if err != nil {
		return &HealthResult{State: "failed", Message: fmt.Sprintf("mongo tcp probe failed: %v", err)}, nil
	}
	conn.Close()

	return &HealthResult{State: "healthy", Message: "mongo wire endpoint reachable"}, nil
}

func (p *mongoProtocol) PoolConfig(cfg ProtocolConfig) PoolOptions {
	return PoolOptions{MaxConns: 20, MaxIdleTime: 300}
}
