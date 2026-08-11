package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestPredictorRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	forecaster := NewForecaster()
	srv := httptest.NewServer(NewMux(forecaster, logger))
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

	t.Run("POST /api/v1/predictor/samples - without license", func(t *testing.T) {
		os.Unsetenv("ENTERPRISE_LICENSE")
		os.Unsetenv("WADDLEAI_ENABLED")

		body, _ := json.Marshal(map[string]interface{}{
			"timestamp":   "2025-04-24T12:00:00Z",
			"resourceId":  "res-1",
			"cpuUsage":    45.5,
			"memoryUsage": 60.2,
		})
		resp, err := http.Post(srv.URL+"/api/v1/predictor/samples", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/predictor/samples - with license", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		body, _ := json.Marshal(map[string]interface{}{
			"timestamp":   "2025-04-24T12:00:00Z",
			"resourceId":  "res-1",
			"cpuUsage":    45.5,
			"memoryUsage": 60.2,
		})
		resp, err := http.Post(srv.URL+"/api/v1/predictor/samples", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected 201, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/predictor/forecast - without license", func(t *testing.T) {
		os.Unsetenv("ENTERPRISE_LICENSE")
		os.Unsetenv("WADDLEAI_ENABLED")

		body, _ := json.Marshal(map[string]interface{}{
			"resourceId": "res-1",
			"hoursAhead": 24,
		})
		resp, err := http.Post(srv.URL+"/api/v1/predictor/forecast", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/predictor/forecast - with license", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		body, _ := json.Marshal(map[string]interface{}{
			"resourceId": "res-1",
			"hoursAhead": 24,
		})
		resp, err := http.Post(srv.URL+"/api/v1/predictor/forecast", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected 200 or 500, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/predictor/forecast - invalid request", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Post(srv.URL+"/api/v1/predictor/forecast", "application/json", bytes.NewReader([]byte("invalid")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/predictor/scale-needed - without license", func(t *testing.T) {
		os.Unsetenv("ENTERPRISE_LICENSE")
		os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/predictor/scale-needed")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/predictor/scale-needed - with license", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/predictor/scale-needed")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if _, ok := result["scale_needed"]; !ok {
			t.Error("expected scale_needed in response")
		}
	})

	t.Run("GET /api/v1/predictor/scale-needed - with hoursAhead parameter", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/predictor/scale-needed?hoursAhead=48")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/predictor/samples - invalid request", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Post(srv.URL+"/api/v1/predictor/samples", "application/json", bytes.NewReader([]byte("invalid")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/predictor/samples - only ENTERPRISE_LICENSE set", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Unsetenv("WADDLEAI_ENABLED")
		defer os.Unsetenv("ENTERPRISE_LICENSE")

		body, _ := json.Marshal(map[string]interface{}{
			"timestamp":   "2025-04-24T12:00:00Z",
			"resourceId":  "res-1",
			"cpuUsage":    45.5,
			"memoryUsage": 60.2,
		})
		resp, err := http.Post(srv.URL+"/api/v1/predictor/samples", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/predictor/samples - only WADDLEAI_ENABLED set", func(t *testing.T) {
		os.Unsetenv("ENTERPRISE_LICENSE")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		body, _ := json.Marshal(map[string]interface{}{
			"timestamp":   "2025-04-24T12:00:00Z",
			"resourceId":  "res-1",
			"cpuUsage":    45.5,
			"memoryUsage": 60.2,
		})
		resp, err := http.Post(srv.URL+"/api/v1/predictor/samples", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/predictor/scale-needed - invalid hoursAhead parameter", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/predictor/scale-needed?hoursAhead=not-a-number")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 (fallback to default), got %d", resp.StatusCode)
		}
		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if _, ok := result["scale_needed"]; !ok {
			t.Error("expected scale_needed in response")
		}
		if _, ok := result["count"]; !ok {
			t.Error("expected count in response")
		}
	})

	t.Run("POST /api/v1/predictor/forecast - invalid hoursAhead", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		body, _ := json.Marshal(map[string]interface{}{
			"resourceId": "res-1",
			"hoursAhead": 0,
		})
		resp, err := http.Post(srv.URL+"/api/v1/predictor/forecast", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusInternalServerError {
			t.Errorf("expected 200 or 500, got %d", resp.StatusCode)
		}
	})
}
