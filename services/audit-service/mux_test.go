package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestAuditServiceRoutes(t *testing.T) {
	t.Setenv("OIDC_JWKS_URL", "test")
	authHdr := "Bearer test-tenant:user-1"
	
	logger, _ := zap.NewDevelopment()
	dal := getTestDAL()
	auditLogger := NewAuditLogger(dal, logger)
	srv := httptest.NewServer(NewMux(auditLogger, "test-license", logger))
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
		body, _ := json.Marshal(map[string]interface{}{
			"actor":     "user-1",
			"action":    "read",
			"resource":  "dataset-1",
			"outcome":   "success",
		})
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/audit/events", bytes.NewReader(body))
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

	t.Run("POST /api/v1/audit/events - without auth", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"action": "read",
		})
		resp, err := http.Post(srv.URL+"/api/v1/audit/events", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/audit/events - without license", func(t *testing.T) {
		noLicenseSrv := httptest.NewServer(NewMux(auditLogger, "", logger))
		defer noLicenseSrv.Close()

		body, _ := json.Marshal(map[string]interface{}{
			"actor":    "user-1",
			"action":   "read",
			"resource": "dataset-1",
			"outcome":  "success",
		})
		req, _ := http.NewRequest("POST", noLicenseSrv.URL+"/api/v1/audit/events", bytes.NewReader(body))
		req.Header.Set("Authorization", authHdr)
		req.Header.Set("Content-Type", "application/json")
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
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/audit/events", bytes.NewReader([]byte("invalid")))
		req.Header.Set("Authorization", authHdr)
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
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/audit/events", nil)
		req.Header.Set("Authorization", authHdr)
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

	t.Run("GET /api/v1/audit/events/{id} - get single event", func(t *testing.T) {
		// First add an event
		body, _ := json.Marshal(map[string]interface{}{
			"actor":     "user-2",
			"action":    "write",
			"resource":  "dataset-2",
			"outcome":   "failure",
		})
		createReq, _ := http.NewRequest("POST", srv.URL+"/api/v1/audit/events", bytes.NewReader(body))
		createReq.Header.Set("Authorization", authHdr)
		createReq.Header.Set("Content-Type", "application/json")
		createResp, _ := http.DefaultClient.Do(createReq)
		var event map[string]interface{}
		json.NewDecoder(createResp.Body).Decode(&event)
		createResp.Body.Close()

		if id, ok := event["id"].(string); ok && id != "" {
			req, _ := http.NewRequest("GET", srv.URL+"/api/v1/audit/events/"+id, nil)
			req.Header.Set("Authorization", authHdr)
			resp, err := http.DefaultClient.Do(req)
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
		req, _ := http.NewRequest("GET", srv.URL+"/api/v1/audit/events/999999", nil)
		req.Header.Set("Authorization", authHdr)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events/{id} - without license", func(t *testing.T) {
		noLicenseSrv := httptest.NewServer(NewMux(auditLogger, "", logger))
		defer noLicenseSrv.Close()

		req, _ := http.NewRequest("GET", noLicenseSrv.URL+"/api/v1/audit/events/some-id", nil)
		req.Header.Set("Authorization", authHdr)
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
		// Note: since we enforce tenant from claims, we need other fields to be missing
		body, _ := json.Marshal(map[string]interface{}{})
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/audit/events", bytes.NewReader(body))
		req.Header.Set("Authorization", authHdr)
		req.Header.Set("Content-Type", "application/json")
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
