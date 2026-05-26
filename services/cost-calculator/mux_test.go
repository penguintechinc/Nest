package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestBillingRoutes(t *testing.T) {
	t.Setenv("OIDC_JWKS_URL", "test")
	authHdr := "Bearer test-tenant:user-1"

	logger, _ := zap.NewDevelopment()
	calc := NewCalculator()
	mux := NewMux(calc, logger)
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

	t.Run("POST /api/v1/billing/{tenantId}/record", func(t *testing.T) {
		reqData := map[string]interface{}{
			"resourceType": "storage",
			"tokens":       100.5,
		}
		body, _ := json.Marshal(reqData)
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/billing/test-tenant/record", bytes.NewReader(body))
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

	t.Run("GET /api/v1/billing/{tenantId}", func(t *testing.T) {
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/billing/test-tenant", nil)
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

	t.Run("GET /api/v1/billing/{tenantId} - mismatch", func(t *testing.T) {
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/billing/other-tenant", nil)
		req.Header.Set("Authorization", authHdr)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})
}
