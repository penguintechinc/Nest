package protocol

import (
	"context"
	"fmt"
	"net/http"
	"time"
)

func init() { Register(&clickhouseProtocol{}) }

type clickhouseProtocol struct{}

func (p *clickhouseProtocol) Name() string       { return "clickhouse" }
func (p *clickhouseProtocol) Protocols() []string { return []string{"clickhouse"} }

func (p *clickhouseProtocol) Dial(ctx context.Context, cfg ProtocolConfig) (BackendConn, error) {
	return &httpConn{}, nil
}

func (p *clickhouseProtocol) HealthCheck(ctx context.Context, cfg ProtocolConfig) (*HealthResult, error) {
	client := &http.Client{Timeout: 5 * time.Second}

	pingURL := cfg.Endpoint
	if pingURL == "" {
		return &HealthResult{State: "unknown", Message: "no endpoint configured"}, nil
	}

	if pingURL[len(pingURL)-1] != '/' {
		pingURL += "/"
	}
	pingURL += "ping"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pingURL, nil)
	if err != nil {
		return &HealthResult{State: "failed", Message: fmt.Sprintf("invalid endpoint: %v", err)}, nil
	}

	resp, err := client.Do(req)
	if err != nil {
		return &HealthResult{State: "failed", Message: fmt.Sprintf("clickhouse ping failed: %v", err)}, nil
	}
	resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		return &HealthResult{State: "healthy", Message: "clickhouse /ping returned 200"}, nil
	}

	return &HealthResult{
		State:   "degraded",
		Message: fmt.Sprintf("clickhouse /ping returned status %d", resp.StatusCode),
	}, nil
}

func (p *clickhouseProtocol) PoolConfig(cfg ProtocolConfig) PoolOptions {
	return PoolOptions{MaxConns: 20, MaxIdleTime: 120}
}
