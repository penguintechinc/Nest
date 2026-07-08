package provider

import (
	"context"
	"fmt"
)

// GCPStorageProvisioner implements StorageProvisioner for GCP (Persistent Disk + GCS).
// Authentication not yet implemented.
type GCPStorageProvisioner struct{}

func NewGCPStorageProvisioner() *GCPStorageProvisioner {
	return &GCPStorageProvisioner{}
}

func (p *GCPStorageProvisioner) ProvisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, spec BlockVolumeSpec) (*BlockVolumeInfo, error) {
	return nil, fmt.Errorf("gcp provider: authentication not implemented — Google OAuth2 service account required; set ExternalProviderConfig.Extra[\"service_account_json\"] or use ADC")
}

func (p *GCPStorageProvisioner) DeprovisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, volumeID string) error {
	return fmt.Errorf("gcp provider: authentication not implemented — Google OAuth2 service account required")
}

func (p *GCPStorageProvisioner) GetBlockVolumeStatus(ctx context.Context, cfg ExternalProviderConfig, volumeID string) (*BlockVolumeInfo, error) {
	return nil, fmt.Errorf("gcp provider: authentication not implemented — Google OAuth2 service account required")
}

func (p *GCPStorageProvisioner) ProvisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, spec ObjectBucketSpec) (*ObjectBucketInfo, error) {
	return nil, fmt.Errorf("gcp provider: authentication not implemented — Google OAuth2 service account required")
}

func (p *GCPStorageProvisioner) DeprovisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, bucketName string) error {
	return fmt.Errorf("gcp provider: authentication not implemented — Google OAuth2 service account required")
}

// Ensure GCPStorageProvisioner implements StorageProvisioner.
var _ StorageProvisioner = (*GCPStorageProvisioner)(nil)
