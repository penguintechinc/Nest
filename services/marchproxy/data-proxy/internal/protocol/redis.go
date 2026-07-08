package protocol

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"
)

func init() { Register(&redisProtocol{}) }

type redisProtocol struct{}

func (p *redisProtocol) Name() string        { return "redis" }
func (p *redisProtocol) Protocols() []string { return []string{"redis", "valkey", "keyvalue"} }

func (p *redisProtocol) Dial(ctx context.Context, cfg ProtocolConfig) (BackendConn, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", cfg.Endpoint)
	if err != nil {
		return nil, fmt.Errorf("redis dial %s: %w", cfg.Endpoint, err)
	}
	return &netConn{conn: conn}, nil
}

func (p *redisProtocol) HealthCheck(ctx context.Context, cfg ProtocolConfig) (*HealthResult, error) {
	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", cfg.Endpoint)
	if err != nil {
		return &HealthResult{State: "failed", Message: fmt.Sprintf("tcp connect failed: %v", err)}, nil
	}
	conn.Close()

	return &HealthResult{State: "healthy", Message: "redis/valkey endpoint reachable"}, nil
}

func TenantKeyPrefix(tenantID string) string {
	return fmt.Sprintf("t:%s:", tenantID)
}

func PrefixKey(tenantID, key string) string {
	prefix := TenantKeyPrefix(tenantID)
	if strings.HasPrefix(key, prefix) {
		return key
	}
	return prefix + key
}

func (p *redisProtocol) PoolConfig(cfg ProtocolConfig) PoolOptions {
	return PoolOptions{MaxConns: 50, MaxIdleTime: 120}
}
