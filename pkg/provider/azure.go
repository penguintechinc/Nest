package provider

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Azure/azure-sdk-for-go/sdk/azcore"
	"github.com/Azure/azure-sdk-for-go/sdk/azcore/to"
	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/compute/armcompute"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/storage/armstorage"
	"github.com/Azure/azure-sdk-for-go/sdk/storage/azblob"
	"github.com/google/uuid"
)

// AzureStorageProvisioner implements StorageProvisioner for Azure (Managed Disk + Blob Storage).
// It uses the official Azure SDK for Go for Compute (Managed Disk) and Blob Storage operations.
// Credentials come from cfg.Extra["client_id"], "client_secret", "tenant_id" (AAD OAuth2 client credentials)
// with fallback to Azure Default Credentials (environment variables, managed identity, etc.).
type AzureStorageProvisioner struct {
	disksClient        *armcompute.DisksClient
	blobClient         *azblob.Client
	blobServicesClient *armstorage.BlobServicesClient
	credential         azcore.TokenCredential
	subscriptionID     string
}

func NewAzureStorageProvisioner() *AzureStorageProvisioner {
	return &AzureStorageProvisioner{}
}

// initClients initializes Compute (Disks) and Blob Storage clients.
// If clients are already set (e.g., by tests injecting mocks), it skips re-initialization.
func (p *AzureStorageProvisioner) initClients(ctx context.Context, cfg ExternalProviderConfig) error {
	if p.disksClient != nil && p.blobClient != nil && p.credential != nil {
		return nil // already initialized (tests may pre-inject via fields)
	}

	subscriptionID, ok := cfg.Extra["subscription_id"]
	if !ok || subscriptionID == "" {
		return fmt.Errorf("azure subscription_id is required")
	}

	storageAccountName, ok := cfg.Extra["storage_account_name"]
	if !ok || storageAccountName == "" {
		return fmt.Errorf("azure storage_account_name is required for blob operations")
	}

	// Credentials: try client_id/client_secret/tenant_id first, then fall back to Azure default credentials
	var cred azcore.TokenCredential
	var err error

	if clientID, ok := cfg.Credential("client_id"); ok {
		clientSecret, ok := cfg.Credential("client_secret")
		if !ok {
			return fmt.Errorf("client_secret required when client_id is provided")
		}
		tenantID, ok := cfg.Credential("tenant_id")
		if !ok {
			return fmt.Errorf("tenant_id required when client_id is provided")
		}
		cred, err = azidentity.NewClientSecretCredential(tenantID, clientID, clientSecret, nil)
		if err != nil {
			return fmt.Errorf("create Azure client secret credential: %w", err)
		}
	} else {
		// Fall back to Azure default credentials (environment variables, managed identity, etc.)
		cred, err = azidentity.NewDefaultAzureCredential(nil)
		if err != nil {
			return fmt.Errorf("create Azure default credential: %w", err)
		}
	}

	p.credential = cred
	p.subscriptionID = subscriptionID

	// Create Compute (Disks) client
	disksClient, err := armcompute.NewDisksClient(subscriptionID, cred, nil)
	if err != nil {
		return fmt.Errorf("create Azure disks client: %w", err)
	}
	p.disksClient = disksClient

	// Create Blob Storage client
	blobURL := fmt.Sprintf("https://%s.blob.core.windows.net/", storageAccountName)
	blobClient, err := azblob.NewClient(blobURL, cred, nil)
	if err != nil {
		return fmt.Errorf("create Azure blob client: %w", err)
	}
	p.blobClient = blobClient

	return nil
}

func (p *AzureStorageProvisioner) ProvisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, spec BlockVolumeSpec) (*BlockVolumeInfo, error) {
	resourceGroup, ok := cfg.Extra["resource_group"]
	if !ok || resourceGroup == "" {
		return nil, fmt.Errorf("azure resource_group is required for Managed Disk provisioning")
	}

	if err := p.initClients(ctx, cfg); err != nil {
		return nil, err
	}

	region := cfg.Region
	if region == "" {
		region = "eastus"
	}

	sizeGB := spec.SizeGB
	if sizeGB == 0 {
		sizeGB = 30
	}

	volumeType := spec.VolumeType
	if volumeType == "" {
		volumeType = "Premium_LRS"
	}

	// Use idempotency token as disk name, or generate one
	diskName := cfg.IdempotencyToken
	if diskName == "" {
		diskName = fmt.Sprintf("disk-%s", uuid.New().String()[:12])
	}

	diskInfo, err := p.createManagedDisk(ctx, resourceGroup, region, diskName, volumeType, sizeGB, spec)
	if err != nil {
		return nil, fmt.Errorf("create managed disk: %w", err)
	}

	return diskInfo, nil
}

func (p *AzureStorageProvisioner) createManagedDisk(ctx context.Context, resourceGroup, region, diskName, diskType string, sizeGB int64, spec BlockVolumeSpec) (*BlockVolumeInfo, error) {
	// Convert sizeGB to int32 for Azure API
	sizeInt32 := int32(sizeGB)

	opt := armcompute.DiskCreateOptionEmpty
	creationData := armcompute.CreationData{
		CreateOption: &opt,
	}

	diskProperties := armcompute.DiskProperties{
		DiskSizeGB:   &sizeInt32,
		CreationData: &creationData,
	}

	// Apply encryption (platform-managed by default)
	encType := armcompute.EncryptionTypeEncryptionAtRestWithPlatformKey
	diskProperties.Encryption = &armcompute.Encryption{
		Type: &encType,
	}

	disk := armcompute.Disk{
		Location:   &region,
		Properties: &diskProperties,
	}

	// Map volume type to Azure SKU
	skuName := armcompute.DiskStorageAccountTypes(diskType)
	disk.SKU = &armcompute.DiskSKU{
		Name: &skuName,
	}

	// Create the disk
	poller, err := p.disksClient.BeginCreateOrUpdate(ctx, resourceGroup, diskName, disk, nil)
	if err != nil {
		return nil, fmt.Errorf("disks.BeginCreateOrUpdate: %w", err)
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("wait for disk creation: %w", err)
	}

	return &BlockVolumeInfo{
		VolumeID:         diskName,
		State:            "creating",
		SizeGB:           sizeGB,
		AvailabilityZone: region,
		Endpoint:         fmt.Sprintf("azure://%s/disks/%s", resourceGroup, diskName),
	}, nil
}

func (p *AzureStorageProvisioner) DeprovisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, volumeID string) error {
	resourceGroup, ok := cfg.Extra["resource_group"]
	if !ok || resourceGroup == "" {
		return fmt.Errorf("azure resource_group is required")
	}

	if err := p.initClients(ctx, cfg); err != nil {
		return err
	}

	poller, err := p.disksClient.BeginDelete(ctx, resourceGroup, volumeID, nil)
	if err != nil {
		// Check if it's a 404 (not found) — treat as idempotent success
		if isAzureNotFound(err) {
			return nil
		}
		return fmt.Errorf("disks.BeginDelete: %w", err)
	}

	_, err = poller.PollUntilDone(ctx, nil)
	if err != nil {
		// If the error is "not found", treat as idempotent success
		if isAzureNotFound(err) {
			return nil
		}
		return fmt.Errorf("wait for disk deletion: %w", err)
	}

	return nil
}

func (p *AzureStorageProvisioner) GetBlockVolumeStatus(ctx context.Context, cfg ExternalProviderConfig, volumeID string) (*BlockVolumeInfo, error) {
	resourceGroup, ok := cfg.Extra["resource_group"]
	if !ok || resourceGroup == "" {
		return nil, fmt.Errorf("azure resource_group is required")
	}

	if err := p.initClients(ctx, cfg); err != nil {
		return nil, err
	}

	region := cfg.Region
	if region == "" {
		region = "eastus"
	}

	resp, err := p.disksClient.Get(ctx, resourceGroup, volumeID, nil)
	if err != nil {
		if isAzureNotFound(err) {
			return nil, fmt.Errorf("disk %s not found", volumeID)
		}
		return nil, fmt.Errorf("disks.Get: %w", err)
	}

	disk := resp.Disk
	sizeGB := int64(0)
	if disk.Properties != nil && disk.Properties.DiskSizeGB != nil {
		sizeGB = int64(*disk.Properties.DiskSizeGB)
	}

	return &BlockVolumeInfo{
		VolumeID:         volumeID,
		State:            "Ready",
		SizeGB:           sizeGB,
		AvailabilityZone: region,
		Endpoint:         fmt.Sprintf("azure://%s/disks/%s", resourceGroup, volumeID),
	}, nil
}

func (p *AzureStorageProvisioner) ProvisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, spec ObjectBucketSpec) (*ObjectBucketInfo, error) {
	bucketName := spec.BucketName
	if bucketName == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	// Validate encryption type before making API calls
	encType := spec.EncryptionType
	if encType != "" && encType != "microsoft-managed" && encType != "customer-managed" {
		return nil, fmt.Errorf("unsupported encryption type: %s", encType)
	}

	if encType == "customer-managed" {
		kmsKeyID, ok := cfg.Extra["kms_key_id"]
		if !ok || kmsKeyID == "" {
			return nil, fmt.Errorf("customer-managed encryption requested but kms_key_id not provided in config")
		}
	}

	// Validate PublicAccessBlock must be true for security
	if !spec.PublicAccessBlock {
		return nil, fmt.Errorf("PublicAccessBlock must be true for security; public buckets are not supported")
	}

	if err := p.initClients(ctx, cfg); err != nil {
		return nil, err
	}

	region := cfg.Region
	if region == "" {
		region = "eastus"
	}

	storageAccountName, ok := cfg.Extra["storage_account_name"]
	if !ok {
		storageAccountName = "unknown"
	}

	resourceGroup, ok := cfg.Extra["resource_group"]
	if !ok || resourceGroup == "" {
		return nil, fmt.Errorf("azure resource_group is required for blob bucket provisioning")
	}

	// Create container
	if err := p.createBlobContainer(ctx, bucketName); err != nil {
		return nil, fmt.Errorf("create blob container: %w", err)
	}

	// Apply versioning if requested
	if spec.Versioning {
		if err := p.setBlobContainerProperties(ctx, resourceGroup, storageAccountName, true); err != nil {
			return nil, fmt.Errorf("enable versioning: %w", err)
		}
	}

	// Apply public access block (in Azure, this is done via anonymous access settings)
	if err := p.setBlobContainerAccessLevel(ctx, bucketName); err != nil {
		return nil, fmt.Errorf("apply public access block: %w", err)
	}

	endpoint := fmt.Sprintf("https://%s.blob.core.windows.net/%s", storageAccountName, bucketName)
	arn := fmt.Sprintf("azure://%s/%s", storageAccountName, bucketName)

	return &ObjectBucketInfo{
		BucketName: bucketName,
		Endpoint:   endpoint,
		Region:     region,
		ARN:        arn,
	}, nil
}

func (p *AzureStorageProvisioner) createBlobContainer(ctx context.Context, containerName string) error {
	_, err := p.blobClient.CreateContainer(ctx, containerName, nil)
	if err != nil {
		// If container already exists, continue (idempotent on creation)
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == 409 {
			return nil
		}
		if strings.Contains(err.Error(), "ContainerAlreadyExists") {
			return nil
		}
		return err
	}
	return nil
}

// setBlobContainerProperties enables blob versioning on the storage account.
// Azure Blob versioning is an account-level setting (not per-container), configured
// via the management-plane Storage Resource Provider, not the azblob data-plane package.
// The blob services client is constructed lazily (only when versioning is actually
// requested) so provisioners that never touch versioning don't pay its init cost.
func (p *AzureStorageProvisioner) setBlobContainerProperties(ctx context.Context, resourceGroup, storageAccountName string, versioning bool) error {
	if p.blobServicesClient == nil {
		client, err := armstorage.NewBlobServicesClient(p.subscriptionID, p.credential, nil)
		if err != nil {
			return fmt.Errorf("create Azure blob services client: %w", err)
		}
		p.blobServicesClient = client
	}

	_, err := p.blobServicesClient.SetServiceProperties(ctx, resourceGroup, storageAccountName, armstorage.BlobServiceProperties{
		BlobServiceProperties: &armstorage.BlobServicePropertiesProperties{
			IsVersioningEnabled: to.Ptr(versioning),
		},
	}, nil)
	return err
}

// setBlobContainerAccessLevel sets the container to private (no public access).
// Passing nil options (an unset Access field) is documented by the SDK as: "If this
// header is not included in the request, container data is private to the account
// owner" — which is exactly the PublicAccessBlock security posture required here.
func (p *AzureStorageProvisioner) setBlobContainerAccessLevel(ctx context.Context, containerName string) error {
	containerClient := p.blobClient.ServiceClient().NewContainerClient(containerName)
	_, err := containerClient.SetAccessPolicy(ctx, nil)
	return err
}

func (p *AzureStorageProvisioner) DeprovisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, bucketName string) error {
	if err := p.initClients(ctx, cfg); err != nil {
		return err
	}

	_, err := p.blobClient.DeleteContainer(ctx, bucketName, nil)
	if err != nil {
		// Treat container not found as success (idempotent)
		var respErr *azcore.ResponseError
		if errors.As(err, &respErr) && respErr.StatusCode == 404 {
			return nil
		}
		if strings.Contains(err.Error(), "ContainerNotFound") {
			return nil
		}
		return fmt.Errorf("delete blob container: %w", err)
	}

	return nil
}

// isAzureNotFound checks if an Azure error indicates the resource was not found.
func isAzureNotFound(err error) bool {
	if err == nil {
		return false
	}
	var respErr *azcore.ResponseError
	if errors.As(err, &respErr) && respErr.StatusCode == 404 {
		return true
	}
	return strings.Contains(err.Error(), "NotFound") || strings.Contains(err.Error(), "404")
}

// Ensure AzureStorageProvisioner implements StorageProvisioner.
var _ StorageProvisioner = (*AzureStorageProvisioner)(nil)
