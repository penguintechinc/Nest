package provider

import (
	"context"

	kprovider "github.com/penguintechinc/nest/pkg/provider"
)

type ExternalResourceInfo struct {
	EngineType    string            `json:"engineType"`
	EngineVersion string            `json:"engineVersion"`
	Endpoint      string            `json:"endpoint"`
	Region        string            `json:"region"`
	Tags          map[string]string `json:"tags,omitempty"`
	SizeGB        int64             `json:"sizeGB,omitempty"`
	ReplicaCount  int               `json:"replicaCount,omitempty"`
}

type ProxyConfig struct {
	Endpoint    string `json:"endpoint"`
	TLSRequired bool   `json:"tlsRequired"`
	AuthType    string `json:"authType"`
}

type CostData struct {
	ProviderCostPerHour float64 `json:"providerCostPerHour"`
	Currency            string  `json:"currency"`
	BillingPeriod       string  `json:"billingPeriod"`
}

type HealthResult struct {
	State   string `json:"state"`
	Message string `json:"message,omitempty"`
}

type ExternalProviderConfig struct {
	Provider         string            `json:"provider"`
	Region           string            `json:"region,omitempty"`
	ResourceID       string            `json:"resourceId,omitempty"`
	CredentialSecret string            `json:"credentialSecret,omitempty"`
	Endpoint         string            `json:"endpoint,omitempty"`
	EngineType       string            `json:"engineType,omitempty"`
	Extra            map[string]string `json:"extra,omitempty"`
}

type ExternalProvider interface {
	Name() string
	SupportsIndexing() bool
	Validate(ctx context.Context, config ExternalProviderConfig) error
	Discover(ctx context.Context, config ExternalProviderConfig) (*ExternalResourceInfo, error)
	SetupProxy(ctx context.Context, config ExternalProviderConfig) (*ProxyConfig, error)
	GetCostData(ctx context.Context, config ExternalProviderConfig) (*CostData, error)
	CheckHealth(ctx context.Context, config ExternalProviderConfig) (*HealthResult, error)
	RotateCredential(ctx context.Context, config ExternalProviderConfig) (newSecret string, err error)
}

type ErrNotSupported struct {
	Provider   string
	Capability string
}

func (e *ErrNotSupported) Error() string {
	return e.Provider + ": " + e.Capability + " is not supported by this provider"
}

// Re-export storage provisioning types from pkg/provider for convenient access
type BlockVolumeSpec = kprovider.BlockVolumeSpec
type BlockVolumeInfo = kprovider.BlockVolumeInfo
type ObjectBucketSpec = kprovider.ObjectBucketSpec
type ObjectBucketInfo = kprovider.ObjectBucketInfo
type StorageProvisioner = kprovider.StorageProvisioner
