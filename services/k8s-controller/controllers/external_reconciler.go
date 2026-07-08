package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
	kprovider "github.com/penguintechinc/nest/pkg/provider"
)

// reconcileExternal handles DataResources with origination: external.
// It provisions cloud-native storage resources (EBS, S3, Azure Disk, etc.)
// by delegating to the appropriate StorageProvisioner.
func (r *DataResourceReconciler) reconcileExternal(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	if dr.Spec.External == nil {
		return fmt.Errorf("external DataResource %s/%s missing spec.external", dr.Namespace, dr.Name)
	}

	logger.Info("reconcile external DataResource",
		"name", dr.Name,
		"namespace", dr.Namespace,
		"type", dr.Spec.Type,
		"provider", dr.Spec.External.Provider,
	)

	prov := providerForType(dr.Spec.Type, dr.Spec.External.Provider)
	if prov == nil {
		return fmt.Errorf("no provisioner available for type=%s provider=%s", dr.Spec.Type, dr.Spec.External.Provider)
	}

	// Resolve credential secret if referenced
	var credentialData map[string][]byte
	if dr.Spec.External.CredentialSecret != "" {
		secret := &corev1.Secret{}
		secretKey := client.ObjectKey{
			Name:      dr.Spec.External.CredentialSecret,
			Namespace: dr.Namespace,
		}
		if err := r.Get(ctx, secretKey, secret); err != nil {
			if errors.IsNotFound(err) {
				return fmt.Errorf("credential secret not found: %s/%s", dr.Namespace, dr.Spec.External.CredentialSecret)
			}
			return fmt.Errorf("failed to read credential secret: %w", err)
		}
		credentialData = secret.Data
	}

	cfg := kprovider.ExternalProviderConfig{
		Provider:         dr.Spec.External.Provider,
		Region:           dr.Spec.External.Region,
		ResourceID:       dr.Spec.External.ResourceID,
		CredentialSecret: dr.Spec.External.CredentialSecret,
		Endpoint:         dr.Spec.External.Endpoint,
		Extra:            dr.Spec.External.Extra,
		// TODO: Pass credential data to provider when provider config struct supports it
		// CredentialData:   credentialData,
	}
	_ = credentialData // Use to prevent "unused" lint warning; remove when provider config updated

	if nestv1.IsCloudStorageType(dr.Spec.Type) {
		return r.reconcileExternalStorage(ctx, dr, prov, cfg)
	}

	return fmt.Errorf("external reconciliation not yet implemented for type=%s", dr.Spec.Type)
}

func (r *DataResourceReconciler) reconcileExternalStorage(ctx context.Context, dr *nestv1.DataResource, prov kprovider.StorageProvisioner, cfg kprovider.ExternalProviderConfig) error {
	switch dr.Spec.Type {
	case nestv1.TypeEBS, nestv1.TypeAzureDisk, nestv1.TypeGCPDisk,
		nestv1.TypeDOVolume, nestv1.TypeVultrBlock, nestv1.TypeLinodeBlock:
		return r.reconcileExternalBlock(ctx, dr, prov, cfg)
	case nestv1.TypeS3, nestv1.TypeGCS, nestv1.TypeAzureBlob,
		nestv1.TypeDOSpaces, nestv1.TypeVultrObject, nestv1.TypeLinodeObject, nestv1.TypeS3Compat:
		return r.reconcileExternalBucket(ctx, dr, prov, cfg)
	default:
		return fmt.Errorf("unknown cloud storage type: %s", dr.Spec.Type)
	}
}

func (r *DataResourceReconciler) reconcileExternalBlock(ctx context.Context, dr *nestv1.DataResource, prov kprovider.StorageProvisioner, cfg kprovider.ExternalProviderConfig) error {
	logger := log.FromContext(ctx)

	// Check if already provisioned by looking at persisted VolumeID in status
	// Use idempotency token to prevent duplicate provisions on status patch failure
	idempotencyToken := resourceIdempotencyToken(dr)

	// Idempotency: if VolumeID is already in status, skip re-provision
	if dr.Status.VolumeID != "" {
		// Volume already provisioned; verify it still exists and update status if needed
		logger.Info("block volume already provisioned", "volumeID", dr.Status.VolumeID)
		// In a real implementation, would verify the volume state here
		return nil
	}

	spec := kprovider.BlockVolumeSpec{}
	if dr.Spec.External.BlockVolume != nil {
		bv := dr.Spec.External.BlockVolume
		spec.SizeGB = bv.SizeGB
		spec.IOPS = bv.IOPS
		spec.Throughput = bv.Throughput
		spec.VolumeType = bv.VolumeType
		spec.AvailabilityZone = bv.AvailabilityZone
		spec.EncryptionKeyID = bv.EncryptionKeyID
		spec.MultiAttach = bv.MultiAttach
	}

	// Idempotency token should be passed via Extra field or provider-specific config
	// (TODO: wire into provider when config struct is updated by provider team)
	_ = idempotencyToken

	info, err := prov.ProvisionBlockVolume(ctx, cfg, spec)
	if err != nil {
		return fmt.Errorf("provision block volume: %w", err)
	}

	// Update status with VolumeID persistence
	patch := client.MergeFrom(dr.DeepCopy())
	if dr.Status.Endpoints == nil {
		dr.Status.Endpoints = &nestv1.ResourceEndpoints{}
	}
	dr.Status.Endpoints.Native = info.Endpoint
	dr.Status.VolumeID = info.VolumeID // Persist the provider-returned VolumeID
	dr.Status.Phase = nestv1.PhasePending
	if info.State == "available" || info.State == "ready" {
		dr.Status.Phase = nestv1.PhaseReady
	}
	dr.Status.ObservedGeneration = dr.Generation

	if err := r.Client.Status().Patch(ctx, dr, patch); err != nil {
		// On patch failure, the VolumeID is not yet persisted, so retry will re-provision
		return fmt.Errorf("patch external block status: %w", err)
	}

	logger.Info("external block volume provisioned",
		"name", dr.Name,
		"endpoint", info.Endpoint,
		"volumeID", info.VolumeID,
	)
	return nil
}

func (r *DataResourceReconciler) reconcileExternalBucket(ctx context.Context, dr *nestv1.DataResource, prov kprovider.StorageProvisioner, cfg kprovider.ExternalProviderConfig) error {
	logger := log.FromContext(ctx)

	if dr.Status.Endpoints != nil && dr.Status.Endpoints.Native != "" {
		return nil
	}

	spec := kprovider.ObjectBucketSpec{}
	if dr.Spec.External.ObjectBucket != nil {
		ob := dr.Spec.External.ObjectBucket
		spec.BucketName = ob.BucketName
		spec.Versioning = ob.Versioning
		spec.EncryptionType = ob.EncryptionType
		spec.LifecycleDays = ob.LifecycleDays
		spec.PublicAccessBlock = ob.PublicAccessBlock
		spec.CrossRegionReplication = ob.CrossRegionReplication
		spec.ReplicationTargetRegion = ob.ReplicationTargetRegion
	}
	if spec.BucketName == "" {
		spec.BucketName = dr.Name
	}

	info, err := prov.ProvisionObjectBucket(ctx, cfg, spec)
	if err != nil {
		return fmt.Errorf("provision object bucket: %w", err)
	}

	patch := client.MergeFrom(dr.DeepCopy())
	if dr.Status.Endpoints == nil {
		dr.Status.Endpoints = &nestv1.ResourceEndpoints{}
	}
	dr.Status.Endpoints.Native = info.Endpoint
	dr.Status.Phase = nestv1.PhaseReady

	if err := r.Client.Status().Patch(ctx, dr, patch); err != nil {
		return fmt.Errorf("patch external bucket status: %w", err)
	}

	logger.Info("external object bucket provisioned",
		"name", dr.Name,
		"bucket", info.BucketName,
		"endpoint", info.Endpoint,
	)
	return nil
}

func (r *DataResourceReconciler) reconcileExternalDelete(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	if dr.Spec.External == nil {
		return nil
	}

	prov := providerForType(dr.Spec.Type, dr.Spec.External.Provider)
	if prov == nil {
		return nil
	}

	// Resolve credential secret if referenced
	if dr.Spec.External.CredentialSecret != "" {
		secret := &corev1.Secret{}
		secretKey := client.ObjectKey{
			Name:      dr.Spec.External.CredentialSecret,
			Namespace: dr.Namespace,
		}
		if err := r.Get(ctx, secretKey, secret); err != nil && !errors.IsNotFound(err) {
			// Log but don't fail if credential secret not found (resource may have been deleted)
			logger.Error(err, "failed to read credential secret for deletion", "name", dr.Spec.External.CredentialSecret)
		}
	}

	cfg := kprovider.ExternalProviderConfig{
		Provider:         dr.Spec.External.Provider,
		Region:           dr.Spec.External.Region,
		ResourceID:       dr.Spec.External.ResourceID,
		CredentialSecret: dr.Spec.External.CredentialSecret,
		Endpoint:         dr.Spec.External.Endpoint,
		Extra:            dr.Spec.External.Extra,
	}

	switch dr.Spec.Type {
	case nestv1.TypeEBS, nestv1.TypeAzureDisk, nestv1.TypeGCPDisk,
		nestv1.TypeDOVolume, nestv1.TypeVultrBlock, nestv1.TypeLinodeBlock:
		// Use persisted VolumeID from status (set during provisioning)
		// Fall back to ResourceID if status not set
		volumeID := dr.Status.VolumeID
		if volumeID == "" {
			volumeID = dr.Spec.External.ResourceID
		}
		if volumeID == "" {
			logger.Info("no volumeID found for deprovision; skipping", "name", dr.Name)
			return nil
		}
		if err := prov.DeprovisionBlockVolume(ctx, cfg, volumeID); err != nil {
			logger.Error(err, "failed to deprovision block volume", "volumeID", volumeID)
			return fmt.Errorf("deprovision block volume: %w", err)
		}
	case nestv1.TypeS3, nestv1.TypeGCS, nestv1.TypeAzureBlob,
		nestv1.TypeDOSpaces, nestv1.TypeVultrObject, nestv1.TypeLinodeObject, nestv1.TypeS3Compat:
		bucketName := dr.Spec.External.ResourceID
		if dr.Spec.External.ObjectBucket != nil && dr.Spec.External.ObjectBucket.BucketName != "" {
			bucketName = dr.Spec.External.ObjectBucket.BucketName
		}
		if bucketName == "" {
			bucketName = dr.Name
		}
		if err := prov.DeprovisionObjectBucket(ctx, cfg, bucketName); err != nil {
			logger.Error(err, "failed to deprovision object bucket", "bucketName", bucketName)
			return fmt.Errorf("deprovision object bucket: %w", err)
		}
	}
	return nil
}

// providerForType returns the appropriate StorageProvisioner for the given resource type and provider name.
func providerForType(resourceType, providerName string) kprovider.StorageProvisioner {
	switch providerName {
	case "aws":
		return kprovider.NewAWSStorageProvisioner()
	case "azure":
		return kprovider.NewAzureStorageProvisioner()
	case "gcp":
		return kprovider.NewGCPStorageProvisioner()
	case "digitalocean":
		return kprovider.NewDOStorageProvisioner()
	case "vultr":
		return kprovider.NewVultrStorageProvisioner()
	case "linode":
		return kprovider.NewLinodeStorageProvisioner()
	case "s3-compat":
		return kprovider.NewS3CompatProvisioner()
	}

	// Fallback: infer provider from type
	switch resourceType {
	case nestv1.TypeEBS, nestv1.TypeS3:
		return kprovider.NewAWSStorageProvisioner()
	case nestv1.TypeAzureDisk, nestv1.TypeAzureBlob:
		return kprovider.NewAzureStorageProvisioner()
	case nestv1.TypeGCPDisk, nestv1.TypeGCS:
		return kprovider.NewGCPStorageProvisioner()
	case nestv1.TypeDOSpaces, nestv1.TypeDOVolume:
		return kprovider.NewDOStorageProvisioner()
	case nestv1.TypeVultrObject, nestv1.TypeVultrBlock:
		return kprovider.NewVultrStorageProvisioner()
	case nestv1.TypeLinodeObject, nestv1.TypeLinodeBlock:
		return kprovider.NewLinodeStorageProvisioner()
	case nestv1.TypeS3Compat:
		return kprovider.NewS3CompatProvisioner()
	}
	return nil
}
