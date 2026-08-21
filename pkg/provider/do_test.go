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
		Extra:    map[string]string{"access_key": "test-access-key", "secret_key": "test-secret-key"},
	}
	err := p.DeprovisionObjectBucket(context.Background(), cfg, "my-bucket")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestDOStorageProvisioner_DeprovisionObjectBucket_NonEmpty(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusConflict)
		w.Write([]byte(`<Error><Code>BucketNotEmpty</Code><Message>The bucket is not empty</Message></Error>`))
	}))
	defer srv.Close()

	p := NewDOStorageProvisioner()
	cfg := ExternalProviderConfig{Provider: "digitalocean", Endpoint: srv.URL, Extra: map[string]string{"access_key": "test-access-key", "secret_key": "test-secret-key"}}
	err := p.DeprovisionObjectBucket(context.Background(), cfg, "my-bucket")
	if err == nil {
		t.Fatal("expected error for non-empty bucket")
	}
	if !strings.Contains(err.Error(), "not empty") {
		t.Errorf("expected 'not empty' in error, got: %v", err)
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
		if r.URL.Path != "/v2/volumes/vol-abc123" {
			t.Errorf("expected path /v2/volumes/vol-abc123, got %s", r.URL.Path)
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
