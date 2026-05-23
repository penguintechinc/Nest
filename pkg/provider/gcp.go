package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

)

// GCPStorageProvisioner implements StorageProvisioner for GCP (Persistent Disk + GCS).
type GCPStorageProvisioner struct {
	httpClient *http.Client
}

func NewGCPStorageProvisioner() *GCPStorageProvisioner {
	return &GCPStorageProvisioner{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *GCPStorageProvisioner) ProvisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, spec BlockVolumeSpec) (*BlockVolumeInfo, error) {
	projectID := cfg.Extra["projectId"]
	zone := spec.AvailabilityZone
	if projectID == "" || zone == "" {
		return nil, fmt.Errorf("GCP requires projectId and availabilityZone (zone)")
	}

	diskName := cfg.ResourceID
	if diskName == "" {
		return nil, fmt.Errorf("resourceId (disk name) is required for GCP PD provisioning")
	}

	sizeGB := spec.SizeGB
	if sizeGB == 0 {
		sizeGB = 20
	}
	diskType := spec.VolumeType
	if diskType == "" {
		diskType = "pd-ssd"
	}

	endpoint := fmt.Sprintf("https://compute.googleapis.com/compute/v1/projects/%s/zones/%s/disks", projectID, zone)
	body := fmt.Sprintf(`{"name":"%s","sizeGb":"%d","type":"zones/%s/diskTypes/%s"}`, diskName, sizeGB, zone, diskType)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	// Note: production requires Bearer token from GCP service account

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GCP Disk Insert API call failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GCP Disk Insert returned %d: %s", resp.StatusCode, respBody)
	}

	return &BlockVolumeInfo{
		VolumeID:         diskName,
		State:            "creating",
		SizeGB:           sizeGB,
		AvailabilityZone: zone,
		Endpoint:         fmt.Sprintf("gcp-disk://%s/%s/%s", projectID, zone, diskName),
	}, nil
}

func (p *GCPStorageProvisioner) DeprovisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, volumeID string) error {
	projectID := cfg.Extra["projectId"]
	zone := cfg.Extra["zone"]
	if projectID == "" || zone == "" {
		return fmt.Errorf("GCP requires projectId and zone")
	}

	endpoint := fmt.Sprintf("https://compute.googleapis.com/compute/v1/projects/%s/zones/%s/disks/%s", projectID, zone, volumeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GCP Disk Delete API call failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("GCP Disk Delete returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

func (p *GCPStorageProvisioner) GetBlockVolumeStatus(ctx context.Context, cfg ExternalProviderConfig, volumeID string) (*BlockVolumeInfo, error) {
	projectID := cfg.Extra["projectId"]
	zone := cfg.Extra["zone"]
	if projectID == "" || zone == "" {
		return nil, fmt.Errorf("GCP requires projectId and zone")
	}

	endpoint := fmt.Sprintf("https://compute.googleapis.com/compute/v1/projects/%s/zones/%s/disks/%s", projectID, zone, volumeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, endpoint, nil)
	if err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GCP Disk Get API call failed: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GCP Disk Get returned %d", resp.StatusCode)
	}

	return &BlockVolumeInfo{
		VolumeID: volumeID,
		State:    "ready",
		Endpoint: fmt.Sprintf("gcp-disk://%s/%s/%s", projectID, zone, volumeID),
	}, nil
}

func (p *GCPStorageProvisioner) ProvisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, spec ObjectBucketSpec) (*ObjectBucketInfo, error) {
	projectID := cfg.Extra["projectId"]
	if projectID == "" {
		return nil, fmt.Errorf("GCP requires projectId")
	}

	bucketName := spec.BucketName
	if bucketName == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	location := cfg.Region
	if location == "" {
		location = "US"
	}

	endpoint := fmt.Sprintf("https://storage.googleapis.com/storage/v1/b?project=%s", projectID)
	body := fmt.Sprintf(`{"name":"%s","location":"%s","storageClass":"STANDARD"}`, bucketName, location)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GCS Insert Bucket API call failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GCS Insert Bucket returned %d: %s", resp.StatusCode, respBody)
	}

	return &ObjectBucketInfo{
		BucketName: bucketName,
		Endpoint:   fmt.Sprintf("https://storage.googleapis.com/%s", bucketName),
		Region:     location,
	}, nil
}

func (p *GCPStorageProvisioner) DeprovisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, bucketName string) error {
	endpoint := fmt.Sprintf("https://storage.googleapis.com/storage/v1/b/%s", bucketName)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("GCS Delete Bucket API call failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("GCS Delete Bucket returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

// Ensure GCPStorageProvisioner implements StorageProvisioner.
var _ StorageProvisioner = (*GCPStorageProvisioner)(nil)
