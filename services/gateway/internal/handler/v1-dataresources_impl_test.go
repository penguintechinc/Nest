package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
)

// ----------------------------------------------------------------------------
// DataResource CRUD — full K8s fake-client tests
// ----------------------------------------------------------------------------

func TestDataresourceCreate_K8s(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := dataresourceCreateHandler(cfg, logger)

	body := map[string]interface{}{
		"name":  "pg1",
		"type":  "postgres",
		"class": "standard",
		"ha":    true,
		"size":  "50Gi",
	}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/dataresources", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("create status = %d, want %d: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	dr := resp["dataresource"].(map[string]interface{})
	if dr["name"] != "pg1" {
		t.Errorf("name = %v, want pg1", dr["name"])
	}
	if dr["type"] != "postgres" {
		t.Errorf("type = %v, want postgres", dr["type"])
	}
	if dr["tenant"] != "tenant1" {
		t.Errorf("tenant = %v, want tenant1", dr["tenant"])
	}

	if w.Header().Get("Location") == "" {
		t.Error("Location header not set")
	}
}

func TestDataresourceCreate_Conflict(t *testing.T) {
	existing := makeDataResource("pg1", "tenant1", "postgres")
	cfg := config.Config{K8sClient: makeFakeK8sClient(existing)}
	logger := zap.NewNop()
	h := dataresourceCreateHandler(cfg, logger)

	body := map[string]interface{}{"name": "pg1", "type": "postgres"}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/dataresources", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestDataresourceList_K8s(t *testing.T) {
	dr1 := makeDataResource("pg1", "tenant1", "postgres")
	dr2 := makeDataResource("kv1", "tenant1", "keyvalue")
	cfg := config.Config{K8sClient: makeFakeK8sClient(dr1, dr2)}
	logger := zap.NewNop()
	h := dataresourceListHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/dataresources", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	items := resp["dataresources"].([]interface{})
	if len(items) != 2 {
		t.Errorf("count = %d, want 2", len(items))
	}

	pagination := resp["pagination"].(map[string]interface{})
	if pagination["total"].(float64) != 2 {
		t.Errorf("total = %v, want 2", pagination["total"])
	}
}

func TestDataresourceList_Empty(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := dataresourceListHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/dataresources", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	items := resp["dataresources"].([]interface{})
	if len(items) != 0 {
		t.Errorf("count = %d, want 0", len(items))
	}
}

func TestDataresourceList_Pagination(t *testing.T) {
	// Create 5 resources
	objs := make([]interface{}, 0)
	_ = objs
	dr1 := makeDataResource("pg1", "tenant1", "postgres")
	dr2 := makeDataResource("pg2", "tenant1", "postgres")
	dr3 := makeDataResource("pg3", "tenant1", "postgres")
	dr4 := makeDataResource("pg4", "tenant1", "postgres")
	dr5 := makeDataResource("pg5", "tenant1", "postgres")

	cfg := config.Config{K8sClient: makeFakeK8sClient(dr1, dr2, dr3, dr4, dr5)}
	logger := zap.NewNop()
	h := dataresourceListHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/dataresources?limit=2&offset=1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	pagination := resp["pagination"].(map[string]interface{})
	if pagination["total"].(float64) != 5 {
		t.Errorf("total = %v, want 5", pagination["total"])
	}
	items := resp["dataresources"].([]interface{})
	if len(items) != 2 {
		t.Errorf("page count = %d, want 2 (limit=2 offset=1)", len(items))
	}
}

func TestDataresourceGet_K8s(t *testing.T) {
	existing := makeDataResource("pg1", "tenant1", "postgres")
	cfg := config.Config{K8sClient: makeFakeK8sClient(existing)}
	logger := zap.NewNop()
	h := dataresourceGetHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/dataresources/pg1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "pg1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("get status = %d: %s", w.Code, w.Body.String())
	}

	var dr dataresourceResponse
	json.NewDecoder(w.Body).Decode(&dr)

	if dr.Name != "pg1" {
		t.Errorf("name = %v, want pg1", dr.Name)
	}
	if dr.Type != "postgres" {
		t.Errorf("type = %v, want postgres", dr.Type)
	}
	if dr.Tenant != "tenant1" {
		t.Errorf("tenant = %v, want tenant1", dr.Tenant)
	}
}

func TestDataresourceGet_NotFound(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := dataresourceGetHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/dataresources/missing", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "missing")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestDataresourceDelete_K8s(t *testing.T) {
	existing := makeDataResource("pg1", "tenant1", "postgres")
	cfg := config.Config{K8sClient: makeFakeK8sClient(existing)}
	logger := zap.NewNop()
	h := dataresourceDeleteHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/dataresources/pg1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "pg1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("delete status = %d: %s", w.Code, w.Body.String())
	}

	// Verify the resource is gone — re-issuing Get should 404
	getHandler := dataresourceGetHandler(cfg, logger)
	req2 := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/dataresources/pg1", nil)
	req2 = req2.WithContext(ctx)
	req2.SetPathValue("tid", "tenant1")
	req2.SetPathValue("name", "pg1")

	w2 := httptest.NewRecorder()
	getHandler(w2, req2)

	if w2.Code != http.StatusNotFound {
		t.Errorf("post-delete get status = %d, want 404", w2.Code)
	}
}

func TestDataresourceDelete_NotFound(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := dataresourceDeleteHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/dataresources/missing", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "missing")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestDataresource_NilK8sClient(t *testing.T) {
	cfg := config.Config{} // nil K8sClient
	logger := zap.NewNop()

	tests := []struct {
		name    string
		handler http.HandlerFunc
		method  string
		url     string
		setName bool
		body    []byte
	}{
		{"list", dataresourceListHandler(cfg, logger), "GET", "/api/v1/tenants/tenant1/dataresources", false, nil},
		{"get", dataresourceGetHandler(cfg, logger), "GET", "/api/v1/tenants/tenant1/dataresources/dr1", true, nil},
		{"create", dataresourceCreateHandler(cfg, logger), "POST", "/api/v1/tenants/tenant1/dataresources", false, mustJSON(map[string]string{"name": "dr1", "type": "postgres"})},
		{"delete", dataresourceDeleteHandler(cfg, logger), "DELETE", "/api/v1/tenants/tenant1/dataresources/dr1", true, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var body *bytes.Reader
			if tt.body != nil {
				body = bytes.NewReader(tt.body)
			} else {
				body = bytes.NewReader(nil)
			}
			req := httptest.NewRequest(tt.method, tt.url, body)
			req = req.WithContext(makeContextWithClaims("user1", "tenant1"))
			req.SetPathValue("tid", "tenant1")
			if tt.setName {
				req.SetPathValue("name", "dr1")
			}
			w := httptest.NewRecorder()
			tt.handler(w, req)
			if w.Code != http.StatusServiceUnavailable {
				t.Errorf("%s: status = %d, want 503", tt.name, w.Code)
			}
		})
	}
}

// ----------------------------------------------------------------------------
// Storage CRUD with type filtering — K8s fake-client tests
// ----------------------------------------------------------------------------

func TestStorageList_FiltersStorageOnly(t *testing.T) {
	// Mix of storage and database types
	s1 := makeDataResource("vol1", "tenant1", nestv1.TypeObject)
	s2 := makeDataResource("vol2", "tenant1", nestv1.TypePVCBlock)
	db1 := makeDataResource("pg1", "tenant1", nestv1.TypePostgres)

	cfg := config.Config{K8sClient: makeFakeK8sClient(s1, s2, db1)}
	logger := zap.NewNop()
	h := storageListHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/resources", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	items := resp["resources"].([]interface{})
	if len(items) != 2 {
		t.Errorf("got %d storage resources, want 2 (only storage types)", len(items))
	}
}

func TestStorageCreate_InvalidType(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := storageCreateHandler(cfg, logger)

	body := map[string]string{"name": "pg1", "type": "postgres"} // postgres is NOT a storage type
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/resources", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (invalid storage type)", w.Code)
	}
}

func TestStorageCreate_ValidType(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := storageCreateHandler(cfg, logger)

	body := map[string]string{"name": "vol1", "type": "object", "storageClass": "standard", "size": "100Gi"}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/resources", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202: %s", w.Code, w.Body.String())
	}
}

func TestStorageGet_WrongType(t *testing.T) {
	// A postgres resource exists but is not a storage type — should 404 under /resources
	pg := makeDataResource("pg1", "tenant1", nestv1.TypePostgres)
	cfg := config.Config{K8sClient: makeFakeK8sClient(pg)}
	logger := zap.NewNop()
	h := storageGetHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/resources/pg1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "pg1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (postgres is not a storage type)", w.Code)
	}
}

// ----------------------------------------------------------------------------
// Database CRUD with type filtering — K8s fake-client tests
// ----------------------------------------------------------------------------

func TestDbList_FiltersDatabaseOnly(t *testing.T) {
	pg1 := makeDataResource("pg1", "tenant1", nestv1.TypePostgres)
	kv1 := makeDataResource("kv1", "tenant1", nestv1.TypeKeyvalue)
	vol1 := makeDataResource("vol1", "tenant1", nestv1.TypeObject) // not a DB type

	cfg := config.Config{K8sClient: makeFakeK8sClient(pg1, kv1, vol1)}
	logger := zap.NewNop()
	h := dbListHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/databases", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	items := resp["databases"].([]interface{})
	if len(items) != 2 {
		t.Errorf("got %d databases, want 2 (postgres + keyvalue)", len(items))
	}
}

func TestDbCreate_InvalidType(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := dbCreateHandler(cfg, logger)

	body := map[string]string{"name": "vol1", "type": "object"} // object is NOT a database type
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/databases", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want 400 (object is not a db type)", w.Code)
	}
}

func TestDbGet_K8s(t *testing.T) {
	existing := makeDataResource("pg1", "tenant1", nestv1.TypePostgres)
	cfg := config.Config{K8sClient: makeFakeK8sClient(existing)}
	logger := zap.NewNop()
	h := dbGetHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/databases/pg1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "pg1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d: %s", w.Code, w.Body.String())
	}

	var dr dataresourceResponse
	json.NewDecoder(w.Body).Decode(&dr)
	if dr.Type != "postgres" {
		t.Errorf("type = %v, want postgres", dr.Type)
	}
}

func TestDbDelete_K8s(t *testing.T) {
	existing := makeDataResource("pg1", "tenant1", nestv1.TypePostgres)
	cfg := config.Config{K8sClient: makeFakeK8sClient(existing)}
	logger := zap.NewNop()
	h := dbDeleteHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/databases/pg1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "pg1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want 202: %s", w.Code, w.Body.String())
	}
}

// ----------------------------------------------------------------------------
// Query handler with K8s client (endpoint lookup)
// ----------------------------------------------------------------------------

func TestQueryHandler_WithEndpoint(t *testing.T) {
	existing := makeDataResource("pg1", "tenant1", nestv1.TypePostgres)
	existing.Status.Endpoints = &nestv1.ResourceEndpoints{
		Native: "postgres://pg1.tenant1.svc.cluster.local:5432/db",
	}
	cfg := config.Config{K8sClient: makeFakeK8sClient(existing)}
	logger := zap.NewNop()
	h := queryHandler(cfg, logger)

	body := map[string]string{"resource": "pg1"}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/query", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want 200", w.Code)
	}
}

// mustJSON marshals v to JSON, panics on error.
func mustJSON(v interface{}) []byte {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}
