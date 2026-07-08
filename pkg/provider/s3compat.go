package provider

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// S3CompatProvisioner provisions object buckets against any S3-wire-protocol endpoint.
// Set ExternalProviderConfig.Endpoint to the base URL (e.g. https://nyc3.digitaloceanspaces.com).
// Set Extra["path_style"]="true" for providers requiring path-style URLs.
// Block volume methods are not supported and return an error.
type S3CompatProvisioner struct {
	httpClient *http.Client
}

var _ StorageProvisioner = (*S3CompatProvisioner)(nil)

func NewS3CompatProvisioner() *S3CompatProvisioner {
	return &S3CompatProvisioner{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *S3CompatProvisioner) ProvisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, spec ObjectBucketSpec) (*ObjectBucketInfo, error) {
	if cfg.Endpoint == "" {
		return nil, fmt.Errorf("s3-compat provider requires endpoint to be set")
	}
	bucketName := spec.BucketName
	if bucketName == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	base := strings.TrimRight(cfg.Endpoint, "/")
	url := fmt.Sprintf("%s/%s", base, bucketName)

	var body io.Reader
	if cfg.Region != "" && cfg.Region != "us-east-1" {
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
		return nil, fmt.Errorf("create bucket request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		b, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("create bucket returned %d: %s", resp.StatusCode, b)
	}

	endpoint := fmt.Sprintf("%s/%s", base, bucketName)

	return &ObjectBucketInfo{
		BucketName: bucketName,
		Endpoint:   endpoint,
		Region:     cfg.Region,
	}, nil
}

func (p *S3CompatProvisioner) DeprovisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, bucketName string) error {
	if cfg.Endpoint == "" {
		return fmt.Errorf("s3-compat provider requires endpoint to be set")
	}

	base := strings.TrimRight(cfg.Endpoint, "/")
	url := fmt.Sprintf("%s/%s", base, bucketName)

	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return fmt.Errorf("build request: %w", err)
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete bucket request failed: %w", err)
	}
	defer resp.Body.Close()

	// 404 Not Found is treated as success (idempotent delete)
	if resp.StatusCode == http.StatusNotFound {
		return nil
	}
	if resp.StatusCode == http.StatusConflict || resp.StatusCode == http.StatusBadRequest {
		b, _ := io.ReadAll(resp.Body)
		body := string(b)
		if strings.Contains(body, "NotEmpty") || strings.Contains(body, "not empty") {
			return fmt.Errorf("bucket %s is not empty; drain it before deleting", bucketName)
		}
		return fmt.Errorf("delete bucket returned %d: %s", resp.StatusCode, body)
	}

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("delete bucket returned %d: %s", resp.StatusCode, b)
	}
	return nil
}

func (p *S3CompatProvisioner) ProvisionBlockVolume(_ context.Context, _ ExternalProviderConfig, _ BlockVolumeSpec) (*BlockVolumeInfo, error) {
	return nil, fmt.Errorf("s3-compat provider does not support block volumes")
}

func (p *S3CompatProvisioner) DeprovisionBlockVolume(_ context.Context, _ ExternalProviderConfig, _ string) error {
	return fmt.Errorf("s3-compat provider does not support block volumes")
}

func (p *S3CompatProvisioner) GetBlockVolumeStatus(_ context.Context, _ ExternalProviderConfig, _ string) (*BlockVolumeInfo, error) {
	return nil, fmt.Errorf("s3-compat provider does not support block volumes")
}
