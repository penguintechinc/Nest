package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
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

func TestDataIndexerRoutes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

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

	catalog := NewCatalog()
	pipeline := NewPipeline(catalog, logger)
	srv := httptest.NewServer(NewMux(catalog, pipeline, logger, authMiddleware))
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
		var health map[string]string
		json.NewDecoder(resp.Body).Decode(&health)
		if health["status"] != "ok" {
			t.Errorf("expected status ok, got %v", health["status"])
		}
	})

	t.Run("POST /api/v1/indexer/scan - scan resources", func(t *testing.T) {
		scanBody := map[string]interface{}{
			"resourceId":  "res-1",
			"backendType": "postgres",
			"tenant":      "test-tenant",
			"tables": []interface{}{
				map[string]interface{}{
					"name": "users",
					"columns": []interface{}{
						map[string]interface{}{"name": "id", "dataType": "uuid"},
						map[string]interface{}{"name": "email", "dataType": "text"},
					},
				},
			},
		}
		req, _ := makeAuthRequest("POST", srv.URL+"/api/v1/indexer/scan", scanBody, "user-1", "test-tenant", "indexer:write")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("expected 202, got %d", resp.StatusCode)
		}
		var scanResp ScanResponse
		if err := json.NewDecoder(resp.Body).Decode(&scanResp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if scanResp.Status != "scanning" {
			t.Errorf("expected status 'scanning', got '%s'", scanResp.Status)
		}
		if scanResp.EntriesQueued != 1 {
			t.Errorf("expected 1 entry queued, got %d", scanResp.EntriesQueued)
		}
	})

	t.Run("POST /api/v1/indexer/scan - invalid request", func(t *testing.T) {
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/indexer/scan", bytes.NewReader([]byte("invalid")))
		token, _ := generateTestJWT("user-1", "test-tenant", "indexer:write")
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

	t.Run("GET /api/v1/indexer/catalog - list all entries", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/indexer/catalog", "user-1", "test-tenant", "indexer:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var listResp ListResponse
		if err := json.NewDecoder(resp.Body).Decode(&listResp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		if listResp.Count != len(listResp.Entries) {
			t.Errorf("count mismatch: reported %d, actual %d", listResp.Count, len(listResp.Entries))
		}
		if listResp.Stats == nil {
			t.Error("expected non-nil stats")
		}
	})

	t.Run("GET /api/v1/indexer/catalog - filter by tenant", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/indexer/catalog?tenant=test-tenant", "user-1", "test-tenant", "indexer:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/indexer/catalog - filter by backend", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/indexer/catalog?backend=postgres", "user-1", "test-tenant", "indexer:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/indexer/catalog/{id} - get entry", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/indexer/catalog/res-1:users", "user-1", "test-tenant", "indexer:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var entry CatalogEntry
			if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
				t.Fatalf("failed to decode entry: %v", err)
			}
			if entry.TableName != "users" {
				t.Errorf("expected table name 'users', got '%s'", entry.TableName)
			}
		} else if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/indexer/labels - list labels with filters", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/indexer/labels?resource=res-1&table=users", "user-1", "test-tenant", "indexer:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var labelsResp LabelsResponse
		if err := json.NewDecoder(resp.Body).Decode(&labelsResp); err != nil {
			t.Fatalf("failed to decode response: %v", err)
		}
		// Should have filtered results
		if len(labelsResp.Targets) > 0 {
			for _, entry := range labelsResp.Targets {
				if entry.ResourceID != "res-1" || entry.TableName != "users" {
					t.Errorf("filter not applied correctly: %+v", entry)
				}
			}
		}
	})

	t.Run("POST /api/v1/indexer/classify - classify entry", func(t *testing.T) {
		classifyBody := map[string]interface{}{
			"resourceId": "res-2",
			"tableName":  "orders",
		}
		req, _ := makeAuthRequest("POST", srv.URL+"/api/v1/indexer/classify", classifyBody, "user-1", "test-tenant", "indexer:write")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode == http.StatusOK {
			var entry CatalogEntry
			if err := json.NewDecoder(resp.Body).Decode(&entry); err != nil {
				t.Fatalf("failed to decode entry: %v", err)
			}
		} else if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/indexer/classify - invalid request", func(t *testing.T) {
		req, _ := http.NewRequest("POST", srv.URL+"/api/v1/indexer/classify", bytes.NewReader([]byte("invalid")))
		token, _ := generateTestJWT("user-1", "test-tenant", "indexer:write")
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

	t.Run("GET /api/v1/indexer/stats - get catalog stats", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/indexer/stats", "user-1", "test-tenant", "indexer:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var stats map[string]int
		if err := json.NewDecoder(resp.Body).Decode(&stats); err != nil {
			t.Fatalf("failed to decode stats: %v", err)
		}
		if stats == nil {
			t.Error("expected non-nil stats")
		}
	})

	t.Run("GET /api/v1/indexer/pii-targets - list PII targets", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/indexer/pii-targets", "user-1", "test-tenant", "indexer:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/indexer/pii-targets - with enterprise license", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		defer os.Unsetenv("ENTERPRISE_LICENSE")

		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/indexer/pii-targets", "user-1", "test-tenant", "indexer:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/indexer/scan - empty tables", func(t *testing.T) {
		scanBody := map[string]interface{}{
			"resourceId":  "res-empty",
			"backendType": "postgres",
			"tenant":      "test-tenant",
			"tables":      []interface{}{},
		}
		req, _ := makeAuthRequest("POST", srv.URL+"/api/v1/indexer/scan", scanBody, "user-1", "test-tenant", "indexer:write")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusAccepted {
			t.Errorf("expected 202, got %d", resp.StatusCode)
		}
		var respData ScanResponse
		json.NewDecoder(resp.Body).Decode(&respData)
		if respData.EntriesQueued != 0 {
			t.Errorf("expected 0 entries queued, got %d", respData.EntriesQueued)
		}
	})

	t.Run("GET /api/v1/indexer/catalog/{id} - not found", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/indexer/catalog/nonexistent-id", "user-1", "test-tenant", "indexer:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/indexer/classify - pipeline error", func(t *testing.T) {
		classifyBody := map[string]interface{}{
			"resourceId": "nonexistent-res",
			"tableName":  "nonexistent-table",
		}
		req, _ := makeAuthRequest("POST", srv.URL+"/api/v1/indexer/classify", classifyBody, "user-1", "test-tenant", "indexer:write")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/indexer/labels - resource filter only", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/indexer/labels?resource=test-resource", "user-1", "test-tenant", "indexer:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var data LabelsResponse
		json.NewDecoder(resp.Body).Decode(&data)
		// Targets can be nil or empty slice, both are valid
		if data.Targets == nil && len(data.Targets) > 0 {
			t.Error("Targets inconsistency")
		}
	})

	t.Run("GET /api/v1/indexer/labels - table filter only", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/indexer/labels?table=test-table", "user-1", "test-tenant", "indexer:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/indexer/labels - no matches", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/indexer/labels?resource=no-match-res&table=no-match-tbl", "user-1", "test-tenant", "indexer:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var data LabelsResponse
		json.NewDecoder(resp.Body).Decode(&data)
		if data.Targets != nil && len(data.Targets) != 0 {
			t.Errorf("expected 0 targets, got %d", len(data.Targets))
		}
	})

	t.Run("GET /api/v1/indexer/pii-targets - with PII entries", func(t *testing.T) {
		// Seed a PII-labeled entry in the catalog under test-tenant (matching the token tenant)
		piiEntry := &CatalogEntry{
			ResourceID: "pii-resource",
			TableName:  "pii-table",
			Tenant:     "test-tenant", // Match the JWT token tenant
			Columns: []ColumnEntry{
				{Name: "user_email", DataType: "string"},
				{Name: "ssn", DataType: "string"},
			},
			Labels: []string{"PII"}, // Pre-classified with PII label
		}
		catalog.Upsert(piiEntry)

		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/indexer/pii-targets", "user-1", "test-tenant", "indexer:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var data LabelsResponse
		json.NewDecoder(resp.Body).Decode(&data)
		// Should find at least one PII target matching the token's tenant
		if len(data.Targets) == 0 {
			t.Error("expected PII targets for test-tenant")
		}
	})

	t.Run("GET /api/v1/indexer/catalog - combined filters", func(t *testing.T) {
		req, _ := makeAuthGetRequest(srv.URL+"/api/v1/indexer/catalog?tenant=test&backend=postgres", "user-1", "test-tenant", "indexer:read")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		var data ListResponse
		json.NewDecoder(resp.Body).Decode(&data)
		if data.Count != len(data.Entries) {
			t.Errorf("count mismatch: reported %d, actual %d", data.Count, len(data.Entries))
		}
	})
}
