package provider

import "context"

// BlockVolumeSpec describes a cloud block volume to provision.
type BlockVolumeSpec struct {
	SizeGB           int64
	IOPS             int64
	Throughput       int64
	VolumeType       string
	AvailabilityZone string
	EncryptionKeyID  string
	MultiAttach      bool
}

// BlockVolumeInfo is the result of a successful block volume provisioning.
type BlockVolumeInfo struct {
	VolumeID         string
	State            string
	SizeGB           int64
	AvailabilityZone string
	Endpoint         string
}

// ObjectBucketSpec describes a cloud object bucket to provision.
type ObjectBucketSpec struct {
	BucketName              string
	Versioning              bool
	EncryptionType          string
	LifecycleDays           int
	PublicAccessBlock       bool
	CrossRegionReplication  bool
	ReplicationTargetRegion string
}

// ObjectBucketInfo is the result of a successful bucket provisioning.
type ObjectBucketInfo struct {
	BucketName string
	Endpoint   string
	Region     string
	ARN        string
}

// ExternalProviderConfig holds configuration for external cloud providers.
type ExternalProviderConfig struct {
	Provider         string
	Region           string
	ResourceID       string
	CredentialSecret string
	Extra            map[string]string
}

// StorageProvisioner is an interface for providers that can create and destroy cloud storage.
// Not all external providers implement this — check with type assertion.
type StorageProvisioner interface {
	ProvisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, spec BlockVolumeSpec) (*BlockVolumeInfo, error)
	DeprovisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, volumeID string) error
	GetBlockVolumeStatus(ctx context.Context, cfg ExternalProviderConfig, volumeID string) (*BlockVolumeInfo, error)
	ProvisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, spec ObjectBucketSpec) (*ObjectBucketInfo, error)
	DeprovisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, bucketName string) error
}

// StorageProvisionerFactory is a function that creates a StorageProvisioner.
type StorageProvisionerFactory func() StorageProvisioner

// ProvisionerFactoryMap maps provider names to provisioner factories.
var ProvisionerFactoryMap = map[string]StorageProvisionerFactory{
	"aws":   func() StorageProvisioner { return NewAWSStorageProvisioner() },
	"azure": func() StorageProvisioner { return NewAzureStorageProvisioner() },
	"gcp":   func() StorageProvisioner { return NewGCPStorageProvisioner() },
}


