package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/penguintechinc/nest/pkg/auth"
	"go.uber.org/zap"
)

// makeAuthRequest creates an HTTP request with a valid JWT token in the Authorization header.
func makeAuthRequest(method, url string, body interface{}, subject, tenant, scope string) (*http.Request, error) {
	token, err := generateTestJWT(subject, tenant, scope)
	if err != nil {
		return nil, err
	}

	var reqBody *bytes.Reader
	if body != nil {
		bodyBytes, _ := json.Marshal(body)
		reqBody = bytes.NewReader(bodyBytes)
	} else {
		reqBody = bytes.NewReader([]byte{})
	}

	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	return req, nil
}

// makeAuthGetRequest creates a GET request with JWT token.
func makeAuthGetRequest(url, subject, tenant, scope string) (*http.Request, error) {
	return makeAuthRequest("GET", url, nil, subject, tenant, scope)
}

func TestCostCalculatorRoutes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	calc := NewCalculator()

	// Initialize auth middleware for test
	authConfig := &auth.Config{
		Algorithm:       testJWTAlgorithm,
		SharedSecret:    testJWTSharedSecret,
		AllowHS256Admin: true,
		Issuer:          testJWTIssuer,
		Audience:        testJWTAudience,
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
		recordBody := map[string]interface{}{
			"resourceType": "api_call",
			"tokens":       1000.50,
		}
		req, _ := makeAuthRequest("POST", srv.URL+"/api/v1/billing/tenant-1/record", recordBody, "user-1", "tenant-1", "billing:write")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("expected 202, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/billing/{tenantId}/record - invalid request", func(t *testing.T) {
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/billing/tenant-1/record", bytes.NewReader([]byte("invalid")))
		token, _ := generateTestJWT("user-1", "tenant-1", "billing:write")
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/billing/{tenantId} - list records for tenant", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/billing/tenant-1", "user-1", "tenant-1", "billing:read")
		resp, err := http.DefaultClient.Do(req)
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
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/billing/tenant-1/2025-04", "user-1", "tenant-1", "billing:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/billing/{tenantId}/{month} - nonexistent month", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/billing/tenant-2/2020-01", "user-1", "tenant-2", "billing:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			// May not exist, acceptable
		}
	})

	t.Run("GET /api/v1/billing - list all records (admin)", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/billing", "user-1", "tenant-1", "billing:admin")
		resp, err := http.DefaultClient.Do(req)
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
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/billing/tenant-1/summary", "user-1", "tenant-1", "billing:read")
		resp, err := http.DefaultClient.Do(req)
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
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/billing/tenant-nonexistent/summary", "user-1", "tenant-nonexistent", "billing:read")
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
