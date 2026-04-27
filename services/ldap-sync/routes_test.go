package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestHealthz(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/healthz")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}
}

func TestGetUsersNoLicense(t *testing.T) {
	// Ensure ENTERPRISE_LICENSE is not set
	os.Unsetenv("ENTERPRISE_LICENSE")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/users")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("Expected 402, got %d", resp.StatusCode)
	}
}

func TestGetUsersWithLicense(t *testing.T) {
	// Set license environment variable
	t.Setenv("ENTERPRISE_LICENSE", "valid-license-key")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/users")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	// Check Content-Type header
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected application/json, got %s", contentType)
	}

	// Check response is valid JSON array
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	bodyStr := string(bodyBytes)
	if !strings.HasPrefix(bodyStr, "[") || !strings.HasSuffix(bodyStr, "]") {
		t.Errorf("Response should be a JSON array, got: %s", bodyStr)
	}
}

func TestGetUserByIDNoLicense(t *testing.T) {
	// Ensure ENTERPRISE_LICENSE is not set
	os.Unsetenv("ENTERPRISE_LICENSE")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/users/nonexistent")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("Expected 402, got %d", resp.StatusCode)
	}
}

func TestGetUserByIDNotFound(t *testing.T) {
	// Set license environment variable
	t.Setenv("ENTERPRISE_LICENSE", "valid-license-key")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/users/nonexistent")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", resp.StatusCode)
	}
}

func TestPostSyncNoLicense(t *testing.T) {
	// Ensure ENTERPRISE_LICENSE is not set
	os.Unsetenv("ENTERPRISE_LICENSE")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/sync", "application/json", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("Expected 402, got %d", resp.StatusCode)
	}
}

func TestPostSyncWithLicense(t *testing.T) {
	// Set license environment variable
	t.Setenv("ENTERPRISE_LICENSE", "valid-license-key")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/sync", "application/json", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	// Check Content-Type header
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected application/json, got %s", contentType)
	}

	// Check response body contains status
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	bodyStr := string(bodyBytes)
	if !strings.Contains(bodyStr, "syncing") {
		t.Errorf("Response should contain 'syncing', got: %s", bodyStr)
	}
}

func TestPostUsersMethodNotAllowed(t *testing.T) {
	// Set license environment variable
	t.Setenv("ENTERPRISE_LICENSE", "valid-license-key")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/users", "application/json", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", resp.StatusCode)
	}
}

func TestGetSyncMethodNotAllowed(t *testing.T) {
	// Set license environment variable
	t.Setenv("ENTERPRISE_LICENSE", "valid-license-key")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/sync")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", resp.StatusCode)
	}
}

func TestGetUserByIDWithLicense(t *testing.T) {
	// Set license environment variable
	t.Setenv("ENTERPRISE_LICENSE", "valid-license-key")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	// Populate stub users manually
	syncer.populateStubUsers()

	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/users/admin")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	// Check Content-Type header
	contentType := resp.Header.Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Expected application/json, got %s", contentType)
	}

	// Check response body
	bodyBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}

	bodyStr := string(bodyBytes)
	if !strings.Contains(bodyStr, "admin") {
		t.Errorf("Response should contain 'admin', got: %s", bodyStr)
	}
}

func TestDeleteUserMethodNotAllowed(t *testing.T) {
	// Set license environment variable
	t.Setenv("ENTERPRISE_LICENSE", "valid-license-key")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("DELETE", srv.URL+"/api/v1/users/admin", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", resp.StatusCode)
	}
}

func TestPutSyncMethodNotAllowed(t *testing.T) {
	// Set license environment variable
	t.Setenv("ENTERPRISE_LICENSE", "valid-license-key")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("PUT", srv.URL+"/api/v1/sync", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", resp.StatusCode)
	}
}

func TestPostUserByIDMethodNotAllowed(t *testing.T) {
	// Set license environment variable
	t.Setenv("ENTERPRISE_LICENSE", "valid-license-key")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/users/admin", "application/json", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", resp.StatusCode)
	}
}

func TestPostUserByIDNoLicense(t *testing.T) {
	// Ensure ENTERPRISE_LICENSE is not set
	os.Unsetenv("ENTERPRISE_LICENSE")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/users/admin", "application/json", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("Expected 402, got %d", resp.StatusCode)
	}
}

func TestGetUsersEmptyListWithLicense(t *testing.T) {
	// Set license and verify empty users list
	t.Setenv("ENTERPRISE_LICENSE", "valid-license-key")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	// Don't populate stub users - should return empty array
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/users")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	// Should be valid empty JSON array
	if bodyStr != "[]" {
		t.Errorf("expected empty array, got: %s", bodyStr)
	}
}

func TestGetUsersSingleUser(t *testing.T) {
	// Set license and populate single user
	t.Setenv("ENTERPRISE_LICENSE", "valid-license-key")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	syncer.populateStubUsers()
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/users")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	// Should contain admin user
	if !strings.Contains(bodyStr, "admin") {
		t.Errorf("expected admin user in response")
	}
}

func TestPostSyncNoLicenseNoEndpoint(t *testing.T) {
	// Ensure ENTERPRISE_LICENSE is not set
	os.Unsetenv("ENTERPRISE_LICENSE")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/sync", "application/json", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusPaymentRequired {
		t.Errorf("Expected 402, got %d", resp.StatusCode)
	}
}

func TestGetUserByIDEmptyUID(t *testing.T) {
	// Set license
	t.Setenv("ENTERPRISE_LICENSE", "valid-license-key")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Empty UID after /api/v1/users/
	resp, err := http.Get(srv.URL + "/api/v1/users/")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// May match /api/v1/users/ route instead of /api/v1/users/{uid}
	t.Logf("GET /api/v1/users/ returned status: %d", resp.StatusCode)
}

func TestDeleteUsersMethodNotAllowed(t *testing.T) {
	// Set license
	t.Setenv("ENTERPRISE_LICENSE", "valid-license-key")

	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("DELETE", srv.URL+"/api/v1/users", nil)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", resp.StatusCode)
	}
}

func TestHealthzWithDifferentMethods(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	setupRoutes(mux, syncer, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// POST to /healthz
	resp, _ := http.Post(srv.URL+"/healthz", "application/json", nil)
	resp.Body.Close()
	t.Logf("POST /healthz: %d", resp.StatusCode)

	// PUT to /healthz
	req, _ := http.NewRequest("PUT", srv.URL+"/healthz", nil)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	t.Logf("PUT /healthz: %d", resp.StatusCode)

	// DELETE to /healthz
	req, _ = http.NewRequest("DELETE", srv.URL+"/healthz", nil)
	resp, _ = http.DefaultClient.Do(req)
	resp.Body.Close()
	t.Logf("DELETE /healthz: %d", resp.StatusCode)
}
