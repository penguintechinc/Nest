package handlers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/penguintechinc/nest/apps/api/middleware"
	"github.com/penguintechinc/nest/apps/api/store"
)

// TestMain sets gin to test mode globally
func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)
	m.Run()
}

// setupTestRouter creates a test router with TenantMiddleware stubbed
// The stub sets the tenantId from the "Authorization" header (format: "Bearer sub:tenantId")
func setupTestRouter() *gin.Engine {
	router := gin.New()

	// Stub middleware: extract tenant from Authorization header
	router.Use(func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth != "" && len(auth) > 7 {
			// Simple stub: parse "Bearer sub:tenantId" or "Bearer sub:tenantId:tier"
			token := auth[7:] // Remove "Bearer "
			parts := []rune(token)
			var tenantID string
			var tier string = "free"

			// Parse token format: sub:tenantId or sub:tenantId:tier
			colonCount := 0
			colonPos := []int{}
			for i, ch := range parts {
				if ch == ':' {
					colonCount++
					colonPos = append(colonPos, i)
				}
			}

			if colonCount >= 1 {
				tenantID = string(parts[colonPos[0]+1 : len(parts)])
				if colonCount >= 2 {
					// Extract tier (last colon segment)
					secondColon := colonPos[1]
					tenantID = string(parts[colonPos[0]+1 : secondColon])
					tier = string(parts[secondColon+1 : len(parts)])
				}
			}

			c.Set(middleware.TenantKey, tenantID)
			c.Set(middleware.ClaimsKey, &middleware.Claims{
				Sub:    "test-user",
				Tenant: tenantID,
				Scopes: []string{"nest:*:admin"},
				Tier:   tier,
			})
		}
		c.Next()
	})

	return router
}

// TestListDataResources_Happy tests successful listing of data resources
func TestListDataResources_Happy(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()

	// Seed some data
	dr1 := &store.DataResourceRecord{
		ID:        "dr-1",
		Name:      "resource-1",
		Tenant:    "tenant-1",
		Type:      "Postgres",
		Class:     "Standard",
		Phase:     "Active",
		CreatedAt: time.Now().UTC(),
	}
	dr2 := &store.DataResourceRecord{
		ID:        "dr-2",
		Name:      "resource-2",
		Tenant:    "tenant-1",
		Type:      "MySQL",
		Class:     "Standard",
		Phase:     "Active",
		CreatedAt: time.Now().UTC(),
	}
	s.CreateDataResource(context.Background(), dr1)
	s.CreateDataResource(context.Background(), dr2)

	router.GET("/api/v1/tenants/:tenantId/data-resources", ListDataResources(s))

	req, _ := http.NewRequest("GET", "/api/v1/tenants/tenant-1/data-resources", nil)
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if items, ok := resp["items"].([]interface{}); !ok || len(items) != 2 {
		t.Errorf("Expected 2 items, got %v", resp["items"])
	}

	meta := resp["meta"].(map[string]interface{})
	if int(meta["count"].(float64)) != 2 {
		t.Errorf("Expected count 2, got %v", meta["count"])
	}
}

// TestListDataResources_EmptyList tests listing when no resources exist
func TestListDataResources_EmptyList(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.GET("/api/v1/tenants/:tenantId/data-resources", ListDataResources(s))

	req, _ := http.NewRequest("GET", "/api/v1/tenants/tenant-1/data-resources", nil)
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	items := resp["items"]
	if items == nil {
		// nil is acceptable for empty list in JSON marshal
		return
	}
	if itemsList, ok := items.([]interface{}); ok && len(itemsList) != 0 {
		t.Errorf("Expected 0 items, got %d", len(itemsList))
	}
}

// TestListDataResources_TenantMismatch tests forbidden access when token tenant != path tenant
func TestListDataResources_TenantMismatch(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.GET("/api/v1/tenants/:tenantId/data-resources", ListDataResources(s))

	req, _ := http.NewRequest("GET", "/api/v1/tenants/tenant-1/data-resources", nil)
	req.Header.Set("Authorization", "Bearer user:tenant-2") // Different tenant
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "nest.auth.tenant_mismatch" {
		t.Errorf("Expected tenant_mismatch error code, got %v", resp["code"])
	}
}

// TestCreateDataResource_Happy tests successful creation
func TestCreateDataResource_Happy(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources", CreateDataResource(s))

	body := CreateDataResourceRequest{
		Name:      "my-resource",
		Type:      "postgres",
		Class:     "postgres-oltp",
		HA:        true,
		Protocols: []string{"tcp", "udp"},
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("Expected 202 Accepted, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["name"] != "my-resource" {
		t.Errorf("Expected name 'my-resource', got %v", resp["name"])
	}
	if resp["type"] != "postgres" {
		t.Errorf("Expected type 'postgres', got %v", resp["type"])
	}
	if resp["phase"] != "Pending" {
		t.Errorf("Expected phase 'Pending', got %v", resp["phase"])
	}

	// Verify Location header
	location := w.Header().Get("Location")
	if location == "" {
		t.Error("Expected Location header")
	}
	if !bytes.Contains([]byte(location), []byte("data-resources")) {
		t.Errorf("Expected Location to contain 'data-resources', got %s", location)
	}

	// Verify X-Operation-ID header
	opID := w.Header().Get("X-Operation-ID")
	if opID == "" {
		t.Error("Expected X-Operation-ID header")
	}
}

// TestCreateDataResource_MissingRequired tests validation of required fields
func TestCreateDataResource_MissingRequired(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources", CreateDataResource(s))

	// Missing Type and Class
	body := CreateDataResourceRequest{
		Name: "my-resource",
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "nest.dataresource.invalid" {
		t.Errorf("Expected invalid code, got %v", resp["code"])
	}
}

// TestCreateDataResource_InvalidJSON tests handling of malformed JSON
func TestCreateDataResource_InvalidJSON(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources", CreateDataResource(s))

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources",
		bytes.NewBuffer([]byte("invalid json")))
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

// TestCreateDataResource_TenantMismatch tests forbidden access
func TestCreateDataResource_TenantMismatch(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources", CreateDataResource(s))

	body := CreateDataResourceRequest{
		Name:  "my-resource",
		Type:  "postgres",
		Class: "postgres-oltp",
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer user:tenant-2") // Different tenant
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403, got %d", w.Code)
	}
}

// TestCreateDataResource_FreeTierLimit tests hitting the 5-resource free tier limit
func TestCreateDataResource_FreeTierLimit(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources", CreateDataResource(s))

	// Seed 5 resources for the tenant
	for i := 1; i <= 5; i++ {
		dr := &store.DataResourceRecord{
			ID:        fmt.Sprintf("dr-%d", i),
			Name:      fmt.Sprintf("resource-%d", i),
			Tenant:    "tenant-1",
			Type:      "Postgres",
			Class:     "Standard",
			Phase:     "Active",
			CreatedAt: time.Now().UTC(),
		}
		s.CreateDataResource(context.Background(), dr)
	}

	body := CreateDataResourceRequest{
		Name:  "resource-6",
		Type:  "postgres",
		Class: "postgres-oltp",
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer user:tenant-1:free") // Free tier
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusPaymentRequired {
		t.Errorf("Expected 402 Payment Required, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "nest.license.limit_exceeded" {
		t.Errorf("Expected limit_exceeded code, got %v", resp["code"])
	}
}

// TestCreateDataResource_FreeTierNoLimit tests that pro tier can exceed 5 resources
func TestCreateDataResource_ProTierNoLimit(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources", CreateDataResource(s))

	// Seed 5 resources for the tenant
	for i := 1; i <= 5; i++ {
		dr := &store.DataResourceRecord{
			ID:        fmt.Sprintf("dr-%d", i),
			Name:      fmt.Sprintf("resource-%d", i),
			Tenant:    "tenant-1",
			Type:      "Postgres",
			Class:     "Standard",
			Phase:     "Active",
			CreatedAt: time.Now().UTC(),
		}
		s.CreateDataResource(context.Background(), dr)
	}

	body := CreateDataResourceRequest{
		Name:  "resource-6",
		Type:  "postgres",
		Class: "postgres-oltp",
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer user:tenant-1:pro") // Pro tier (not free)
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("Expected 202, got %d", w.Code)
	}
}

// TestGetDataResource_Happy tests successful retrieval
func TestGetDataResource_Happy(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()

	// Seed a resource
	dr := &store.DataResourceRecord{
		ID:        "dr-1",
		Name:      "resource-1",
		Tenant:    "tenant-1",
		Type:      "Postgres",
		Class:     "Standard",
		Phase:     "Active",
		HA:        true,
		CreatedAt: time.Now().UTC(),
	}
	s.CreateDataResource(context.Background(), dr)

	router.GET("/api/v1/tenants/:tenantId/data-resources/:name", GetDataResource(s))

	req, _ := http.NewRequest("GET", "/api/v1/tenants/tenant-1/data-resources/resource-1", nil)
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["name"] != "resource-1" {
		t.Errorf("Expected name 'resource-1', got %v", resp["name"])
	}
	if resp["type"] != "Postgres" {
		t.Errorf("Expected type 'Postgres', got %v", resp["type"])
	}
	if resp["tenant"] != "tenant-1" {
		t.Errorf("Expected tenant 'tenant-1', got %v", resp["tenant"])
	}
}

// TestGetDataResource_NotFound tests 404 when resource doesn't exist
func TestGetDataResource_NotFound(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.GET("/api/v1/tenants/:tenantId/data-resources/:name", GetDataResource(s))

	req, _ := http.NewRequest("GET", "/api/v1/tenants/tenant-1/data-resources/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "nest.dataresource.not_found" {
		t.Errorf("Expected not_found code, got %v", resp["code"])
	}
}

// TestGetDataResource_TenantMismatch tests forbidden access
func TestGetDataResource_TenantMismatch(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()

	// Seed a resource for tenant-1
	dr := &store.DataResourceRecord{
		ID:        "dr-1",
		Name:      "resource-1",
		Tenant:    "tenant-1",
		Type:      "Postgres",
		Class:     "Standard",
		Phase:     "Active",
		CreatedAt: time.Now().UTC(),
	}
	s.CreateDataResource(context.Background(), dr)

	router.GET("/api/v1/tenants/:tenantId/data-resources/:name", GetDataResource(s))

	req, _ := http.NewRequest("GET", "/api/v1/tenants/tenant-1/data-resources/resource-1", nil)
	req.Header.Set("Authorization", "Bearer user:tenant-2") // Different tenant
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403, got %d", w.Code)
	}
}

// TestDeleteDataResource_Happy tests successful deletion
func TestDeleteDataResource_Happy(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()

	// Seed a resource
	dr := &store.DataResourceRecord{
		ID:        "dr-1",
		Name:      "resource-1",
		Tenant:    "tenant-1",
		Type:      "Postgres",
		Class:     "Standard",
		Phase:     "Active",
		CreatedAt: time.Now().UTC(),
	}
	s.CreateDataResource(context.Background(), dr)

	router.DELETE("/api/v1/tenants/:tenantId/data-resources/:name", DeleteDataResource(s))

	req, _ := http.NewRequest("DELETE", "/api/v1/tenants/tenant-1/data-resources/resource-1", nil)
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("Expected 204 No Content, got %d", w.Code)
	}

	// Verify resource is actually deleted
	_, err := s.GetDataResource(context.Background(), "tenant-1", "resource-1")
	if err == nil {
		t.Error("Expected resource to be deleted, but it still exists")
	}
}

// TestDeleteDataResource_NotFound tests 404 when resource doesn't exist
func TestDeleteDataResource_NotFound(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.DELETE("/api/v1/tenants/:tenantId/data-resources/:name", DeleteDataResource(s))

	req, _ := http.NewRequest("DELETE", "/api/v1/tenants/tenant-1/data-resources/nonexistent", nil)
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("Expected 404, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "nest.dataresource.not_found" {
		t.Errorf("Expected not_found code, got %v", resp["code"])
	}
}

// TestDeleteDataResource_TenantMismatch tests forbidden access
func TestDeleteDataResource_TenantMismatch(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()

	// Seed a resource for tenant-1
	dr := &store.DataResourceRecord{
		ID:        "dr-1",
		Name:      "resource-1",
		Tenant:    "tenant-1",
		Type:      "Postgres",
		Class:     "Standard",
		Phase:     "Active",
		CreatedAt: time.Now().UTC(),
	}
	s.CreateDataResource(context.Background(), dr)

	router.DELETE("/api/v1/tenants/:tenantId/data-resources/:name", DeleteDataResource(s))

	req, _ := http.NewRequest("DELETE", "/api/v1/tenants/tenant-1/data-resources/resource-1", nil)
	req.Header.Set("Authorization", "Bearer user:tenant-2") // Different tenant
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403, got %d", w.Code)
	}
}

// TestCreateDataResource_WithOptionalFields tests creation with optional fields set
func TestCreateDataResource_WithOptionalFields(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources", CreateDataResource(s))

	body := CreateDataResourceRequest{
		Name:        "my-resource",
		Type:        "pvc/file",
		Class:       "nest-cephfs",
		HA:          true,
		Protocols:   []string{"nfs"},
		Origination: "imported",
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("Expected 202, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	// Verify optional fields were preserved
	if resp["type"] != "pvc/file" {
		t.Errorf("Expected type 'pvc/file', got %v", resp["type"])
	}
}

// TestListDataResources_MultiTenant tests that list only returns tenant's own resources
func TestListDataResources_MultiTenant(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()

	// Seed resources for two different tenants
	dr1 := &store.DataResourceRecord{
		ID:        "dr-1",
		Name:      "resource-1",
		Tenant:    "tenant-1",
		Type:      "Postgres",
		Class:     "Standard",
		Phase:     "Active",
		CreatedAt: time.Now().UTC(),
	}
	dr2 := &store.DataResourceRecord{
		ID:        "dr-2",
		Name:      "resource-2",
		Tenant:    "tenant-2",
		Type:      "MySQL",
		Class:     "Standard",
		Phase:     "Active",
		CreatedAt: time.Now().UTC(),
	}
	s.CreateDataResource(context.Background(), dr1)
	s.CreateDataResource(context.Background(), dr2)

	router.GET("/api/v1/tenants/:tenantId/data-resources", ListDataResources(s))

	// Request as tenant-1
	req, _ := http.NewRequest("GET", "/api/v1/tenants/tenant-1/data-resources", nil)
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected 200, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	// Should only have 1 resource (for tenant-1)
	items := resp["items"]
	if items == nil {
		t.Error("Expected items field in response")
		return
	}
	if itemsList, ok := items.([]interface{}); ok && len(itemsList) != 1 {
		t.Errorf("Expected 1 item for tenant-1, got %d", len(itemsList))
	}
}

// TestGetDataResource_IsolationAcrossTenants tests that you can't get another tenant's resource via header mismatch
func TestGetDataResource_IsolationAcrossTenants(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()

	// Seed a resource for tenant-2
	dr := &store.DataResourceRecord{
		ID:        "dr-1",
		Name:      "resource-1",
		Tenant:    "tenant-2",
		Type:      "Postgres",
		Class:     "Standard",
		Phase:     "Active",
		CreatedAt: time.Now().UTC(),
	}
	s.CreateDataResource(context.Background(), dr)

	router.GET("/api/v1/tenants/:tenantId/data-resources/:name", GetDataResource(s))

	// Attempt to get as tenant-1 but with token for tenant-2 requesting through tenant-1 path
	req, _ := http.NewRequest("GET", "/api/v1/tenants/tenant-1/data-resources/resource-1", nil)
	req.Header.Set("Authorization", "Bearer user:tenant-2") // Token says tenant-2, but path says tenant-1
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should get 403 due to tenant mismatch (header tenant != path tenant)
	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden for tenant mismatch, got %d", w.Code)
	}
}

// TestCreateDataResource_Origination tests that default origination is "managed"
func TestCreateDataResource_OriginationDefault(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources", CreateDataResource(s))

	body := CreateDataResourceRequest{
		Name:  "my-resource",
		Type:  "postgres",
		Class: "postgres-oltp",
		// Origination not specified
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("Expected 202, got %d", w.Code)
	}

	// Verify via store that default origination was set
	dr, _ := s.GetDataResource(context.Background(), "tenant-1", "my-resource")
	if dr.Origination != "managed" {
		t.Errorf("Expected default origination 'managed', got %s", dr.Origination)
	}
}

// TestListDataResources_StoreError tests handling of store errors
func TestListDataResources_StoreError(t *testing.T) {
	s := &MockStore{
		listErr: fmt.Errorf("database connection failed"),
	}
	router := setupTestRouter()
	router.GET("/api/v1/tenants/:tenantId/data-resources", ListDataResources(s))

	req, _ := http.NewRequest("GET", "/api/v1/tenants/tenant-1/data-resources", nil)
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "nest.internal" {
		t.Errorf("Expected internal error code, got %v", resp["code"])
	}
}

// TestCreateDataResource_StoreError tests handling of store creation errors
func TestCreateDataResource_StoreError(t *testing.T) {
	s := &MockStore{
		createErr: fmt.Errorf("database write failed"),
	}
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources", CreateDataResource(s))

	body := CreateDataResourceRequest{
		Name:  "my-resource",
		Type:  "postgres",
		Class: "postgres-oltp",
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("Expected 500, got %d", w.Code)
	}
}

// TestCreateDataResource_NilClaims tests free tier check when claims is nil (should not limit)
func TestCreateDataResource_NilClaims(t *testing.T) {
	s := store.NewMemoryStore()
	router := gin.New()

	// Middleware that sets tenant but NOT claims (simulating missing claims)
	router.Use(func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if auth != "" && len(auth) > 7 {
			token := auth[7:]
			parts := []rune(token)
			var tenantID string

			colonCount := 0
			colonPos := []int{}
			for i, ch := range parts {
				if ch == ':' {
					colonCount++
					colonPos = append(colonPos, i)
				}
			}

			if colonCount >= 1 {
				tenantID = string(parts[colonPos[0]+1 : len(parts)])
				if colonCount >= 2 {
					secondColon := colonPos[1]
					tenantID = string(parts[colonPos[0]+1 : secondColon])
				}
			}

			c.Set(middleware.TenantKey, tenantID)
			// Intentionally NOT setting ClaimsKey
		}
		c.Next()
	})

	router.POST("/api/v1/tenants/:tenantId/data-resources", CreateDataResource(s))

	body := CreateDataResourceRequest{
		Name:  "my-resource",
		Type:  "postgres",
		Class: "postgres-oltp",
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should succeed (no free tier check when claims is nil)
	if w.Code != http.StatusAccepted {
		t.Errorf("Expected 202 when claims is nil, got %d", w.Code)
	}
}

// TestCreateDataResource_MissingName tests validation when name is not provided (ShouldBindJSON validation)
func TestCreateDataResource_MissingName(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources", CreateDataResource(s))

	body := map[string]interface{}{
		// name is required by binding:"required"
		"type":  "postgres",
		"class": "postgres-oltp",
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400, got %d", w.Code)
	}
}

// TestCreateDataResource_CountError tests handling of count errors when checking free tier limit
func TestCreateDataResource_CountError(t *testing.T) {
	s := &MockStore{
		countErr: fmt.Errorf("count failed"),
	}
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources", CreateDataResource(s))

	body := CreateDataResourceRequest{
		Name:  "my-resource",
		Type:  "postgres",
		Class: "postgres-oltp",
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer user:tenant-1:free")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	// Should succeed (error is silently ignored in count check)
	if w.Code != http.StatusAccepted {
		t.Errorf("Expected 202 despite count error, got %d", w.Code)
	}
}

// MockStore is a mock implementation of Store for testing error cases
type MockStore struct {
	listErr   error
	createErr error
	getErr    error
	deleteErr error
	countErr  error
}

func (m *MockStore) ListDataResources(ctx context.Context, tenant string) ([]*store.DataResourceRecord, error) {
	if m.listErr != nil {
		return nil, m.listErr
	}
	return []*store.DataResourceRecord{}, nil
}

func (m *MockStore) CreateDataResource(ctx context.Context, dr *store.DataResourceRecord) error {
	if m.createErr != nil {
		return m.createErr
	}
	return nil
}

func (m *MockStore) GetDataResource(ctx context.Context, tenant, name string) (*store.DataResourceRecord, error) {
	if m.getErr != nil {
		return nil, m.getErr
	}
	return &store.DataResourceRecord{}, nil
}

func (m *MockStore) DeleteDataResource(ctx context.Context, tenant, name string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	return nil
}

func (m *MockStore) CountDataResources(ctx context.Context, tenant string) (int, error) {
	if m.countErr != nil {
		return 0, m.countErr
	}
	return 0, nil
}

// TestSnapshotDataResource_Happy tests that snapshot returns 202 with operationId
func TestSnapshotDataResource_Happy(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources/:name/snapshot", SnapshotDataResource(s))

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources/my-db/snapshot", nil)
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("Expected 202 Accepted, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["operationId"] == "" || resp["operationId"] == nil {
		t.Error("Expected non-empty operationId in response")
	}
	if resp["type"] != "snapshot" {
		t.Errorf("Expected type 'snapshot', got %v", resp["type"])
	}
	if resp["resource"] != "my-db" {
		t.Errorf("Expected resource 'my-db', got %v", resp["resource"])
	}
	if resp["tenant"] != "tenant-1" {
		t.Errorf("Expected tenant 'tenant-1', got %v", resp["tenant"])
	}
	if resp["status"] != "RUNNING" {
		t.Errorf("Expected status 'RUNNING', got %v", resp["status"])
	}
	if resp["startedAt"] == nil || resp["startedAt"] == "" {
		t.Error("Expected non-empty startedAt in response")
	}

	// Verify Location header points to an operation
	location := w.Header().Get("Location")
	if location == "" {
		t.Error("Expected Location header")
	}
	if !bytes.Contains([]byte(location), []byte("operations")) {
		t.Errorf("Expected Location to contain 'operations', got %s", location)
	}
}

// TestSnapshotDataResource_TenantMismatch tests that snapshot returns 403 on tenant mismatch
func TestSnapshotDataResource_TenantMismatch(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources/:name/snapshot", SnapshotDataResource(s))

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources/my-db/snapshot", nil)
	req.Header.Set("Authorization", "Bearer user:tenant-2") // Different tenant
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "nest.auth.tenant_mismatch" {
		t.Errorf("Expected tenant_mismatch code, got %v", resp["code"])
	}
}

// TestRestoreDataResource_Happy tests that restore returns 202 with operationId and mode
func TestRestoreDataResource_Happy(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources/:name/restore", RestoreDataResource(s))

	body := map[string]string{
		"snapshotId": "snap-abc123",
		"mode":       "in-place",
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources/my-db/restore",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("Expected 202 Accepted, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["operationId"] == "" || resp["operationId"] == nil {
		t.Error("Expected non-empty operationId in response")
	}
	if resp["type"] != "restore" {
		t.Errorf("Expected type 'restore', got %v", resp["type"])
	}
	if resp["resource"] != "my-db" {
		t.Errorf("Expected resource 'my-db', got %v", resp["resource"])
	}
	if resp["tenant"] != "tenant-1" {
		t.Errorf("Expected tenant 'tenant-1', got %v", resp["tenant"])
	}
	if resp["mode"] != "in-place" {
		t.Errorf("Expected mode 'in-place', got %v", resp["mode"])
	}
	if resp["status"] != "RUNNING" {
		t.Errorf("Expected status 'RUNNING', got %v", resp["status"])
	}
	if resp["startedAt"] == nil || resp["startedAt"] == "" {
		t.Error("Expected non-empty startedAt in response")
	}

	// Verify Location header
	location := w.Header().Get("Location")
	if location == "" {
		t.Error("Expected Location header")
	}
	if !bytes.Contains([]byte(location), []byte("operations")) {
		t.Errorf("Expected Location to contain 'operations', got %s", location)
	}
}

// TestRestoreDataResource_DefaultMode tests that restore defaults mode to "side-by-side" when not provided
func TestRestoreDataResource_DefaultMode(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources/:name/restore", RestoreDataResource(s))

	// Send request without mode
	body := map[string]string{
		"snapshotId": "snap-abc123",
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources/my-db/restore",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("Expected 202 Accepted, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	if resp["mode"] != "side-by-side" {
		t.Errorf("Expected default mode 'side-by-side', got %v", resp["mode"])
	}
}

// TestRestoreDataResource_TenantMismatch tests that restore returns 403 on tenant mismatch
func TestRestoreDataResource_TenantMismatch(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources/:name/restore", RestoreDataResource(s))

	body := map[string]string{"mode": "in-place"}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources/my-db/restore",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer user:tenant-2") // Different tenant
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected 403 Forbidden, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "nest.auth.tenant_mismatch" {
		t.Errorf("Expected tenant_mismatch code, got %v", resp["code"])
	}
}

// TestCreateDataResource_InvalidType tests that unknown resource types are rejected
func TestCreateDataResource_InvalidType(t *testing.T) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.POST("/api/v1/tenants/:tenantId/data-resources", CreateDataResource(s))

	body := CreateDataResourceRequest{
		Name:  "my-resource",
		Type:  "UnknownType",
		Class: "Standard",
	}
	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources",
		bytes.NewBuffer(bodyBytes))
	req.Header.Set("Authorization", "Bearer user:tenant-1")
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("Expected 400 Bad Request for unknown type, got %d", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["code"] != "nest.dataresource.invalid_type" {
		t.Errorf("Expected invalid_type code, got %v", resp["code"])
	}
}

// TestCreateDataResource_ValidP2Types tests that all P2 types are accepted
func TestCreateDataResource_ValidP2Types(t *testing.T) {
	validTypes := []struct {
		typ   string
		class string
	}{
		{"pvc/block", "nest-rbd"},
		{"pvc/file", "nest-cephfs"},
		{"object", "nest-object"},
		{"nfs", "nest-nfs"},
		{"iscsi", "nest-iscsi"},
		{"postgres", "postgres-oltp"},
		{"keyvalue", "keyvalue-shared"},
	}

	for _, tt := range validTypes {
		t.Run(tt.typ, func(t *testing.T) {
			s := store.NewMemoryStore()
			router := setupTestRouter()
			router.POST("/api/v1/tenants/:tenantId/data-resources", CreateDataResource(s))

			body := CreateDataResourceRequest{
				Name:  "my-resource",
				Type:  tt.typ,
				Class: tt.class,
			}
			bodyBytes, _ := json.Marshal(body)

			req, _ := http.NewRequest("POST", "/api/v1/tenants/tenant-1/data-resources",
				bytes.NewBuffer(bodyBytes))
			req.Header.Set("Authorization", "Bearer user:tenant-1")
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()

			router.ServeHTTP(w, req)

			if w.Code != http.StatusAccepted {
				t.Errorf("Expected 202 for type %q, got %d", tt.typ, w.Code)
			}
		})
	}
}

// BenchmarkListDataResources benchmarks the list operation
func BenchmarkListDataResources(b *testing.B) {
	s := store.NewMemoryStore()
	router := setupTestRouter()
	router.GET("/api/v1/tenants/:tenantId/data-resources", ListDataResources(s))

	// Seed 100 resources
	for i := 0; i < 100; i++ {
		dr := &store.DataResourceRecord{
			ID:        fmt.Sprintf("dr-%d", i),
			Name:      fmt.Sprintf("resource-%d", i),
			Tenant:    "tenant-1",
			Type:      "Postgres",
			Class:     "Standard",
			Phase:     "Active",
			CreatedAt: time.Now().UTC(),
		}
		s.CreateDataResource(context.Background(), dr)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("GET", "/api/v1/tenants/tenant-1/data-resources", nil)
		req.Header.Set("Authorization", "Bearer user:tenant-1")
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}
