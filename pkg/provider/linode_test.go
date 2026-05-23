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
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		if r.URL.Path != "/v4/volumes/12345" {
			t.Errorf("expected path /v4/volumes/12345, got %s", r.URL.Path)
		}
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
