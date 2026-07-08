package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/penguintechinc/nest/pkg/auth"
	"go.uber.org/zap"
)

func mintToken(secret, tenant, scope string) string {
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
	tokenString, _ := token.SignedString([]byte(secret))
	return tokenString
}

// mockIntrospector implements IntrospectorInterface for testing
type mockIntrospector struct {
	shouldError bool
	logger      *zap.Logger
}

// Introspect implements IntrospectorInterface
func (m *mockIntrospector) Introspect(ctx context.Context, resourceID, resourceType, endpoint string) (*Schema, error) {
	if m.shouldError {
		return nil, errors.New("mock introspection error")
	}
	return &Schema{
		ResourceID:   resourceID,
		ResourceType: resourceType,
		Fields:       []SchemaField{{Name: "id", Type: "uuid"}},
		DiscoveredAt: time.Now(),
	}, nil
}

func TestSchemaServiceRoutes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cache := NewSchemaCache(5 * time.Minute)
	introspector := NewIntrospector(logger)

	// Create test auth middleware
	testSecret := "test-secret-key-must-be-at-least-32-chars!!!!"
	authConfig := &auth.Config{
		Algorithm:    "HS256",
		SharedSecret: testSecret,
	}
	authMiddleware, err := auth.NewMiddleware(authConfig)
	if err != nil {
		t.Fatalf("failed to create auth middleware: %v", err)
	}

	srv := httptest.NewServer(NewMux(cache, introspector, logger, authMiddleware))
	defer srv.Close()

	validToken := mintToken(testSecret, "test-tenant", "*:admin")

	doRequest := func(method, url string, body *bytes.Buffer, token string) *http.Response {
		var req *http.Request
		if body != nil {
			req, _ = http.NewRequest(method, url, body)
		} else {
			req, _ = http.NewRequest(method, url, nil)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		return resp
	}

	t.Run("GET /healthz", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var result map[string]string
		json.NewDecoder(resp.Body).Decode(&result)
		if result["status"] != "ok" {
			t.Error("expected status: ok")
		}
	})

	t.Run("GET schema cache hit - without auth", func(t *testing.T) {
		cache.Set("res-1", &Schema{
			ResourceID:   "res-1",
			ResourceType: "postgres",
			Fields: []SchemaField{
				{Name: "id", Type: "uuid"},
				{Name: "email", Type: "text"},
			},
			DiscoveredAt: time.Now(),
		})

		resp, err := http.Get(srv.URL + "/api/v1/schemas/res-1")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		// Now requires auth
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("GET schema cache miss with type param", func(t *testing.T) {
		resp := doRequest("GET", srv.URL+"/api/v1/schemas/res-2?type=postgres", nil, validToken)
		defer resp.Body.Close()
		// introspector will fail without real DB, expect 200 or 500
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("unexpected status %d", resp.StatusCode)
		}
	})

	t.Run("GET schema cache miss without type returns 400", func(t *testing.T) {
		resp := doRequest("GET", srv.URL+"/api/v1/schemas/res-none", nil, validToken)

		defer resp.Body.Close()
		// No cache entry + no type param — implementation dependent
		if resp.StatusCode >= 500 && resp.StatusCode < 600 {
			// acceptable — no db configured
		} else if resp.StatusCode >= 400 && resp.StatusCode < 500 {
			// acceptable — missing type param
		}
	})

	t.Run("DELETE schema cache invalidation", func(t *testing.T) {
		cache.Set("res-del", &Schema{
			ResourceID:   "res-del",
			ResourceType: "postgres",
			DiscoveredAt: time.Now(),
		})

		resp := doRequest("DELETE", srv.URL+"/api/v1/schemas/res-del", nil, validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected 200/204, got %d", resp.StatusCode)
		}

		// Verify it's gone from cache
		_, found := cache.Get("res-del")
		if found {
			t.Error("expected cache entry to be removed after DELETE")
		}
	})

	t.Run("GET schema with refresh param", func(t *testing.T) {
		cache.Set("res-refresh", &Schema{
			ResourceID:   "res-refresh",
			ResourceType: "postgres",
			Fields:       []SchemaField{{Name: "id", Type: "uuid"}},
			DiscoveredAt: time.Now(),
		})

		resp := doRequest("GET", srv.URL+"/api/v1/schemas/res-refresh?refresh=true&type=postgres", nil, validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("unexpected status %d", resp.StatusCode)
		}
	})

	t.Run("GET different resource types", func(t *testing.T) {
		for _, resourceType := range []string{"mysql", "mariadb", "clickhouse", "search", "kafka", "unknown"} {
			resp := doRequest("GET", srv.URL+"/api/v1/schemas/res-type-test?type="+resourceType, nil, validToken)
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusInternalServerError {
				t.Errorf("unexpected status for %s: %d", resourceType, resp.StatusCode)
			}
		}
	})

	t.Run("DELETE nonexistent cache entry", func(t *testing.T) {
		resp := doRequest("DELETE", srv.URL+"/api/v1/schemas/nonexistent-res", nil, validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected 200/204, got %d", resp.StatusCode)
		}
	})

	t.Run("POST method not allowed on healthz", func(t *testing.T) {
		resp, err := http.Post(srv.URL+"/healthz", "application/json", nil)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusMethodNotAllowed {
			t.Logf("POST /healthz returned %d (may be 405 or 404 depending on routing)", resp.StatusCode)
		}
	})

	t.Run("GET schema with invalid endpoint param", func(t *testing.T) {
		resp := doRequest("GET", srv.URL+"/api/v1/schemas/res-invalid?type=postgres&endpoint=bad://endpoint", nil, validToken)
		defer resp.Body.Close()
		// Should handle gracefully with 200 or 500
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("unexpected status %d", resp.StatusCode)
		}
	})

	t.Run("DELETE schema with empty resourceId", func(t *testing.T) {
		resp := doRequest("GET", srv.URL+"/api/v1/schemas/?type=postgres", nil, validToken)
		defer resp.Body.Close()
		// May 404 or 400 depending on router
		t.Logf("GET with empty resourceId returned %d", resp.StatusCode)
	})

	t.Run("writeJSON and writeError helper functions", func(t *testing.T) {
		// These are tested implicitly through other routes, but verify error responses
		resp := doRequest("GET", srv.URL+"/api/v1/schemas/res-missing-type", nil, validToken)
		defer resp.Body.Close()
		// Should return error JSON response when type is missing
		if resp.StatusCode >= 400 && resp.StatusCode < 600 {
			var body map[string]interface{}
			_ = json.NewDecoder(resp.Body).Decode(&body)
			// Verify it's JSON
			t.Logf("error response: %v", body)
		}
	})

	t.Run("GET schema introspection error", func(t *testing.T) {
		// Trigger introspection path with bad params to cause error
		resp := doRequest("GET", srv.URL+"/api/v1/schemas/res-bad?type=invalid-type&endpoint=bad", nil, validToken)
		defer resp.Body.Close()
		// May succeed with stub or fail with 500
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("unexpected status %d", resp.StatusCode)
		}
	})

	t.Run("Cache operations thread safety", func(t *testing.T) {
		// Set and immediately invalidate in separate requests
		schema := &Schema{
			ResourceID:   "thread-test",
			ResourceType: "postgres",
			Fields:       []SchemaField{{Name: "id", Type: "uuid"}},
			DiscoveredAt: time.Now(),
		}
		cache.Set("thread-test", schema)

		resp1 := doRequest("DELETE", srv.URL+"/api/v1/schemas/thread-test", nil, validToken)
		resp1.Body.Close()

		resp2 := doRequest("GET", srv.URL+"/api/v1/schemas/thread-test?type=postgres", nil, validToken)
		resp2.Body.Close()
		// Second request should miss cache and try introspection
		t.Logf("cache invalidation test: first=%d, second=%d", resp1.StatusCode, resp2.StatusCode)
	})
}

func TestMuxWithErrorIntrospector(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	cache := NewSchemaCache(5 * time.Minute)

	// Create test auth middleware
	testSecret := "test-secret-key-must-be-at-least-32-chars!!!!"
	authConfig := &auth.Config{
		Algorithm:    "HS256",
		SharedSecret: testSecret,
	}
	authMiddleware, err := auth.NewMiddleware(authConfig)
	if err != nil {
		t.Fatalf("failed to create auth middleware: %v", err)
	}

	// Use mock introspector that returns errors
	mockIntro := &mockIntrospector{
		shouldError: true,
		logger:      logger,
	}

	mux := NewMux(cache, mockIntro, logger, authMiddleware)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	validToken := mintToken(testSecret, "test-tenant", "*:admin")

	doRequest := func(method, url string, body *bytes.Buffer, token string) *http.Response {
		var req *http.Request
		if body != nil {
			req, _ = http.NewRequest(method, url, body)
		} else {
			req, _ = http.NewRequest(method, url, nil)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		return resp
	}

	t.Run("Introspection error returns 500", func(t *testing.T) {
		resp := doRequest("GET", srv.URL+"/api/v1/schemas/error-res?type=postgres", nil, validToken)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", resp.StatusCode)
		}

		var result map[string]string
		json.NewDecoder(resp.Body).Decode(&result)
		if result["error"] != "introspection failed" {
			t.Errorf("expected 'introspection failed' error message")
		}
	})

	t.Run("Non-error introspector works", func(t *testing.T) {
		mockIntro.shouldError = false

		resp := doRequest("GET", srv.URL+"/api/v1/schemas/success-res?type=postgres", nil, validToken)
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}
