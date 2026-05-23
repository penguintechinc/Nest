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

func TestVultrStorageProvisioner_DeprovisionObjectBucket_NonEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`<Error><Code>BucketNotEmpty</Code><Message>The bucket is not empty</Message></Error>`))
	}))
	defer srv.Close()

	p := NewVultrStorageProvisioner()
	cfg := ExternalProviderConfig{Provider: "vultr", Endpoint: srv.URL}
	err := p.DeprovisionObjectBucket(context.Background(), cfg, "vultr-bucket")
	if err == nil {
		t.Fatal("expected error for non-empty bucket")
	}
	if !strings.Contains(err.Error(), "not empty") {
		t.Errorf("expected 'not empty' in error, got: %v", err)
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
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v2/blocks/vultr-block-123" {
			t.Errorf("expected path /v2/blocks/vultr-block-123, got %s", r.URL.Path)
		}
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
