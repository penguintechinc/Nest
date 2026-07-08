package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/penguintechinc/nest/pkg/auth"
)

func TestAnomalyDetectorRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	detector := NewDetector()

	// Create test auth middleware (HS256 with test secret)
	authConfig := &auth.Config{
		Algorithm:    "HS256",
		SharedSecret: "test-secret-key-must-be-at-least-32-chars!!!!",
	}
	authMiddleware, err := auth.NewMiddleware(authConfig)
	if err != nil {
		t.Fatalf("failed to create auth middleware: %v", err)
	}

	srv := httptest.NewServer(NewMux(detector, logger, authMiddleware))
	defer srv.Close()

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

	t.Run("POST /api/v1/anomaly/samples - without auth token", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"timestamp": "2025-04-24T12:00:00Z",
			"metric":    "cpu_usage",
			"value":     45.5,
		})
		resp, err := http.Post(srv.URL+"/api/v1/anomaly/samples", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		// Now requires auth, so should get 401 Unauthorized
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/anomaly/samples - with license but no tenant claim", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		body, _ := json.Marshal(map[string]interface{}{
			"timestamp": "2025-04-24T12:00:00Z",
			"metric":    "cpu_usage",
			"value":     45.5,
		})
		resp, err := http.Post(srv.URL+"/api/v1/anomaly/samples", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		// Still 401 because no token provided
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/anomaly/samples - invalid request (no auth)", func(t *testing.T) {
		resp, err := http.Post(srv.URL+"/api/v1/anomaly/samples", "application/json", bytes.NewReader([]byte("invalid")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		// 401 because no auth token, not 400 for invalid request
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/anomaly/current - without auth", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/anomaly/current")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/anomaly/current - with license (but no auth)", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/anomaly/current")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		// Still 401 because no auth token
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/anomaly/stats - without auth", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/anomaly/stats")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/anomaly/stats - with license (but no auth)", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/anomaly/stats")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		// Still 401 because no auth token
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})
}
