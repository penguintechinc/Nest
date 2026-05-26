package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestPolicyEngineRoutes(t *testing.T) {
	t.Setenv("DB_TYPE", "sqlite")
	t.Setenv("DB_NAME", ":memory:")
	t.Setenv("OIDC_JWKS_URL", "test")
	authHdr := "Bearer test-tenant:user-1"

	logger, _ := zap.NewDevelopment()
	store := NewPolicyStore(getTestDAL())
	mux := NewMux(store, logger)
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

	t.Run("POST /api/v1/policies", func(t *testing.T) {
		rule := &PolicyRule{
			Name:   "test-rule",
			Labels: []string{"PII"},
			Action: "deny",
		}
		body, _ := json.Marshal(rule)
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/policies", bytes.NewReader(body))
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

	t.Run("GET /api/v1/policies", func(t *testing.T) {
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/policies", nil)
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

	t.Run("POST /api/v1/evaluate", func(t *testing.T) {
		reqData := map[string]interface{}{
			"resourceId": "res-1",
			"labels":     []string{"PII"},
		}
		body, _ := json.Marshal(reqData)
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/evaluate", bytes.NewReader(body))
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
