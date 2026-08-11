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
	// CredentialData holds the decoded contents of the referenced Kubernetes
	// Secret (CredentialSecret). Credentials belong here, not in Extra, which is
	// stored in cleartext on the DataResource spec.
	CredentialData map[string][]byte
	Endpoint       string // optional base URL override (required for s3-compat)
	Extra          map[string]string
	// IdempotencyToken is a stable per-resource token used as the provider's
	// client/request token so a retried provision returns the existing resource
	// instead of creating a duplicate. Empty means the provider generates its own.
	IdempotencyToken string
}

// Credential returns the value for a credential key, preferring data sourced from
// the referenced Kubernetes Secret (CredentialData) over the plaintext spec Extra
// map. Storing secrets in Extra lands them in the DataResource spec in cleartext,
// so CredentialData always wins when both carry the key.
func (c ExternalProviderConfig) Credential(key string) (string, bool) {
	if c.CredentialData != nil {
		if v, ok := c.CredentialData[key]; ok {
			return string(v), true
		}
	}
	if c.Extra != nil {
		if v, ok := c.Extra[key]; ok {
			return v, true
		}
	}
	return "", false
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
	"aws":          func() StorageProvisioner { return NewAWSStorageProvisioner() },
	"azure":        func() StorageProvisioner { return NewAzureStorageProvisioner() },
	"gcp":          func() StorageProvisioner { return NewGCPStorageProvisioner() },
	"digitalocean": func() StorageProvisioner { return NewDOStorageProvisioner() },
	"vultr":        func() StorageProvisioner { return NewVultrStorageProvisioner() },
	"linode":       func() StorageProvisioner { return NewLinodeStorageProvisioner() },
	"s3-compat":    func() StorageProvisioner { return NewS3CompatProvisioner() },
}
