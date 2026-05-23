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
