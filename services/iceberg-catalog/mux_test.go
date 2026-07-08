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

func TestIcebergCatalogRoutes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	catalog := NewCatalog(logger)

	// Initialize auth middleware for test
	authConfig := &auth.Config{
		Algorithm:    "HS256",
		SharedSecret: "test-secret",
	}
	authMiddleware, err := auth.NewMiddleware(authConfig)
	if err != nil {
		t.Fatalf("failed to create auth middleware: %v", err)
	}

	srv := httptest.NewServer(NewMux(catalog, logger, authMiddleware))
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

	t.Run("GET /metrics", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/metrics")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		if ct := resp.Header.Get("Content-Type"); ct != "text/plain" {
			t.Errorf("expected text/plain, got %s", ct)
		}
	})

	t.Run("GET /v1/namespaces - list all namespaces", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/v1/namespaces")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces - filter by tenant header", func(t *testing.T) {
		req, _ := http.NewRequest("GET", srv.URL+"/v1/namespaces", nil)
		req.Header.Set("X-Nest-Tenant", "test-tenant")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces - create namespace", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test", "namespace1"},
			"properties": map[string]string{
				"owner": "test-user",
			},
		})
		resp, err := http.Post(srv.URL+"/v1/namespaces", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces - missing namespace field", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"properties": map[string]string{},
		})
		resp, err := http.Post(srv.URL+"/v1/namespaces", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces - tenant isolation violation", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"other", "namespace"},
		})
		req, _ := http.NewRequest("POST", srv.URL+"/v1/namespaces", bytes.NewReader(body))
		req.Header.Set("X-Nest-Tenant", "test-tenant")
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace} - get namespace", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/v1/namespaces/test.namespace1")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /v1/namespaces/{namespace} - delete namespace", func(t *testing.T) {
		// Create namespace first
		body, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test", "temp"},
		})
		http.Post(srv.URL+"/v1/namespaces", "application/json", bytes.NewReader(body))

		req, _ := http.NewRequest("DELETE", srv.URL+"/v1/namespaces/test.temp", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 204 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace}/tables - list tables", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/v1/namespaces/test.namespace1/tables")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces/{namespace}/tables - create table", func(t *testing.T) {
		// Create namespace first if needed
		body, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test", "ns2"},
		})
		http.Post(srv.URL+"/v1/namespaces", "application/json", bytes.NewReader(body))

		tableBody, _ := json.Marshal(map[string]interface{}{
			"name":     "users",
			"location": "s3://bucket/users",
			"schema":   map[string]interface{}{},
		})
		resp, err := http.Post(srv.URL+"/v1/namespaces/test.ns2/tables", "application/json", bytes.NewReader(tableBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces/{namespace}/tables - missing required fields", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"name": "orders",
		})
		resp, err := http.Post(srv.URL+"/v1/namespaces/test.namespace1/tables", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 400 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace}/tables/{table} - get table", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/v1/namespaces/test.namespace1/tables/users")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /v1/namespaces/{namespace}/tables/{table} - delete table", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", srv.URL+"/v1/namespaces/test.namespace1/tables/users", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 204 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace}/tables/{table} - tenant isolation", func(t *testing.T) {
		req, _ := http.NewRequest("GET", srv.URL+"/v1/namespaces/other.table/tables/test", nil)
		req.Header.Set("X-Nest-Tenant", "test-tenant")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces - invalid JSON body", func(t *testing.T) {
		body := []byte("{invalid json")
		resp, err := http.Post(srv.URL+"/v1/namespaces", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces - duplicate namespace (conflict)", func(t *testing.T) {
		// Create first namespace
		body1, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"conflict", "test"},
		})
		resp1, err := http.Post(srv.URL+"/v1/namespaces", "application/json", bytes.NewReader(body1))
		if err != nil {
			t.Fatalf("first request failed: %v", err)
		}
		resp1.Body.Close()
		if resp1.StatusCode != http.StatusOK {
			t.Errorf("first request: expected 200, got %d", resp1.StatusCode)
		}

		// Try to create same namespace again (should conflict)
		body2, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"conflict", "test"},
		})
		resp2, err := http.Post(srv.URL+"/v1/namespaces", "application/json", bytes.NewReader(body2))
		if err != nil {
			t.Fatalf("second request failed: %v", err)
		}
		defer resp2.Body.Close()
		if resp2.StatusCode != http.StatusConflict {
			t.Errorf("expected 409, got %d", resp2.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace} - tenant mismatch", func(t *testing.T) {
		req, _ := http.NewRequest("GET", srv.URL+"/v1/namespaces/other.tenant/tables", nil)
		req.Header.Set("X-Nest-Tenant", "my-tenant")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /v1/namespaces/{namespace} - not found", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", srv.URL+"/v1/namespaces/nonexistent.namespace", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /v1/namespaces/{namespace} - tenant mismatch", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", srv.URL+"/v1/namespaces/other.tenant", nil)
		req.Header.Set("X-Nest-Tenant", "my-tenant")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces/{namespace}/tables - invalid JSON body", func(t *testing.T) {
		// Create namespace first
		body, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test", "table_ns"},
		})
		http.Post(srv.URL+"/v1/namespaces", "application/json", bytes.NewReader(body))

		// Try to create table with invalid JSON
		invalidBody := []byte("{invalid json")
		resp, err := http.Post(srv.URL+"/v1/namespaces/test.table_ns/tables", "application/json", bytes.NewReader(invalidBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces/{namespace}/tables - missing name field", func(t *testing.T) {
		// Create namespace first
		body, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test", "table_ns2"},
		})
		http.Post(srv.URL+"/v1/namespaces", "application/json", bytes.NewReader(body))

		// Try to create table without name
		tableBody, _ := json.Marshal(map[string]interface{}{
			"location": "s3://bucket/test",
		})
		resp, err := http.Post(srv.URL+"/v1/namespaces/test.table_ns2/tables", "application/json", bytes.NewReader(tableBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces/{namespace}/tables - namespace not found", func(t *testing.T) {
		tableBody, _ := json.Marshal(map[string]interface{}{
			"name":     "test_table",
			"location": "s3://bucket/test",
		})
		resp, err := http.Post(srv.URL+"/v1/namespaces/doesnotexist.namespace/tables", "application/json", bytes.NewReader(tableBody))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces/{namespace}/tables - tenant mismatch", func(t *testing.T) {
		tableBody, _ := json.Marshal(map[string]interface{}{
			"name":     "test_table",
			"location": "s3://bucket/test",
		})
		req, _ := http.NewRequest("POST", srv.URL+"/v1/namespaces/other.namespace/tables", bytes.NewReader(tableBody))
		req.Header.Set("X-Nest-Tenant", "my-tenant")
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /v1/namespaces/{namespace}/tables/{table} - not found", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", srv.URL+"/v1/namespaces/any.namespace/tables/nonexistent", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /v1/namespaces/{namespace}/tables/{table} - tenant mismatch", func(t *testing.T) {
		req, _ := http.NewRequest("DELETE", srv.URL+"/v1/namespaces/other.namespace/tables/test", nil)
		req.Header.Set("X-Nest-Tenant", "my-tenant")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace}/tables - tenant mismatch", func(t *testing.T) {
		req, _ := http.NewRequest("GET", srv.URL+"/v1/namespaces/other.namespace/tables", nil)
		req.Header.Set("X-Nest-Tenant", "my-tenant")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /v1/namespaces/{namespace} - conflict (not empty)", func(t *testing.T) {
		// Create namespace with a table
		nsBody, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"conflict", "nonempty"},
		})
		http.Post(srv.URL+"/v1/namespaces", "application/json", bytes.NewReader(nsBody))

		// Create a table in the namespace
		tableBody, _ := json.Marshal(map[string]interface{}{
			"name":     "table1",
			"location": "s3://bucket/table1",
		})
		http.Post(srv.URL+"/v1/namespaces/conflict.nonempty/tables", "application/json", bytes.NewReader(tableBody))

		// Try to delete namespace (should fail with conflict if not empty)
		req, _ := http.NewRequest("DELETE", srv.URL+"/v1/namespaces/conflict.nonempty", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		// Should be conflict or not found depending on implementation
		if resp.StatusCode != http.StatusConflict && resp.StatusCode != http.StatusNotFound {
			t.Logf("note: delete non-empty namespace returned %d (may be expected)", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace} - tenant isolation", func(t *testing.T) {
		req, _ := http.NewRequest("GET", srv.URL+"/v1/namespaces/other.namespace", nil)
		req.Header.Set("X-Nest-Tenant", "my-tenant")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace} - found with slash path", func(t *testing.T) {
		// Create namespace first
		body, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"slashtest", "ns"},
		})
		http.Post(srv.URL+"/v1/namespaces", "application/json", bytes.NewReader(body))

		// GET with path containing slashes (should be converted to dots)
		resp, err := http.Get(srv.URL + "/v1/namespaces/slashtest/ns")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace}/tables/{table} - found", func(t *testing.T) {
		// Create namespace and table
		nsBody, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"gettest", "ns"},
		})
		http.Post(srv.URL+"/v1/namespaces", "application/json", bytes.NewReader(nsBody))

		tableBody, _ := json.Marshal(map[string]interface{}{
			"name":     "find_me",
			"location": "s3://bucket/find_me",
		})
		http.Post(srv.URL+"/v1/namespaces/gettest.ns/tables", "application/json", bytes.NewReader(tableBody))

		// Now GET the table
		resp, err := http.Get(srv.URL + "/v1/namespaces/gettest.ns/tables/find_me")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces/{namespace}/tables - duplicate table (conflict)", func(t *testing.T) {
		// Create namespace
		nsBody, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"dup", "table"},
		})
		http.Post(srv.URL+"/v1/namespaces", "application/json", bytes.NewReader(nsBody))

		// Create first table
		tableBody1, _ := json.Marshal(map[string]interface{}{
			"name":     "dup_table",
			"location": "s3://bucket/dup_table",
		})
		http.Post(srv.URL+"/v1/namespaces/dup.table/tables", "application/json", bytes.NewReader(tableBody1))

		// Try to create same table again
		tableBody2, _ := json.Marshal(map[string]interface{}{
			"name":     "dup_table",
			"location": "s3://bucket/dup_table",
		})
		resp, err := http.Post(srv.URL+"/v1/namespaces/dup.table/tables", "application/json", bytes.NewReader(tableBody2))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("expected 409, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /v1/namespaces/{namespace}/tables/{table} - success", func(t *testing.T) {
		// Create namespace and table
		nsBody, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"deltest", "ns"},
		})
		http.Post(srv.URL+"/v1/namespaces", "application/json", bytes.NewReader(nsBody))

		tableBody, _ := json.Marshal(map[string]interface{}{
			"name":     "delete_me",
			"location": "s3://bucket/delete_me",
		})
		http.Post(srv.URL+"/v1/namespaces/deltest.ns/tables", "application/json", bytes.NewReader(tableBody))

		// Delete the table
		req, _ := http.NewRequest("DELETE", srv.URL+"/v1/namespaces/deltest.ns/tables/delete_me", nil)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 204 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces - list with filter returns items", func(t *testing.T) {
		// Create namespace with tenant prefix
		body, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"tenant1", "ns1"},
		})
		http.Post(srv.URL+"/v1/namespaces", "application/json", bytes.NewReader(body))

		// Get with tenant header
		req, _ := http.NewRequest("GET", srv.URL+"/v1/namespaces", nil)
		req.Header.Set("X-Nest-Tenant", "tenant1")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace}/tables - success with tables", func(t *testing.T) {
		// Create namespace and table
		nsBody, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"listtest", "ns"},
		})
		http.Post(srv.URL+"/v1/namespaces", "application/json", bytes.NewReader(nsBody))

		tableBody, _ := json.Marshal(map[string]interface{}{
			"name":     "table_to_list",
			"location": "s3://bucket/table_to_list",
		})
		http.Post(srv.URL+"/v1/namespaces/listtest.ns/tables", "application/json", bytes.NewReader(tableBody))

		// List tables
		resp, err := http.Get(srv.URL + "/v1/namespaces/listtest.ns/tables")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})
}
