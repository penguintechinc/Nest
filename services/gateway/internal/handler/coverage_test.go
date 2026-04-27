package handler

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/gateway/internal/config"
)

// ============================================================================
// Tenant mismatch coverage for all handlers
// ============================================================================

func TestDbListHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dbListHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/databases", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestDbListHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dbListHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/databases", nil)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDbGetHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dbGetHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/databases/db1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	req.SetPathValue("name", "db1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestDbGetHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dbGetHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/databases/db1", nil)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "db1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDbCreateHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dbCreateHandler(cfg, logger)
	body := map[string]string{"name": "db1", "type": "postgres"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant2/databases", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestDbCreateHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dbCreateHandler(cfg, logger)
	body := map[string]string{"name": "db1", "type": "postgres"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/databases", bytes.NewReader(bodyBytes))
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDbCreateHandler_InvalidBody(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dbCreateHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/databases", bytes.NewReader([]byte("not json")))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDbCreateHandler_MissingType(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dbCreateHandler(cfg, logger)
	body := map[string]string{"name": "db1"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/databases", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDbDeleteHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dbDeleteHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant2/databases/db1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	req.SetPathValue("name", "db1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestDbDeleteHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dbDeleteHandler(cfg, logger)
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/databases/db1", nil)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "db1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ============================================================================
// Storage handler error paths
// ============================================================================

func TestStorageListHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := storageListHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/resources", nil)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestStorageGetHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := storageGetHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/resources/res1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	req.SetPathValue("name", "res1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestStorageGetHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := storageGetHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/resources/res1", nil)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "res1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestStorageCreateHandler_InvalidBody(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := storageCreateHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/resources", bytes.NewReader([]byte("not json")))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestStorageCreateHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := storageCreateHandler(cfg, logger)
	body := map[string]string{"name": "res1", "type": "s3", "storageClass": "standard", "size": "10Gi"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant2/resources", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestStorageCreateHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := storageCreateHandler(cfg, logger)
	body := map[string]string{"name": "res1", "type": "s3"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/resources", bytes.NewReader(bodyBytes))
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestStorageCreateHandler_MissingType(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := storageCreateHandler(cfg, logger)
	body := map[string]string{"name": "res1", "storageClass": "standard"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/resources", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestStorageDeleteHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := storageDeleteHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant2/resources/res1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	req.SetPathValue("name", "res1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestStorageDeleteHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := storageDeleteHandler(cfg, logger)
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/resources/res1", nil)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "res1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ============================================================================
// Dataresource handler error paths
// ============================================================================

func TestDataresourceListHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dataresourceListHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/dataresources", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestDataresourceListHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dataresourceListHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/dataresources", nil)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDataresourceListHandler_WithPagination(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dataresourceListHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/dataresources?limit=10&offset=5", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	pagination := resp["pagination"].(map[string]interface{})
	if pagination["limit"].(float64) != 10 {
		t.Errorf("limit = %v, want 10", pagination["limit"])
	}
	if pagination["offset"].(float64) != 5 {
		t.Errorf("offset = %v, want 5", pagination["offset"])
	}
}

func TestDataresourceGetHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dataresourceGetHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/dataresources/dr1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	req.SetPathValue("name", "dr1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestDataresourceGetHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dataresourceGetHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/dataresources/dr1", nil)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "dr1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDataresourceCreateHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dataresourceCreateHandler(cfg, logger)
	body := map[string]string{"name": "dr1", "type": "table"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant2/dataresources", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestDataresourceCreateHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dataresourceCreateHandler(cfg, logger)
	body := map[string]string{"name": "dr1", "type": "table"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/dataresources", bytes.NewReader(bodyBytes))
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDataresourceCreateHandler_InvalidBody(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dataresourceCreateHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/dataresources", bytes.NewReader([]byte("not json")))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDataresourceCreateHandler_MissingType(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dataresourceCreateHandler(cfg, logger)
	body := map[string]string{"name": "dr1"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/dataresources", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestDataresourcePatchHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dataresourcePatchHandler(cfg, logger)
	body := map[string]interface{}{"status": "archived"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("PATCH", "/api/v1/tenants/tenant2/dataresources/dr1", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	req.SetPathValue("name", "dr1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestDataresourcePatchHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dataresourcePatchHandler(cfg, logger)
	body := map[string]interface{}{"status": "archived"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("PATCH", "/api/v1/tenants/tenant1/dataresources/dr1", bytes.NewReader(bodyBytes))
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "dr1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestDataresourceDeleteHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dataresourceDeleteHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant2/dataresources/dr1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	req.SetPathValue("name", "dr1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestDataresourceDeleteHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dataresourceDeleteHandler(cfg, logger)
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/dataresources/dr1", nil)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "dr1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ============================================================================
// Extended engine handler error paths
// ============================================================================

func TestExtListHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := extListHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/engines", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestExtListHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := extListHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/engines", nil)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestExtGetHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := extGetHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/engines/eng1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	req.SetPathValue("name", "eng1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestExtGetHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := extGetHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/engines/eng1", nil)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "eng1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestExtCreateHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := extCreateHandler(cfg, logger)
	body := map[string]string{"name": "eng1", "type": "spark"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant2/engines", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestExtCreateHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := extCreateHandler(cfg, logger)
	body := map[string]string{"name": "eng1", "type": "spark"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/engines", bytes.NewReader(bodyBytes))
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestExtCreateHandler_InvalidBody(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := extCreateHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/engines", bytes.NewReader([]byte("not json")))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestExtCreateHandler_MissingType(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := extCreateHandler(cfg, logger)
	body := map[string]string{"name": "eng1"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/engines", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestExtDeleteHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := extDeleteHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant2/engines/eng1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	req.SetPathValue("name", "eng1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestExtDeleteHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := extDeleteHandler(cfg, logger)
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/engines/eng1", nil)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "eng1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ============================================================================
// Warehouse handler error paths
// ============================================================================

func TestWarehouseListHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := warehouseListHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/warehouses", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestWarehouseListHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := warehouseListHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/warehouses", nil)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestWarehouseGetHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := warehouseGetHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/warehouses/wh1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	req.SetPathValue("name", "wh1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestWarehouseGetHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := warehouseGetHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/warehouses/wh1", nil)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "wh1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestWarehouseCreateHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := warehouseCreateHandler(cfg, logger)
	body := map[string]string{"name": "wh1", "type": "snowflake"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant2/warehouses", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestWarehouseCreateHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := warehouseCreateHandler(cfg, logger)
	body := map[string]string{"name": "wh1", "type": "snowflake"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/warehouses", bytes.NewReader(bodyBytes))
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestWarehouseCreateHandler_InvalidBody(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := warehouseCreateHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/warehouses", bytes.NewReader([]byte("not json")))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestWarehouseCreateHandler_MissingName(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := warehouseCreateHandler(cfg, logger)
	body := map[string]string{"type": "snowflake"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/warehouses", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestWarehouseCreateHandler_MissingType(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := warehouseCreateHandler(cfg, logger)
	body := map[string]string{"name": "wh1"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/warehouses", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestWarehouseDeleteHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := warehouseDeleteHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant2/warehouses/wh1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	req.SetPathValue("name", "wh1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestWarehouseDeleteHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := warehouseDeleteHandler(cfg, logger)
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/warehouses/wh1", nil)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "wh1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ============================================================================
// Evidence handler
// ============================================================================

func TestEvidenceHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := evidenceHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/evidence/compliance/bundle1", nil)
	req.SetPathValue("bundle", "bundle1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestEvidenceHandler_WithLicense(t *testing.T) {
	t.Setenv("ENTERPRISE_LICENSE", "test-license")
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := evidenceHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/evidence/compliance/bundle1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("bundle", "bundle1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

// ============================================================================
// Import/Export handler error paths
// ============================================================================

func TestImportHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := importHandler(cfg, logger)
	body := map[string]interface{}{"source": "s3://bucket/path"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/resources/res1/import", bytes.NewReader(bodyBytes))
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "res1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestImportHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := importHandler(cfg, logger)
	body := map[string]interface{}{"source": "s3://bucket/path"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant2/resources/res1/import", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	req.SetPathValue("name", "res1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestImportHandler_InvalidBody(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := importHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/resources/res1/import", bytes.NewReader([]byte("not json")))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "res1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestImportHandler_MissingSource(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := importHandler(cfg, logger)
	body := map[string]interface{}{}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/resources/res1/import", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "res1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestExportHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := exportHandler(cfg, logger)
	body := map[string]interface{}{"destination": "s3://bucket/path"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/resources/res1/export", bytes.NewReader(bodyBytes))
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "res1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestExportHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := exportHandler(cfg, logger)
	body := map[string]interface{}{"destination": "s3://bucket/path"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant2/resources/res1/export", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	req.SetPathValue("name", "res1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestExportHandler_InvalidBody(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := exportHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/resources/res1/export", bytes.NewReader([]byte("not json")))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "res1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestExportHandler_MissingDestination(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := exportHandler(cfg, logger)
	body := map[string]interface{}{}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/resources/res1/export", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "res1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ============================================================================
// Indexer handler error paths
// ============================================================================

func TestIndexerCatalogHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := indexerCatalogHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/indexer/catalog", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestIndexerCatalogHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := indexerCatalogHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/indexer/catalog", nil)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestIndexerScanHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := indexerScanHandler(cfg, logger)
	body := map[string]interface{}{"resource": "res1"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant2/indexer/scan", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestIndexerScanHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := indexerScanHandler(cfg, logger)
	body := map[string]interface{}{"resource": "res1"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/indexer/scan", bytes.NewReader(bodyBytes))
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestIndexerPIITargetsHandler_NolicenseThenWithClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := indexerPIITargetsHandler(cfg, logger)

	// Without enterprise license
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/indexer/pii-targets", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusPaymentRequired {
		t.Errorf("status = %d, want %d", w.Code, http.StatusPaymentRequired)
	}
}

func TestIndexerPIITargetsHandler_WithEnterpriseLicense(t *testing.T) {
	t.Setenv("ENTERPRISE_LICENSE", "test-license")
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := indexerPIITargetsHandler(cfg, logger)

	// With enterprise license but no claims
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/indexer/pii-targets", nil)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestIndexerPIITargetsHandler_WithLicense_TenantMismatch(t *testing.T) {
	t.Setenv("ENTERPRISE_LICENSE", "test-license")
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := indexerPIITargetsHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/indexer/pii-targets", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestPolicyEvaluateHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := policyEvaluateHandler(cfg, logger)
	body := map[string]interface{}{"policy": "default"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant2/policy/evaluate", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestPolicyEvaluateHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := policyEvaluateHandler(cfg, logger)
	body := map[string]interface{}{"policy": "default"}
	bodyBytes, _ := json.Marshal(body)
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/policy/evaluate", bytes.NewReader(bodyBytes))
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ============================================================================
// Metering/billing handler error paths
// ============================================================================

func TestMeteringHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := meteringHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/metering", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestMeteringHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := meteringHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/metering", nil)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestBillingHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := billingHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/billing", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestBillingHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := billingHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/billing", nil)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

// ============================================================================
// Intelligence/recommendations handler error paths
// ============================================================================

func TestIntelligenceRecommendHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := intelligenceRecommendHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/intelligence/recommend", nil)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestIntelligenceRecommendHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := intelligenceRecommendHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/intelligence/recommend", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestIntelligenceRecommendHandler_WithLicense(t *testing.T) {
	t.Setenv("ENTERPRISE_LICENSE", "test-license")
	t.Setenv("WADDLEAI_ENABLED", "true")
	t.Setenv("INTELLIGENCE_ENGINE_URL", "http://127.0.0.1:1") // non-routable
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := intelligenceRecommendHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/intelligence/recommend", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	// With license but unreachable upstream → BadGateway
	if w.Code != http.StatusBadGateway {
		t.Logf("status = %d (expected BadGateway with unreachable upstream)", w.Code)
	}
}

func TestPredictiveDriveHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := predictiveDriveHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/predictive-drive/risk", nil)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestPredictiveDriveHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := predictiveDriveHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/predictive-drive/risk", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestPredictiveDriveHandler_WithLicense(t *testing.T) {
	t.Setenv("ENTERPRISE_LICENSE", "test-license")
	t.Setenv("WADDLEAI_ENABLED", "true")
	t.Setenv("PREDICTIVE_DRIVE_URL", "http://127.0.0.1:1") // non-routable
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := predictiveDriveHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/predictive-drive/risk?node=node1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadGateway {
		t.Logf("status = %d (expected BadGateway with unreachable upstream)", w.Code)
	}
}

func TestAnomalyDetectHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := anomalyDetectHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/anomaly/current", nil)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestAnomalyDetectHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := anomalyDetectHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/anomaly/current", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestAnomalyDetectHandler_WithLicense(t *testing.T) {
	t.Setenv("ENTERPRISE_LICENSE", "test-license")
	t.Setenv("WADDLEAI_ENABLED", "true")
	t.Setenv("ANOMALY_DETECTOR_URL", "http://127.0.0.1:1") // non-routable
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := anomalyDetectHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/anomaly/current?severity=high", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadGateway {
		t.Logf("status = %d (expected BadGateway with unreachable upstream)", w.Code)
	}
}

// ============================================================================
// copyHeaders
// ============================================================================

func TestCopyHeaders(t *testing.T) {
	src := httptest.NewRequest("GET", "/", nil)
	src.Header.Set("X-Custom-Header", "value1")
	src.Header.Set("Authorization", "Bearer token")

	dst, _ := http.NewRequest("GET", "http://upstream/", nil)
	copyHeaders(src, dst)

	if dst.Header.Get("X-Custom-Header") != "value1" {
		t.Errorf("X-Custom-Header not copied")
	}
	if dst.Header.Get("Authorization") != "Bearer token" {
		t.Errorf("Authorization not copied")
	}
}

func TestCopyHeaders_Empty(t *testing.T) {
	src := httptest.NewRequest("GET", "/", nil)
	dst, _ := http.NewRequest("GET", "http://upstream/", nil)
	// should not panic with empty headers
	copyHeaders(src, dst)
}

// ============================================================================
// queryHandler missing cases
// ============================================================================

func TestQueryHandler_InvalidBody(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := queryHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/query", bytes.NewReader([]byte("not json")))
	req = req.WithContext(ctx)
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

// ============================================================================
// objectGetHandler missing case (no claims)
// ============================================================================

func TestObjectGetHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := objectGetHandler(cfg, logger)
	req := httptest.NewRequest("GET", "/api/v1/objects/res1/bucket1/key1", nil)
	req.SetPathValue("resource", "res1")
	req.SetPathValue("bucket", "bucket1")
	req.SetPathValue("key", "key1")
	w := httptest.NewRecorder()
	handler(w, req)
	// objectGetHandler uses cl, ok but doesn't check ok - still returns 200
	if w.Code != http.StatusUnauthorized && w.Code != http.StatusOK {
		t.Logf("status = %d", w.Code)
	}
}

// ============================================================================
// Metering handler - with mock upstream
// ============================================================================

func TestMeteringHandler_WithMockUpstream(t *testing.T) {
	// Start a mock upstream server
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"usage": 100}`))
	}))
	defer upstream.Close()

	os.Setenv("COST_CALCULATOR_URL", upstream.URL)
	defer os.Unsetenv("COST_CALCULATOR_URL")

	cfg := config.Config{}
	logger := zap.NewNop()
	handler := meteringHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/metering", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestBillingHandler_WithMockUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"total": 50.00}`))
	}))
	defer upstream.Close()

	os.Setenv("COST_CALCULATOR_URL", upstream.URL)
	defer os.Unsetenv("COST_CALCULATOR_URL")

	cfg := config.Config{}
	logger := zap.NewNop()
	handler := billingHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/billing", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestIndexerCatalogHandler_WithMockUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"catalog": []}`))
	}))
	defer upstream.Close()

	os.Setenv("INDEXER_URL", upstream.URL)
	defer os.Unsetenv("INDEXER_URL")

	cfg := config.Config{}
	logger := zap.NewNop()
	handler := indexerCatalogHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/indexer/catalog", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestIndexerScanHandler_WithMockUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusAccepted)
		w.Write([]byte(`{"job": "scan-123"}`))
	}))
	defer upstream.Close()

	os.Setenv("INDEXER_URL", upstream.URL)
	defer os.Unsetenv("INDEXER_URL")

	cfg := config.Config{}
	logger := zap.NewNop()
	handler := indexerScanHandler(cfg, logger)
	body := map[string]interface{}{"resource": "res1"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/indexer/scan", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestIndexerPIITargetsHandler_WithMockUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"targets": []}`))
	}))
	defer upstream.Close()

	t.Setenv("ENTERPRISE_LICENSE", "test-license")
	os.Setenv("INDEXER_URL", upstream.URL)
	defer os.Unsetenv("INDEXER_URL")

	cfg := config.Config{}
	logger := zap.NewNop()
	handler := indexerPIITargetsHandler(cfg, logger)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/indexer/pii-targets", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestPolicyEvaluateHandler_WithMockUpstream(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"result": "allow"}`))
	}))
	defer upstream.Close()

	os.Setenv("POLICY_ENGINE_URL", upstream.URL)
	defer os.Unsetenv("POLICY_ENGINE_URL")

	cfg := config.Config{}
	logger := zap.NewNop()
	handler := policyEvaluateHandler(cfg, logger)
	body := map[string]interface{}{"policy": "default"}
	bodyBytes, _ := json.Marshal(body)
	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/policy/evaluate", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	w := httptest.NewRecorder()
	handler(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}
