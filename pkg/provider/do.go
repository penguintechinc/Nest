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

// DOStorageProvisioner implements StorageProvisioner for DigitalOcean.
// Spaces (object) uses S3-compatible XML protocol against the DO Spaces endpoint.
// Volumes (block) uses the DigitalOcean Volumes REST API.
//
// Credential secret keys:
//   - access_key / secret_key: for Spaces
//   - do_token: for Volumes API (Bearer token)
//
// For Spaces, set ExternalProviderConfig.Endpoint or it defaults to
// https://<region>.digitaloceanspaces.com.
type DOStorageProvisioner struct {
	httpClient     *http.Client
	volumesAPIBase string
}

var _ StorageProvisioner = (*DOStorageProvisioner)(nil)

func NewDOStorageProvisioner() *DOStorageProvisioner {
	return &DOStorageProvisioner{
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		volumesAPIBase: "https://api.digitalocean.com",
	}
}

func newDOStorageProvisionerWithBase(volumesAPIBase string) *DOStorageProvisioner {
	return &DOStorageProvisioner{
		httpClient:     &http.Client{Timeout: 30 * time.Second},
		volumesAPIBase: volumesAPIBase,
	}
}

// ProvisionObjectBucket creates a DO Spaces bucket using the S3-compatible API.
func (p *DOStorageProvisioner) ProvisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, spec ObjectBucketSpec) (*ObjectBucketInfo, error) {
	bucketName := spec.BucketName
	if bucketName == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	base := p.spacesEndpoint(cfg)
	url := fmt.Sprintf("%s/%s", base, bucketName)

	var bodyBytes []byte
	if cfg.Region != "" {
		bodyBytes = []byte(fmt.Sprintf(`<CreateBucketConfiguration><LocationConstraint>%s</LocationConstraint></CreateBucketConfiguration>`, cfg.Region))
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	if len(bodyBytes) > 0 {
		req.Header.Set("Content-Type", "application/xml")
	}

	// SigV4-sign with the S3 credentials, which are distinct from the
	// management API token used by the block-volume calls.
	if err := signS3Request(req, cfg, "DigitalOcean Spaces", "nyc3", bodyBytes); err != nil {
		return nil, err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("create DO Spaces bucket: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("DO Spaces create bucket returned %d: %s", resp.StatusCode, b)
	}

	return &ObjectBucketInfo{
		BucketName: bucketName,
		Endpoint:   fmt.Sprintf("s3://%s.%s.digitaloceanspaces.com", bucketName, cfg.Region),
		Region:     cfg.Region,
	}, nil
}

// DeprovisionObjectBucket deletes a DO Spaces bucket.
func (p *DOStorageProvisioner) DeprovisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, bucketName string) error {
	base := p.spacesEndpoint(cfg)
	url := fmt.Sprintf("%s/%s", base, bucketName)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	if err := signS3Request(req, cfg, "DigitalOcean Spaces", "nyc3", nil); err != nil {
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete DO Spaces bucket: %w", err)
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

// ProvisionBlockVolume creates a DigitalOcean Volume.
func (p *DOStorageProvisioner) ProvisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, spec BlockVolumeSpec) (*BlockVolumeInfo, error) {
	token, err := p.token(cfg)
	if err != nil {
		return nil, err
	}

	region := cfg.Region
	if region == "" {
		region = spec.AvailabilityZone
	}
	if region == "" {
		return nil, fmt.Errorf("region is required for DigitalOcean volume provisioning")
	}

	sizeGB := spec.SizeGB
	if sizeGB == 0 {
		sizeGB = 10
	}

	payload := map[string]interface{}{
		"size_gigabytes": sizeGB,
		"region":         region,
		"name":           cfg.ResourceID,
	}
	b, err := json.Marshal(payload)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v2/volumes", p.volumesAPIBase)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(b))
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DO Volumes API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusCreated && resp.StatusCode != http.StatusOK {
		rb, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("DO Volumes create returned %d: %s", resp.StatusCode, rb)
	}

	var result struct {
		Volume struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			SizeGB int64  `json:"size_gigabytes"`
			Region struct {
				Slug string `json:"slug"`
			} `json:"region"`
		} `json:"volume"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode DO Volumes response: %w", err)
	}

	return &BlockVolumeInfo{
		VolumeID:         result.Volume.ID,
		State:            result.Volume.Status,
		SizeGB:           result.Volume.SizeGB,
		AvailabilityZone: result.Volume.Region.Slug,
		Endpoint:         fmt.Sprintf("do-volume://%s", result.Volume.ID),
	}, nil
}

// DeprovisionBlockVolume deletes a DigitalOcean Volume by ID.
func (p *DOStorageProvisioner) DeprovisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, volumeID string) error {
	token, err := p.token(cfg)
	if err != nil {
		return err
	}

	url := fmt.Sprintf("%s/v2/volumes/%s", p.volumesAPIBase, volumeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("DO Volumes delete: %w", err)
	}
	defer resp.Body.Close()

	// 404 Not Found is treated as success (idempotent delete)
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DO Volumes delete returned %d: %s", resp.StatusCode, b)
	}
	return nil
}

// GetBlockVolumeStatus fetches current status of a DigitalOcean Volume.
func (p *DOStorageProvisioner) GetBlockVolumeStatus(ctx context.Context, cfg ExternalProviderConfig, volumeID string) (*BlockVolumeInfo, error) {
	token, err := p.token(cfg)
	if err != nil {
		return nil, err
	}

	url := fmt.Sprintf("%s/v2/volumes/%s", p.volumesAPIBase, volumeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("build request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("DO Volumes get: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("DO Volumes get returned %d: %s", resp.StatusCode, b)
	}

	var result struct {
		Volume struct {
			ID     string `json:"id"`
			Status string `json:"status"`
			SizeGB int64  `json:"size_gigabytes"`
			Region struct {
				Slug string `json:"slug"`
			} `json:"region"`
		} `json:"volume"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode DO Volumes status: %w", err)
	}

	return &BlockVolumeInfo{
		VolumeID:         result.Volume.ID,
		State:            result.Volume.Status,
		SizeGB:           result.Volume.SizeGB,
		AvailabilityZone: result.Volume.Region.Slug,
		Endpoint:         fmt.Sprintf("do-volume://%s", result.Volume.ID),
	}, nil
}

func (p *DOStorageProvisioner) spacesEndpoint(cfg ExternalProviderConfig) string {
	if cfg.Endpoint != "" {
		return strings.TrimRight(cfg.Endpoint, "/")
	}
	if cfg.Region != "" {
		return fmt.Sprintf("https://%s.digitaloceanspaces.com", cfg.Region)
	}
	return "https://nyc3.digitaloceanspaces.com"
}

func (p *DOStorageProvisioner) token(cfg ExternalProviderConfig) (string, error) {
	if t, ok := cfg.Credential("do_token"); ok && t != "" {
		return t, nil
	}
	return "", fmt.Errorf("do_token credential missing: required for DigitalOcean block volumes API; provide it in the referenced credential Secret (key \"do_token\")")
}
