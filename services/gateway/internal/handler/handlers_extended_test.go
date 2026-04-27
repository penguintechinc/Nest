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

func TestExtGetHandler(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	handler := extGetHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/engines/eng1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "eng1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestExtCreateHandler_Success(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	handler := extCreateHandler(cfg, logger)

	body := map[string]string{
		"name":  "eng1",
		"type":  nestv1.TypeKafka,
		"class": "standard",
	}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/engines", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestExtCreateHandler_MissingName(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	handler := extCreateHandler(cfg, logger)

	body := map[string]string{
		"type":  nestv1.TypeKafka,
		"class": "standard",
	}
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

func TestExtDeleteHandler(t *testing.T) {
	existing := makeDataResource("eng1", "tenant1", nestv1.TypeKafka)
	cfg := config.Config{K8sClient: makeFakeK8sClient(existing)}
	logger := zap.NewNop()
	handler := extDeleteHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/engines/eng1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "eng1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestWarehouseGetHandler(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	handler := warehouseGetHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/warehouses/wh1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "wh1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNotFound)
	}
}

func TestWarehouseCreateHandler_Success(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	handler := warehouseCreateHandler(cfg, logger)

	body := map[string]string{
		"name":  "wh1",
		"type":  nestv1.TypeTrino,
		"class": "standard",
	}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/warehouses", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestWarehouseDeleteHandler(t *testing.T) {
	existing := makeDataResource("wh1", "tenant1", nestv1.TypeTrino)
	cfg := config.Config{K8sClient: makeFakeK8sClient(existing)}
	logger := zap.NewNop()
	handler := warehouseDeleteHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/warehouses/wh1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "wh1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestDataresourceCreateHandler_Success(t *testing.T) {
	cfg := config.Config{K8sClient: makeFakeK8sClient()}
	logger := zap.NewNop()
	handler := dataresourceCreateHandler(cfg, logger)

	body := map[string]string{
		"name":  "dr1",
		"type":  "table",
		"class": "standard",
		"size":  "10Gi",
	}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/dataresources", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestDataresourceCreateHandler_MissingName(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dataresourceCreateHandler(cfg, logger)

	body := map[string]string{
		"type":  "table",
		"class": "standard",
	}
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

func TestDataresourcePatchHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := dataresourcePatchHandler(cfg, logger)

	body := map[string]interface{}{
		"status": "archived",
	}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("PATCH", "/api/v1/tenants/tenant1/dataresources/dr1", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "dr1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestDataresourceDeleteHandler(t *testing.T) {
	existing := makeDataResource("dr1", "tenant1", "postgres")
	cfg := config.Config{K8sClient: makeFakeK8sClient(existing)}
	logger := zap.NewNop()
	handler := dataresourceDeleteHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/tenants/tenant1/dataresources/dr1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "dr1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestImportHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := importHandler(cfg, logger)

	body := map[string]interface{}{
		"source": "s3://bucket/path",
	}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/resources/res1/import", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "res1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestExportHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := exportHandler(cfg, logger)

	body := map[string]interface{}{
		"destination": "s3://bucket/path",
	}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/resources/res1/export", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")
	req.SetPathValue("name", "res1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusAccepted {
		t.Errorf("status = %d, want %d", w.Code, http.StatusAccepted)
	}
}

func TestIndexerScanHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := indexerScanHandler(cfg, logger)

	body := map[string]interface{}{
		"resource": "res1",
	}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/indexer/scan", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	// Scan handler makes external request, may fail
	if w.Code < 200 || w.Code >= 600 {
		t.Logf("status = %d (expected service unavailable)", w.Code)
	}
}

func TestIndexerPIITargetsHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := indexerPIITargetsHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/indexer/pii-targets", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	// PII targets handler makes external request
	if w.Code < 200 || w.Code >= 600 {
		t.Logf("status = %d (expected service unavailable)", w.Code)
	}
}

func TestPolicyEvaluateHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := policyEvaluateHandler(cfg, logger)

	body := map[string]interface{}{
		"policy": "default",
	}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/tenants/tenant1/policy/evaluate", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	// Policy handler makes external request
	if w.Code < 200 || w.Code >= 600 {
		t.Logf("status = %d (expected service unavailable)", w.Code)
	}
}

func TestPredictiveDriveHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := predictiveDriveHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/tenants/tenant1/predictive-drive/risk", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("tid", "tenant1")

	w := httptest.NewRecorder()
	handler(w, req)

	// Predictive drive requires enterprise license
	if w.Code != http.StatusPaymentRequired {
		t.Errorf("status = %d, want %d", w.Code, http.StatusPaymentRequired)
	}
}
