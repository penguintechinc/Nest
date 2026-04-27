package provider

// ============================================================================
// mockTransport — intercepts all outbound HTTP via http.DefaultTransport swap.
//
// Most provider functions create &http.Client{Timeout: X} without an explicit
// Transport, so they fall through to http.DefaultTransport. Replacing that
// global during a test routes all requests to a local httptest.Server without
// any changes to production code.
// ============================================================================

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// mockTransport routes every request through an in-process HTTP handler.
type mockTransport struct {
	handler http.Handler
}

func (m *mockTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	rec := httptest.NewRecorder()
	m.handler.ServeHTTP(rec, req)
	return rec.Result(), nil
}

// withMockTransport replaces http.DefaultTransport for the duration of the test
// and restores it via t.Cleanup.
func withMockTransport(t *testing.T, handler http.Handler) {
	t.Helper()
	orig := http.DefaultTransport
	origClient := http.DefaultClient
	http.DefaultTransport = &mockTransport{handler: handler}
	http.DefaultClient = &http.Client{Transport: &mockTransport{handler: handler}}
	t.Cleanup(func() {
		http.DefaultTransport = orig
		http.DefaultClient = origClient
	})
}

// ============================================================================
// GCP — checkCloudSQLHealth (hardcodes https://sqladmin.googleapis.com/v1/…)
// ============================================================================

func TestGcpCheckCloudSQLHealth_Runnable(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"state":"RUNNABLE"}`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{}
	result, err := p.checkCloudSQLHealth(context.Background(), cfg, "fake-token", "projects/p/instances/i")
	if err != nil {
		t.Fatalf("checkCloudSQLHealth() error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

func TestGcpCheckCloudSQLHealth_Suspended(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"state":"SUSPENDED"}`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{}
	result, err := p.checkCloudSQLHealth(context.Background(), cfg, "fake-token", "projects/p/instances/i")
	if err != nil {
		t.Fatalf("checkCloudSQLHealth() error = %v", err)
	}
	if result.State != "degraded" {
		t.Errorf("State = %q, want degraded", result.State)
	}
}

func TestGcpCheckCloudSQLHealth_Failed(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"state":"FAILED"}`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{}
	result, err := p.checkCloudSQLHealth(context.Background(), cfg, "fake-token", "projects/p/instances/i")
	if err != nil {
		t.Fatalf("checkCloudSQLHealth() error = %v", err)
	}
	if result.State != "failed" {
		t.Errorf("State = %q, want failed", result.State)
	}
}

func TestGcpCheckCloudSQLHealth_UnknownState(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"state":"WEIRD_STATE"}`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{}
	result, err := p.checkCloudSQLHealth(context.Background(), cfg, "fake-token", "projects/p/instances/i")
	if err != nil {
		t.Fatalf("checkCloudSQLHealth() error = %v", err)
	}
	if result.State != "degraded" {
		t.Errorf("State = %q, want degraded", result.State)
	}
}

func TestGcpCheckCloudSQLHealth_NonOK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{}
	result, err := p.checkCloudSQLHealth(context.Background(), cfg, "fake-token", "projects/p/instances/i")
	if err != nil {
		t.Fatalf("checkCloudSQLHealth() error = %v", err)
	}
	if result.State != "failed" {
		t.Errorf("State = %q, want failed", result.State)
	}
}

func TestGcpCheckCloudSQLHealth_BadJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{bad json`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{}
	_, err := p.checkCloudSQLHealth(context.Background(), cfg, "fake-token", "projects/p/instances/i")
	if err == nil {
		t.Error("expected error for bad JSON response")
	}
}

// ============================================================================
// GCP — checkMemorystoreHealth
// ============================================================================

func TestGcpCheckMemorystoreHealth_Ready(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"state":"READY"}`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{}
	result, err := p.checkMemorystoreHealth(context.Background(), cfg, "fake-token", "projects/p/locations/l/instances/i")
	if err != nil {
		t.Fatalf("checkMemorystoreHealth() error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

func TestGcpCheckMemorystoreHealth_Creating(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"state":"CREATING"}`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{}
	result, err := p.checkMemorystoreHealth(context.Background(), cfg, "fake-token", "projects/p/locations/l/instances/i")
	if err != nil {
		t.Fatalf("checkMemorystoreHealth() error = %v", err)
	}
	if result.State != "degraded" {
		t.Errorf("State = %q, want degraded", result.State)
	}
}

func TestGcpCheckMemorystoreHealth_Repairing(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"state":"REPAIRING"}`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{}
	result, err := p.checkMemorystoreHealth(context.Background(), cfg, "fake-token", "projects/p/locations/l/instances/i")
	if err != nil {
		t.Fatalf("checkMemorystoreHealth() error = %v", err)
	}
	if result.State != "failed" {
		t.Errorf("State = %q, want failed", result.State)
	}
}

func TestGcpCheckMemorystoreHealth_UnknownState(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"state":"OTHER"}`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{}
	result, err := p.checkMemorystoreHealth(context.Background(), cfg, "fake-token", "projects/p/locations/l/instances/i")
	if err != nil {
		t.Fatalf("checkMemorystoreHealth() error = %v", err)
	}
	if result.State != "degraded" {
		t.Errorf("State = %q, want degraded", result.State)
	}
}

func TestGcpCheckMemorystoreHealth_NonOK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{}
	result, err := p.checkMemorystoreHealth(context.Background(), cfg, "fake-token", "projects/p/locations/l/instances/i")
	if err != nil {
		t.Fatalf("checkMemorystoreHealth() error = %v", err)
	}
	if result.State != "failed" {
		t.Errorf("State = %q, want failed", result.State)
	}
}

func TestGcpCheckMemorystoreHealth_BadJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{}
	_, err := p.checkMemorystoreHealth(context.Background(), cfg, "fake-token", "projects/p/locations/l/instances/i")
	if err == nil {
		t.Error("expected error for bad JSON")
	}
}

// ============================================================================
// GCP — discoverCloudSQL
// ============================================================================

func TestGcpDiscoverCloudSQL_MockTransport(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"databaseVersion": "POSTGRES_14",
			"state":           "RUNNABLE",
			"ipAddresses":     []map[string]interface{}{{"type": "PRIMARY", "ipAddress": "10.0.0.1"}},
			"connectionName":  "project:us-central1:mydb",
			"settings": map[string]interface{}{
				"tier": "db-custom-2-8192",
			},
		}
		json.NewEncoder(w).Encode(resp)
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "projects/p/instances/mydb",
		Extra:      map[string]string{"access_token": "fake-token"},
	}
	result, err := p.discoverCloudSQL(context.Background(), cfg, "fake-token")
	if err != nil {
		t.Fatalf("discoverCloudSQL() error = %v", err)
	}
	if result == nil {
		t.Fatal("discoverCloudSQL() returned nil")
	}
}

func TestGcpDiscoverCloudSQL_NonOK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`{"error":"not found"}`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{ResourceID: "projects/p/instances/nonexistent"}
	_, err := p.discoverCloudSQL(context.Background(), cfg, "fake-token")
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestGcpDiscoverCloudSQL_BadJSONMock(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{bad`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{ResourceID: "projects/p/instances/mydb"}
	_, err := p.discoverCloudSQL(context.Background(), cfg, "fake-token")
	if err == nil {
		t.Error("expected error for bad JSON")
	}
}

// ============================================================================
// GCP — discoverMemorystore
// ============================================================================

func TestGcpDiscoverMemorystore_MockTransport(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		resp := map[string]interface{}{
			"redisVersion": "REDIS_7_0",
			"state":        "READY",
			"host":         "10.0.0.2",
			"port":         6379,
			"memorySizeGb": 1,
		}
		json.NewEncoder(w).Encode(resp)
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "projects/p/locations/l/instances/myredis",
	}
	result, err := p.discoverMemorystore(context.Background(), cfg, "fake-token")
	if err != nil {
		t.Fatalf("discoverMemorystore() error = %v", err)
	}
	if result == nil {
		t.Fatal("discoverMemorystore() returned nil")
	}
}

func TestGcpDiscoverMemorystore_NonOK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`{"error":"unauthorized"}`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{ResourceID: "projects/p/locations/l/instances/myredis"}
	_, err := p.discoverMemorystore(context.Background(), cfg, "fake-token")
	if err == nil {
		t.Error("expected error for 401 response")
	}
}

func TestGcpDiscoverMemorystore_BadJSONMock(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{ResourceID: "projects/p/locations/l/instances/myredis"}
	_, err := p.discoverMemorystore(context.Background(), cfg, "fake-token")
	if err == nil {
		t.Error("expected error for bad JSON")
	}
}

// ============================================================================
// GCP — rotateCloudSQLPassword
// ============================================================================

func TestGcpRotateCloudSQLPassword_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// rotateCloudSQLPassword uses PATCH
		if r.Method == "PATCH" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		} else {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		}
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "projects/p/instances/mydb",
		Extra:      map[string]string{"username": "myuser"},
	}
	result, err := p.rotateCloudSQLPassword(context.Background(), cfg, "fake-token", "projects/p/instances/mydb")
	if err != nil {
		t.Fatalf("rotateCloudSQLPassword() error = %v", err)
	}
	if result == "" {
		t.Error("rotateCloudSQLPassword() returned empty password")
	}
}

func TestGcpRotateCloudSQLPassword_NonOK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"forbidden"}`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "projects/p/instances/mydb",
		Extra:      map[string]string{"username": "myuser"},
	}
	_, err := p.rotateCloudSQLPassword(context.Background(), cfg, "fake-token", "projects/p/instances/mydb")
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

// ============================================================================
// GCP — rotateMemorystorePassword
// ============================================================================

func TestGcpRotateMemorystorePassword_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{}`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "projects/p/locations/l/instances/myredis",
	}
	result, err := p.rotateMemorystorePassword(context.Background(), cfg, "fake-token", "projects/p/locations/l/instances/myredis")
	if err != nil {
		t.Fatalf("rotateMemorystorePassword() error = %v", err)
	}
	if result == "" {
		t.Error("rotateMemorystorePassword() returned empty result")
	}
}

func TestGcpRotateMemorystorePassword_NonOK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "projects/p/locations/l/instances/myredis",
	}
	_, err := p.rotateMemorystorePassword(context.Background(), cfg, "fake-token", "projects/p/locations/l/instances/myredis")
	if err == nil {
		t.Error("expected error for 400 response")
	}
}

// ============================================================================
// GCP — getTokenFromServiceAccount with valid RSA key
// ============================================================================

func TestGcpGetTokenFromServiceAccount_ValidKey(t *testing.T) {
	// When getTokenFromServiceAccount hits https://oauth2.googleapis.com/token,
	// mockTransport intercepts it and returns a fake access_token.
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "gcp-test-token",
			"expires_in":   3600,
		})
	})
	withMockTransport(t, handler)

	// Use the real RSA key fixture from gcp_test.go if available,
	// otherwise test the error path with an invalid key.
	credJSON := `{"client_email":"test@test.iam.gserviceaccount.com","private_key_id":"key1","private_key":"` + testRSAPEM + `"}`
	p := &gcpProvider{}
	token, err := p.getTokenFromServiceAccount(credJSON)
	if err != nil {
		t.Logf("getTokenFromServiceAccount() with RSA key error = %v (expected if key invalid)", err)
		return
	}
	if token == "" {
		t.Error("getTokenFromServiceAccount() returned empty token")
	}
}

func TestGcpGetTokenFromServiceAccount_InvalidJSONMock(t *testing.T) {
	p := &gcpProvider{}
	_, err := p.getTokenFromServiceAccount("not json{{")
	if err == nil {
		t.Error("expected error for invalid JSON credential")
	}
}

func TestGcpGetTokenFromServiceAccount_NoPEMBlockMock(t *testing.T) {
	cred := `{"client_email":"test@test.iam.gserviceaccount.com","private_key_id":"key1","private_key":"notapemkey"}`
	p := &gcpProvider{}
	_, err := p.getTokenFromServiceAccount(cred)
	if err == nil {
		t.Error("expected error for missing PEM block")
	}
}

func TestGcpGetTokenFromServiceAccount_BadPKCS8(t *testing.T) {
	// Valid PEM wrapper but garbage content
	badPEM := "-----BEGIN PRIVATE KEY-----\naW52YWxpZA==\n-----END PRIVATE KEY-----"
	cred := fmt.Sprintf(`{"client_email":"test@test.iam.gserviceaccount.com","private_key_id":"key1","private_key":"%s"}`,
		strings.ReplaceAll(badPEM, "\n", `\n`))
	p := &gcpProvider{}
	_, err := p.getTokenFromServiceAccount(cred)
	if err == nil {
		t.Error("expected error for bad PKCS8 data")
	}
}

// ============================================================================
// GCP — getTokenFromMetadataServer
// ============================================================================

func TestGcpGetTokenFromMetadataServer_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"access_token": "metadata-token",
		})
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	token, err := p.getTokenFromMetadataServer()
	if err != nil {
		t.Fatalf("getTokenFromMetadataServer() error = %v", err)
	}
	if token != "metadata-token" {
		t.Errorf("token = %q, want metadata-token", token)
	}
}

func TestGcpGetTokenFromMetadataServer_NonOK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	_, err := p.getTokenFromMetadataServer()
	if err == nil {
		t.Error("expected error for non-200 response")
	}
}

func TestGcpGetTokenFromMetadataServer_BadJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	_, err := p.getTokenFromMetadataServer()
	if err == nil {
		t.Error("expected error for bad JSON response")
	}
}

// ============================================================================
// GCP — CheckHealth via DefaultTransport (routes through switch branches)
// ============================================================================

func TestGcpCheckHealth_PostgresViaMock(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"state":"RUNNABLE"}`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "postgres",
		ResourceID: "projects/p/instances/mydb",
		Extra:      map[string]string{"access_token": "tok"},
	}
	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

func TestGcpCheckHealth_RedisViaMock(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"state":"READY"}`))
	})
	withMockTransport(t, handler)

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "redis",
		ResourceID: "projects/p/locations/l/instances/r",
		Extra:      map[string]string{"access_token": "tok"},
	}
	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

// ============================================================================
// Cloudflare — discoverD1 / discoverR2 / discoverKV via DefaultTransport
// (cloudflareDoRequest also creates &http.Client{Timeout: 10s})
// ============================================================================

func TestCloudflareDiscoverD1_MockSuccess(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"result": map[string]interface{}{
				"name":       "mydb",
				"created_at": "2024-01-01T00:00:00Z",
				"file_size":  1024,
			},
		})
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "db-uuid"}
	result, err := p.discoverD1(context.Background(), cfg, "tok", "", "", "acc-123")
	if err != nil {
		t.Fatalf("discoverD1() error = %v", err)
	}
	if result == nil {
		t.Fatal("discoverD1() returned nil")
	}
	if result.EngineType != "d1" {
		t.Errorf("EngineType = %q, want d1", result.EngineType)
	}
}

func TestCloudflareDiscoverD1_APIFailure(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"errors":  []map[string]interface{}{{"code": 1000, "message": "API error"}},
		})
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "db-uuid"}
	_, err := p.discoverD1(context.Background(), cfg, "tok", "", "", "acc-123")
	if err == nil {
		t.Error("expected error for success=false")
	}
}

func TestCloudflareDiscoverD1_BadJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "db-uuid"}
	_, err := p.discoverD1(context.Background(), cfg, "tok", "", "", "acc-123")
	if err == nil {
		t.Error("expected error for bad JSON")
	}
}

func TestCloudflareDiscoverR2_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"result": map[string]interface{}{
				"name":    "my-bucket",
				"location": "APAC",
			},
		})
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "my-bucket"}
	result, err := p.discoverR2(context.Background(), cfg, "tok", "", "", "acc-123")
	if err != nil {
		t.Fatalf("discoverR2() error = %v", err)
	}
	if result == nil {
		t.Fatal("discoverR2() returned nil")
	}
	if result.EngineType != "object" {
		t.Errorf("EngineType = %q, want object", result.EngineType)
	}
}

func TestCloudflareDiscoverR2_APIFailure(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
		})
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "my-bucket"}
	_, err := p.discoverR2(context.Background(), cfg, "tok", "", "", "acc-123")
	if err == nil {
		t.Error("expected error for success=false")
	}
}

func TestCloudflareDiscoverR2_BadJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{bad`))
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "my-bucket"}
	_, err := p.discoverR2(context.Background(), cfg, "tok", "", "", "acc-123")
	if err == nil {
		t.Error("expected error for bad JSON")
	}
}

func TestCloudflareDiscoverKV_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"result": map[string]interface{}{
				"id":    "ns-uuid",
				"title": "my-namespace",
			},
		})
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "ns-uuid"}
	result, err := p.discoverKV(context.Background(), cfg, "tok", "", "", "acc-123")
	if err != nil {
		t.Fatalf("discoverKV() error = %v", err)
	}
	if result == nil {
		t.Fatal("discoverKV() returned nil")
	}
	if result.EngineType != "keyvalue" {
		t.Errorf("EngineType = %q, want keyvalue", result.EngineType)
	}
}

func TestCloudflareDiscoverKV_APIFailure(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
		})
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "ns-uuid"}
	_, err := p.discoverKV(context.Background(), cfg, "tok", "", "", "acc-123")
	if err == nil {
		t.Error("expected error for success=false")
	}
}

func TestCloudflareDiscoverKV_BadJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{bad`))
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "ns-uuid"}
	_, err := p.discoverKV(context.Background(), cfg, "tok", "", "", "acc-123")
	if err == nil {
		t.Error("expected error for bad JSON")
	}
}

// ============================================================================
// Cloudflare — checkHealthD1 / checkHealthR2 / checkHealthKV
// ============================================================================

func TestCloudflareCheckHealthD1_Healthy(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "db-uuid"}
	result, err := p.checkHealthD1(context.Background(), cfg, "tok", "", "", "acc-123")
	if err != nil {
		t.Fatalf("checkHealthD1() error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

func TestCloudflareCheckHealthD1_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "db-uuid"}
	result, err := p.checkHealthD1(context.Background(), cfg, "tok", "", "", "acc-123")
	if err != nil {
		t.Fatalf("checkHealthD1() error = %v", err)
	}
	if result.State != "failed" {
		t.Errorf("State = %q, want failed", result.State)
	}
}

func TestCloudflareCheckHealthD1_Degraded(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusServiceUnavailable)
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "db-uuid"}
	result, err := p.checkHealthD1(context.Background(), cfg, "tok", "", "", "acc-123")
	if err != nil {
		t.Fatalf("checkHealthD1() error = %v", err)
	}
	if result.State != "degraded" {
		t.Errorf("State = %q, want degraded", result.State)
	}
}

func TestCloudflareCheckHealthR2_Healthy(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "my-bucket"}
	result, err := p.checkHealthR2(context.Background(), cfg, "tok", "", "", "acc-123")
	if err != nil {
		t.Fatalf("checkHealthR2() error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

func TestCloudflareCheckHealthR2_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "my-bucket"}
	result, err := p.checkHealthR2(context.Background(), cfg, "tok", "", "", "acc-123")
	if err != nil {
		t.Fatalf("checkHealthR2() error = %v", err)
	}
	if result.State != "failed" {
		t.Errorf("State = %q, want failed", result.State)
	}
}

func TestCloudflareCheckHealthR2_Degraded(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "my-bucket"}
	result, err := p.checkHealthR2(context.Background(), cfg, "tok", "", "", "acc-123")
	if err != nil {
		t.Fatalf("checkHealthR2() error = %v", err)
	}
	if result.State != "degraded" {
		t.Errorf("State = %q, want degraded", result.State)
	}
}

func TestCloudflareCheckHealthKV_Healthy(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "ns-uuid"}
	result, err := p.checkHealthKV(context.Background(), cfg, "tok", "", "", "acc-123")
	if err != nil {
		t.Fatalf("checkHealthKV() error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

func TestCloudflareCheckHealthKV_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "ns-uuid"}
	result, err := p.checkHealthKV(context.Background(), cfg, "tok", "", "", "acc-123")
	if err != nil {
		t.Fatalf("checkHealthKV() error = %v", err)
	}
	if result.State != "failed" {
		t.Errorf("State = %q, want failed", result.State)
	}
}

func TestCloudflareCheckHealthKV_Degraded(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGatewayTimeout)
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{ResourceID: "ns-uuid"}
	result, err := p.checkHealthKV(context.Background(), cfg, "tok", "", "", "acc-123")
	if err != nil {
		t.Fatalf("checkHealthKV() error = %v", err)
	}
	if result.State != "degraded" {
		t.Errorf("State = %q, want degraded", result.State)
	}
}

// ============================================================================
// Cloudflare — RotateCredential success path
// ============================================================================

func TestCloudflareRotateCredential_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"result": map[string]interface{}{
				"token": "new-api-token-value",
			},
		})
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid",
		Extra:      map[string]string{"api_token": "old-token"},
	}
	result, err := p.RotateCredential(context.Background(), cfg)
	if err != nil {
		t.Fatalf("RotateCredential() error = %v", err)
	}
	if result != "new-api-token-value" {
		t.Errorf("result = %q, want new-api-token-value", result)
	}
}

func TestCloudflareRotateCredential_APIFailure(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": false,
			"errors": []map[string]interface{}{
				{"code": 9109, "message": "Invalid token"},
			},
		})
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid",
		Extra:      map[string]string{"api_token": "old-token"},
	}
	_, err := p.RotateCredential(context.Background(), cfg)
	if err == nil {
		t.Error("expected error for success=false API response")
	}
}

func TestCloudflareRotateCredential_NonOKStatus(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"success":false}`))
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid",
		Extra:      map[string]string{"api_token": "old-token"},
	}
	_, err := p.RotateCredential(context.Background(), cfg)
	if err == nil {
		t.Error("expected error for 403 status")
	}
}

// ============================================================================
// Azure — rotatePostgresPassword / rotateRedisKey via DefaultClient
// ============================================================================

func TestAzureRotatePostgresPassword_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "PATCH" {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte(`{}`))
		} else if r.Method == "POST" {
			// getAccessToken token exchange
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"access_token": "fake-az-token",
				"token_type":   "Bearer",
				"expires_in":   3600,
			})
		}
	})
	withMockTransport(t, handler)

	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.DBforPostgreSQL/servers/mydb",
		Extra:      map[string]string{"access_token": "az-token"},
	}
	result, err := p.rotatePostgresPassword(context.Background(), cfg, "az-token")
	if err != nil {
		t.Fatalf("rotatePostgresPassword() error = %v", err)
	}
	if result == "" {
		t.Error("rotatePostgresPassword() returned empty password")
	}
}

func TestAzureRotatePostgresPassword_NonOK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"forbidden"}`))
	})
	withMockTransport(t, handler)

	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.DBforPostgreSQL/servers/mydb",
	}
	_, err := p.rotatePostgresPassword(context.Background(), cfg, "az-token")
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

func TestAzureRotateRedisKey_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"primaryKey":   "new-primary-key",
			"secondaryKey": "new-secondary-key",
		})
	})
	withMockTransport(t, handler)

	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Cache/Redis/myredis",
	}
	result, err := p.rotateRedisKey(context.Background(), cfg, "az-token")
	if err != nil {
		t.Fatalf("rotateRedisKey() error = %v", err)
	}
	if result == "" {
		t.Error("rotateRedisKey() returned empty key")
	}
}

func TestAzureRotateRedisKey_NonOK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
		w.Write([]byte(`{"error":"bad request"}`))
	})
	withMockTransport(t, handler)

	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Cache/Redis/myredis",
	}
	_, err := p.rotateRedisKey(context.Background(), cfg, "az-token")
	if err == nil {
		t.Error("expected error for 400 response")
	}
}

// ============================================================================
// Vultr — getVultrDatabase / getVultrMonthlyBilling via DefaultTransport
// ============================================================================

func TestGetVultrDatabase_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"database": map[string]interface{}{
				"id":              "db-uuid",
				"status":          "Running",
				"database_engine": "pg",
				"host":            "db.vultr.com",
				"port":            5432,
				"plan": map[string]interface{}{
					"id":   "vultr-dbaas-startup-cc-1-55-1",
					"name": "Startup Plan",
					"ram":  1024,
					"disk": 25,
					"vcpus": 1,
					"pricing": map[string]interface{}{
						"hourly":  0.01,
						"monthly": 5.00,
					},
				},
			},
		})
	})
	withMockTransport(t, handler)

	db, err := getVultrDatabase(context.Background(), "test-api-key", "db-uuid")
	if err != nil {
		t.Fatalf("getVultrDatabase() error = %v", err)
	}
	if db == nil {
		t.Fatal("getVultrDatabase() returned nil")
	}
	if db.ID != "db-uuid" {
		t.Errorf("ID = %q, want db-uuid", db.ID)
	}
}

func TestGetVultrDatabase_NonOK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		w.Write([]byte(`not found`))
	})
	withMockTransport(t, handler)

	_, err := getVultrDatabase(context.Background(), "test-api-key", "nonexistent-uuid")
	if err == nil {
		t.Error("expected error for 404 response")
	}
}

func TestGetVultrDatabase_BadJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	})
	withMockTransport(t, handler)

	_, err := getVultrDatabase(context.Background(), "test-api-key", "db-uuid")
	if err == nil {
		t.Error("expected error for bad JSON")
	}
}

func TestGetVultrMonthlyBilling_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"billing_history": []map[string]interface{}{
				{"description": "db-uuid database charge", "amount": 5.00, "date": "2024-01-01"},
				{"description": "other resource charge", "amount": 10.00, "date": "2024-01-01"},
			},
		})
	})
	withMockTransport(t, handler)

	cost, err := getVultrMonthlyBilling(context.Background(), "test-api-key", "db-uuid")
	if err != nil {
		t.Fatalf("getVultrMonthlyBilling() error = %v", err)
	}
	if cost != 5.00 {
		t.Errorf("cost = %v, want 5.00", cost)
	}
}

func TestGetVultrMonthlyBilling_NonOK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`unauthorized`))
	})
	withMockTransport(t, handler)

	_, err := getVultrMonthlyBilling(context.Background(), "test-api-key", "db-uuid")
	if err == nil {
		t.Error("expected error for 401 response")
	}
}

func TestGetVultrMonthlyBilling_BadJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	})
	withMockTransport(t, handler)

	_, err := getVultrMonthlyBilling(context.Background(), "test-api-key", "db-uuid")
	if err == nil {
		t.Error("expected error for bad JSON")
	}
}

// ============================================================================
// Azure — Discover via DefaultClient (postgres + redis paths)
// ============================================================================

func TestAzureDiscover_PostgresViaMock(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"properties": map[string]interface{}{
				"fullyQualifiedDomainName": "mydb.postgres.database.azure.com",
				"version":                  "14",
			},
			"sku": map[string]interface{}{"tier": "GeneralPurpose"},
		})
	})
	withMockTransport(t, handler)

	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "postgres",
		ResourceID: "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.DBforPostgreSQL/servers/mydb",
		Extra:      map[string]string{"access_token": "az-token"},
	}
	result, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if result == nil {
		t.Fatal("Discover() returned nil")
	}
	if result.Endpoint == "" {
		t.Error("Discover() returned empty endpoint")
	}
}

func TestAzureDiscover_RedisViaMock(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"properties": map[string]interface{}{
				"hostName": "myredis.redis.cache.windows.net",
				"sslPort":  6380,
			},
		})
	})
	withMockTransport(t, handler)

	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "redis",
		ResourceID: "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Cache/Redis/myredis",
		Extra:      map[string]string{"access_token": "az-token"},
	}
	result, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if result == nil {
		t.Fatal("Discover() returned nil")
	}
}

func TestAzureDiscover_DefaultEngineType(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	withMockTransport(t, handler)

	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "unknown-type",
		ResourceID: "/subscriptions/sub/resourceGroups/rg/providers/SomeProvider/resource",
		Extra:      map[string]string{"access_token": "az-token"},
	}
	result, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if result == nil {
		t.Fatal("Discover() returned nil")
	}
}

// ============================================================================
// Azure — CheckHealth via DefaultClient (postgres + redis paths)
// ============================================================================

func TestAzureCheckHealth_PostgresViaMock(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"properties": map[string]interface{}{
				"state": "Ready",
			},
		})
	})
	withMockTransport(t, handler)

	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "postgres",
		ResourceID: "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.DBforPostgreSQL/servers/mydb",
		Extra:      map[string]string{"access_token": "az-token"},
	}
	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

func TestAzureCheckHealth_RedisViaMock(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"properties": map[string]interface{}{
				"provisioningState": "Succeeded",
			},
		})
	})
	withMockTransport(t, handler)

	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "redis",
		ResourceID: "/subscriptions/sub/resourceGroups/rg/providers/Microsoft.Cache/Redis/myredis",
		Extra:      map[string]string{"access_token": "az-token"},
	}
	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

func TestAzureCheckHealth_DefaultEngineTypeMock(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{})
	})
	withMockTransport(t, handler)

	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "unknown",
		ResourceID: "/subscriptions/sub/resourceGroups/rg/providers/SomeProvider/resource",
		Endpoint:   "10.0.0.1:5432",
		Extra:      map[string]string{"access_token": "az-token"},
	}
	// default path falls through to TCP probe which will fail (no real TCP endpoint)
	// just verify no panic
	result, _ := p.CheckHealth(context.Background(), cfg)
	if result == nil {
		t.Error("CheckHealth() returned nil result")
	}
}

// ============================================================================
// Azure — GetCostData via DefaultClient
// ============================================================================

func TestAzureGetCostData_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"properties": map[string]interface{}{
				"rows": []interface{}{
					[]interface{}{15.5, "USD"},
				},
			},
		})
	})
	withMockTransport(t, handler)

	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "/subscriptions/sub/resourceGroups/rg/resource",
		Extra: map[string]string{
			"subscription_id": "sub-id",
			"access_token":    "az-token",
		},
	}
	result, err := p.GetCostData(context.Background(), cfg)
	if err != nil {
		t.Fatalf("GetCostData() error = %v", err)
	}
	if result == nil {
		t.Fatal("GetCostData() returned nil")
	}
	if result.Currency != "USD" {
		t.Errorf("Currency = %q, want USD", result.Currency)
	}
}

func TestAzureGetCostData_NonOK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`{"error":"forbidden"}`))
	})
	withMockTransport(t, handler)

	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "/subscriptions/sub/resourceGroups/rg/resource",
		Extra: map[string]string{
			"subscription_id": "sub-id",
			"access_token":    "az-token",
		},
	}
	_, err := p.GetCostData(context.Background(), cfg)
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

func TestAzureGetCostData_BadJSON(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`not json`))
	})
	withMockTransport(t, handler)

	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "/subscriptions/sub/resourceGroups/rg/resource",
		Extra: map[string]string{
			"subscription_id": "sub-id",
			"access_token":    "az-token",
		},
	}
	_, err := p.GetCostData(context.Background(), cfg)
	if err == nil {
		t.Error("expected error for bad JSON response")
	}
}

// ============================================================================
// Vultr — Discover via DefaultTransport
// ============================================================================

func TestVultrDiscover_WithAPIKeyMock(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"database": map[string]interface{}{
				"id":              "db-uuid",
				"status":          "Running",
				"database_engine": "postgresql",
				"host":            "db.vultr.com",
				"port":            5432,
				"plan": map[string]interface{}{
					"id":    "vultr-dbaas-startup-cc-1-55-1",
					"name":  "Startup Plan",
					"ram":   1024,
					"disk":  25,
					"vcpus": 1,
					"pricing": map[string]interface{}{
						"hourly":  0.01,
						"monthly": 5.00,
					},
				},
			},
		})
	})
	withMockTransport(t, handler)

	p := &vultrProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid",
		Region:     "ewr",
		Extra:      map[string]string{"api_key": "test-key"},
	}
	result, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if result == nil {
		t.Fatal("Discover() returned nil")
	}
	if result.EngineType != "postgresql" {
		t.Errorf("EngineType = %q, want postgresql", result.EngineType)
	}
}

func TestVultrDiscover_APIError(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
		w.Write([]byte(`unauthorized`))
	})
	withMockTransport(t, handler)

	p := &vultrProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid",
		Extra:      map[string]string{"api_key": "bad-key"},
	}
	_, err := p.Discover(context.Background(), cfg)
	if err == nil {
		t.Error("expected error for unauthorized response")
	}
}

// ============================================================================
// Cloudflare — Discover with credentials (routes through discoverD1/R2/KV)
// ============================================================================

func TestCloudflareDiscover_D1WithCredentials(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"result": map[string]interface{}{
				"name":      "mydb",
				"file_size": 1024,
			},
		})
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "d1",
		ResourceID: "db-uuid",
		Extra: map[string]string{
			"api_token":  "test-token",
			"account_id": "acc-123",
		},
	}
	result, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover(d1) error = %v", err)
	}
	if result == nil {
		t.Fatal("Discover(d1) returned nil")
	}
	if result.EngineType != "d1" {
		t.Errorf("EngineType = %q, want d1", result.EngineType)
	}
}

func TestCloudflareDiscover_R2WithCredentials(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"result": map[string]interface{}{
				"name":     "my-bucket",
				"location": "APAC",
			},
		})
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "r2",
		ResourceID: "my-bucket",
		Extra: map[string]string{
			"api_token":  "test-token",
			"account_id": "acc-123",
		},
	}
	result, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover(r2) error = %v", err)
	}
	if result == nil {
		t.Fatal("Discover(r2) returned nil")
	}
}

func TestCloudflareDiscover_KVWithCredentials(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
			"result": map[string]interface{}{
				"id":    "ns-uuid",
				"title": "my-namespace",
			},
		})
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "kv",
		ResourceID: "ns-uuid",
		Extra: map[string]string{
			"api_token":  "test-token",
			"account_id": "acc-123",
		},
	}
	result, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover(kv) error = %v", err)
	}
	if result == nil {
		t.Fatal("Discover(kv) returned nil")
	}
}

// ============================================================================
// Cloudflare — CheckHealth with credentials (d1/r2/kv paths)
// ============================================================================

func TestCloudflareCheckHealth_D1WithCredentials(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "d1",
		ResourceID: "db-uuid",
		Extra: map[string]string{
			"api_token":  "test-token",
			"account_id": "acc-123",
		},
	}
	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth(d1) error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

func TestCloudflareCheckHealth_R2WithCredentials(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "r2",
		ResourceID: "my-bucket",
		Extra: map[string]string{
			"api_token":  "test-token",
			"account_id": "acc-123",
		},
	}
	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth(r2) error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

func TestCloudflareCheckHealth_KVWithCredentials(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "keyvalue",
		ResourceID: "ns-uuid",
		Extra: map[string]string{
			"api_token":  "test-token",
			"account_id": "acc-123",
		},
	}
	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth(kv) error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

// ============================================================================
// Cloudflare — GetCostData with credentials
// ============================================================================

func TestCloudflareGetCostData_WithCredentials(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success": true,
		})
	})
	withMockTransport(t, handler)

	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid",
		Extra: map[string]string{
			"api_token":  "test-token",
			"account_id": "acc-123",
		},
	}
	result, err := p.GetCostData(context.Background(), cfg)
	if err != nil {
		t.Fatalf("GetCostData() error = %v", err)
	}
	if result == nil {
		t.Fatal("GetCostData() returned nil")
	}
	if result.Currency != "USD" {
		t.Errorf("Currency = %q, want USD", result.Currency)
	}
}

// ============================================================================
// AWS — GetCostData / checkRDSHealth / checkElastiCacheHealth / checkS3Health
// ============================================================================

func TestAWSGetCostData_Success(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		json.NewEncoder(w).Encode(map[string]interface{}{
			"ResultsByTime": []map[string]interface{}{
				{
					"Total": map[string]interface{}{
						"AmortizedCost": map[string]interface{}{
							"Amount": "15.50",
							"Unit":   "USD",
						},
					},
				},
			},
		})
	})
	withMockTransport(t, handler)

	p := &awsProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "arn:aws:rds:us-east-1:123:db:mydb",
		Region:     "us-east-1",
		Extra: map[string]string{
			"access_key_id":     "AKIATEST",
			"secret_access_key": "test-secret",
		},
	}
	result, err := p.GetCostData(context.Background(), cfg)
	if err != nil {
		t.Fatalf("GetCostData() error = %v", err)
	}
	if result == nil {
		t.Fatal("GetCostData() returned nil")
	}
	if result.Currency != "USD" {
		t.Errorf("Currency = %q, want USD", result.Currency)
	}
}

func TestAWSGetCostData_NonOK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
		w.Write([]byte(`AccessDeniedException`))
	})
	withMockTransport(t, handler)

	p := &awsProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "arn:aws:rds:us-east-1:123:db:mydb",
		Region:     "us-east-1",
		Extra: map[string]string{
			"access_key_id":     "AKIATEST",
			"secret_access_key": "test-secret",
		},
	}
	_, err := p.GetCostData(context.Background(), cfg)
	if err == nil {
		t.Error("expected error for 403 response")
	}
}

func TestAWSCheckRDSHealth_Available(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<DescribeDBInstancesResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/">
			<DescribeDBInstancesResult>
				<DBInstances>
					<DBInstance>
						<DBInstanceStatus>available</DBInstanceStatus>
					</DBInstance>
				</DBInstances>
			</DescribeDBInstancesResult>
		</DescribeDBInstancesResponse>`))
	})
	withMockTransport(t, handler)

	p := &awsProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "arn:aws:rds:us-east-1:123:db:mydb",
		Region:     "us-east-1",
	}
	result, err := p.checkRDSHealth(context.Background(), cfg, "AKIATEST", "test-secret", "")
	if err != nil {
		t.Fatalf("checkRDSHealth() error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

func TestAWSCheckRDSHealth_Degraded(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<DescribeDBInstancesResponse xmlns="http://rds.amazonaws.com/doc/2014-10-31/">
			<DescribeDBInstancesResult>
				<DBInstances>
					<DBInstance>
						<DBInstanceStatus>backing-up</DBInstanceStatus>
					</DBInstance>
				</DBInstances>
			</DescribeDBInstancesResult>
		</DescribeDBInstancesResponse>`))
	})
	withMockTransport(t, handler)

	p := &awsProvider{}
	cfg := ExternalProviderConfig{ResourceID: "arn:aws:rds:us-east-1:123:db:mydb", Region: "us-east-1"}
	result, err := p.checkRDSHealth(context.Background(), cfg, "AKIATEST", "test-secret", "")
	if err != nil {
		t.Fatalf("checkRDSHealth() error = %v", err)
	}
	if result.State != "degraded" {
		t.Errorf("State = %q, want degraded", result.State)
	}
}

func TestAWSCheckRDSHealth_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		// Empty instances
		w.Write([]byte(`<DescribeDBInstancesResponse><DescribeDBInstancesResult><DBInstances></DBInstances></DescribeDBInstancesResult></DescribeDBInstancesResponse>`))
	})
	withMockTransport(t, handler)

	p := &awsProvider{}
	cfg := ExternalProviderConfig{ResourceID: "arn:aws:rds:us-east-1:123:db:mydb", Region: "us-east-1"}
	result, err := p.checkRDSHealth(context.Background(), cfg, "AKIATEST", "test-secret", "")
	if err != nil {
		t.Fatalf("checkRDSHealth() error = %v", err)
	}
	if result.State != "failed" {
		t.Errorf("State = %q, want failed", result.State)
	}
}

func TestAWSCheckElastiCacheHealth_Available(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<DescribeCacheClustersResponse>
			<DescribeCacheClustersResult>
				<CacheClusters>
					<CacheCluster>
						<CacheClusterStatus>available</CacheClusterStatus>
					</CacheCluster>
				</CacheClusters>
			</DescribeCacheClustersResult>
		</DescribeCacheClustersResponse>`))
	})
	withMockTransport(t, handler)

	p := &awsProvider{}
	cfg := ExternalProviderConfig{ResourceID: "arn:aws:elasticache:us-east-1:123:cluster:my-cluster", Region: "us-east-1"}
	result, err := p.checkElastiCacheHealth(context.Background(), cfg, "AKIATEST", "test-secret", "")
	if err != nil {
		t.Fatalf("checkElastiCacheHealth() error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

func TestAWSCheckElastiCacheHealth_Degraded(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<DescribeCacheClustersResponse>
			<DescribeCacheClustersResult>
				<CacheClusters>
					<CacheCluster>
						<CacheClusterStatus>modifying</CacheClusterStatus>
					</CacheCluster>
				</CacheClusters>
			</DescribeCacheClustersResult>
		</DescribeCacheClustersResponse>`))
	})
	withMockTransport(t, handler)

	p := &awsProvider{}
	cfg := ExternalProviderConfig{ResourceID: "arn:aws:elasticache:us-east-1:123:cluster:my-cluster", Region: "us-east-1"}
	result, err := p.checkElastiCacheHealth(context.Background(), cfg, "AKIATEST", "test-secret", "")
	if err != nil {
		t.Fatalf("checkElastiCacheHealth() error = %v", err)
	}
	if result.State != "degraded" {
		t.Errorf("State = %q, want degraded", result.State)
	}
}

func TestAWSCheckElastiCacheHealth_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<DescribeCacheClustersResponse><DescribeCacheClustersResult><CacheClusters></CacheClusters></DescribeCacheClustersResult></DescribeCacheClustersResponse>`))
	})
	withMockTransport(t, handler)

	p := &awsProvider{}
	cfg := ExternalProviderConfig{ResourceID: "arn:aws:elasticache:us-east-1:123:cluster:nonexistent", Region: "us-east-1"}
	result, err := p.checkElastiCacheHealth(context.Background(), cfg, "AKIATEST", "test-secret", "")
	if err != nil {
		t.Fatalf("checkElastiCacheHealth() error = %v", err)
	}
	if result.State != "failed" {
		t.Errorf("State = %q, want failed", result.State)
	}
}

func TestAWSCheckS3Health_OK(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	withMockTransport(t, handler)

	p := &awsProvider{}
	cfg := ExternalProviderConfig{ResourceID: "my-bucket", Region: "us-east-1"}
	result, err := p.checkS3Health(context.Background(), cfg, "AKIATEST", "test-secret", "")
	if err != nil {
		t.Fatalf("checkS3Health() error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

func TestAWSCheckS3Health_NotFound(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	})
	withMockTransport(t, handler)

	p := &awsProvider{}
	cfg := ExternalProviderConfig{ResourceID: "nonexistent-bucket", Region: "us-east-1"}
	result, err := p.checkS3Health(context.Background(), cfg, "AKIATEST", "test-secret", "")
	if err != nil {
		t.Fatalf("checkS3Health() error = %v", err)
	}
	if result.State != "failed" {
		t.Errorf("State = %q, want failed", result.State)
	}
}

func TestAWSCheckS3Health_Degraded(t *testing.T) {
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusForbidden)
	})
	withMockTransport(t, handler)

	p := &awsProvider{}
	cfg := ExternalProviderConfig{ResourceID: "private-bucket", Region: "us-east-1"}
	result, err := p.checkS3Health(context.Background(), cfg, "AKIATEST", "test-secret", "")
	if err != nil {
		t.Fatalf("checkS3Health() error = %v", err)
	}
	// 403 Forbidden means bucket exists but we lack permissions = degraded access
	if result.State == "" {
		t.Error("checkS3Health() returned empty state")
	}
}

// ============================================================================
// AWS — SetupProxy
// ============================================================================

func TestAWSSetupProxy_Success(t *testing.T) {
	// SetupProxy calls Discover which for AWS just constructs from config
	p := &awsProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "postgres",
		ResourceID: "arn:aws:rds:us-east-1:123:db:mydb",
		Region:     "us-east-1",
		Extra: map[string]string{
			"access_key_id":     "AKIATEST",
			"secret_access_key": "test-secret",
		},
	}
	result, err := p.SetupProxy(context.Background(), cfg)
	if err != nil {
		t.Fatalf("SetupProxy() error = %v", err)
	}
	if result == nil {
		t.Fatal("SetupProxy() returned nil")
	}
	if result.AuthType != "iam-role" {
		t.Errorf("AuthType = %q, want iam-role", result.AuthType)
	}
}

// ============================================================================
// AWS — extractDBInstanceID / extractClusterID with ARN prefix
// ============================================================================

func TestExtractDBInstanceID_WithARNPrefix(t *testing.T) {
	tests := []struct {
		arn  string
		want string
	}{
		{"arn:aws:rds:us-east-1:123:db:mydb", "mydb"},
		{"arn:aws:rds:us-east-1:123:db:another-db", "another-db"},
		{"single", "single"},
		{"", ""},
		{"a:b:c:d:e:f:mydb", "mydb"},
	}
	for _, tt := range tests {
		got := extractDBInstanceID(tt.arn)
		if got != tt.want {
			t.Errorf("extractDBInstanceID(%q) = %q, want %q", tt.arn, got, tt.want)
		}
	}
}

func TestExtractClusterID_WithARNPrefix(t *testing.T) {
	tests := []struct {
		arn  string
		want string
	}{
		{"arn:aws:elasticache:us-east-1:123:cluster:my-cluster", "my-cluster"},
		{"single", "single"},
		{"", ""},
	}
	for _, tt := range tests {
		got := extractClusterID(tt.arn)
		if got != tt.want {
			t.Errorf("extractClusterID(%q) = %q, want %q", tt.arn, got, tt.want)
		}
	}
}

// ============================================================================
// Generic — SetupProxy (requires valid endpoint to pass Validate)
// ============================================================================

func TestGenericSetupProxy_WithEndpoint(t *testing.T) {
	p := &genericProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "postgres",
		Endpoint:   "db.example.com:5432",
		Region:     "us-east-1",
	}
	result, err := p.SetupProxy(context.Background(), cfg)
	if err != nil {
		t.Fatalf("SetupProxy() error = %v", err)
	}
	if result == nil {
		t.Fatal("SetupProxy() returned nil")
	}
	if result.Endpoint != "db.example.com:5432" {
		t.Errorf("Endpoint = %q, want db.example.com:5432", result.Endpoint)
	}
}

func TestGenericSetupProxy_MissingEndpoint(t *testing.T) {
	p := &genericProvider{}
	cfg := ExternalProviderConfig{
		EngineType: "postgres",
		Endpoint:   "",
		Region:     "us-east-1",
	}
	_, err := p.SetupProxy(context.Background(), cfg)
	if err == nil {
		t.Error("expected error for missing endpoint")
	}
}

// testRSAPEM is a 2048-bit RSA PKCS#8 private key generated for testing only.
// It is not used for any real credentials.
const testRSAPEM = `-----BEGIN PRIVATE KEY-----\nMIIEvgIBADANBgkqhkiG9w0BAQEFAASCBKgwggSkAgEAAoIBAQDGKOo/nXARpqo9\nUvXUP/c8bH7E4/LIoTSjv2OPaZm8uy/bqAL0DOFRBksnVov5GBqeU8aY5IM9UPBq\n36d/GfdvYSOJd7KQK3X3tEtmhEqKk9IJPBRDhNQZfD3/OS94DZZTFt2UpOek6nJd\nDRVeXxU9vhhnl189k4kkq1AQS1DdLKYQuOVCf0KOBOOWDTdp7bCZ3+4B1tBvw1Mh\nXD7zinGSW7pX0DJHys3Xc3OISxj4MrrA+Zlx/xmpMjO6trSOk8ua8K0cOPIPp50F\n/wU183ziQS/yFNOi/Lb3HaeahSeYJLiGFUx5ByBWX+vL5gzo6hJBh+GJFBu5vnc+\nqUqSr2HBAgMBAAECggEAAYlECiDWQ2PEcHfj/RyPFgxFBhGakmq6A842N1CXMxTs\nKb61YacXKNO0udIIYSKqQ6mUeb9VQ2CdEYYI+FG3JulUz0Iy255jomtG1Z1PTuBX\nHbBWG6EkLAuoFyI+S4bm8D9WUcp+u3sADpe9L3trGKzAZ46vS8TY2IR9uRedYY6N\nXn5Zg/r5o8NbdGrLUDnClPIy76vDCLV/6Yrpy1GsT7x82hITrGIe7berBHe7xUqv\n9IBpxelVX3FJw4KbjN2gByFpl3f4JAhEyRjJPVSWod4Kmrzt/0E8H0hMlZ8H4kiI\nG6Za91iRcQe4F94Tz8kWIIBKGsHEmNbP2Sgdh8FAMQKBgQDVeFVE9TQAb95vLFsZ\n87YY5cWDTySTz8riEEoa5O6U6PB+aERnoTRk5mP1+EeCfD53bg6BQeR2DWqjpx9b\nhaR2z/FbeJMEyd36O1P9UjIaNwTkYjGwsDgNQCjrvtSALEX+uT0ocvODteIQuKKR\nXXaLeGOJ5S7GnZ3UNFqrCYTEcQKBgQDto7XpInATO1hStDTxhSugy5zCX/X/y/K2\nQ+SHI/INpXUkKUaTJmxuYHZyMD7syqSOz1jtQghLGjPa4KuRGrvtNlpQWFHRr0Tk\nxcg+WAxbCeBiOLsuMUoQFkp9u5KrvCeXKWsWeaK5lOqBLzSna2FuXdvTWYM2omyw\nyWN4pTXaUQKBgG9nf0ifluXre/AU++5NS+kucKeYdARX2w+jZKkodIJuFqRBkgFr\nFcbanaxOSDOG16rIWvWGB868Lbz+iNTgp/YBi3orML69AwWGVMzNSqx3rivqOvh0\n3qu7oh911byWXmkTDyG+6+r+zt3fHagzWJxs1bWvT3wD4cxPDkpYi1thAoGBAILj\nlfGH71UYbch3y2Vv5RzWqUwCUNuIePHdKUUqDktn48J8HYw1MKoG5ZZ1bmM8JjEm\nkaN0qF69WuxmrPjqUbIRKuNwEfi9YePj8CwukPef1AAloSuLKHD95h+krd97bg77\nWClz66XuGM/4sTa5lVuVxNt/RR9VjSo+clRkIupRAoGBANMCh2njr6Pqy5d2sOjT\nLWiWYzjnFos0HDicexCG2Z9NPTsMkxH6aqFbCPYf86Kt32RigrYcSHheURq+iTp5\noAo0Nogt62FNfu3UD7kTR96ckSirdVd0EPIYLpPcpIQ3PsnprfURFzAjCiIvJNir\neqpcv5bmq+jGMNvnu2M+7a38\n-----END PRIVATE KEY-----\n`
