package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"log/slog"
)

func TestAnomalyDetectorRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	detector := NewDetector()
	srv := httptest.NewServer(NewMux(detector, logger))
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

	t.Run("POST /api/v1/anomaly/samples - without license", func(t *testing.T) {
		os.Unsetenv("ENTERPRISE_LICENSE")
		os.Unsetenv("WADDLEAI_ENABLED")

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
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/anomaly/samples - with license", func(t *testing.T) {
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
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected 201, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/anomaly/samples - invalid request", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Post(srv.URL+"/api/v1/anomaly/samples", "application/json", bytes.NewReader([]byte("invalid")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/anomaly/current - without license", func(t *testing.T) {
		os.Unsetenv("ENTERPRISE_LICENSE")
		os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/anomaly/current")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/anomaly/current - with license", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/anomaly/current")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/anomaly/current - filter by tenant and severity", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/anomaly/current?tenant=test-tenant&severity=high&limit=10")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if _, ok := result["anomalies"]; !ok {
			t.Error("expected anomalies in response")
		}
	})

	t.Run("GET /api/v1/anomaly/stats - without license", func(t *testing.T) {
		os.Unsetenv("ENTERPRISE_LICENSE")
		os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/anomaly/stats")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/anomaly/stats - with license", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/anomaly/stats")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}
