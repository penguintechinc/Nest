package protocol

import (
	"context"
	"fmt"
	"net"
	"time"
)

func init() { Register(&kafkaProtocol{}) }

type kafkaProtocol struct{}

func (p *kafkaProtocol) Name() string       { return "kafka" }
func (p *kafkaProtocol) Protocols() []string { return []string{"kafka"} }

func (p *kafkaProtocol) Dial(ctx context.Context, cfg ProtocolConfig) (BackendConn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("kafka dial %s: %w", cfg.Endpoint, err)
	}
	return &netConn{conn: conn}, nil
}

func (p *kafkaProtocol) HealthCheck(ctx context.Context, cfg ProtocolConfig) (*HealthResult, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", cfg.Endpoint)
	if err != nil {
		return &HealthResult{State: "failed", Message: fmt.Sprintf("kafka broker unreachable: %v", err)}, nil
	}
	conn.Close()

	return &HealthResult{State: "healthy", Message: "kafka broker reachable"}, nil
}

func (p *kafkaProtocol) PoolConfig(cfg ProtocolConfig) PoolOptions {
	return PoolOptions{MaxConns: 10, MaxIdleTime: 300}
}
