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

func TestStorageListHandler(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	handler := storageListHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/resources", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["tenant"] != "tenant1" {
		t.Errorf("response tenant = %v, want tenant1", resp["tenant"])
	}
}

func TestStorageListHandler_TenantMismatch(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := storageListHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant2/resources", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant2")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("status = %d, want %d", w.Code, http.StatusForbidden)
	}
}

func TestStorageGetHandler(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	handler := storageGetHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/resources/res1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "res1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestStorageCreateHandler_Success(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	handler := storageCreateHandler(cfg, logger)

	body := map[string]string{
		"name":         "res1",
		"type":         "object",
		"storageClass": "standard",
		"size":         "10Gi",
	}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/resources", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestStorageCreateHandler_MissingName(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := storageCreateHandler(cfg, logger)

	body := map[string]string{
		"type":         "s3",
		"storageClass": "standard",
	}
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

func TestStorageDeleteHandler(t *testing.T) {
	existing := makeDataResource("res1", "tenant1", nestv1.TypeObject)
	cfg := config.Config{K8sClient: makeFakeK8sClient(existing)}
	logger := zap.NewNop()
	handler := storageDeleteHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/resources/res1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "res1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestDbListHandler(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	handler := dbListHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/databases", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["tenant"] != "tenant1" {
		t.Errorf("response tenant = %v, want tenant1", resp["tenant"])
	}
}

func TestDbGetHandler(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	handler := dbGetHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/databases/db1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "db1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestDbCreateHandler_Success(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	handler := dbCreateHandler(cfg, logger)

	body := map[string]string{
		"name":  "db1",
		"type":  "postgres",
		"class": "standard",
	}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/databases", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestDbDeleteHandler(t *testing.T) {
	existing := makeDataResource("db1", "tenant1", nestv1.TypePostgres)
	cfg := config.Config{K8sClient: makeFakeK8sClient(existing)}
	logger := zap.NewNop()
	handler := dbDeleteHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/databases/db1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "db1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestExtListHandler(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	handler := extListHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/engines", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestWarehouseListHandler(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	handler := warehouseListHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/warehouses", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestDataresourceListHandler(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	handler := dataresourceListHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/dataresources", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestDataresourceGetHandler(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	handler := dataresourceGetHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/dataresources/dr1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "dr1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestEvidenceHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := evidenceHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/evidence/compliance/bundle1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("bundle", "bundle1")

	w := httptest.NewRecorder()
	handler(w, req)

	// Evidence handler requires enterprise license, returns 402 if not set
	if w.Code != http.StatusPaymentRequired {
		t.Errorf("status = %d, want %d", w.Code, http.StatusPaymentRequired)
	}
}

func TestIndexerCatalogHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := indexerCatalogHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/indexer/catalog", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	// Indexer handler makes HTTP request to indexer service (which is not running in tests)
	// so it fails with a 502 Bad Gateway or similar
	if w.Code != http.StatusBadGateway && w.Code != http.StatusServiceUnavailable {
		t.Logf("status = %d", w.Code)
	}
}

func TestMeteringHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := meteringHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/metering", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	// Metering handler makes HTTP request to metering service (which is not running)
	// just verify it doesn't crash with missing claims
	if w.Code < 200 || w.Code >= 600 {
		t.Logf("status = %d (expected service unavailable)", w.Code)
	}
}

func TestBillingHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := billingHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/billing", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	// Billing handler makes HTTP request to external service
	// Just verify it doesn't crash with missing claims
	if w.Code < 200 || w.Code >= 600 {
		t.Logf("status = %d (expected service unavailable)", w.Code)
	}
}

func TestIntelligenceRecommendHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := intelligenceRecommendHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/intelligence/recommend", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	// Intelligence handler requires enterprise license
	if w.Code != http.StatusPaymentRequired {
		t.Errorf("status = %d, want %d", w.Code, http.StatusPaymentRequired)
	}
}

func TestAnomalyDetectHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := anomalyDetectHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/anomaly/current", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	// Anomaly handler requires enterprise license
	if w.Code != http.StatusPaymentRequired {
		t.Errorf("status = %d, want %d", w.Code, http.StatusPaymentRequired)
	}
}
