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

func TestAnomalyDetectorRoutes(t *testing.T) {
	t.Setenv("OIDC_JWKS_URL", "test")
	t.Setenv("WADDLEAI_ENABLED", "true")
	authHdr := "Bearer test-tenant:user-1"
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	detector := NewDetector()
	
	t.Run("GET /healthz", func(t *testing.T) {
		srv := httptest.NewServer(NewMux(detector, "test-license", logger))
		defer srv.Close()
		resp, err := http.Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/anomaly/samples", func(t *testing.T) {
		srv := httptest.NewServer(NewMux(detector, "test-license", logger))
		defer srv.Close()
		sample := MetricSample{
			Resource:   "res-1",
			MetricName: "cpu_usage",
			Value:      95.5,
		}
		body, _ := json.Marshal(sample)
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/anomaly/samples", bytes.NewReader(body))
		req.Header.Set("Authorization", authHdr)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected 201, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/anomaly/current", func(t *testing.T) {
		srv := httptest.NewServer(NewMux(detector, "test-license", logger))
		defer srv.Close()
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/anomaly/current", nil)
		req.Header.Set("Authorization", authHdr)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}
