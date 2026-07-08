package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// LinodeStorageProvisioner implements StorageProvisioner for Linode (Akamai Cloud).
// Object Storage uses S3-compatible XML protocol against https://<region>.linodeobjects.com.
// Block Volumes use the Linode Volumes REST API.
//
// Credential secret keys:
//   - access_key / secret_key: for Object Storage
//   - linode_token: for Volumes API (Bearer token)
//
// Note: Linode requires volumes to be detached before deletion.
// DeprovisionBlockVolume returns a clear error if the volume is still attached.
type LinodeStorageProvisioner struct {
	httpClient     *http.Client
	volumesAPIBase string
}

var _ StorageProvisioner = (*LinodeStorageProvisioner)(nil)

func NewLinodeStorageProvisioner() *LinodeStorageProvisioner {
	return &LinodeStorageProvisioner{
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		volumesAPIBase: "https://api.linode.com",
	}
}

func newLinodeStorageProvisionerWithBase(volumesAPIBase string) *LinodeStorageProvisioner {
	return &LinodeStorageProvisioner{
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		volumesAPIBase: volumesAPIBase,
	}
}

func (p *LinodeStorageProvisioner) ProvisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, spec ObjectBucketSpec) (*ObjectBucketInfo, error) {
	bucketName := spec.BucketName
	if bucketName == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	base := p.objectEndpoint(cfg)
	url := fmt.Sprintf("%s/%s", base, bucketName)

	var body io.Reader
	if cfg.Region != "" {
		xml := fmt.Sprintf(`<CreateBucketConfiguration><LocationConstraint>%s</LocationConstraint></CreateBucketConfiguration>`, cfg.Region)
		body = strings.NewReader(xml)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, body)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/xml")
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create Linode object bucket: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Linode create bucket returned %d: %s", resp.StatusCode, b)
	}

	return &ObjectBucketInfo{
		BucketName: bucketName,
		Endpoint:   fmt.Sprintf("s3://%s.%s.linodeobjects.com", bucketName, cfg.Region),
		Region:     cfg.Region,
	}, nil
}

func (p *LinodeStorageProvisioner) DeprovisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, bucketName string) error {
	base := p.objectEndpoint(cfg)
	url := fmt.Sprintf("%s/%s", base, bucketName)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete Linode object bucket: %w", err)
	}
	defer resp.Body.Close()

	// 404 Not Found is treated as success (idempotent delete)
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode == http.StatusConflict {
		b, _ := io.ReadAll(resp.Body)
		if strings.Contains(string(b), "NotEmpty") {
			return fmt.Errorf("bucket %s is not empty; drain it before deleting", bucketName)
		}
		return fmt.Errorf("delete bucket returned %d: %s", resp.StatusCode, b)
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete bucket returned %d: %s", resp.StatusCode, b)
	}
	return nil
}

func (p *LinodeStorageProvisioner) ProvisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, spec BlockVolumeSpec) (*BlockVolumeInfo, error) {
	token, err := p.token(cfg)
	if err != nil {
		return nil, err
	}

	region := cfg.Region
	if region == "" {
		return nil, fmt.Errorf("region is required for Linode volume provisioning")
	}

	sizeGB := spec.SizeGB
	if sizeGB < 20 {
		sizeGB = 20
	}

	label := cfg.ResourceID
	if label == "" {
		label = "nest-volume"
	}

	payload := map[string]interface{}{
		"region": region,
		"size":   sizeGB,
		"label":  label,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v4/volumes", p.volumesAPIBase)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Linode Volumes API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		rb, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Linode Volumes create returned %d: %s", resp.StatusCode, rb)
	}

	var result struct {
		ID     int    `json:"id"`
		Status string `json:"status"`
		Size   int64  `json:"size"`
		Region string `json:"region"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Linode Volumes response: %w", err)
	}

	id := strconv.Itoa(result.ID)
	return &BlockVolumeInfo{
		VolumeID:         id,
		State:            result.Status,
		SizeGB:           result.Size,
		AvailabilityZone: result.Region,
		Endpoint:         fmt.Sprintf("linode-volume://%s", id),
	}, nil
}

func (p *LinodeStorageProvisioner) DeprovisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, volumeID string) error {
	token, err := p.token(cfg)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/v4/volumes/%s", p.volumesAPIBase, volumeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Linode Volumes delete: %w", err)
	}
	defer resp.Body.Close()

	// 404 Not Found is treated as success (idempotent delete)
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode == http.StatusUnprocessableEntity {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("volume %s must be detached before deleting: %s", volumeID, b)
	}
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Linode Volumes delete returned %d: %s", resp.StatusCode, b)
	}
	return nil
}

func (p *LinodeStorageProvisioner) GetBlockVolumeStatus(ctx context.Context, cfg ExternalProviderConfig, volumeID string) (*BlockVolumeInfo, error) {
	token, err := p.token(cfg)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v4/volumes/%s", p.volumesAPIBase, volumeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Linode Volumes get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Linode Volumes get returned %d: %s", resp.StatusCode, b)
	}

	var result struct {
		ID     int    `json:"id"`
		Status string `json:"status"`
		Size   int64  `json:"size"`
		Region string `json:"region"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Linode Volumes status: %w", err)
	}

	id := strconv.Itoa(result.ID)
	return &BlockVolumeInfo{
		VolumeID:         id,
		State:            result.Status,
		SizeGB:           result.Size,
		AvailabilityZone: result.Region,
		Endpoint:         fmt.Sprintf("linode-volume://%s", id),
	}, nil
}

func (p *LinodeStorageProvisioner) objectEndpoint(cfg ExternalProviderConfig) string {
	if cfg.Endpoint != "" {
		return strings.TrimRight(cfg.Endpoint, "/")
	}
	if cfg.Region != "" {
		return fmt.Sprintf("https://%s.linodeobjects.com", cfg.Region)
	}
	return "https://us-east-1.linodeobjects.com"
}

func (p *LinodeStorageProvisioner) token(cfg ExternalProviderConfig) (string, error) {
	if cfg.Extra != nil {
		if t := cfg.Extra["linode_token"]; t != "" {
			return t, nil
		}
	}
	return "", fmt.Errorf("linode_token credential missing: required for Linode volumes API; provide in ExternalProviderConfig.Extra[\"linode_token\"]")
}
