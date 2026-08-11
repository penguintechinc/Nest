package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"cloud.google.com/go/storage"
	"google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// GCPStorageProvisioner implements StorageProvisioner for GCP (Persistent Disk + GCS).
// It uses the Google Cloud SDKs for Compute Engine (Persistent Disk) and Cloud Storage (GCS).
// Credentials come from cfg.Extra["service_account_json"] (service account key JSON)
// with fallback to Application Default Credentials (ADC).
type GCPStorageProvisioner struct {
	computeService *compute.Service
	storageClient  *storage.Client
}

func NewGCPStorageProvisioner() *GCPStorageProvisioner {
	return &GCPStorageProvisioner{}
}

// initClients initializes Compute and Storage clients using credentials from cfg or ADC.
// If clients are already set (e.g., by tests injecting mocks), it skips re-initialization.
func (p *GCPStorageProvisioner) initClients(ctx context.Context, cfg ExternalProviderConfig) error {
	if p.computeService != nil && p.storageClient != nil {
		return nil // already initialized (tests may pre-inject via fields)
	}

	var opts []option.ClientOption

	// Try to load credentials from service account JSON
	if saJSON, ok := cfg.Credential("service_account_json"); ok {
		opts = append(opts, option.WithAuthCredentialsJSON(option.ServiceAccount, []byte(saJSON)))
	}
	// Otherwise, use Application Default Credentials

	var err error
	p.computeService, err = compute.NewService(ctx, opts...)
	if err != nil {
		return fmt.Errorf("create GCP compute service: %w", err)
	}

	p.storageClient, err = storage.NewClient(ctx, opts...)
	if err != nil {
		return fmt.Errorf("create GCP storage client: %w", err)
	}

	return nil
}

func (p *GCPStorageProvisioner) ProvisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, spec BlockVolumeSpec) (*BlockVolumeInfo, error) {
	project := cfg.Extra["project_id"]
	if project == "" {
		return nil, fmt.Errorf("GCP project_id is required for Persistent Disk provisioning")
	}

	if err := p.initClients(ctx, cfg); err != nil {
		return nil, err
	}

	// Parse zone from AvailabilityZone or use default
	zone := spec.AvailabilityZone
	if zone == "" {
		zone = "us-central1-a"
	}

	sizeGB := spec.SizeGB
	if sizeGB == 0 {
		sizeGB = 100
	}

	volumeType := spec.VolumeType
	if volumeType == "" {
		volumeType = "pd-standard"
	}

	diskName, err := p.createDisk(ctx, project, zone, volumeType, sizeGB, spec, cfg.IdempotencyToken)
	if err != nil {
		return nil, fmt.Errorf("create persistent disk: %w", err)
	}

	return &BlockVolumeInfo{
		VolumeID:         diskName,
		State:            "creating",
		SizeGB:           sizeGB,
		AvailabilityZone: zone,
		Endpoint:         fmt.Sprintf("gce://%s/zones/%s/disks/%s", project, zone, diskName),
	}, nil
}

func (p *GCPStorageProvisioner) createDisk(ctx context.Context, project, zone, diskType string, sizeGB int64, spec BlockVolumeSpec, clientToken string) (string, error) {
	// Generate a disk name from the idempotency token or use a generated name
	diskName := clientToken
	if diskName == "" {
		diskName = fmt.Sprintf("disk-%d", sizeGB)
	}

	// Build the disk resource
	disk := &compute.Disk{
		Name:   diskName,
		SizeGb: sizeGB,
		Type:   fmt.Sprintf("projects/%s/zones/%s/diskTypes/%s", project, zone, diskType),
	}

	// Apply encryption if specified (customer-supplied encryption key)
	if spec.EncryptionKeyID != "" {
		disk.DiskEncryptionKey = &compute.CustomerEncryptionKey{
			RawKey: spec.EncryptionKeyID,
		}
	}

	// Create the disk using the Compute API
	op, err := p.computeService.Disks.Insert(project, zone, disk).RequestId(clientToken).Context(ctx).Do()
	if err != nil {
		return "", fmt.Errorf("disks.Insert: %w", err)
	}

	// Wait for the operation to complete
	if err := p.waitForOperation(ctx, project, zone, op.Name); err != nil {
		return "", fmt.Errorf("wait for disk creation: %w", err)
	}

	return diskName, nil
}

// waitForOperation polls a GCP zone operation until it reaches status DONE,
// an error occurs, or ctx is cancelled/times out. Callers bound the wait via
// context.WithTimeout.
func (p *GCPStorageProvisioner) waitForOperation(ctx context.Context, project, zone, opName string) error {
	for {
		op, err := p.computeService.ZoneOperations.Get(project, zone, opName).Context(ctx).Do()
		if err != nil {
			return fmt.Errorf("get operation status: %w", err)
		}

		if op.Status == "DONE" {
			if op.Error != nil && len(op.Error.Errors) > 0 {
				return fmt.Errorf("operation failed: %v", op.Error.Errors[0])
			}
			return nil
		}

		select {
		case <-ctx.Done():
			return fmt.Errorf("wait for operation %s: %w", opName, ctx.Err())
		case <-time.After(2 * time.Second):
		}
	}
}

func (p *GCPStorageProvisioner) DeprovisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, volumeID string) error {
	project := cfg.Extra["project_id"]
	if project == "" {
		return fmt.Errorf("GCP project_id is required for Persistent Disk deprovisioning")
	}

	if err := p.initClients(ctx, cfg); err != nil {
		return err
	}

	zone := cfg.Region
	if zone == "" {
		zone = "us-central1-a"
	}

	op, err := p.computeService.Disks.Delete(project, zone, volumeID).Context(ctx).Do()
	if err != nil {
		// Treat disk not found as success (idempotent)
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("disks.Delete: %w", err)
	}

	// Wait for the operation to complete
	if err := p.waitForOperation(ctx, project, zone, op.Name); err != nil {
		// If the error is "not found", treat as idempotent success
		if isNotFound(err) {
			return nil
		}
		return fmt.Errorf("wait for disk deletion: %w", err)
	}

	return nil
}

func (p *GCPStorageProvisioner) GetBlockVolumeStatus(ctx context.Context, cfg ExternalProviderConfig, volumeID string) (*BlockVolumeInfo, error) {
	project := cfg.Extra["project_id"]
	if project == "" {
		return nil, fmt.Errorf("GCP project_id is required for Persistent Disk status check")
	}

	if err := p.initClients(ctx, cfg); err != nil {
		return nil, err
	}

	zone := cfg.Region
	if zone == "" {
		zone = "us-central1-a"
	}

	disk, err := p.computeService.Disks.Get(project, zone, volumeID).Context(ctx).Do()
	if err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("disk %s not found", volumeID)
		}
		return nil, fmt.Errorf("disks.Get: %w", err)
	}

	return &BlockVolumeInfo{
		VolumeID:         volumeID,
		State:            disk.Status,
		SizeGB:           disk.SizeGb,
		AvailabilityZone: zone,
		Endpoint:         fmt.Sprintf("gce://%s/zones/%s/disks/%s", project, zone, volumeID),
	}, nil
}

func (p *GCPStorageProvisioner) ProvisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, spec ObjectBucketSpec) (*ObjectBucketInfo, error) {
	project := cfg.Extra["project_id"]
	if project == "" {
		return nil, fmt.Errorf("GCP project_id is required for GCS bucket provisioning")
	}

	bucketName := spec.BucketName
	if bucketName == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	// Validate encryption config before making API calls
	encType := spec.EncryptionType
	if encType != "" && encType != "google-managed" && encType != "AES256" {
		if encType == "customer-managed" {
			kmsKeyID, ok := cfg.Extra["kms_key_id"]
			if !ok || kmsKeyID == "" {
				return nil, fmt.Errorf("customer-managed encryption requested but kms_key_id not provided in config")
			}
		} else {
			return nil, fmt.Errorf("unsupported encryption type: %s", encType)
		}
	}

	if err := p.initClients(ctx, cfg); err != nil {
		return nil, err
	}

	region := cfg.Region
	if region == "" {
		region = "US"
	}

	// Create bucket
	if err := p.createGCSBucket(ctx, project, region, bucketName); err != nil {
		return nil, fmt.Errorf("create GCS bucket: %w", err)
	}

	// Apply versioning if requested
	if spec.Versioning {
		if err := p.putBucketVersioning(ctx, bucketName); err != nil {
			return nil, fmt.Errorf("enable versioning: %w", err)
		}
	}

	// Apply encryption (Google-managed by default; CMEK if requested)
	if encType == "customer-managed" {
		kmsKeyID := cfg.Extra["kms_key_id"]
		if err := p.putBucketEncryption(ctx, bucketName, kmsKeyID); err != nil {
			return nil, fmt.Errorf("enable KMS encryption: %w", err)
		}
	}

	// Block public access
	if spec.PublicAccessBlock {
		if err := p.putPublicAccessPrevention(ctx, bucketName); err != nil {
			return nil, fmt.Errorf("apply public access prevention: %w", err)
		}
	} else {
		return nil, fmt.Errorf("PublicAccessBlock must be true for security; public buckets are not supported")
	}

	endpoint := fmt.Sprintf("https://storage.googleapis.com/%s", bucketName)
	arn := fmt.Sprintf("gs://%s", bucketName)

	return &ObjectBucketInfo{
		BucketName: bucketName,
		Endpoint:   endpoint,
		Region:     region,
		ARN:        arn,
	}, nil
}

func (p *GCPStorageProvisioner) createGCSBucket(ctx context.Context, project, region, bucketName string) error {
	bkt := p.storageClient.Bucket(bucketName)
	if err := bkt.Create(ctx, project, &storage.BucketAttrs{
		Location:                 region,
		UniformBucketLevelAccess: storage.UniformBucketLevelAccess{Enabled: true},
	}); err != nil {
		// If bucket already exists, continue (idempotent on creation)
		if !isGCSNotFound(err) && !strings.Contains(err.Error(), "already exists") {
			return err
		}
	}
	return nil
}

func (p *GCPStorageProvisioner) putBucketVersioning(ctx context.Context, bucketName string) error {
	bkt := p.storageClient.Bucket(bucketName)
	_, err := bkt.Update(ctx, storage.BucketAttrsToUpdate{VersioningEnabled: true})
	return err
}

func (p *GCPStorageProvisioner) putBucketEncryption(ctx context.Context, bucketName, kmsKeyID string) error {
	bkt := p.storageClient.Bucket(bucketName)
	_, err := bkt.Update(ctx, storage.BucketAttrsToUpdate{
		Encryption: &storage.BucketEncryption{DefaultKMSKeyName: kmsKeyID},
	})
	return err
}

func (p *GCPStorageProvisioner) putPublicAccessPrevention(ctx context.Context, bucketName string) error {
	bkt := p.storageClient.Bucket(bucketName)
	_, err := bkt.Update(ctx, storage.BucketAttrsToUpdate{
		PublicAccessPrevention: storage.PublicAccessPreventionEnforced,
	})
	return err
}

func (p *GCPStorageProvisioner) DeprovisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, bucketName string) error {
	if err := p.initClients(ctx, cfg); err != nil {
		return err
	}

	bkt := p.storageClient.Bucket(bucketName)

	// GCS requires buckets to be empty before deletion
	// For now, we'll attempt to delete and treat not-found as idempotent
	if err := bkt.Delete(ctx); err != nil {
		// Treat bucket not found as success (idempotent)
		if isGCSNotFound(err) {
			return nil
		}
		return fmt.Errorf("delete GCS bucket: %w", err)
	}

	return nil
}

// isNotFound checks if a GCP Compute API error indicates the resource was not found.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	apiErr, ok := err.(*googleapi.Error)
	if ok && apiErr.Code == 404 {
		return true
	}
	return strings.Contains(err.Error(), "notFound")
}

// isGCSNotFound checks if a GCS storage error indicates the bucket was not found.
func isGCSNotFound(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, storage.ErrBucketNotExist) {
		return true
	}
	var apiErr *googleapi.Error
	if errors.As(err, &apiErr) && apiErr.Code == 404 {
		return true
	}
	return strings.Contains(err.Error(), "notFound") || strings.Contains(err.Error(), "Not Found")
}

// Ensure GCPStorageProvisioner implements StorageProvisioner.
var _ StorageProvisioner = (*GCPStorageProvisioner)(nil)
