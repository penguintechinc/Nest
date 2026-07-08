package provider

import (
	"context"
	"fmt"
)

// AzureStorageProvisioner implements StorageProvisioner for Azure (Managed Disk + Blob).
// Authentication not yet implemented.
type AzureStorageProvisioner struct{}

func NewAzureStorageProvisioner() *AzureStorageProvisioner {
	return &AzureStorageProvisioner{}
}

func (p *AzureStorageProvisioner) ProvisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, spec BlockVolumeSpec) (*BlockVolumeInfo, error) {
	return nil, fmt.Errorf("azure provider: authentication not implemented — AAD OAuth2 client credentials required; set ExternalProviderConfig.Extra[\"client_id\", \"client_secret\", \"tenant_id\", \"subscription_id\", \"resource_group\"]")
}

func (p *AzureStorageProvisioner) DeprovisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, volumeID string) error {
	return fmt.Errorf("azure provider: authentication not implemented — AAD OAuth2 client credentials required")
}

func (p *AzureStorageProvisioner) GetBlockVolumeStatus(ctx context.Context, cfg ExternalProviderConfig, volumeID string) (*BlockVolumeInfo, error) {
	return nil, fmt.Errorf("azure provider: authentication not implemented — AAD OAuth2 client credentials required")
}

func (p *AzureStorageProvisioner) ProvisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, spec ObjectBucketSpec) (*ObjectBucketInfo, error) {
	return nil, fmt.Errorf("azure provider: authentication not implemented — AAD OAuth2 client credentials required")
}

func (p *AzureStorageProvisioner) DeprovisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, bucketName string) error {
	return fmt.Errorf("azure provider: authentication not implemented — AAD OAuth2 client credentials required")
}

// Ensure AzureStorageProvisioner implements StorageProvisioner.
var _ StorageProvisioner = (*AzureStorageProvisioner)(nil)
