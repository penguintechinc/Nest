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

func TestPolicyEngineRoutes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewPolicyStore(getTestDAL())

	// Create test auth middleware
	authMiddleware, err := auth.NewMiddleware(&auth.Config{
		Algorithm:    testJWTAlgorithm,
		SharedSecret: testJWTSharedSecret,
		Issuer:       testJWTIssuer,
		Audience:     testJWTAudience,
	})
	if err != nil {
		t.Fatalf("failed to create auth middleware: %v", err)
	}

	srv := httptest.NewServer(NewMux(store, logger, authMiddleware))
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

	t.Run("GET /api/v1/policies - list policies", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/policies", "user-1", "test-tenant", "policy:read")
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
		if _, ok := result["rules"]; !ok {
			t.Error("expected rules in response")
		}
	})

	t.Run("GET /api/v1/policies - filter by tenant", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/policies?tenant=test-tenant", "user-1", "test-tenant", "policy:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/policies - create policy", func(t *testing.T) {
		policyBody := map[string]interface{}{
			"id":       "policy-1",
			"name":     "Test Policy",
			"tenant":   "test-tenant",
			"rules":    []interface{}{},
			"priority": 1,
		}
		req, _ := makeAuthRequest("POST", srv.URL+"/api/v1/policies", policyBody, "user-1", "test-tenant", "policy:write")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected 201, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/policies - invalid request", func(t *testing.T) {
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/policies", bytes.NewReader([]byte("invalid")))
		token, _ := generateTestJWT("user-1", "test-tenant", "policy:write")
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

	t.Run("GET /api/v1/policies/{id} - get policy", func(t *testing.T) {
		policyBody := map[string]interface{}{
			"name":     "Test Policy 2",
			"tenant":   "test-tenant",
			"labels":   []interface{}{"INTERNAL"},
			"action":   "allow",
			"priority": 2,
		}
		crReq, _ := makeAuthRequest("POST", srv.URL+"/api/v1/policies", policyBody, "user-1", "test-tenant", "policy:write")
		cr, _ := http.DefaultClient.Do(crReq)
		defer cr.Body.Close()
		var rule map[string]interface{}
		json.NewDecoder(cr.Body).Decode(&rule)
		id := rule["id"].(string)

		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/policies/"+id, "user-1", "test-tenant", "policy:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/policies/{id} - not found", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/policies/nonexistent", "user-1", "test-tenant", "policy:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /api/v1/policies/{id} - delete policy", func(t *testing.T) {
		policyBody := map[string]interface{}{
			"name":     "Test Policy 3",
			"tenant":   "test-tenant",
			"labels":   []interface{}{"INTERNAL"},
			"action":   "allow",
			"priority": 3,
		}
		crReq, _ := makeAuthRequest("POST", srv.URL+"/api/v1/policies", policyBody, "user-1", "test-tenant", "policy:write")
		cr, _ := http.DefaultClient.Do(crReq)
		defer cr.Body.Close()
		var rule3 map[string]interface{}
		json.NewDecoder(cr.Body).Decode(&rule3)
		delID := rule3["id"].(string)

		req, _ := makeAuthRequest("DELETE", srv.URL+"/api/v1/policies/"+delID, nil, "user-1", "test-tenant", "policy:write")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent {
			t.Errorf("expected 204, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /api/v1/policies/{id} - not found", func(t *testing.T) {
		req, _ := makeAuthRequest("DELETE", srv.URL+"/api/v1/policies/nonexistent", nil, "user-1", "test-tenant", "policy:write")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/evaluate - evaluate with PII label", func(t *testing.T) {
		evalBody := map[string]interface{}{
			"resourceId":     "res-1",
			"userRole":       "viewer",
			"requestedScope": "read",
			"region":         "us-east-1",
			"labels":         []string{"PII"},
		}
		req, _ := makeAuthRequest("POST", srv.URL+"/api/v1/evaluate", evalBody, "user-1", "test-tenant", "policy:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var decision map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&decision)
		if _, ok := decision["decision"]; !ok {
			t.Error("expected decision in response")
		}
	})

	t.Run("POST /api/v1/evaluate - invalid request", func(t *testing.T) {
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/evaluate", bytes.NewReader([]byte("invalid")))
		token, _ := generateTestJWT("user-1", "test-tenant", "policy:read")
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

	t.Run("POST /api/v1/batch-evaluate - evaluate multiple requests", func(t *testing.T) {
		batchBody := map[string]interface{}{
			"requests": []interface{}{
				map[string]interface{}{
					"resourceId":     "res-1",
					"userRole":       "admin",
					"requestedScope": "read",
					"region":         "us-east-1",
					"labels":         []string{},
				},
				map[string]interface{}{
					"resourceId":     "res-2",
					"userRole":       "viewer",
					"requestedScope": "write",
					"region":         "eu-west-1",
					"labels":         []string{"PII"},
				},
			},
		}
		req, _ := makeAuthRequest("POST", srv.URL+"/api/v1/batch-evaluate", batchBody, "user-1", "test-tenant", "policy:read")
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
		if _, ok := result["decisions"]; !ok {
			t.Error("expected decisions in response")
		}
	})

	t.Run("POST /api/v1/batch-evaluate - invalid request", func(t *testing.T) {
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/batch-evaluate", bytes.NewReader([]byte("invalid")))
		token, _ := generateTestJWT("user-1", "test-tenant", "policy:read")
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
}
