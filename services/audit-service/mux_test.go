package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/penguintechinc/nest/pkg/auth"
	"go.uber.org/zap"
)

func createTestAuthMiddleware() *auth.Middleware {
	authMiddleware, _ := auth.NewMiddleware(&auth.Config{
		Algorithm:    testJWTAlgorithm,
		SharedSecret: testJWTSharedSecret,
		Issuer:       testJWTIssuer,
		Audience:     testJWTAudience,
	})
	return authMiddleware
}

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

func TestAuditServiceRoutes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	auditLogger := NewAuditLogger(logger)
	authMiddleware := createTestAuthMiddleware()
	srv := httptest.NewServer(NewMux(auditLogger, "test-license", logger, authMiddleware))
	defer srv.Close()

	t.Run("GET /healthz - no license required", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/audit/events - with license", func(t *testing.T) {
		eventBody := map[string]interface{}{
			"tenant":    "test-tenant",
			"actor":     "user-1",
			"action":    "read",
			"resource":  "dataset-1",
			"outcome":   "success",
		}
		req, _ := makeAuthRequest("POST", srv.URL+"/api/v1/audit/events", eventBody, "user-1", "test-tenant", "audit:write")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected 201, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/audit/events - without license", func(t *testing.T) {
		noLicenseSrv := httptest.NewServer(NewMux(auditLogger, "", logger, authMiddleware))
		defer noLicenseSrv.Close()

		eventBody := map[string]interface{}{
			"tenant":   "test-tenant",
			"actor":    "user-1",
			"action":   "read",
			"resource": "dataset-1",
			"outcome":  "success",
		}
		req, _ := makeAuthRequest("POST", noLicenseSrv.URL+"/api/v1/audit/events", eventBody, "user-1", "test-tenant", "audit:write")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/audit/events - invalid request", func(t *testing.T) {
		// Create request with invalid JSON but valid JWT token
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/audit/events", bytes.NewReader([]byte("invalid")))
		token, _ := generateTestJWT("user-1", "test-tenant", "audit:write")
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

	t.Run("GET /api/v1/audit/events - list all events", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/audit/events", "user-1", "test-tenant", "audit:read")
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
		if _, ok := result["events"]; !ok {
			t.Error("expected events in response")
		}
	})

	t.Run("GET /api/v1/audit/events - filter by tenant", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/audit/events?tenant=test-tenant", "user-1", "test-tenant", "audit:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - filter by actor", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/audit/events?actor=user-1", "user-1", "test-tenant", "audit:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - filter by action", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/audit/events?action=read", "user-1", "test-tenant", "audit:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - filter by resource", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/audit/events?resource=dataset-1", "user-1", "test-tenant", "audit:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - filter by outcome", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/audit/events?outcome=success", "user-1", "test-tenant", "audit:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - filter by time range", func(t *testing.T) {
		now := time.Now()
		startTime := now.Add(-24 * time.Hour).Format(time.RFC3339)
		endTime := now.Format(time.RFC3339)

		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/audit/events?start_time="+startTime+"&end_time="+endTime, "user-1", "test-tenant", "audit:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - filter with limit and offset", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/audit/events?limit=10&offset=0", "user-1", "test-tenant", "audit:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events/{id} - get single event", func(t *testing.T) {
		// First add an event with valid JWT
		eventBody := map[string]interface{}{
			"tenant":    "test-tenant",
			"actor":     "user-2",
			"action":    "write",
			"resource":  "dataset-2",
			"outcome":   "failure",
		}
		createReq, _ := makeAuthRequest("POST", srv.URL+"/api/v1/audit/events", eventBody, "user-2", "test-tenant", "audit:write")
		createResp, _ := http.DefaultClient.Do(createReq)
		var event map[string]interface{}
		json.NewDecoder(createResp.Body).Decode(&event)
		createResp.Body.Close()

		if id, ok := event["id"].(string); ok && id != "" {
			getReq, _ := makeAuthGetRequest(srv.URL+"/api/v1/audit/events/"+id, "user-2", "test-tenant", "audit:read")
			resp, err := http.DefaultClient.Do(getReq)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected 200, got %d", resp.StatusCode)
			}
		}
	})

	t.Run("GET /api/v1/audit/events/{id} - not found", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/audit/events/nonexistent-id", "user-1", "test-tenant", "audit:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - invalid start_time", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/audit/events?start_time=invalid-date", "user-1", "test-tenant", "audit:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 (invalid time ignored), got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - invalid end_time", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/audit/events?end_time=bad-date", "user-1", "test-tenant", "audit:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 (invalid time ignored), got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - invalid limit", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/audit/events?limit=not-a-number", "user-1", "test-tenant", "audit:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 (invalid limit ignored), got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - invalid offset", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/audit/events?offset=abc", "user-1", "test-tenant", "audit:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 (invalid offset ignored), got %d", resp.StatusCode)
		}
	})

	t.Run("GET /notfound - unrecognized path", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/notfound")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events/{id} - without license", func(t *testing.T) {
		noLicenseSrv := httptest.NewServer(NewMux(auditLogger, "", logger, authMiddleware))
		defer noLicenseSrv.Close()

		req, _ := makeAuthGetRequest(noLicenseSrv.URL+"/api/v1/audit/events/some-id", "user-1", "test-tenant", "audit:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - without license", func(t *testing.T) {
		noLicenseSrv := httptest.NewServer(NewMux(auditLogger, "", logger, authMiddleware))
		defer noLicenseSrv.Close()

		req, _ := makeAuthGetRequest(noLicenseSrv.URL+"/api/v1/audit/events", "user-1", "test-tenant", "audit:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/audit/events - append error", func(t *testing.T) {
		// Send event with missing required fields to trigger Append error
		// But include valid JWT
		eventBody := map[string]interface{}{}
		req, _ := makeAuthRequest("POST", srv.URL+"/api/v1/audit/events", eventBody, "user-1", "test-tenant", "audit:write")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 on append error, got %d", resp.StatusCode)
		}
	})
}
