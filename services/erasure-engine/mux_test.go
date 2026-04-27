package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestErasureEngineRoutes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewErasureStore(logger)
	srv := httptest.NewServer(NewMux(store, logger))
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

	t.Run("POST /api/v1/erase - create erasure request (sync)", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"subjectId":      "user-123",
			"tenant":         "test-tenant",
			"async":          false,
			"backends":       []string{"postgres", "s3"},
			"idempotencyKey": "key-1",
		})
		resp, err := http.Post(srv.URL+"/api/v1/erase", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/erase - create erasure request (async)", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"subjectId":      "user-124",
			"tenant":         "test-tenant",
			"async":          true,
			"backends":       []string{"postgres"},
			"idempotencyKey": "key-2",
		})
		resp, err := http.Post(srv.URL+"/api/v1/erase", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("expected 202, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/erase - missing subjectId", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"tenant": "test-tenant",
		})
		resp, err := http.Post(srv.URL+"/api/v1/erase", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/erase - missing tenant", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"subjectId": "user-125",
		})
		resp, err := http.Post(srv.URL+"/api/v1/erase", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/erase - invalid request", func(t *testing.T) {
		resp, err := http.Post(srv.URL+"/api/v1/erase", "application/json", bytes.NewReader([]byte("invalid")))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/erase/{id} - get erasure request", func(t *testing.T) {
		// Create a request first
		body, _ := json.Marshal(map[string]interface{}{
			"subjectId":      "user-126",
			"tenant":         "test-tenant",
			"async":          false,
			"backends":       []string{"postgres"},
			"idempotencyKey": "key-3",
		})
		createResp, _ := http.Post(srv.URL+"/api/v1/erase", "application/json", bytes.NewReader(body))
		var eraseReq map[string]interface{}
		json.NewDecoder(createResp.Body).Decode(&eraseReq)
		createResp.Body.Close()

		if id, ok := eraseReq["id"].(string); ok && id != "" {
			resp, err := http.Get(srv.URL + "/api/v1/erase/" + id)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				t.Errorf("expected 200, got %d", resp.StatusCode)
			}
		}
	})

	t.Run("GET /api/v1/erase/{id} - not found", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/erase/nonexistent-id")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/erase - list by tenant", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/erase?tenant=test-tenant")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/erase - missing tenant parameter", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/erase")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})
}
