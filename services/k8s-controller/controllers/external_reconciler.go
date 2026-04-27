package controllers

import (
	"context"
	"fmt"

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

	cfg := kprovider.ExternalProviderConfig{
		Provider:         dr.Spec.External.Provider,
		Region:           dr.Spec.External.Region,
		ResourceID:       dr.Spec.External.ResourceID,
		CredentialSecret: dr.Spec.External.CredentialSecret,
		Extra:            dr.Spec.External.Extra,
	}

	if nestv1.IsCloudStorageType(dr.Spec.Type) {
		return r.reconcileExternalStorage(ctx, dr, prov, cfg)
	}

	return fmt.Errorf("external reconciliation not yet implemented for type=%s", dr.Spec.Type)
}

func (r *DataResourceReconciler) reconcileExternalStorage(ctx context.Context, dr *nestv1.DataResource, prov kprovider.StorageProvisioner, cfg kprovider.ExternalProviderConfig) error {
	switch dr.Spec.Type {
	case nestv1.TypeEBS, nestv1.TypeAzureDisk, nestv1.TypeGCPDisk:
		return r.reconcileExternalBlock(ctx, dr, prov, cfg)
	case nestv1.TypeS3, nestv1.TypeGCS, nestv1.TypeAzureBlob:
		return r.reconcileExternalBucket(ctx, dr, prov, cfg)
	default:
		return fmt.Errorf("unknown cloud storage type: %s", dr.Spec.Type)
	}
}

func (r *DataResourceReconciler) reconcileExternalBlock(ctx context.Context, dr *nestv1.DataResource, prov kprovider.StorageProvisioner, cfg kprovider.ExternalProviderConfig) error {
	logger := log.FromContext(ctx)

	// If status already has an endpoint, volume is already provisioned
	if dr.Status.Endpoints != nil && dr.Status.Endpoints.Native != "" {
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

	info, err := prov.ProvisionBlockVolume(ctx, cfg, spec)
	if err != nil {
		return fmt.Errorf("provision block volume: %w", err)
	}

	// Update status
	patch := client.MergeFrom(dr.DeepCopy())
	if dr.Status.Endpoints == nil {
		dr.Status.Endpoints = &nestv1.ResourceEndpoints{}
	}
	dr.Status.Endpoints.Native = info.Endpoint
	dr.Status.Phase = nestv1.PhasePending
	if info.State == "available" || info.State == "ready" {
		dr.Status.Phase = nestv1.PhaseReady
	}

	if err := r.Client.Status().Patch(ctx, dr, patch); err != nil {
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

	cfg := kprovider.ExternalProviderConfig{
		Provider:         dr.Spec.External.Provider,
		Region:           dr.Spec.External.Region,
		ResourceID:       dr.Spec.External.ResourceID,
		CredentialSecret: dr.Spec.External.CredentialSecret,
		Extra:            dr.Spec.External.Extra,
	}

	volumeID := dr.Spec.External.ResourceID
	if dr.Status.Endpoints != nil && dr.Status.Endpoints.Native != "" {
		// Extract volumeID from endpoint or use ResourceID
		volumeID = dr.Spec.External.ResourceID
	}

	switch dr.Spec.Type {
	case nestv1.TypeEBS, nestv1.TypeAzureDisk, nestv1.TypeGCPDisk:
		if volumeID == "" {
			return nil
		}
		if err := prov.DeprovisionBlockVolume(ctx, cfg, volumeID); err != nil {
			logger.Error(err, "failed to deprovision block volume", "volumeID", volumeID)
			return fmt.Errorf("deprovision block volume: %w", err)
		}
	case nestv1.TypeS3, nestv1.TypeGCS, nestv1.TypeAzureBlob:
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
	}

	// Fallback: infer provider from type
	switch resourceType {
	case nestv1.TypeEBS, nestv1.TypeS3:
		return kprovider.NewAWSStorageProvisioner()
	case nestv1.TypeAzureDisk, nestv1.TypeAzureBlob:
		return kprovider.NewAzureStorageProvisioner()
	case nestv1.TypeGCPDisk, nestv1.TypeGCS:
		return kprovider.NewGCPStorageProvisioner()
	}
	return nil
}
