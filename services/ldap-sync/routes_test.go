package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/penguintechinc/nest/pkg/auth"
	"go.uber.org/zap"
)

// testAuthMiddleware creates auth middleware for testing.
func testAuthMiddleware() *auth.Middleware {
	middleware, _ := auth.NewMiddleware(&auth.Config{
		Algorithm:    "HS256",
		SharedSecret: "test-secret",
		Issuer:       "test-issuer",
		Audience:     "test-audience",
	})
	return middleware
}

// createTestToken creates a valid JWT token for testing with the given tenant and scope.
func createTestToken(tenant, scope string) string {
	claims := jwt.MapClaims{
		"sub":    "test-user",
		"iss":    "test-issuer",
		"aud":    []string{"test-audience"},
		"iat":    time.Now().Unix(),
		"exp":    time.Now().Add(1 * time.Hour).Unix(),
		"tenant": tenant,
		"scope":  scope,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte("test-secret"))
	return tokenString
}

func TestHealthz(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	authMiddleware := testAuthMiddleware()
	setupRoutes(mux, syncer, logger, authMiddleware)
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

func TestGetUsersNoAuth(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	authMiddleware := testAuthMiddleware()
	setupRoutes(mux, syncer, logger, authMiddleware)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/users")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", resp.StatusCode)
	}
}

func TestGetUsersWithValidAuth(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	syncer.populateStubUsers("tenant-1")

	mux := http.NewServeMux()
	authMiddleware := testAuthMiddleware()
	setupRoutes(mux, syncer, logger, authMiddleware)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	token := createTestToken("tenant-1", "users:read")
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
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

func TestGetUserByIDNoAuth(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	authMiddleware := testAuthMiddleware()
	setupRoutes(mux, syncer, logger, authMiddleware)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v1/users/nonexistent")
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", resp.StatusCode)
	}
}

func TestGetUserByIDNotFound(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	authMiddleware := testAuthMiddleware()
	setupRoutes(mux, syncer, logger, authMiddleware)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	token := createTestToken("tenant-1", "users:read")
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/users/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", resp.StatusCode)
	}
}

func TestPostSyncNoAuth(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	authMiddleware := testAuthMiddleware()
	setupRoutes(mux, syncer, logger, authMiddleware)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/api/v1/sync", "application/json", nil)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected 401, got %d", resp.StatusCode)
	}
}

func TestPostSyncWithValidAuth(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	authMiddleware := testAuthMiddleware()
	setupRoutes(mux, syncer, logger, authMiddleware)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	token := createTestToken("tenant-1", "users:admin")
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/sync", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
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
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	authMiddleware := testAuthMiddleware()
	setupRoutes(mux, syncer, logger, authMiddleware)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	token := createTestToken("tenant-1", "users:read")
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", resp.StatusCode)
	}
}

func TestGetSyncMethodNotAllowed(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	authMiddleware := testAuthMiddleware()
	setupRoutes(mux, syncer, logger, authMiddleware)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	token := createTestToken("tenant-1", "users:admin")
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/sync", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("Expected 405, got %d", resp.StatusCode)
	}
}

func TestGetUserByIDWithValidAuth(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	// Populate stub users for tenant-1
	syncer.populateStubUsers("tenant-1")

	mux := http.NewServeMux()
	authMiddleware := testAuthMiddleware()
	setupRoutes(mux, syncer, logger, authMiddleware)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	token := createTestToken("tenant-1", "users:read")
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/users/admin", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
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

func TestGetUsersBadScope(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	syncer.populateStubUsers("tenant-1")

	mux := http.NewServeMux()
	authMiddleware := testAuthMiddleware()
	setupRoutes(mux, syncer, logger, authMiddleware)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Token with wrong scope (other:read instead of users:read)
	token := createTestToken("tenant-1", "other:read")
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("Expected 403 for insufficient scope, got %d", resp.StatusCode)
	}
}

func TestGetUsersEmptyListWithAuth(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	// Don't populate stub users - should return empty array

	mux := http.NewServeMux()
	authMiddleware := testAuthMiddleware()
	setupRoutes(mux, syncer, logger, authMiddleware)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	token := createTestToken("tenant-1", "users:read")
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
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

func TestGetUsersTenantIsolation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)

	// Populate stub users for both tenants
	syncer.populateStubUsers("tenant-1")
	syncer.populateStubUsers("tenant-2")

	mux := http.NewServeMux()
	authMiddleware := testAuthMiddleware()
	setupRoutes(mux, syncer, logger, authMiddleware)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Request as tenant-1
	token := createTestToken("tenant-1", "users:read")
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/users", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected 200, got %d", resp.StatusCode)
	}

	bodyBytes, _ := io.ReadAll(resp.Body)
	bodyStr := string(bodyBytes)

	// Should contain tenant field with correct tenant
	if !strings.Contains(bodyStr, `"tenant":"tenant-1"`) {
		t.Errorf("Expected tenant-1 in response, got: %s", bodyStr)
	}
}

func TestPostSyncInsufficientScope(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	syncer := NewSyncer("", 1*time.Hour, logger)
	mux := http.NewServeMux()
	authMiddleware := testAuthMiddleware()
	setupRoutes(mux, syncer, logger, authMiddleware)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Token with read scope instead of admin
	token := createTestToken("tenant-1", "users:read")
	req, _ := http.NewRequest("POST", srv.URL+"/api/v1/sync", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		t.Errorf("Expected 403 for insufficient scope, got %d", resp.StatusCode)
	}
}
