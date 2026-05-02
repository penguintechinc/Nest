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

// ============================================================================
// Warehouse Handler Tests
// ============================================================================

func TestWarehouseListEmpty(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := warehouseListHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/warehouses", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if count := resp["count"].(float64); count != 0 {
		t.Errorf("count = %v, want 0", count)
	}
	if len(resp["warehouses"].([]interface{})) != 0 {
		t.Errorf("warehouses array should be empty")
	}
}

func TestWarehouseListWithItems(t *testing.T) {
	dr1 := makeDataResource("trino1", "tenant1", nestv1.TypeTrino)
	dr2 := makeDataResource("iceberg1", "tenant1", nestv1.TypeIceberg)
	dr3 := makeDataResource("pg1", "tenant1", nestv1.TypePostgres) // non-warehouse

	cfg := config.Config{K8sClient: makeFakeK8sClient(dr1, dr2, dr3)}
	logger := zap.NewNop()
	h := warehouseListHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/warehouses", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if count := resp["count"].(float64); count != 2 {
		t.Errorf("count = %v, want 2 (only warehouses)", count)
	}
}

func TestWarehouseGetExisting(t *testing.T) {
	dr := makeDataResource("trino1", "tenant1", nestv1.TypeTrino)
	cfg := config.Config{K8sClient: makeFakeK8sClient(dr)}
	logger := zap.NewNop()
	h := warehouseGetHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/warehouses/trino1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "trino1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["name"] != "trino1" {
		t.Errorf("name = %v, want trino1", resp["name"])
	}
	if resp["type"] != nestv1.TypeTrino {
		t.Errorf("type = %v, want %s", resp["type"], nestv1.TypeTrino)
	}
}

func TestWarehouseGetMissing(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := warehouseGetHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/warehouses/missing", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "missing")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestWarehouseGetWrongType(t *testing.T) {
	dr := makeDataResource("pg1", "tenant1", nestv1.TypePostgres)
	cfg := config.Config{K8sClient: makeFakeK8sClient(dr)}
	logger := zap.NewNop()
	h := warehouseGetHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/warehouses/pg1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "pg1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (resource exists but wrong type)", w.Code, http.StatusNotFound)
	}
}

func TestWarehouseCreateValid(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := warehouseCreateHandler(cfg, logger)

	body := map[string]interface{}{
		"name":  "trino1",
		"type":  nestv1.TypeTrino,
		"class": "standard",
	}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/warehouses", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	warehouse := resp["warehouse"].(map[string]interface{})
	if warehouse["name"] != "trino1" {
		t.Errorf("name = %v, want trino1", warehouse["name"])
	}
	if warehouse["type"] != nestv1.TypeTrino {
		t.Errorf("type = %v, want %s", warehouse["type"], nestv1.TypeTrino)
	}

	if w.Header().Get("Location") == "" {
		t.Error("Location header not set")
	}
}

func TestWarehouseCreateDuplicate(t *testing.T) {
	existing := makeDataResource("trino1", "tenant1", nestv1.TypeTrino)
	cfg := config.Config{K8sClient: makeFakeK8sClient(existing)}
	logger := zap.NewNop()
	h := warehouseCreateHandler(cfg, logger)

	body := map[string]interface{}{"name": "trino1", "type": nestv1.TypeTrino}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/warehouses", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestWarehouseCreateInvalidType(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := warehouseCreateHandler(cfg, logger)

	body := map[string]interface{}{"name": "kafka1", "type": nestv1.TypeKafka}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/warehouses", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (invalid warehouse type)", w.Code, http.StatusBadRequest)
	}
}

func TestWarehouseDeleteExisting(t *testing.T) {
	dr := makeDataResource("trino1", "tenant1", nestv1.TypeTrino)
	cfg := config.Config{K8sClient: makeFakeK8sClient(dr)}
	logger := zap.NewNop()
	h := warehouseDeleteHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/warehouses/trino1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "trino1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestWarehouseDeleteMissing(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := warehouseDeleteHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/warehouses/missing", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "missing")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

// ============================================================================
// Extended Engine Handler Tests
// ============================================================================

func TestExtListEmpty(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := extListHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/engines", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if count := resp["count"].(float64); count != 0 {
		t.Errorf("count = %v, want 0", count)
	}
}

func TestExtListWithItems(t *testing.T) {
	dr1 := makeDataResource("kafka1", "tenant1", nestv1.TypeKafka)
	dr2 := makeDataResource("search1", "tenant1", nestv1.TypeSearch)
	dr3 := makeDataResource("pg1", "tenant1", nestv1.TypePostgres) // non-extended

	cfg := config.Config{K8sClient: makeFakeK8sClient(dr1, dr2, dr3)}
	logger := zap.NewNop()
	h := extListHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/engines", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if count := resp["count"].(float64); count != 2 {
		t.Errorf("count = %v, want 2 (only extended types)", count)
	}
}

func TestExtGetExisting(t *testing.T) {
	dr := makeDataResource("kafka1", "tenant1", nestv1.TypeKafka)
	cfg := config.Config{K8sClient: makeFakeK8sClient(dr)}
	logger := zap.NewNop()
	h := extGetHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/engines/kafka1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "kafka1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["name"] != "kafka1" {
		t.Errorf("name = %v, want kafka1", resp["name"])
	}
	if resp["type"] != nestv1.TypeKafka {
		t.Errorf("type = %v, want %s", resp["type"], nestv1.TypeKafka)
	}
}

func TestExtGetMissing(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := extGetHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/engines/missing", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "missing")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestExtGetWrongType(t *testing.T) {
	dr := makeDataResource("pg1", "tenant1", nestv1.TypePostgres)
	cfg := config.Config{K8sClient: makeFakeK8sClient(dr)}
	logger := zap.NewNop()
	h := extGetHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/engines/pg1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "pg1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d (resource exists but wrong type)", w.Code, http.StatusNotFound)
	}
}

func TestExtCreateValid(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := extCreateHandler(cfg, logger)

	body := map[string]interface{}{
		"name":  "kafka1",
		"type":  nestv1.TypeKafka,
		"class": "standard",
	}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/engines", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("status = %d, want %d: %s", w.Code, http.StatusAccepted, w.Body.String())
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)

	engine := resp["engine"].(map[string]interface{})
	if engine["name"] != "kafka1" {
		t.Errorf("name = %v, want kafka1", engine["name"])
	}
	if engine["type"] != nestv1.TypeKafka {
		t.Errorf("type = %v, want %s", engine["type"], nestv1.TypeKafka)
	}

	if w.Header().Get("Location") == "" {
		t.Error("Location header not set")
	}
}

func TestExtCreateDuplicate(t *testing.T) {
	existing := makeDataResource("kafka1", "tenant1", nestv1.TypeKafka)
	cfg := config.Config{K8sClient: makeFakeK8sClient(existing)}
	logger := zap.NewNop()
	h := extCreateHandler(cfg, logger)

	body := map[string]interface{}{"name": "kafka1", "type": nestv1.TypeKafka}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/engines", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusConflict {
		t.Errorf("status = %d, want %d", w.Code, http.StatusConflict)
	}
}

func TestExtCreateInvalidType(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := extCreateHandler(cfg, logger)

	body := map[string]interface{}{"name": "pg1", "type": nestv1.TypePostgres}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/engines", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d (invalid extended type)", w.Code, http.StatusBadRequest)
	}
}

func TestExtDeleteExisting(t *testing.T) {
	dr := makeDataResource("kafka1", "tenant1", nestv1.TypeKafka)
	cfg := config.Config{K8sClient: makeFakeK8sClient(dr)}
	logger := zap.NewNop()
	h := extDeleteHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/engines/kafka1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "kafka1")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestExtDeleteMissing(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	h := extDeleteHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/engines/missing", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "missing")

	w := httptest.NewRecorder()
	h(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}
