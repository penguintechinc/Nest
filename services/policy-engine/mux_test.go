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

func TestPolicyEngineRoutes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewPolicyStore()

	// Create test auth middleware
	authMiddleware, err := auth.NewMiddleware(&auth.Config{
		Algorithm:    "HS256",
		SharedSecret: "test-secret",
		Issuer:       "test-issuer",
		Audience:     "test-audience",
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
		resp, err := http.Get(srv.URL + "/api/v1/policies")
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
		resp, err := http.Get(srv.URL + "/api/v1/policies?tenant=test-tenant")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/policies - create policy", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"id":       "policy-1",
			"name":     "Test Policy",
			"tenant":   "test-tenant",
			"rules":    []interface{}{},
			"priority": 1,
		})
		resp, err := http.Post(srv.URL+"/api/v1/policies", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected 201, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/policies - invalid request", func(t *testing.T) {
		resp, err := http.Post(srv.URL+"/api/v1/policies", "application/json", bytes.NewReader([]byte("invalid")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/policies/{id} - get policy", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"name":     "Test Policy 2",
			"tenant":   "test-tenant",
			"labels":   []interface{}{"INTERNAL"},
			"action":   "allow",
			"priority": 2,
		})
		cr, _ := http.Post(srv.URL+"/api/v1/policies", "application/json", bytes.NewReader(body))
		defer cr.Body.Close()
		var rule map[string]interface{}
		json.NewDecoder(cr.Body).Decode(&rule)
		id := rule["id"].(string)

		resp, err := http.Get(srv.URL + "/api/v1/policies/" + id)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/policies/{id} - not found", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/policies/nonexistent")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /api/v1/policies/{id} - delete policy", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"name":     "Test Policy 3",
			"tenant":   "test-tenant",
			"labels":   []interface{}{"INTERNAL"},
			"action":   "allow",
			"priority": 3,
		})
		cr, _ := http.Post(srv.URL+"/api/v1/policies", "application/json", bytes.NewReader(body))
		defer cr.Body.Close()
		var rule3 map[string]interface{}
		json.NewDecoder(cr.Body).Decode(&rule3)
		delID := rule3["id"].(string)

		req, _ := http.NewRequest("DELETE", srv.URL+"/api/v1/policies/"+delID, nil)
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
		req, _ := http.NewRequest("DELETE", srv.URL+"/api/v1/policies/nonexistent", nil)
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
		body, _ := json.Marshal(map[string]interface{}{
			"resourceId":    "res-1",
			"userRole":      "viewer",
			"requestedScope": "read",
			"region":        "us-east-1",
			"labels":        []string{"PII"},
		})
		resp, err := http.Post(srv.URL+"/api/v1/evaluate", "application/json", bytes.NewReader(body))
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
		resp, err := http.Post(srv.URL+"/api/v1/evaluate", "application/json", bytes.NewReader([]byte("invalid")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/batch-evaluate - evaluate multiple requests", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"requests": []interface{}{
				map[string]interface{}{
					"resourceId":    "res-1",
					"userRole":      "admin",
					"requestedScope": "read",
					"region":        "us-east-1",
					"labels":        []string{},
				},
				map[string]interface{}{
					"resourceId":    "res-2",
					"userRole":      "viewer",
					"requestedScope": "write",
					"region":        "eu-west-1",
					"labels":        []string{"PII"},
				},
			},
		})
		resp, err := http.Post(srv.URL+"/api/v1/batch-evaluate", "application/json", bytes.NewReader(body))
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
		resp, err := http.Post(srv.URL+"/api/v1/batch-evaluate", "application/json", bytes.NewReader([]byte("invalid")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})
}
