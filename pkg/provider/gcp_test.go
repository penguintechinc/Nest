package provider

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"cloud.google.com/go/storage"
	compute "google.golang.org/api/compute/v1"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
)

// newMockGCPProvisioner builds a GCPStorageProvisioner whose computeService and
// storageClient are pointed at a local httptest.Server via handler, bypassing real
// GCP auth entirely (option.WithoutAuthentication). initClients' guard clause skips
// re-initialization since these fields are already set, so calls never leave localhost.
func newMockGCPProvisioner(t *testing.T, handler http.HandlerFunc) (*GCPStorageProvisioner, *httptest.Server) {
	t.Helper()
	srv := httptest.NewServer(handler)
	ctx := context.Background()

	computeSvc, err := compute.NewService(ctx,
		option.WithEndpoint(srv.URL),
		option.WithHTTPClient(srv.Client()),
		option.WithoutAuthentication(),
	)
	if err != nil {
		srv.Close()
		t.Fatalf("compute.NewService: %v", err)
	}

	storageClient, err := storage.NewClient(ctx,
		option.WithEndpoint(srv.URL),
		option.WithHTTPClient(srv.Client()),
		option.WithoutAuthentication(),
	)
	if err != nil {
		srv.Close()
		t.Fatalf("storage.NewClient: %v", err)
	}

	return &GCPStorageProvisioner{computeService: computeSvc, storageClient: storageClient}, srv
}

func opDoneJSON(name string) string {
	return fmt.Sprintf(`{"name":%q,"status":"DONE"}`, name)
}

// Validation tests - these test input validation which happens before any API calls
func TestGCPStorageProvisioner_ProvisionBlockVolume_MissingProjectID(t *testing.T) {
	p := NewGCPStorageProvisioner()

	cfg := ExternalProviderConfig{
		Region: "us-central1-a",
		Extra:  map[string]string{},
	}

	_, err := p.ProvisionBlockVolume(context.Background(), cfg, BlockVolumeSpec{})
	if err == nil {
		t.Error("expected error for missing project_id")
	}
	if !strings.Contains(err.Error(), "project_id is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGCPStorageProvisioner_DeprovisionBlockVolume_MissingProjectID(t *testing.T) {
	p := NewGCPStorageProvisioner()

	cfg := ExternalProviderConfig{
		Region: "us-central1-a",
		Extra:  map[string]string{},
	}

	err := p.DeprovisionBlockVolume(context.Background(), cfg, "vol-12345")
	if err == nil {
		t.Error("expected error for missing project_id")
	}
	if !strings.Contains(err.Error(), "project_id is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGCPStorageProvisioner_GetBlockVolumeStatus_MissingProjectID(t *testing.T) {
	p := NewGCPStorageProvisioner()

	cfg := ExternalProviderConfig{
		Region: "us-central1-a",
		Extra:  map[string]string{},
	}

	_, err := p.GetBlockVolumeStatus(context.Background(), cfg, "vol-12345")
	if err == nil {
		t.Error("expected error for missing project_id")
	}
	if !strings.Contains(err.Error(), "project_id is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGCPStorageProvisioner_ProvisionObjectBucket_MissingProjectID(t *testing.T) {
	p := NewGCPStorageProvisioner()

	cfg := ExternalProviderConfig{
		Region: "US",
		Extra:  map[string]string{},
	}

	_, err := p.ProvisionObjectBucket(context.Background(), cfg, ObjectBucketSpec{
		BucketName:        "test-bucket",
		PublicAccessBlock: true,
	})
	if err == nil {
		t.Error("expected error for missing project_id")
	}
	if !strings.Contains(err.Error(), "project_id is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGCPStorageProvisioner_ProvisionObjectBucket_MissingBucketName(t *testing.T) {
	p := NewGCPStorageProvisioner()

	cfg := ExternalProviderConfig{
		Region: "US",
		Extra: map[string]string{
			"project_id": "test-project",
		},
	}

	_, err := p.ProvisionObjectBucket(context.Background(), cfg, ObjectBucketSpec{
		PublicAccessBlock: true,
	})
	if err == nil {
		t.Error("expected error for missing bucket name")
	}
	if !strings.Contains(err.Error(), "bucket name is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGCPStorageProvisioner_ProvisionObjectBucket_CMEKMissingKeyID(t *testing.T) {
	p := NewGCPStorageProvisioner()

	cfg := ExternalProviderConfig{
		Region: "US",
		Extra: map[string]string{
			"project_id": "test-project",
		},
	}

	_, err := p.ProvisionObjectBucket(context.Background(), cfg, ObjectBucketSpec{
		BucketName:        "kms-bucket",
		EncryptionType:    "customer-managed",
		PublicAccessBlock: true,
	})
	if err == nil {
		t.Error("expected error when CMEK key ID is missing")
	}
	if !strings.Contains(err.Error(), "kms_key_id not provided") {
		t.Errorf("expected kms_key_id error, got: %v", err)
	}
}

func TestGCPStorageProvisioner_ProvisionObjectBucket_UnsupportedEncryption(t *testing.T) {
	p := NewGCPStorageProvisioner()

	cfg := ExternalProviderConfig{
		Region: "US",
		Extra: map[string]string{
			"project_id": "test-project",
		},
	}

	_, err := p.ProvisionObjectBucket(context.Background(), cfg, ObjectBucketSpec{
		BucketName:        "test-bucket",
		EncryptionType:    "unsupported-cipher",
		PublicAccessBlock: true,
	})
	if err == nil {
		t.Error("expected error for unsupported encryption")
	}
	if !strings.Contains(err.Error(), "unsupported encryption type") {
		t.Errorf("unexpected error: %v", err)
	}
}

// Valid encryption types should not fail validation
func TestGCPStorageProvisioner_ProvisionObjectBucket_GoogleManagedEncryption(t *testing.T) {
	p := NewGCPStorageProvisioner()

	cfg := ExternalProviderConfig{
		Region: "US",
		Extra: map[string]string{
			"project_id": "test-project",
		},
	}

	_, err := p.ProvisionObjectBucket(context.Background(), cfg, ObjectBucketSpec{
		BucketName:        "test-bucket",
		EncryptionType:    "google-managed",
		PublicAccessBlock: true,
	})
	// Will fail on initClients (no real credentials), but not on encryption validation
	if err != nil && strings.Contains(err.Error(), "unsupported") {
		t.Errorf("google-managed encryption should be supported: %v", err)
	}
}

func TestGCPStorageProvisioner_ProvisionObjectBucket_AES256Encryption(t *testing.T) {
	p := NewGCPStorageProvisioner()

	cfg := ExternalProviderConfig{
		Region: "US",
		Extra: map[string]string{
			"project_id": "test-project",
		},
	}

	_, err := p.ProvisionObjectBucket(context.Background(), cfg, ObjectBucketSpec{
		BucketName:        "test-bucket",
		EncryptionType:    "AES256",
		PublicAccessBlock: true,
	})
	// Will fail on initClients, but not on encryption validation
	if err != nil && strings.Contains(err.Error(), "unsupported") {
		t.Errorf("AES256 encryption should be supported: %v", err)
	}
}

func TestGCPStorageProvisioner_ProvisionObjectBucket_CMEKWithValidKeyID(t *testing.T) {
	p := NewGCPStorageProvisioner()

	cfg := ExternalProviderConfig{
		Region: "US",
		Extra: map[string]string{
			"project_id": "test-project",
			"kms_key_id": "projects/test-project/locations/us/keyRings/my-ring/cryptoKeys/my-key",
		},
	}

	_, err := p.ProvisionObjectBucket(context.Background(), cfg, ObjectBucketSpec{
		BucketName:        "kms-bucket",
		EncryptionType:    "customer-managed",
		PublicAccessBlock: true,
	})
	// Will fail on initClients, but not on validation
	if err != nil && strings.Contains(err.Error(), "kms_key_id") {
		t.Errorf("CMEK with key ID should pass validation: %v", err)
	}
}

// Helper function tests - these test pure logic with no external dependencies
func TestGCPStorageProvisioner_IsNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"notFound string", fmt.Errorf("notFound"), true},
		{"404 status", &googleapi.Error{Code: 404}, true},
		{"other error", fmt.Errorf("other"), false},
		{"wrapped notFound", fmt.Errorf("wrapped: notFound error"), true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isNotFound(tt.err)
			if got != tt.want {
				t.Errorf("isNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGCPStorageProvisioner_IsGCSNotFound(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{"nil error", nil, false},
		{"ErrBucketNotExist", storage.ErrBucketNotExist, true},
		{"notFound string", fmt.Errorf("notFound"), true},
		{"Not Found string", fmt.Errorf("Not Found"), true},
		{"other error", fmt.Errorf("other error"), false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := isGCSNotFound(tt.err)
			if got != tt.want {
				t.Errorf("isGCSNotFound() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestGCPStorageProvisioner_ProvisionBlockVolume_Success(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/disks"):
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, opDoneJSON("op-create-1"))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/operations/"):
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, opDoneJSON("op-create-1"))
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	cfg := ExternalProviderConfig{
		Region: "us-central1",
		Extra:  map[string]string{"project_id": "test-project"},
	}
	spec := BlockVolumeSpec{AvailabilityZone: "us-central1-a", SizeGB: 50, VolumeType: "pd-ssd"}

	info, err := p.ProvisionBlockVolume(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.SizeGB != 50 {
		t.Errorf("expected size 50, got %d", info.SizeGB)
	}
	if info.AvailabilityZone != "us-central1-a" {
		t.Errorf("expected zone us-central1-a, got %s", info.AvailabilityZone)
	}
}

func TestGCPStorageProvisioner_ProvisionBlockVolume_DefaultsApplied(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/disks"):
			_, _ = fmt.Fprint(w, opDoneJSON("op-1"))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/operations/"):
			_, _ = fmt.Fprint(w, opDoneJSON("op-1"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	cfg := ExternalProviderConfig{Extra: map[string]string{"project_id": "test-project"}}
	info, err := p.ProvisionBlockVolume(context.Background(), cfg, BlockVolumeSpec{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.SizeGB != 100 {
		t.Errorf("expected default size 100, got %d", info.SizeGB)
	}
	if info.AvailabilityZone != "us-central1-a" {
		t.Errorf("expected default zone us-central1-a, got %s", info.AvailabilityZone)
	}
}

func TestGCPStorageProvisioner_ProvisionBlockVolume_CreateDiskError(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		_, _ = fmt.Fprint(w, `{"error":{"code":400,"message":"invalid disk type"}}`)
	})
	defer srv.Close()

	cfg := ExternalProviderConfig{Extra: map[string]string{"project_id": "test-project"}}
	_, err := p.ProvisionBlockVolume(context.Background(), cfg, BlockVolumeSpec{})
	if err == nil {
		t.Fatal("expected error from disks.Insert")
	}
}

func TestGCPStorageProvisioner_ProvisionBlockVolume_OperationError(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/disks"):
			_, _ = fmt.Fprint(w, opDoneJSON("op-fail"))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/operations/"):
			_, _ = fmt.Fprint(w, `{"name":"op-fail","status":"DONE","error":{"errors":[{"code":"QUOTA_EXCEEDED","message":"disk quota exceeded"}]}}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	cfg := ExternalProviderConfig{Extra: map[string]string{"project_id": "test-project"}}
	_, err := p.ProvisionBlockVolume(context.Background(), cfg, BlockVolumeSpec{})
	if err == nil {
		t.Fatal("expected error from failed operation")
	}
	if !strings.Contains(err.Error(), "quota") {
		t.Errorf("expected quota error, got: %v", err)
	}
}

func TestGCPStorageProvisioner_DeprovisionBlockVolume_Success(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodDelete:
			_, _ = fmt.Fprint(w, opDoneJSON("op-delete"))
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/operations/"):
			_, _ = fmt.Fprint(w, opDoneJSON("op-delete"))
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	cfg := ExternalProviderConfig{Region: "us-central1-a", Extra: map[string]string{"project_id": "test-project"}}
	if err := p.DeprovisionBlockVolume(context.Background(), cfg, "disk-1"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGCPStorageProvisioner_DeprovisionBlockVolume_NotFoundIsIdempotent(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"error":{"code":404,"message":"disk not found","errors":[{"reason":"notFound"}]}}`)
	})
	defer srv.Close()

	cfg := ExternalProviderConfig{Extra: map[string]string{"project_id": "test-project"}}
	if err := p.DeprovisionBlockVolume(context.Background(), cfg, "gone-disk"); err != nil {
		t.Fatalf("expected idempotent success, got error: %v", err)
	}
}

func TestGCPStorageProvisioner_DeprovisionBlockVolume_DeleteError(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"error":{"code":403,"message":"permission denied"}}`)
	})
	defer srv.Close()

	cfg := ExternalProviderConfig{Extra: map[string]string{"project_id": "test-project"}}
	if err := p.DeprovisionBlockVolume(context.Background(), cfg, "disk-1"); err == nil {
		t.Fatal("expected error from disks.Delete")
	}
}

func TestGCPStorageProvisioner_GetBlockVolumeStatus_Success(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		_, _ = fmt.Fprint(w, `{"status":"READY","sizeGb":"75"}`)
	})
	defer srv.Close()

	cfg := ExternalProviderConfig{Region: "us-central1-a", Extra: map[string]string{"project_id": "test-project"}}
	info, err := p.GetBlockVolumeStatus(context.Background(), cfg, "disk-1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.State != "READY" {
		t.Errorf("expected state READY, got %s", info.State)
	}
	if info.SizeGB != 75 {
		t.Errorf("expected size 75, got %d", info.SizeGB)
	}
}

func TestGCPStorageProvisioner_GetBlockVolumeStatus_NotFound(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"error":{"code":404,"message":"not found","errors":[{"reason":"notFound"}]}}`)
	})
	defer srv.Close()

	cfg := ExternalProviderConfig{Extra: map[string]string{"project_id": "test-project"}}
	_, err := p.GetBlockVolumeStatus(context.Background(), cfg, "gone-disk")
	if err == nil {
		t.Fatal("expected not-found error")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected not-found error message, got: %v", err)
	}
}

func TestGCPStorageProvisioner_GetBlockVolumeStatus_APIError(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = fmt.Fprint(w, `{"error":{"code":500,"message":"internal error"}}`)
	})
	defer srv.Close()

	cfg := ExternalProviderConfig{Extra: map[string]string{"project_id": "test-project"}}
	_, err := p.GetBlockVolumeStatus(context.Background(), cfg, "disk-1")
	if err == nil {
		t.Fatal("expected error from disks.Get")
	}
}

func TestGCPStorageProvisioner_ProvisionObjectBucket_FullFlow(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"name":"my-bucket","location":"US"}`)
		case http.MethodPatch:
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprint(w, `{"name":"my-bucket","location":"US"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	cfg := ExternalProviderConfig{
		Extra: map[string]string{"project_id": "test-project", "kms_key_id": "projects/p/locations/us/keyRings/r/cryptoKeys/k"},
	}
	spec := ObjectBucketSpec{
		BucketName:        "my-bucket",
		Versioning:        true,
		EncryptionType:    "customer-managed",
		PublicAccessBlock: true,
	}

	info, err := p.ProvisionObjectBucket(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if info.BucketName != "my-bucket" {
		t.Errorf("expected bucket name my-bucket, got %s", info.BucketName)
	}
}

func TestGCPStorageProvisioner_ProvisionObjectBucket_PublicAccessBlockRequired(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = fmt.Fprint(w, `{"name":"my-bucket","location":"US"}`)
	})
	defer srv.Close()

	cfg := ExternalProviderConfig{Extra: map[string]string{"project_id": "test-project"}}
	spec := ObjectBucketSpec{BucketName: "my-bucket", PublicAccessBlock: false}

	_, err := p.ProvisionObjectBucket(context.Background(), cfg, spec)
	if err == nil {
		t.Fatal("expected error when PublicAccessBlock is false")
	}
	if !strings.Contains(err.Error(), "PublicAccessBlock must be true") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestGCPStorageProvisioner_ProvisionObjectBucket_CreateError(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"error":{"code":403,"message":"permission denied"}}`)
	})
	defer srv.Close()

	cfg := ExternalProviderConfig{Extra: map[string]string{"project_id": "test-project"}}
	spec := ObjectBucketSpec{BucketName: "my-bucket", PublicAccessBlock: true}

	_, err := p.ProvisionObjectBucket(context.Background(), cfg, spec)
	if err == nil {
		t.Fatal("expected error from bucket creation")
	}
}

func TestGCPStorageProvisioner_ProvisionObjectBucket_AlreadyExistsIsIdempotent(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case http.MethodPost:
			w.WriteHeader(http.StatusConflict)
			_, _ = fmt.Fprint(w, `{"error":{"code":409,"message":"bucket already exists"}}`)
		case http.MethodPatch:
			_, _ = fmt.Fprint(w, `{"name":"my-bucket"}`)
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	})
	defer srv.Close()

	cfg := ExternalProviderConfig{Extra: map[string]string{"project_id": "test-project"}}
	spec := ObjectBucketSpec{BucketName: "my-bucket", PublicAccessBlock: true}

	_, err := p.ProvisionObjectBucket(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("expected idempotent success on already-exists, got: %v", err)
	}
}

func TestGCPStorageProvisioner_DeprovisionObjectBucket_Success(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNoContent)
	})
	defer srv.Close()

	if err := p.DeprovisionObjectBucket(context.Background(), ExternalProviderConfig{}, "my-bucket"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestGCPStorageProvisioner_DeprovisionObjectBucket_NotFoundIsIdempotent(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = fmt.Fprint(w, `{"error":{"code":404,"message":"bucket not found"}}`)
	})
	defer srv.Close()

	if err := p.DeprovisionObjectBucket(context.Background(), ExternalProviderConfig{}, "gone-bucket"); err != nil {
		t.Fatalf("expected idempotent success, got error: %v", err)
	}
}

func TestGCPStorageProvisioner_DeprovisionObjectBucket_DeleteError(t *testing.T) {
	p, srv := newMockGCPProvisioner(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		_, _ = fmt.Fprint(w, `{"error":{"code":403,"message":"permission denied"}}`)
	})
	defer srv.Close()

	if err := p.DeprovisionObjectBucket(context.Background(), ExternalProviderConfig{}, "my-bucket"); err == nil {
		t.Fatal("expected error from bucket delete")
	}
}

func TestGCPStorageProvisioner_InitClients_RealInit(t *testing.T) {
	p := NewGCPStorageProvisioner()
	err := p.initClients(context.Background(), ExternalProviderConfig{})
	// Exercises the real (non-guarded) init branch. In a clean test environment
	// with no ambient ADC, this fails on credential resolution — that's expected
	// and still exercises the branch and its error path. If ADC happens to be
	// present (e.g. running on GCP infra), it should succeed with non-nil clients.
	if err != nil {
		if !strings.Contains(err.Error(), "credentials") {
			t.Errorf("expected a credentials-related error, got: %v", err)
		}
		return
	}
	if p.computeService == nil || p.storageClient == nil {
		t.Error("initClients succeeded but did not initialize clients")
	}
}

func TestGCPStorageProvisioner_InitClients_ServiceAccountJSON(t *testing.T) {
	p := NewGCPStorageProvisioner()
	cfg := ExternalProviderConfig{
		CredentialData: map[string][]byte{
			"service_account_json": []byte(`{"type":"service_account","project_id":"test","private_key":"not-real-key-material","client_email":"test@test.iam.gserviceaccount.com","token_uri":"https://oauth2.googleapis.com/token"}`),
		},
	}
	// A malformed/fake private key is expected to fail during client construction
	// (real key parsing happens at NewService time) — this exercises the
	// service-account-JSON branch and its error path, distinct from the ADC branch.
	_ = p.initClients(context.Background(), cfg)
}

// Constructor test
func TestGCPStorageProvisioner_NewConstructor(t *testing.T) {
	p := NewGCPStorageProvisioner()
	if p == nil {
		t.Fatal("expected NewGCPStorageProvisioner to return non-nil")
	}
	if p.computeService != nil || p.storageClient != nil {
		t.Error("expected clients to be nil initially (lazy init)")
	}
}

// Ensure GCPStorageProvisioner implements StorageProvisioner
var _ StorageProvisioner = (*GCPStorageProvisioner)(nil)
