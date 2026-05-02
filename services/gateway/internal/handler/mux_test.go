package handler

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/gateway/internal/claims"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
)

func TestNewMux(t *testing.T) {
	cfg := config.Config{OIDCAudience: "test"}
	logger := zap.NewNop()

	mux := NewMux(cfg, logger)
	if mux == nil {
		t.Error("NewMux() returned nil")
	}
}

func TestHealthEndpoint(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	mux := NewMux(cfg, logger)

	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("health status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("ok")) {
		t.Errorf("health response should contain 'ok', got %q", body)
	}
}

func TestReadyEndpoint(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	mux := NewMux(cfg, logger)

	req := httptest.NewRequest("GET", "/ready", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("ready status = %d, want %d", w.Code, http.StatusOK)
	}

	body := w.Body.String()
	if !bytes.Contains([]byte(body), []byte("ok")) {
		t.Errorf("ready response should contain 'ok', got %q", body)
	}
}

func TestMetricsEndpoint(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	mux := NewMux(cfg, logger)

	req := httptest.NewRequest("GET", "/metrics/probes", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("metrics status = %d, want %d", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "text/plain; version=0.0.4" {
		t.Errorf("Content-Type = %q, want text/plain; version=0.0.4", contentType)
	}
}

func TestQueryHandler_Success(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := queryHandler(cfg, logger)

	body := map[string]string{
		"resource": "test-resource",
		"sql":      "SELECT * FROM table",
	}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/query", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("query status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["tenant"] != "tenant1" || resp["resource"] != "test-resource" {
		t.Errorf("query response = %+v", resp)
	}
}

func TestQueryHandler_MissingClaims(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := queryHandler(cfg, logger)

	body := map[string]string{"resource": "test"}
	bodyBytes, _ := json.Marshal(body)

	req := httptest.NewRequest("POST", "/api/v1/query", bytes.NewReader(bodyBytes))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want %d", w.Code, http.StatusUnauthorized)
	}
}

func TestQueryHandler_MissingResource(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := queryHandler(cfg, logger)

	body := map[string]string{"sql": "SELECT *"}
	bodyBytes, _ := json.Marshal(body)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("POST", "/api/v1/query", bytes.NewReader(bodyBytes))
	req = req.WithContext(ctx)

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}
}

func TestObjectGetHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := objectGetHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/objects/res1/bucket1/key1", nil)
	req = req.WithContext(ctx)

	// Manually set path values since httptest doesn't use real routing
	req.SetPathValue("resource", "res1")
	req.SetPathValue("bucket", "bucket1")
	req.SetPathValue("key", "key1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestObjectPutHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := objectPutHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("PUT", "/api/v1/objects/res1/bucket1/key1", bytes.NewReader([]byte("data")))
	req = req.WithContext(ctx)
	req.SetPathValue("resource", "res1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}
}

func TestObjectDeleteHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := objectDeleteHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("DELETE", "/api/v1/objects/res1/bucket1/key1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("resource", "res1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("status = %d, want %d", w.Code, http.StatusNoContent)
	}
}

func TestVolumeInfoHandler(t *testing.T) {
	cfg := config.Config{}
	logger := zap.NewNop()
	handler := volumeInfoHandler(cfg, logger)

	ctx := makeContextWithClaims("user1", "tenant1")
	req := httptest.NewRequest("GET", "/api/v1/volumes/res1", nil)
	req = req.WithContext(ctx)
	req.SetPathValue("resource", "res1")

	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]interface{}
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["resource"] != "res1" || resp["tenant"] != "tenant1" {
		t.Errorf("response = %+v", resp)
	}
}

func TestWriteJSON(t *testing.T) {
	w := httptest.NewRecorder()
	data := map[string]string{"key": "value"}

	writeJSON(w, http.StatusOK, data)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["key"] != "value" {
		t.Errorf("response = %+v", resp)
	}
}

func TestWriteError(t *testing.T) {
	w := httptest.NewRecorder()
	writeError(w, http.StatusBadRequest, "test error")

	if w.Code != http.StatusBadRequest {
		t.Errorf("status = %d, want %d", w.Code, http.StatusBadRequest)
	}

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", contentType)
	}

	var resp map[string]string
	json.NewDecoder(w.Body).Decode(&resp)
	if resp["error"] != "test error" {
		t.Errorf("response = %+v", resp)
	}
}

// Helper function to create context with claims
func makeContextWithClaims(sub, tenant string) context.Context {
	cl := &claims.Claims{
		Subject: sub,
		Tenant:  tenant,
	}
	return claims.WithClaims(context.Background(), cl)
}
