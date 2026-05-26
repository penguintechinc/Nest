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

func TestIntelligenceEngineRoutes(t *testing.T) {
	t.Setenv("OIDC_JWKS_URL", "test")
	t.Setenv("WADDLEAI_ENABLED", "true")
	authHdr := "Bearer test-tenant:user-1"
	
	logger := slog.New(slog.NewTextHandler(os.Stdout, nil))
	classifier := NewClassifier()

	t.Run("GET /healthz", func(t *testing.T) {
		srv := httptest.NewServer(NewMux(classifier, "test-license", logger))
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

	t.Run("POST /api/v1/intelligence/classify", func(t *testing.T) {
		srv := httptest.NewServer(NewMux(classifier, "test-license", logger))
		defer srv.Close()
		metrics := WorkloadMetrics{
			ResourceID: "res-1",
			ReadRPS:    100.0,
			WriteRPS:   50.0,
		}
		body, _ := json.Marshal(metrics)
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/intelligence/classify", bytes.NewReader(body))
		req.Header.Set("Authorization", authHdr)
		req.Header.Set("Content-Type", "application/json")
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
