package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

)

// AzureStorageProvisioner implements StorageProvisioner for Azure (Managed Disk + Blob).
type AzureStorageProvisioner struct {
	httpClient *http.Client
}

func NewAzureStorageProvisioner() *AzureStorageProvisioner {
	return &AzureStorageProvisioner{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *AzureStorageProvisioner) ProvisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, spec BlockVolumeSpec) (*BlockVolumeInfo, error) {
	subscriptionID := cfg.Extra["subscriptionId"]
	resourceGroup := cfg.Extra["resourceGroup"]
	location := cfg.Region
	if subscriptionID == "" || resourceGroup == "" || location == "" {
		return nil, fmt.Errorf("azure requires subscriptionId, resourceGroup, and region")
	}

	diskName := cfg.ResourceID
	if diskName == "" {
		return nil, fmt.Errorf("resourceId (disk name) is required for Azure Managed Disk provisioning")
	}

	sizeGB := spec.SizeGB
	if sizeGB == 0 {
		sizeGB = 32
	}

	endpoint := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/disks/%s?api-version=2023-10-02",
		subscriptionID, resourceGroup, diskName,
	)
	body := fmt.Sprintf(`{"location":"%s","sku":{"name":"Premium_LRS"},"properties":{"diskSizeGB":%d,"creationData":{"createOption":"Empty"}}}`, location, sizeGB)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// Note: production requires Bearer token from Azure AD

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Azure Disk Create API call failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("Azure Disk Create returned %d: %s", resp.StatusCode, respBody)
	}

	return &BlockVolumeInfo{
		VolumeID:         diskName,
		State:            "creating",
		SizeGB:           sizeGB,
		AvailabilityZone: location,
		Endpoint:         fmt.Sprintf("azure-disk://%s/%s/%s", subscriptionID, resourceGroup, diskName),
	}, nil
}

func (p *AzureStorageProvisioner) DeprovisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, volumeID string) error {
	subscriptionID := cfg.Extra["subscriptionId"]
	resourceGroup := cfg.Extra["resourceGroup"]
	if subscriptionID == "" || resourceGroup == "" {
		return fmt.Errorf("azure requires subscriptionId and resourceGroup")
	}

	endpoint := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/disks/%s?api-version=2023-10-02",
		subscriptionID, resourceGroup, volumeID,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Azure Disk Delete API call failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Azure Disk Delete returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

func (p *AzureStorageProvisioner) GetBlockVolumeStatus(ctx context.Context, cfg ExternalProviderConfig, volumeID string) (*BlockVolumeInfo, error) {
	subscriptionID := cfg.Extra["subscriptionId"]
	resourceGroup := cfg.Extra["resourceGroup"]
	if subscriptionID == "" || resourceGroup == "" {
		return nil, fmt.Errorf("azure requires subscriptionId and resourceGroup")
	}

	endpoint := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Compute/disks/%s?api-version=2023-10-02",
		subscriptionID, resourceGroup, volumeID,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Azure Disk Get API call failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("Azure Disk Get returned %d", resp.StatusCode)
	}

	return &BlockVolumeInfo{
		VolumeID: volumeID,
		State:    "available",
		Endpoint: fmt.Sprintf("azure-disk://%s/%s/%s", subscriptionID, resourceGroup, volumeID),
	}, nil
}

func (p *AzureStorageProvisioner) ProvisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, spec ObjectBucketSpec) (*ObjectBucketInfo, error) {
	subscriptionID := cfg.Extra["subscriptionId"]
	resourceGroup := cfg.Extra["resourceGroup"]
	storageAccountName := cfg.Extra["storageAccountName"]
	if subscriptionID == "" || resourceGroup == "" || storageAccountName == "" {
		return nil, fmt.Errorf("azure requires subscriptionId, resourceGroup, and storageAccountName")
	}

	containerName := spec.BucketName
	if containerName == "" {
		return nil, fmt.Errorf("bucket name (container name) is required")
	}

	endpoint := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s/blobServices/default/containers/%s?api-version=2023-05-01",
		subscriptionID, resourceGroup, storageAccountName, containerName,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, strings.NewReader(`{"properties":{"publicAccess":"None"}}`))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Azure Blob Create API call failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return nil, fmt.Errorf("Azure Blob Create returned %d: %s", resp.StatusCode, respBody)
	}

	return &ObjectBucketInfo{
		BucketName: containerName,
		Endpoint:   fmt.Sprintf("https://%s.blob.core.windows.net/%s", storageAccountName, containerName),
		Region:     cfg.Region,
	}, nil
}

func (p *AzureStorageProvisioner) DeprovisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, bucketName string) error {
	subscriptionID := cfg.Extra["subscriptionId"]
	resourceGroup := cfg.Extra["resourceGroup"]
	storageAccountName := cfg.Extra["storageAccountName"]
	if subscriptionID == "" || resourceGroup == "" || storageAccountName == "" {
		return fmt.Errorf("azure requires subscriptionId, resourceGroup, and storageAccountName")
	}

	endpoint := fmt.Sprintf(
		"https://management.azure.com/subscriptions/%s/resourceGroups/%s/providers/Microsoft.Storage/storageAccounts/%s/blobServices/default/containers/%s?api-version=2023-05-01",
		subscriptionID, resourceGroup, storageAccountName, bucketName,
	)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Azure Blob Delete API call failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("Azure Blob Delete returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

// Ensure AzureStorageProvisioner implements StorageProvisioner.
var _ StorageProvisioner = (*AzureStorageProvisioner)(nil)
