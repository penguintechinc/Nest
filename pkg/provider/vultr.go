package provider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// VultrStorageProvisioner implements StorageProvisioner for Vultr.
// Object Storage uses S3-compatible XML protocol against https://<region>.vultrobjects.com.
// Block Storage uses the Vultr Block Storage REST API.
//
// Credential secret keys:
//   - access_key / secret_key: for Object Storage
//   - vultr_api_key: for Block Storage API (Bearer token)
type VultrStorageProvisioner struct {
	httpClient    *http.Client
	blocksAPIBase string
}

func NewVultrStorageProvisioner() *VultrStorageProvisioner {
	return &VultrStorageProvisioner{
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		blocksAPIBase: "https://api.vultr.com",
	}
}

func newVultrStorageProvisionerWithBase(blocksAPIBase string) *VultrStorageProvisioner {
	return &VultrStorageProvisioner{
		httpClient:    &http.Client{Timeout: 30 * time.Second},
		blocksAPIBase: blocksAPIBase,
	}
}

func (p *VultrStorageProvisioner) ProvisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, spec ObjectBucketSpec) (*ObjectBucketInfo, error) {
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
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/xml")
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create Vultr object bucket: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Vultr create bucket returned %d: %s", resp.StatusCode, b)
	}

	return &ObjectBucketInfo{
		BucketName: bucketName,
		Endpoint:   fmt.Sprintf("s3://%s.%s.vultrobjects.com", bucketName, cfg.Region),
		Region:     cfg.Region,
	}, nil
}

func (p *VultrStorageProvisioner) DeprovisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, bucketName string) error {
	base := p.objectEndpoint(cfg)
	url := fmt.Sprintf("%s/%s", base, bucketName)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete Vultr object bucket: %w", err)
	}
	defer resp.Body.Close()

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

func (p *VultrStorageProvisioner) ProvisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, spec BlockVolumeSpec) (*BlockVolumeInfo, error) {
	region := cfg.Region
	if region == "" {
		return nil, fmt.Errorf("region is required for Vultr block storage")
	}

	sizeGB := spec.SizeGB
	if sizeGB < 10 {
		sizeGB = 10
	}

	payload := map[string]interface{}{
		"region":  region,
		"size_gb": sizeGB,
		"label":   cfg.ResourceID,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v2/blocks", p.blocksAPIBase)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.apiKey(cfg))

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Vultr blocks API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		rb, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Vultr blocks create returned %d: %s", resp.StatusCode, rb)
	}

	var result struct {
		Block struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			SizeGB int64  `json:"size_gb"`
			Region string `json:"region"`
		} `json:"block"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Vultr blocks response: %w", err)
	}

	return &BlockVolumeInfo{
		VolumeID:         result.Block.ID,
		State:            result.Block.Status,
		SizeGB:           result.Block.SizeGB,
		AvailabilityZone: result.Block.Region,
		Endpoint:         fmt.Sprintf("vultr-block://%s", result.Block.ID),
	}, nil
}

func (p *VultrStorageProvisioner) DeprovisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, volumeID string) error {
	url := fmt.Sprintf("%s/v2/blocks/%s", p.blocksAPIBase, volumeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey(cfg))

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Vultr blocks delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("Vultr blocks delete returned %d: %s", resp.StatusCode, b)
	}
	return nil
}

func (p *VultrStorageProvisioner) GetBlockVolumeStatus(ctx context.Context, cfg ExternalProviderConfig, volumeID string) (*BlockVolumeInfo, error) {
	url := fmt.Sprintf("%s/v2/blocks/%s", p.blocksAPIBase, volumeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.apiKey(cfg))

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("Vultr blocks get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Vultr blocks get returned %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Block struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			SizeGB int64  `json:"size_gb"`
			Region string `json:"region"`
		} `json:"block"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode Vultr blocks status: %w", err)
	}

	return &BlockVolumeInfo{
		VolumeID:         result.Block.ID,
		State:            result.Block.Status,
		SizeGB:           result.Block.SizeGB,
		AvailabilityZone: result.Block.Region,
		Endpoint:         fmt.Sprintf("vultr-block://%s", result.Block.ID),
	}, nil
}

func (p *VultrStorageProvisioner) objectEndpoint(cfg ExternalProviderConfig) string {
	if cfg.Endpoint != "" {
		return strings.TrimRight(cfg.Endpoint, "/")
	}
	if cfg.Region != "" {
		return fmt.Sprintf("https://%s.vultrobjects.com", cfg.Region)
	}
	return "https://ewr1.vultrobjects.com"
}

func (p *VultrStorageProvisioner) apiKey(cfg ExternalProviderConfig) string {
	if cfg.Extra != nil {
		if k := cfg.Extra["vultr_api_key"]; k != "" {
			return k
		}
	}
	return ""
}
