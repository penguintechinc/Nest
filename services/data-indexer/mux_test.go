package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestIndexerRoutes(t *testing.T) {
	t.Setenv("OIDC_JWKS_URL", "test")
	authHdr := "Bearer test-tenant:user-1"

	logger, _ := zap.NewDevelopment()
	catalog := NewCatalog()
	pipeline := NewPipeline(catalog, logger)
	mux := NewMux(catalog, pipeline, logger)
	srv := httptest.NewServer(mux)
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

	t.Run("POST /api/v1/indexer/scan", func(t *testing.T) {
		reqData := ScanRequest{
			ResourceID:  "res-1",
			BackendType: "postgres",
			Tables: []struct {
				Name    string `json:"name"`
				Columns []struct {
					Name     string `json:"name"`
					DataType string `json:"dataType"`
				} `json:"columns"`
			}{
				{
					Name: "users",
					Columns: []struct {
						Name     string `json:"name"`
						DataType string `json:"dataType"`
					}{
						{Name: "id", DataType: "int"},
						{Name: "email", DataType: "text"},
					},
				},
			},
		}
		body, _ := json.Marshal(reqData)
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/indexer/scan", bytes.NewReader(body))
		req.Header.Set("Authorization", authHdr)
		req.Header.Set("Content-Type", "application/json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("expected 202, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/indexer/catalog", func(t *testing.T) {
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/indexer/catalog", nil)
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
