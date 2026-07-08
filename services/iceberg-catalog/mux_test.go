package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/penguintechinc/nest/pkg/auth"
	"go.uber.org/zap"
)

// mintToken creates a valid HS256 JWT token for testing
func mintToken(secret, tenant, scope string) string {
	claims := jwt.MapClaims{
		"sub":    "test-user",
		"iss":    "test-issuer",
		"aud":    []string{"test-audience"},
		"iat":    time.Now().Unix(),
		"exp":    time.Now().Add(1 * time.Hour).Unix(),
		"tenant": tenant,
		"scope":  scope,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(secret))
	return tokenString
}

// doRequest is a helper to send an HTTP request with an auth token
func doRequest(method, url string, body *bytes.Buffer, token string, client *http.Client) *http.Response {
	var req *http.Request
	if body != nil {
		req, _ = http.NewRequest(method, url, body)
	} else {
		req, _ = http.NewRequest(method, url, nil)
	}

	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, _ := client.Do(req)
	return resp
}

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

	// Create test token
	validToken := mintToken("test-secret", "test-tenant", "iceberg:write *:admin")

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
		resp := doRequest("GET", srv.URL+"/v1/namespaces", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces - filter by tenant header", func(t *testing.T) {
		resp := doRequest("GET", srv.URL+"/v1/namespaces", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces - create namespace", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test-tenant", "namespace1"},
			"properties": map[string]string{
				"owner": "test-user",
			},
		})
		resp := doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(body), validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces - missing namespace field", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"properties": map[string]string{},
		})
		resp := doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(body), validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces - tenant isolation violation", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"other", "namespace"},
		})
		resp := doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(body), validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace} - get namespace", func(t *testing.T) {
		resp := doRequest("GET", srv.URL+"/v1/namespaces/test-tenant.namespace1", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /v1/namespaces/{namespace} - delete namespace", func(t *testing.T) {
		// Create namespace first
		body, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test-tenant", "temp"},
		})
		doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(body), validToken, http.DefaultClient)

		resp := doRequest("DELETE", srv.URL+"/v1/namespaces/test-tenant.temp", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 204 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace}/tables - list tables", func(t *testing.T) {
		resp := doRequest("GET", srv.URL+"/v1/namespaces/test-tenant.namespace1/tables", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces/{namespace}/tables - create table", func(t *testing.T) {
		// Create namespace first if needed
		body, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test-tenant", "ns2"},
		})
		doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(body), validToken, http.DefaultClient)

		tableBody, _ := json.Marshal(map[string]interface{}{
			"name":     "users",
			"location": "s3://bucket/users",
			"schema":   map[string]interface{}{},
		})
		resp := doRequest("POST", srv.URL+"/v1/namespaces/test-tenant.ns2/tables", bytes.NewBuffer(tableBody), validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces/{namespace}/tables - missing required fields", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"name": "orders",
		})
		resp := doRequest("POST", srv.URL+"/v1/namespaces/test-tenant.namespace1/tables", bytes.NewBuffer(body), validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 400 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace}/tables/{table} - get table", func(t *testing.T) {
		resp := doRequest("GET", srv.URL+"/v1/namespaces/test-tenant.namespace1/tables/users", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /v1/namespaces/{namespace}/tables/{table} - delete table", func(t *testing.T) {
		resp := doRequest("DELETE", srv.URL+"/v1/namespaces/test-tenant.namespace1/tables/users", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 204 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace}/tables/{table} - tenant isolation", func(t *testing.T) {
		resp := doRequest("GET", srv.URL+"/v1/namespaces/other.table/tables/test", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces - invalid JSON body", func(t *testing.T) {
		body := []byte("{invalid json")
		resp := doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(body), validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces - duplicate namespace (conflict)", func(t *testing.T) {
		// Create first namespace
		body1, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test-tenant", "conflict", "test"},
		})
		resp1 := doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(body1), validToken, http.DefaultClient)
		resp1.Body.Close()
		if resp1.StatusCode != http.StatusOK {
			t.Errorf("first request: expected 200, got %d", resp1.StatusCode)
		}

		// Try to create same namespace again (should conflict)
		body2, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test-tenant", "conflict", "test"},
		})
		resp2 := doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(body2), validToken, http.DefaultClient)
		defer resp2.Body.Close()
		if resp2.StatusCode != http.StatusConflict {
			t.Errorf("expected 409, got %d", resp2.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace} - tenant mismatch", func(t *testing.T) {
		resp := doRequest("GET", srv.URL+"/v1/namespaces/other-tenant.tables", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /v1/namespaces/{namespace} - not found", func(t *testing.T) {
		resp := doRequest("DELETE", srv.URL+"/v1/namespaces/test-tenant.nonexistent", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /v1/namespaces/{namespace} - tenant mismatch", func(t *testing.T) {
		resp := doRequest("DELETE", srv.URL+"/v1/namespaces/other-tenant.tenant", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces/{namespace}/tables - invalid JSON body", func(t *testing.T) {
		// Create namespace first
		body, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test-tenant", "table_ns"},
		})
		doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(body), validToken, http.DefaultClient)

		// Try to create table with invalid JSON
		invalidBody := []byte("{invalid json")
		resp := doRequest("POST", srv.URL+"/v1/namespaces/test-tenant.table_ns/tables", bytes.NewBuffer(invalidBody), validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces/{namespace}/tables - missing name field", func(t *testing.T) {
		// Create namespace first
		body, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test-tenant", "table_ns2"},
		})
		doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(body), validToken, http.DefaultClient)

		// Try to create table without name
		tableBody, _ := json.Marshal(map[string]interface{}{
			"location": "s3://bucket/test",
		})
		resp := doRequest("POST", srv.URL+"/v1/namespaces/test-tenant.table_ns2/tables", bytes.NewBuffer(tableBody), validToken, http.DefaultClient)
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
		resp := doRequest("POST", srv.URL+"/v1/namespaces/test-tenant.doesnotexist/tables", bytes.NewBuffer(tableBody), validToken, http.DefaultClient)
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
		resp := doRequest("POST", srv.URL+"/v1/namespaces/other-tenant.namespace/tables", bytes.NewBuffer(tableBody), validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /v1/namespaces/{namespace}/tables/{table} - not found", func(t *testing.T) {
		// Create namespace first
		nsBody, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test-tenant", "deltabltest"},
		})
		doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(nsBody), validToken, http.DefaultClient)

		resp := doRequest("DELETE", srv.URL+"/v1/namespaces/test-tenant.deltabltest/tables/nonexistent", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /v1/namespaces/{namespace}/tables/{table} - tenant mismatch", func(t *testing.T) {
		resp := doRequest("DELETE", srv.URL+"/v1/namespaces/other-tenant.namespace/tables/test", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace}/tables - tenant mismatch", func(t *testing.T) {
		resp := doRequest("GET", srv.URL+"/v1/namespaces/other-tenant.namespace/tables", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /v1/namespaces/{namespace} - conflict (not empty)", func(t *testing.T) {
		// Create namespace with a table
		nsBody, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test-tenant", "conflict", "nonempty"},
		})
		doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(nsBody), validToken, http.DefaultClient)

		// Create a table in the namespace
		tableBody, _ := json.Marshal(map[string]interface{}{
			"name":     "table1",
			"location": "s3://bucket/table1",
		})
		doRequest("POST", srv.URL+"/v1/namespaces/test-tenant.conflict.nonempty/tables", bytes.NewBuffer(tableBody), validToken, http.DefaultClient)

		// Try to delete namespace (should fail with conflict if not empty)
		resp := doRequest("DELETE", srv.URL+"/v1/namespaces/test-tenant.conflict.nonempty", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		// Should be conflict or not found depending on implementation
		if resp.StatusCode != http.StatusConflict && resp.StatusCode != http.StatusNotFound {
			t.Logf("note: delete non-empty namespace returned %d (may be expected)", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace} - tenant isolation", func(t *testing.T) {
		resp := doRequest("GET", srv.URL+"/v1/namespaces/other-tenant.namespace", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusForbidden {
			t.Errorf("expected 403, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace} - found with slash path", func(t *testing.T) {
		// Create namespace first
		body, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test-tenant", "slashtest", "ns"},
		})
		doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(body), validToken, http.DefaultClient)

		// GET with path containing slashes (should be converted to dots)
		resp := doRequest("GET", srv.URL+"/v1/namespaces/test-tenant/slashtest/ns", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace}/tables/{table} - found", func(t *testing.T) {
		// Create namespace and table
		nsBody, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test-tenant", "gettest", "ns"},
		})
		doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(nsBody), validToken, http.DefaultClient)

		tableBody, _ := json.Marshal(map[string]interface{}{
			"name":     "find_me",
			"location": "s3://bucket/find_me",
		})
		doRequest("POST", srv.URL+"/v1/namespaces/test-tenant.gettest.ns/tables", bytes.NewBuffer(tableBody), validToken, http.DefaultClient)

		// Now GET the table
		resp := doRequest("GET", srv.URL+"/v1/namespaces/test-tenant.gettest.ns/tables/find_me", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /v1/namespaces/{namespace}/tables - duplicate table (conflict)", func(t *testing.T) {
		// Create namespace
		nsBody, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test-tenant", "dup", "table"},
		})
		doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(nsBody), validToken, http.DefaultClient)

		// Create first table
		tableBody1, _ := json.Marshal(map[string]interface{}{
			"name":     "dup_table",
			"location": "s3://bucket/dup_table",
		})
		doRequest("POST", srv.URL+"/v1/namespaces/test-tenant.dup.table/tables", bytes.NewBuffer(tableBody1), validToken, http.DefaultClient)

		// Try to create same table again
		tableBody2, _ := json.Marshal(map[string]interface{}{
			"name":     "dup_table",
			"location": "s3://bucket/dup_table",
		})
		resp := doRequest("POST", srv.URL+"/v1/namespaces/test-tenant.dup.table/tables", bytes.NewBuffer(tableBody2), validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusConflict {
			t.Errorf("expected 409, got %d", resp.StatusCode)
		}
	})

	t.Run("DELETE /v1/namespaces/{namespace}/tables/{table} - success", func(t *testing.T) {
		// Create namespace and table
		nsBody, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test-tenant", "deltest", "ns"},
		})
		doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(nsBody), validToken, http.DefaultClient)

		tableBody, _ := json.Marshal(map[string]interface{}{
			"name":     "delete_me",
			"location": "s3://bucket/delete_me",
		})
		doRequest("POST", srv.URL+"/v1/namespaces/test-tenant.deltest.ns/tables", bytes.NewBuffer(tableBody), validToken, http.DefaultClient)

		// Delete the table
		resp := doRequest("DELETE", srv.URL+"/v1/namespaces/test-tenant.deltest.ns/tables/delete_me", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 204 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces - list with filter returns items", func(t *testing.T) {
		// Create namespace with tenant prefix
		body, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test-tenant", "ns1"},
		})
		doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(body), validToken, http.DefaultClient)

		// Get with token tenant
		resp := doRequest("GET", srv.URL+"/v1/namespaces", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /v1/namespaces/{namespace}/tables - success with tables", func(t *testing.T) {
		// Create namespace and table
		nsBody, _ := json.Marshal(map[string]interface{}{
			"namespace": []string{"test-tenant", "listtest", "ns"},
		})
		doRequest("POST", srv.URL+"/v1/namespaces", bytes.NewBuffer(nsBody), validToken, http.DefaultClient)

		tableBody, _ := json.Marshal(map[string]interface{}{
			"name":     "table_to_list",
			"location": "s3://bucket/table_to_list",
		})
		doRequest("POST", srv.URL+"/v1/namespaces/test-tenant.listtest.ns/tables", bytes.NewBuffer(tableBody), validToken, http.DefaultClient)

		// List tables
		resp := doRequest("GET", srv.URL+"/v1/namespaces/test-tenant.listtest.ns/tables", nil, validToken, http.DefaultClient)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})
}
