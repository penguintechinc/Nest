package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

func createTestToken(tenant string) string {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":    "test-user",
		"exp":    now.Add(1 * time.Hour).Unix(),
		"tenant": tenant,
		"scope":  "admin:write",
	})
	tokenString, _ := token.SignedString([]byte("test-secret"))
	return tokenString
}

func TestErasureEngineRoutes(t *testing.T) {
	// Set up JWT environment for auth middleware
	os.Setenv("JWT_ALGORITHM", "HS256")
	os.Setenv("JWT_SHARED_SECRET", "test-secret")
	os.Setenv("JWT_ISSUER", "test-issuer")
	os.Setenv("JWT_AUDIENCE", "test-audience")
	defer func() {
		os.Unsetenv("JWT_ALGORITHM")
		os.Unsetenv("JWT_SHARED_SECRET")
		os.Unsetenv("JWT_ISSUER")
		os.Unsetenv("JWT_AUDIENCE")
	}()

	logger, _ := zap.NewDevelopment()
	db, _ := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	store, _ := NewErasureStoreWithDB(db, logger)
	srv := httptest.NewServer(NewMux(store, logger))
	defer srv.Close()

	// Create a test token
	testToken := createTestToken("test-tenant")

	t.Run("GET /healthz", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/erase - create erasure request (sync)", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"subjectId":      "user-123",
			"tenant":         "test-tenant",
			"async":          false,
			"backends":       []string{"postgres", "s3"},
			"idempotencyKey": "key-1",
		})
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/erase", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/erase - create erasure request (async)", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"subjectId":      "user-124",
			"tenant":         "test-tenant",
			"async":          true,
			"backends":       []string{"postgres"},
			"idempotencyKey": "key-2",
		})
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/erase", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("expected 202, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/erase - missing subjectId", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"tenant": "test-tenant",
		})
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/erase", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/erase - missing tenant", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"subjectId": "user-125",
		})
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/erase", bytes.NewReader(body))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/erase - invalid request", func(t *testing.T) {
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/erase", bytes.NewReader([]byte("invalid")))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/erase/{id} - get erasure request", func(t *testing.T) {
		// Create a request first
		body, _ := json.Marshal(map[string]interface{}{
			"subjectId":      "user-126",
			"tenant":         "test-tenant",
			"async":          false,
			"backends":       []string{"postgres"},
			"idempotencyKey": "key-3",
		})
		createReq, _ := http.NewRequest("POST", srv.URL+"/api/v1/erase", bytes.NewReader(body))
		createReq.Header.Set("Content-Type", "application/json")
		createReq.Header.Set("Authorization", "Bearer "+testToken)
		createResp, _ := http.DefaultClient.Do(createReq)
		var eraseReq map[string]interface{}
		json.NewDecoder(createResp.Body).Decode(&eraseReq)
		createResp.Body.Close()

		if id, ok := eraseReq["id"].(string); ok && id != "" {
			getReq, _ := http.NewRequest("GET", srv.URL+"/api/v1/erase/"+id, nil)
			getReq.Header.Set("Authorization", "Bearer "+testToken)
			resp, err := http.DefaultClient.Do(getReq)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected 200, got %d", resp.StatusCode)
			}
		}
	})

	t.Run("GET /api/v1/erase/{id} - not found", func(t *testing.T) {
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/erase/nonexistent-id", nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/erase - list by tenant", func(t *testing.T) {
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/erase?tenant=test-tenant", nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/erase - missing tenant parameter", func(t *testing.T) {
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/erase", nil)
		req.Header.Set("Authorization", "Bearer "+testToken)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})
}
