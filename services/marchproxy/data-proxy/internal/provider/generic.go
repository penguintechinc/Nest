package provider

import (
	"context"
	"fmt"
	"net"
	"time"
)

func init() { Register(&genericProvider{}) }

type genericProvider struct{}

func (p *genericProvider) Name() string          { return "generic" }
func (p *genericProvider) SupportsIndexing() bool { return false }

func (p *genericProvider) Validate(ctx context.Context, cfg ExternalProviderConfig) error {
	if cfg.Endpoint == "" {
		return fmt.Errorf("generic: endpoint is required for Tier 2 standard-protocol providers")
	}
	if cfg.EngineType == "" {
		return fmt.Errorf("generic: engineType is required (postgres, mysql, redis, kafka, s3)")
	}
	return nil
}

func (p *genericProvider) Discover(ctx context.Context, cfg ExternalProviderConfig) (*ExternalResourceInfo, error) {
	info := &ExternalResourceInfo{
		EngineType: cfg.EngineType,
		Endpoint:   cfg.Endpoint,
	}
	conn, err := net.DialTimeout("tcp", cfg.Endpoint, 3*time.Second)
	if err != nil {
		return info, fmt.Errorf("cannot reach %s: %w", cfg.Endpoint, err)
	}
	conn.Close()
	return info, nil
}

func (p *genericProvider) SetupProxy(ctx context.Context, cfg ExternalProviderConfig) (*ProxyConfig, error) {
	if err := p.Validate(ctx, cfg); err != nil {
		return nil, err
	}
	return &ProxyConfig{
		Endpoint:    cfg.Endpoint,
		TLSRequired: false,
		AuthType:    "basic",
	}, nil
}

func (p *genericProvider) GetCostData(ctx context.Context, cfg ExternalProviderConfig) (*CostData, error) {
	return nil, &ErrNotSupported{Provider: "generic", Capability: "GetCostData"}
}

func (p *genericProvider) CheckHealth(ctx context.Context, cfg ExternalProviderConfig) (*HealthResult, error) {
	if cfg.Endpoint == "" {
		return &HealthResult{State: "unreachable", Message: "no endpoint configured"}, nil
	}
	conn, err := net.DialTimeout("tcp", cfg.Endpoint, 3*time.Second)
	if err != nil {
		return &HealthResult{State: "unreachable", Message: err.Error()}, nil
	}
	conn.Close()
	return &HealthResult{State: "healthy"}, nil
}

func (p *genericProvider) RotateCredential(ctx context.Context, cfg ExternalProviderConfig) (string, error) {
	return "", &ErrNotSupported{Provider: "generic", Capability: "RotateCredential"}
}
