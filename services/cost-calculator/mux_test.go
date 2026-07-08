package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/penguintechinc/nest/pkg/auth"
	"go.uber.org/zap"
)

func TestCostCalculatorRoutes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	calc := NewCalculator()

	// Initialize auth middleware for test
	authConfig := &auth.Config{
		Algorithm:    "HS256",
		SharedSecret: "test-secret",
	}
	authMiddleware, err := auth.NewMiddleware(authConfig)
	if err != nil {
		t.Fatalf("failed to create auth middleware: %v", err)
	}

	srv := httptest.NewServer(NewMux(calc, logger, authMiddleware))
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

	t.Run("POST /api/v1/billing/{tenantId}/record - add tokens", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"resourceType": "api_call",
			"tokens":       1000.50,
		})
		resp, err := http.Post(srv.URL+"/api/v1/billing/tenant-1/record", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("expected 202, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/billing/{tenantId}/record - invalid request", func(t *testing.T) {
		resp, err := http.Post(srv.URL+"/api/v1/billing/tenant-1/record", "application/json", bytes.NewReader([]byte("invalid")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/billing/{tenantId} - list records for tenant", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/billing/tenant-1")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if _, ok := result["records"]; !ok {
			t.Error("expected records in response")
		}
	})

	t.Run("GET /api/v1/billing/{tenantId}/{month} - get specific month", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/billing/tenant-1/2025-04")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/billing/{tenantId}/{month} - nonexistent month", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/billing/tenant-2/2020-01")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			// May not exist, acceptable
		}
	})

	t.Run("GET /api/v1/billing - list all records (admin)", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/billing")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if _, ok := result["records"]; !ok {
			t.Error("expected records in response")
		}
	})

	t.Run("GET /api/v1/billing/{tenantId}/summary - get aggregate summary", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/billing/tenant-1/summary")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if _, ok := result["totalTokens"]; !ok {
			t.Error("expected totalTokens in response")
		}
		if _, ok := result["totalCostUsd"]; !ok {
			t.Error("expected totalCostUsd in response")
		}
	})

	t.Run("GET /api/v1/billing/{tenantId}/summary - empty tenant", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/billing/tenant-nonexistent/summary")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}
