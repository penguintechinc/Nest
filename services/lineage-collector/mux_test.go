package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestLineageCollectorRoutes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	store := NewLineageStore()
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

	t.Run("POST /api/v1/lineage/events - ingest event", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"id":          "evt-1",
			"jobName":     "test-job",
			"runId":       "run-1",
			"datasetName": "dataset1",
		})
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/lineage/events", bytes.NewReader(body))
		req.Header.Set("X-Nest-Tenant", "test-tenant")
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

	t.Run("POST /api/v1/lineage - alternate ingest endpoint", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"id":          "evt-2",
			"jobName":     "test-job-2",
			"runId":       "run-2",
			"datasetName": "dataset2",
		})
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/lineage", bytes.NewReader(body))
		req.Header.Set("X-Nest-Tenant", "test-tenant")
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

	t.Run("POST /api/v1/lineage/events - invalid request", func(t *testing.T) {
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/lineage/events", bytes.NewReader([]byte("invalid")))
		req.Header.Set("X-Nest-Tenant", "test-tenant")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/lineage/events - list all events", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/lineage/events")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/lineage/events - filter by tenant", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/lineage/events?tenant=test-tenant")
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

	t.Run("GET /api/v1/lineage/events - filter by job", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/lineage/events?job=test-job")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/lineage/events - filter by run_id", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/lineage/events?run_id=run-1")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/lineage/events - filter with limit", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/lineage/events?limit=5")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/lineage/events - multiple filters", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/lineage/events?tenant=test-tenant&job=test-job&limit=10")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/lineage/datasets/{namespace}/{name} - get dataset lineage", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/lineage/datasets/default/dataset1")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/lineage/datasets/{namespace}/{name} - nonexistent dataset", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/api/v1/lineage/datasets/default/nonexistent")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}
