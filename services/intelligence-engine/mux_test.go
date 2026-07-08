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

func TestIntelligenceEngineRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	classifier := NewClassifier()

	// Create test auth middleware
	authConfig := &auth.Config{
		Algorithm:    "HS256",
		SharedSecret: "test-secret-key-must-be-at-least-32-chars!!!!",
	}
	authMiddleware, err := auth.NewMiddleware(authConfig)
	if err != nil {
		t.Fatalf("failed to create auth middleware: %v", err)
	}

	srv := httptest.NewServer(NewMux(classifier, logger, authMiddleware))
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

	t.Run("POST /api/v1/intelligence/classify - without auth", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"cpuUsage":    45.5,
			"memoryUsage": 60.2,
		})
		resp, err := http.Post(srv.URL+"/api/v1/intelligence/classify", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		// Now requires auth
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/intelligence/classify - with license", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		body, _ := json.Marshal(map[string]interface{}{
			"cpuUsage":    45.5,
			"memoryUsage": 60.2,
		})
		resp, err := http.Post(srv.URL+"/api/v1/intelligence/classify", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/intelligence/classify - invalid request", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Post(srv.URL+"/api/v1/intelligence/classify", "application/json", bytes.NewReader([]byte("invalid")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations - without license", func(t *testing.T) {
		os.Unsetenv("ENTERPRISE_LICENSE")
		os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/intelligence/recommendations")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations - with license", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/intelligence/recommendations")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if _, ok := result["recommendations"]; !ok {
			t.Error("expected recommendations in response")
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations - filter by tenant", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/intelligence/recommendations?tenant=test-tenant")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations/{resourceId} - without license", func(t *testing.T) {
		os.Unsetenv("ENTERPRISE_LICENSE")
		os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/intelligence/recommendations/res-1")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations/{resourceId} - with license", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/intelligence/recommendations/res-1")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/intelligence/classify - only ENTERPRISE_LICENSE set", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Unsetenv("WADDLEAI_ENABLED")
		defer os.Unsetenv("ENTERPRISE_LICENSE")

		body, _ := json.Marshal(map[string]interface{}{
			"cpuUsage":    45.5,
			"memoryUsage": 60.2,
		})
		resp, err := http.Post(srv.URL+"/api/v1/intelligence/classify", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402 (WADDLEAI_ENABLED missing), got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/intelligence/classify - only WADDLEAI_ENABLED set", func(t *testing.T) {
		os.Unsetenv("ENTERPRISE_LICENSE")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		body, _ := json.Marshal(map[string]interface{}{
			"cpuUsage":    45.5,
			"memoryUsage": 60.2,
		})
		resp, err := http.Post(srv.URL+"/api/v1/intelligence/classify", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402 (ENTERPRISE_LICENSE missing), got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations - only WADDLEAI_ENABLED set", func(t *testing.T) {
		os.Unsetenv("ENTERPRISE_LICENSE")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp, err := http.Get(srv.URL + "/api/v1/intelligence/recommendations")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations/{resourceId} - only ENTERPRISE_LICENSE set", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Unsetenv("WADDLEAI_ENABLED")
		defer os.Unsetenv("ENTERPRISE_LICENSE")

		resp, err := http.Get(srv.URL + "/api/v1/intelligence/recommendations/res-1")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/intelligence/classify - valid metrics", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		body, _ := json.Marshal(map[string]interface{}{
			"resourceId":   "test-res",
			"tenant":       "test-tenant",
			"readRps":      100.0,
			"writeRps":     50.0,
			"avgLatencyMs": 10.0,
			"p99LatencyMs": 50.0,
			"dataSizeGb":   1000.0,
			"scanRatio":    0.8,
		})
		resp, err := http.Post(srv.URL+"/api/v1/intelligence/classify", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if _, ok := result["workloadType"]; !ok {
			t.Error("expected workloadType in response")
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations - with results", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		// First, classify something
		body, _ := json.Marshal(map[string]interface{}{
			"resourceId":   "test-res-2",
			"tenant":       "test-tenant",
			"readRps":      100.0,
			"writeRps":     50.0,
			"avgLatencyMs": 10.0,
			"p99LatencyMs": 50.0,
			"dataSizeGb":   1000.0,
			"scanRatio":    0.8,
		})
		http.Post(srv.URL+"/api/v1/intelligence/classify", "application/json", bytes.NewReader(body))

		// Then list
		resp, err := http.Get(srv.URL + "/api/v1/intelligence/recommendations?tenant=all")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/intelligence/classify - OLTP classification", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		body, _ := json.Marshal(map[string]interface{}{
			"resourceId":   "oltp-res",
			"tenant":       "test-tenant",
			"readRps":      5000.0,
			"writeRps":     5000.0,
			"avgLatencyMs": 2.0,
			"p99LatencyMs": 2.0,
			"dataSizeGb":   100.0,
			"scanRatio":    0.1,
		})
		resp, err := http.Post(srv.URL+"/api/v1/intelligence/classify", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations/{resourceId} - found resource", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		// First, classify something
		body, _ := json.Marshal(map[string]interface{}{
			"resourceId":   "found-res",
			"tenant":       "test-tenant",
			"readRps":      100.0,
			"writeRps":     50.0,
			"avgLatencyMs": 10.0,
			"p99LatencyMs": 50.0,
			"dataSizeGb":   1000.0,
			"scanRatio":    0.8,
		})
		http.Post(srv.URL+"/api/v1/intelligence/classify", "application/json", bytes.NewReader(body))

		// Then get specific resource
		resp, err := http.Get(srv.URL + "/api/v1/intelligence/recommendations/found-res")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if _, ok := result["resourceId"]; !ok {
			t.Error("expected resourceId in response")
		}
	})
}
