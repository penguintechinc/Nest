# Cloud Storage Provider Expansion Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add DigitalOcean, Vultr, Linode, and generic S3-compatible storage providers to the Nest DataResource external origination pathway.

**Architecture:** Four new `StorageProvisioner` implementations in `pkg/provider/` (one per provider + one generic), each handling both object and block storage via their respective REST APIs. Type constants and controller switch cases wire them into the existing reconciliation pipeline with zero changes to the reconciler loop.

**Tech Stack:** Go 1.25, `net/http`, `net/http/httptest` (tests), controller-runtime fake client, `github.com/penguintechinc/nest` module.

---

## File Map

| Action | File | Responsibility |
|--------|------|---------------|
| Modify | `apis/v1/storage_types.go` | 7 new type constants + CloudStorageTypes entries |
| Modify | `pkg/provider/storage.go` | Add `Endpoint` to ExternalProviderConfig; 4 new ProvisionerFactoryMap entries |
| Modify | `services/k8s-controller/controllers/external_reconciler.go` | Pass Endpoint in cfg; new cases in providerForType, reconcileExternalStorage, reconcileExternalDelete |
| Create | `pkg/provider/s3compat.go` | Generic S3-compatible object store provisioner |
| Create | `pkg/provider/s3compat_test.go` | Unit tests for S3-compat provisioner |
| Create | `pkg/provider/do.go` | DigitalOcean Spaces + Volumes provisioner |
| Create | `pkg/provider/do_test.go` | Unit tests for DO provisioner |
| Create | `pkg/provider/vultr.go` | Vultr Object Storage + Block Storage provisioner |
| Create | `pkg/provider/vultr_test.go` | Unit tests for Vultr provisioner |
| Create | `pkg/provider/linode.go` | Linode Object Storage + Block Volumes provisioner |
| Create | `pkg/provider/linode_test.go` | Unit tests for Linode provisioner |
| Modify | `services/k8s-controller/controllers/external_reconciler_test.go` | New providerForType tests for all 4 providers |

---

## Task 1: Type Constants + ExternalProviderConfig Endpoint Field

**Files:**
- Modify: `apis/v1/storage_types.go`
- Modify: `pkg/provider/storage.go`
- Modify: `services/k8s-controller/controllers/external_reconciler.go`

- [ ] **Step 1: Add type constants to storage_types.go**

In `apis/v1/storage_types.go`, append to the `Cloud-native external storage types` const block (after `TypeAzureBlob`):

```go
	TypeDOSpaces     = "do-spaces"      // DigitalOcean Spaces (object)
	TypeDOVolume     = "do-volume"      // DigitalOcean Volumes (block)
	TypeVultrObject  = "vultr-object"   // Vultr Object Storage (object)
	TypeVultrBlock   = "vultr-block"    // Vultr Block Storage (block)
	TypeLinodeObject = "linode-object"  // Linode Object Storage (object)
	TypeLinodeBlock  = "linode-block"   // Linode Block Volumes (block)
	TypeS3Compat     = "s3-compat"      // Generic S3-compatible object store
```

In the same file, add to the `CloudStorageTypes` map:

```go
	TypeDOSpaces:     true,
	TypeDOVolume:     true,
	TypeVultrObject:  true,
	TypeVultrBlock:   true,
	TypeLinodeObject: true,
	TypeLinodeBlock:  true,
	TypeS3Compat:     true,
```

- [ ] **Step 2: Add Endpoint field to ExternalProviderConfig**

In `pkg/provider/storage.go`, add `Endpoint` to `ExternalProviderConfig`:

```go
type ExternalProviderConfig struct {
	Provider         string
	Region           string
	ResourceID       string
	CredentialSecret string
	Endpoint         string            // optional base URL override (required for s3-compat)
	Extra            map[string]string
}
```

- [ ] **Step 3: Pass Endpoint from ExternalSpec in the reconciler**

In `services/k8s-controller/controllers/external_reconciler.go`, update the `cfg` construction in `reconcileExternal`:

```go
cfg := kprovider.ExternalProviderConfig{
	Provider:         dr.Spec.External.Provider,
	Region:           dr.Spec.External.Region,
	ResourceID:       dr.Spec.External.ResourceID,
	CredentialSecret: dr.Spec.External.CredentialSecret,
	Endpoint:         dr.Spec.External.Endpoint,
	Extra:            dr.Spec.External.Extra,
}
```

- [ ] **Step 4: Build and verify no compilation errors**

```bash
cd /home/penguin/code/nest && go build ./...
```

Expected: exits 0 with no output.

- [ ] **Step 5: Commit**

```bash
git add apis/v1/storage_types.go pkg/provider/storage.go services/k8s-controller/controllers/external_reconciler.go
git commit -m "feat(storage): add DO/Vultr/Linode/s3-compat type constants and Endpoint config field"
```

---

## Task 2: Generic S3-Compatible Provider (TDD)

**Files:**
- Create: `pkg/provider/s3compat.go`
- Create: `pkg/provider/s3compat_test.go`

`S3CompatProvisioner` handles object buckets only. `ExternalProviderConfig.Endpoint` is the required base URL. Block volume methods return a clear error. Uses the `extractXMLField` helper already defined in `aws.go` (same package).

- [ ] **Step 1: Write the failing tests**

Create `pkg/provider/s3compat_test.go`:

```go
package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestS3CompatProvisioner_ProvisionObjectBucket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		if !strings.Contains(r.URL.Path, "test-bucket") {
			t.Errorf("expected path to contain bucket name, got %s", r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewS3CompatProvisioner()
	cfg := ExternalProviderConfig{
		Provider: "s3-compat",
		Region:   "us-east-1",
		Endpoint: srv.URL,
		Extra:    map[string]string{"access_key": "test-key", "secret_key": "test-secret"},
	}
	spec := ObjectBucketSpec{BucketName: "test-bucket"}

	info, err := p.ProvisionObjectBucket(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.BucketName != "test-bucket" {
		t.Errorf("expected bucket name test-bucket, got %s", info.BucketName)
	}
	if info.Endpoint == "" {
		t.Error("expected non-empty endpoint")
	}
}

func TestS3CompatProvisioner_ProvisionObjectBucket_MissingEndpoint(t *testing.T) {
	p := NewS3CompatProvisioner()
	cfg := ExternalProviderConfig{Provider: "s3-compat"}
	spec := ObjectBucketSpec{BucketName: "test-bucket"}

	_, err := p.ProvisionObjectBucket(context.Background(), cfg, spec)
	if err == nil {
		t.Fatal("expected error for missing endpoint")
	}
	if !strings.Contains(err.Error(), "endpoint") {
		t.Errorf("expected error to mention endpoint, got: %v", err)
	}
}

func TestS3CompatProvisioner_DeprovisionObjectBucket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p := NewS3CompatProvisioner()
	cfg := ExternalProviderConfig{
		Provider: "s3-compat",
		Endpoint: srv.URL,
		Extra:    map[string]string{"access_key": "test-key", "secret_key": "test-secret"},
	}

	err := p.DeprovisionObjectBucket(context.Background(), cfg, "test-bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestS3CompatProvisioner_DeprovisionObjectBucket_NonEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`<Error><Code>BucketNotEmpty</Code><Message>The bucket you tried to delete is not empty</Message></Error>`))
	}))
	defer srv.Close()

	p := NewS3CompatProvisioner()
	cfg := ExternalProviderConfig{
		Provider: "s3-compat",
		Endpoint: srv.URL,
		Extra:    map[string]string{"access_key": "test-key", "secret_key": "test-secret"},
	}

	err := p.DeprovisionObjectBucket(context.Background(), cfg, "test-bucket")
	if err == nil {
		t.Fatal("expected error for non-empty bucket")
	}
	if !strings.Contains(err.Error(), "not empty") {
		t.Errorf("expected 'not empty' in error, got: %v", err)
	}
}

func TestS3CompatProvisioner_BlockVolumesNotSupported(t *testing.T) {
	p := NewS3CompatProvisioner()
	cfg := ExternalProviderConfig{Provider: "s3-compat", Endpoint: "https://example.com"}

	_, err := p.ProvisionBlockVolume(context.Background(), cfg, BlockVolumeSpec{})
	if err == nil {
		t.Fatal("expected error for block volume")
	}

	err = p.DeprovisionBlockVolume(context.Background(), cfg, "vol-123")
	if err == nil {
		t.Fatal("expected error for block volume")
	}

	_, err = p.GetBlockVolumeStatus(context.Background(), cfg, "vol-123")
	if err == nil {
		t.Fatal("expected error for block volume")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/penguin/code/nest && go test ./pkg/provider/ -run TestS3Compat -v
```

Expected: compilation error — `NewS3CompatProvisioner` undefined.

- [ ] **Step 3: Implement s3compat.go**

Create `pkg/provider/s3compat.go`:

```go
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
		return nil, err
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
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete bucket request failed: %w", err)
	}
	defer resp.Body.Close()

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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/penguin/code/nest && go test ./pkg/provider/ -run TestS3Compat -v
```

Expected: all 5 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/provider/s3compat.go pkg/provider/s3compat_test.go
git commit -m "feat(provider): add generic S3-compatible object storage provisioner"
```

---

## Task 3: DigitalOcean Provider (TDD)

**Files:**
- Create: `pkg/provider/do.go`
- Create: `pkg/provider/do_test.go`

DO Spaces uses S3-compatible XML protocol. DO Volumes uses the DO REST API at `https://api.digitalocean.com/v2/volumes`. The `volumesAPIBase` field allows test override.

- [ ] **Step 1: Write the failing tests**

Create `pkg/provider/do_test.go`:

```go
package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestDOStorageProvisioner_ProvisionObjectBucket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewDOStorageProvisioner()
	cfg := ExternalProviderConfig{
		Provider: "digitalocean",
		Region:   "nyc3",
		Endpoint: srv.URL,
		Extra:    map[string]string{"access_key": "test-key", "secret_key": "test-secret"},
	}
	spec := ObjectBucketSpec{BucketName: "my-bucket"}

	info, err := p.ProvisionObjectBucket(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.BucketName != "my-bucket" {
		t.Errorf("expected my-bucket, got %s", info.BucketName)
	}
}

func TestDOStorageProvisioner_DeprovisionObjectBucket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p := NewDOStorageProvisioner()
	cfg := ExternalProviderConfig{
		Provider: "digitalocean",
		Endpoint: srv.URL,
	}
	err := p.DeprovisionObjectBucket(context.Background(), cfg, "my-bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDOStorageProvisioner_ProvisionBlockVolume(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/volumes" {
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"volume": map[string]interface{}{
				"id":             "vol-abc123",
				"status":         "available",
				"size_gigabytes": 20,
				"region":         map[string]string{"slug": "nyc3"},
			},
		})
	}))
	defer srv.Close()

	p := newDOStorageProvisionerWithBase(srv.URL)
	cfg := ExternalProviderConfig{
		Provider: "digitalocean",
		Region:   "nyc3",
		Extra:    map[string]string{"do_token": "test-token"},
	}
	spec := BlockVolumeSpec{SizeGB: 20, AvailabilityZone: "nyc3"}

	info, err := p.ProvisionBlockVolume(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.VolumeID != "vol-abc123" {
		t.Errorf("expected vol-abc123, got %s", info.VolumeID)
	}
	if !strings.HasPrefix(info.Endpoint, "do-volume://") {
		t.Errorf("expected do-volume:// endpoint, got %s", info.Endpoint)
	}
}

func TestDOStorageProvisioner_GetBlockVolumeStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"volume": map[string]interface{}{
				"id":             "vol-abc123",
				"status":         "available",
				"size_gigabytes": 20,
				"region":         map[string]string{"slug": "nyc3"},
			},
		})
	}))
	defer srv.Close()

	p := newDOStorageProvisionerWithBase(srv.URL)
	cfg := ExternalProviderConfig{Extra: map[string]string{"do_token": "test-token"}}

	info, err := p.GetBlockVolumeStatus(context.Background(), cfg, "vol-abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.State != "available" {
		t.Errorf("expected available, got %s", info.State)
	}
}

func TestDOStorageProvisioner_DeprovisionBlockVolume(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p := newDOStorageProvisionerWithBase(srv.URL)
	cfg := ExternalProviderConfig{Extra: map[string]string{"do_token": "test-token"}}

	err := p.DeprovisionBlockVolume(context.Background(), cfg, "vol-abc123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDOStorageProvisioner_ProvisionBlockVolume_MissingRegion(t *testing.T) {
	p := NewDOStorageProvisioner()
	cfg := ExternalProviderConfig{Provider: "digitalocean"}
	_, err := p.ProvisionBlockVolume(context.Background(), cfg, BlockVolumeSpec{SizeGB: 20})
	if err == nil {
		t.Fatal("expected error for missing region")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/penguin/code/nest && go test ./pkg/provider/ -run TestDOStorage -v
```

Expected: compilation error — `NewDOStorageProvisioner` undefined.

- [ ] **Step 3: Implement do.go**

Create `pkg/provider/do.go`:

```go
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
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete DO Spaces bucket: %w", err)
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

// ProvisionBlockVolume creates a DigitalOcean Volume.
func (p *DOStorageProvisioner) ProvisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, spec BlockVolumeSpec) (*BlockVolumeInfo, error) {
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
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.token(cfg))

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
	url := fmt.Sprintf("%s/v2/volumes/%s", p.volumesAPIBase, volumeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.token(cfg))

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("DO Volumes delete: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		b, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("DO Volumes delete returned %d: %s", resp.StatusCode, b)
	}
	return nil
}

// GetBlockVolumeStatus fetches current status of a DigitalOcean Volume.
func (p *DOStorageProvisioner) GetBlockVolumeStatus(ctx context.Context, cfg ExternalProviderConfig, volumeID string) (*BlockVolumeInfo, error) {
	url := fmt.Sprintf("%s/v2/volumes/%s", p.volumesAPIBase, volumeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.token(cfg))

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

func (p *DOStorageProvisioner) token(cfg ExternalProviderConfig) string {
	if cfg.Extra != nil {
		if t := cfg.Extra["do_token"]; t != "" {
			return t
		}
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/penguin/code/nest && go test ./pkg/provider/ -run TestDOStorage -v
```

Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/provider/do.go pkg/provider/do_test.go
git commit -m "feat(provider): add DigitalOcean Spaces and Volumes provisioner"
```

---

## Task 4: Vultr Provider (TDD)

**Files:**
- Create: `pkg/provider/vultr.go`
- Create: `pkg/provider/vultr_test.go`

Vultr Object Storage is S3-compatible at `https://<region>.vultrobjects.com`. Vultr Block Storage uses `https://api.vultr.com/v2/blocks`.

- [ ] **Step 1: Write the failing tests**

Create `pkg/provider/vultr_test.go`:

```go
package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestVultrStorageProvisioner_ProvisionObjectBucket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewVultrStorageProvisioner()
	cfg := ExternalProviderConfig{
		Provider: "vultr",
		Region:   "ewr",
		Endpoint: srv.URL,
		Extra:    map[string]string{"access_key": "test-key", "secret_key": "test-secret"},
	}
	spec := ObjectBucketSpec{BucketName: "vultr-bucket"}

	info, err := p.ProvisionObjectBucket(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.BucketName != "vultr-bucket" {
		t.Errorf("expected vultr-bucket, got %s", info.BucketName)
	}
}

func TestVultrStorageProvisioner_DeprovisionObjectBucket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p := NewVultrStorageProvisioner()
	cfg := ExternalProviderConfig{
		Provider: "vultr",
		Endpoint: srv.URL,
	}
	err := p.DeprovisionObjectBucket(context.Background(), cfg, "vultr-bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVultrStorageProvisioner_ProvisionBlockVolume(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/blocks" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusCreated)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"block": map[string]interface{}{
				"id":      "vultr-block-123",
				"status":  "active",
				"size_gb": 40,
				"region":  "ewr",
			},
		})
	}))
	defer srv.Close()

	p := newVultrStorageProvisionerWithBase(srv.URL)
	cfg := ExternalProviderConfig{
		Provider: "vultr",
		Region:   "ewr",
		Extra:    map[string]string{"vultr_api_key": "test-key"},
	}
	spec := BlockVolumeSpec{SizeGB: 40}

	info, err := p.ProvisionBlockVolume(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.VolumeID != "vultr-block-123" {
		t.Errorf("expected vultr-block-123, got %s", info.VolumeID)
	}
	if !strings.HasPrefix(info.Endpoint, "vultr-block://") {
		t.Errorf("expected vultr-block:// endpoint, got %s", info.Endpoint)
	}
}

func TestVultrStorageProvisioner_GetBlockVolumeStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"block": map[string]interface{}{
				"id":      "vultr-block-123",
				"status":  "active",
				"size_gb": 40,
				"region":  "ewr",
			},
		})
	}))
	defer srv.Close()

	p := newVultrStorageProvisionerWithBase(srv.URL)
	cfg := ExternalProviderConfig{Extra: map[string]string{"vultr_api_key": "test-key"}}

	info, err := p.GetBlockVolumeStatus(context.Background(), cfg, "vultr-block-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.State != "active" {
		t.Errorf("expected active, got %s", info.State)
	}
}

func TestVultrStorageProvisioner_DeprovisionBlockVolume(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p := newVultrStorageProvisionerWithBase(srv.URL)
	cfg := ExternalProviderConfig{Extra: map[string]string{"vultr_api_key": "test-key"}}

	err := p.DeprovisionBlockVolume(context.Background(), cfg, "vultr-block-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVultrStorageProvisioner_MissingRegion(t *testing.T) {
	p := NewVultrStorageProvisioner()
	cfg := ExternalProviderConfig{Provider: "vultr"}
	_, err := p.ProvisionBlockVolume(context.Background(), cfg, BlockVolumeSpec{SizeGB: 40})
	if err == nil {
		t.Fatal("expected error for missing region")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/penguin/code/nest && go test ./pkg/provider/ -run TestVultr -v
```

Expected: compilation error — `NewVultrStorageProvisioner` undefined.

- [ ] **Step 3: Implement vultr.go**

Create `pkg/provider/vultr.go`:

```go
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
	httpClient  *http.Client
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

	volumeType := spec.VolumeType
	if volumeType == "" {
		volumeType = "ssd_optimized"
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
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/penguin/code/nest && go test ./pkg/provider/ -run TestVultr -v
```

Expected: all 6 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/provider/vultr.go pkg/provider/vultr_test.go
git commit -m "feat(provider): add Vultr Object Storage and Block Storage provisioner"
```

---

## Task 5: Linode Provider (TDD)

**Files:**
- Create: `pkg/provider/linode.go`
- Create: `pkg/provider/linode_test.go`

Linode Object Storage is S3-compatible at `https://<region>.linodeobjects.com`. Linode Block Volumes use `https://api.linode.com/v4/volumes`. Note: Linode requires volumes to be detached before deletion; the provisioner returns a clear error if the volume is still attached.

- [ ] **Step 1: Write the failing tests**

Create `pkg/provider/linode_test.go`:

```go
package provider

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestLinodeStorageProvisioner_ProvisionObjectBucket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPut {
			t.Errorf("expected PUT, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := NewLinodeStorageProvisioner()
	cfg := ExternalProviderConfig{
		Provider: "linode",
		Region:   "us-east",
		Endpoint: srv.URL,
		Extra:    map[string]string{"access_key": "test-key", "secret_key": "test-secret"},
	}
	spec := ObjectBucketSpec{BucketName: "linode-bucket"}

	info, err := p.ProvisionObjectBucket(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.BucketName != "linode-bucket" {
		t.Errorf("expected linode-bucket, got %s", info.BucketName)
	}
}

func TestLinodeStorageProvisioner_DeprovisionObjectBucket(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusNoContent)
	}))
	defer srv.Close()

	p := NewLinodeStorageProvisioner()
	cfg := ExternalProviderConfig{Provider: "linode", Endpoint: srv.URL}
	err := p.DeprovisionObjectBucket(context.Background(), cfg, "linode-bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLinodeStorageProvisioner_ProvisionBlockVolume(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v4/volumes" {
			t.Errorf("unexpected: %s %s", r.Method, r.URL.Path)
		}
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     12345,
			"status": "active",
			"size":   20,
			"region": "us-east",
			"label":  "test-vol",
		})
	}))
	defer srv.Close()

	p := newLinodeStorageProvisionerWithBase(srv.URL)
	cfg := ExternalProviderConfig{
		Provider:   "linode",
		Region:     "us-east",
		ResourceID: "test-vol",
		Extra:      map[string]string{"linode_token": "test-token"},
	}
	spec := BlockVolumeSpec{SizeGB: 20}

	info, err := p.ProvisionBlockVolume(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.VolumeID != "12345" {
		t.Errorf("expected 12345, got %s", info.VolumeID)
	}
	if !strings.HasPrefix(info.Endpoint, "linode-volume://") {
		t.Errorf("expected linode-volume:// endpoint, got %s", info.Endpoint)
	}
}

func TestLinodeStorageProvisioner_GetBlockVolumeStatus(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"id":     12345,
			"status": "active",
			"size":   20,
			"region": "us-east",
		})
	}))
	defer srv.Close()

	p := newLinodeStorageProvisionerWithBase(srv.URL)
	cfg := ExternalProviderConfig{Extra: map[string]string{"linode_token": "test-token"}}

	info, err := p.GetBlockVolumeStatus(context.Background(), cfg, "12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.State != "active" {
		t.Errorf("expected active, got %s", info.State)
	}
}

func TestLinodeStorageProvisioner_DeprovisionBlockVolume(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("expected DELETE, got %s", r.Method)
		}
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	p := newLinodeStorageProvisionerWithBase(srv.URL)
	cfg := ExternalProviderConfig{Extra: map[string]string{"linode_token": "test-token"}}

	err := p.DeprovisionBlockVolume(context.Background(), cfg, "12345")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestLinodeStorageProvisioner_DeprovisionBlockVolume_Attached(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnprocessableEntity)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"errors": []map[string]string{{"reason": "Volume must be detached before deleting."}},
		})
	}))
	defer srv.Close()

	p := newLinodeStorageProvisionerWithBase(srv.URL)
	cfg := ExternalProviderConfig{Extra: map[string]string{"linode_token": "test-token"}}

	err := p.DeprovisionBlockVolume(context.Background(), cfg, "12345")
	if err == nil {
		t.Fatal("expected error for attached volume")
	}
	if !strings.Contains(err.Error(), "detach") {
		t.Errorf("expected 'detach' in error, got: %v", err)
	}
}

func TestLinodeStorageProvisioner_MissingRegion(t *testing.T) {
	p := NewLinodeStorageProvisioner()
	cfg := ExternalProviderConfig{Provider: "linode"}
	_, err := p.ProvisionBlockVolume(context.Background(), cfg, BlockVolumeSpec{SizeGB: 20})
	if err == nil {
		t.Fatal("expected error for missing region")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/penguin/code/nest && go test ./pkg/provider/ -run TestLinode -v
```

Expected: compilation error — `NewLinodeStorageProvisioner` undefined.

- [ ] **Step 3: Implement linode.go**

Create `pkg/provider/linode.go`:

```go
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
	httpClient    *http.Client
	volumesAPIBase string
}

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
		return nil, err
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
		return err
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("delete Linode object bucket: %w", err)
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

func (p *LinodeStorageProvisioner) ProvisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, spec BlockVolumeSpec) (*BlockVolumeInfo, error) {
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
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+p.token(cfg))

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
	url := fmt.Sprintf("%s/v4/volumes/%s", p.volumesAPIBase, volumeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, url, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+p.token(cfg))

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("Linode Volumes delete: %w", err)
	}
	defer resp.Body.Close()

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
	url := fmt.Sprintf("%s/v4/volumes/%s", p.volumesAPIBase, volumeID)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+p.token(cfg))

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

func (p *LinodeStorageProvisioner) token(cfg ExternalProviderConfig) string {
	if cfg.Extra != nil {
		if t := cfg.Extra["linode_token"]; t != "" {
			return t
		}
	}
	return ""
}
```

- [ ] **Step 4: Run tests to verify they pass**

```bash
cd /home/penguin/code/nest && go test ./pkg/provider/ -run TestLinode -v
```

Expected: all 7 tests PASS.

- [ ] **Step 5: Commit**

```bash
git add pkg/provider/linode.go pkg/provider/linode_test.go
git commit -m "feat(provider): add Linode Object Storage and Block Volumes provisioner"
```

---

## Task 6: Register Providers + Controller Switch Updates + Controller Tests

**Files:**
- Modify: `pkg/provider/storage.go`
- Modify: `services/k8s-controller/controllers/external_reconciler.go`
- Modify: `services/k8s-controller/controllers/external_reconciler_test.go`

- [ ] **Step 1: Write failing controller tests first**

Add to `services/k8s-controller/controllers/external_reconciler_test.go`:

```go
func TestProviderForType_DigitalOcean(t *testing.T) {
	prov := providerForType(nestv1.TypeDOSpaces, "digitalocean")
	if prov == nil {
		t.Fatal("expected DO provisioner for do-spaces type")
	}
	prov2 := providerForType(nestv1.TypeDOVolume, "digitalocean")
	if prov2 == nil {
		t.Fatal("expected DO provisioner for do-volume type")
	}
}

func TestProviderForType_Vultr(t *testing.T) {
	prov := providerForType(nestv1.TypeVultrObject, "vultr")
	if prov == nil {
		t.Fatal("expected Vultr provisioner for vultr-object type")
	}
	prov2 := providerForType(nestv1.TypeVultrBlock, "vultr")
	if prov2 == nil {
		t.Fatal("expected Vultr provisioner for vultr-block type")
	}
}

func TestProviderForType_Linode(t *testing.T) {
	prov := providerForType(nestv1.TypeLinodeObject, "linode")
	if prov == nil {
		t.Fatal("expected Linode provisioner for linode-object type")
	}
	prov2 := providerForType(nestv1.TypeLinodeBlock, "linode")
	if prov2 == nil {
		t.Fatal("expected Linode provisioner for linode-block type")
	}
}

func TestProviderForType_S3Compat(t *testing.T) {
	prov := providerForType(nestv1.TypeS3Compat, "s3-compat")
	if prov == nil {
		t.Fatal("expected S3Compat provisioner for s3-compat type")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```bash
cd /home/penguin/code/nest/services/k8s-controller && go test ./controllers/ -run TestProviderForType_DigitalOcean -v
```

Expected: FAIL — `providerForType(nestv1.TypeDOSpaces, "digitalocean")` returns nil (provider not yet registered in the switch).

- [ ] **Step 3: Register all four providers in ProvisionerFactoryMap**

In `pkg/provider/storage.go`, update `ProvisionerFactoryMap`:

```go
var ProvisionerFactoryMap = map[string]StorageProvisionerFactory{
	"aws":          func() StorageProvisioner { return NewAWSStorageProvisioner() },
	"azure":        func() StorageProvisioner { return NewAzureStorageProvisioner() },
	"gcp":          func() StorageProvisioner { return NewGCPStorageProvisioner() },
	"digitalocean": func() StorageProvisioner { return NewDOStorageProvisioner() },
	"vultr":        func() StorageProvisioner { return NewVultrStorageProvisioner() },
	"linode":       func() StorageProvisioner { return NewLinodeStorageProvisioner() },
	"s3-compat":    func() StorageProvisioner { return NewS3CompatProvisioner() },
}
```

- [ ] **Step 4: Update providerForType in external_reconciler.go**

In `services/k8s-controller/controllers/external_reconciler.go`, update the `providerForType` function's provider name switch:

```go
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
```

- [ ] **Step 5: Update reconcileExternalStorage in external_reconciler.go**

Replace the `reconcileExternalStorage` switch:

```go
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
```

- [ ] **Step 6: Update reconcileExternalDelete in external_reconciler.go**

Replace the delete switch:

```go
	switch dr.Spec.Type {
	case nestv1.TypeEBS, nestv1.TypeAzureDisk, nestv1.TypeGCPDisk,
		nestv1.TypeDOVolume, nestv1.TypeVultrBlock, nestv1.TypeLinodeBlock:
		if volumeID == "" {
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
```

- [ ] **Step 7: Run all tests to verify they pass**

```bash
cd /home/penguin/code/nest && go test ./pkg/provider/... -v
cd /home/penguin/code/nest/services/k8s-controller && go test ./controllers/... -v
```

Expected: all tests PASS.

- [ ] **Step 8: Full build verification**

```bash
cd /home/penguin/code/nest && go build ./...
```

Expected: exits 0 with no output.

- [ ] **Step 9: Commit**

```bash
git add pkg/provider/storage.go \
        services/k8s-controller/controllers/external_reconciler.go \
        services/k8s-controller/controllers/external_reconciler_test.go
git commit -m "feat(controller): register DO/Vultr/Linode/s3-compat providers and wire into reconciler"
```

---

## Final Verification

- [ ] **Run full provider test suite**

```bash
cd /home/penguin/code/nest && go test ./pkg/provider/... -v -count=1
```

Expected: 30+ tests, all PASS.

- [ ] **Run full controller test suite**

```bash
cd /home/penguin/code/nest/services/k8s-controller && go test ./... -v -count=1
```

Expected: all existing + new tests PASS.

- [ ] **Verify all 7 new types are in CloudStorageTypes**

```bash
cd /home/penguin/code/nest && grep -A 20 "CloudStorageTypes" apis/v1/storage_types.go
```

Expected: 13 entries total (6 original + 7 new).
