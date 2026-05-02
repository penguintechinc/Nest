package protocol

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

func init() { Register(&s3Protocol{}) }

type s3Protocol struct{}

func (p *s3Protocol) Name() string       { return "s3" }
func (p *s3Protocol) Protocols() []string { return []string{"s3", "object"} }

func (p *s3Protocol) Dial(ctx context.Context, cfg ProtocolConfig) (BackendConn, error) {
	return &httpConn{}, nil
}

func (p *s3Protocol) HealthCheck(ctx context.Context, cfg ProtocolConfig) (*HealthResult, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	endpoint := cfg.Endpoint
	if endpoint == "" {
		return &HealthResult{State: "unknown", Message: "no endpoint configured"}, nil
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return &HealthResult{State: "failed", Message: fmt.Sprintf("invalid endpoint: %v", err)}, nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return &HealthResult{State: "failed", Message: fmt.Sprintf("s3 endpoint unreachable: %v", err)}, nil
	}
	resp.Body.Close()

	return &HealthResult{State: "healthy", Message: fmt.Sprintf("s3 endpoint reachable (status %d)", resp.StatusCode)}, nil
}

func (p *s3Protocol) PoolConfig(cfg ProtocolConfig) PoolOptions {
	return PoolOptions{MaxConns: 100, MaxIdleTime: 60}
}

type httpConn struct{}

func (c *httpConn) Close() error { return nil }
