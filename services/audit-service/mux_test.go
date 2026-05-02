package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestAuditServiceRoutes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	auditLogger := NewAuditLogger(logger)
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
			"tenant":    "test-tenant",
			"actor":     "user-1",
			"action":    "read",
			"resource":  "dataset-1",
			"outcome":   "success",
		})
		resp, err := http.Post(srv.URL+"/api/v1/audit/events", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected 201, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/audit/events - without license", func(t *testing.T) {
		noLicenseSrv := httptest.NewServer(NewMux(auditLogger, "", logger))
		defer noLicenseSrv.Close()

		body, _ := json.Marshal(map[string]interface{}{
			"tenant":   "test-tenant",
			"actor":    "user-1",
			"action":   "read",
			"resource": "dataset-1",
			"outcome":  "success",
		})
		resp, err := http.Post(noLicenseSrv.URL+"/api/v1/audit/events", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/audit/events - invalid request", func(t *testing.T) {
		resp, err := http.Post(srv.URL+"/api/v1/audit/events", "application/json", bytes.NewReader([]byte("invalid")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - list all events", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/audit/events")
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
		resp, err := http.Get(srv.URL + "/api/v1/audit/events?tenant=test-tenant")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - filter by actor", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/audit/events?actor=user-1")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - filter by action", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/audit/events?action=read")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - filter by resource", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/audit/events?resource=dataset-1")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - filter by outcome", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/audit/events?outcome=success")
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

		resp, err := http.Get(srv.URL + "/api/v1/audit/events?start_time=" + startTime + "&end_time=" + endTime)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - filter with limit and offset", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/audit/events?limit=10&offset=0")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events/{id} - get single event", func(t *testing.T) {
		// First add an event
		body, _ := json.Marshal(map[string]interface{}{
			"tenant":    "test-tenant",
			"actor":     "user-2",
			"action":    "write",
			"resource":  "dataset-2",
			"outcome":   "failure",
		})
		createResp, _ := http.Post(srv.URL+"/api/v1/audit/events", "application/json", bytes.NewReader(body))
		var event map[string]interface{}
		json.NewDecoder(createResp.Body).Decode(&event)
		createResp.Body.Close()

		if id, ok := event["id"].(string); ok && id != "" {
			resp, err := http.Get(srv.URL + "/api/v1/audit/events/" + id)
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
		resp, err := http.Get(srv.URL + "/api/v1/audit/events/nonexistent-id")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - invalid start_time", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/audit/events?start_time=invalid-date")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 (invalid time ignored), got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - invalid end_time", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/audit/events?end_time=bad-date")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 (invalid time ignored), got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - invalid limit", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/audit/events?limit=not-a-number")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200 (invalid limit ignored), got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - invalid offset", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/audit/events?offset=abc")
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
		noLicenseSrv := httptest.NewServer(NewMux(auditLogger, "", logger))
		defer noLicenseSrv.Close()

		resp, err := http.Get(noLicenseSrv.URL + "/api/v1/audit/events/some-id")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/audit/events - without license", func(t *testing.T) {
		noLicenseSrv := httptest.NewServer(NewMux(auditLogger, "", logger))
		defer noLicenseSrv.Close()

		resp, err := http.Get(noLicenseSrv.URL + "/api/v1/audit/events")
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
		body, _ := json.Marshal(map[string]interface{}{})
		resp, err := http.Post(srv.URL+"/api/v1/audit/events", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400 on append error, got %d", resp.StatusCode)
		}
	})
}
