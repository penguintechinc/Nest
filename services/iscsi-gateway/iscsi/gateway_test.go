package iscsi_test

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/penguintechinc/nest/services/iscsi-gateway/iscsi"
)

func newTestGatewayWithMockAPI(t *testing.T) (*iscsi.Gateway, *httptest.Server) {
	// Create a mock Ceph-iSCSI API server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/target":
			var req map[string]string
			json.NewDecoder(r.Body).Decode(&req)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"target_iqn": req["target_iqn"],
				"status":     "created",
			})
		case r.Method == http.MethodPost && r.URL.Path[0:len("/api/target")] == "/api/target" && len(r.URL.Path) > len("/api/target"):
			// Disk attachment
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "attached",
			})
		case r.Method == http.MethodDelete:
			// Delete target
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "deleted",
			})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	// Set the env var to use our mock server
	os.Setenv("CEPH_ISCSI_API_URL", mockServer.URL)
	gw := iscsi.New(iscsi.Config{
		CephISCSIEndpoint: mockServer.URL,
		Logger:            log.New(os.Stderr, "", 0),
	})

	return gw, mockServer
}

func newTestGateway() *iscsi.Gateway {
	// Create a minimal mock server for backward compatibility
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/target":
			var req map[string]string
			json.NewDecoder(r.Body).Decode(&req)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"target_iqn": req["target_iqn"],
				"status":     "created",
			})
		case r.Method == http.MethodPost && len(r.URL.Path) > len("/api/target"):
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "attached"})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))

	os.Setenv("CEPH_ISCSI_API_URL", mockServer.URL)
	gw := iscsi.New(iscsi.Config{
		CephISCSIEndpoint: mockServer.URL,
		Logger:            log.New(os.Stderr, "", 0),
	})
	return gw
}

func TestCreateTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := newTestGateway()
	r := gin.New()
	r.POST("/targets", gw.CreateTarget)

	body := `{"name":"vol1","tenant":"acme","rbdImage":"nest-rbd-acme-vol1"}`
	req := httptest.NewRequest(http.MethodPost, "/targets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	iqn, _ := resp["iqn"].(string)
	if iqn != "iqn.2024-01.io.penguintech.nest:acme:vol1" {
		t.Fatalf("unexpected IQN: %s", iqn)
	}
}

func TestDeleteTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := newTestGateway()
	r := gin.New()
	r.POST("/targets", gw.CreateTarget)
	r.DELETE("/targets/:targetId", gw.DeleteTarget)

	body := `{"name":"vol1","tenant":"acme","rbdImage":"nest-rbd-acme-vol1"}`
	req := httptest.NewRequest(http.MethodPost, "/targets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"].(string)

	req = httptest.NewRequest(http.MethodDelete, "/targets/"+id, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestListTargets(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := newTestGateway()
	r := gin.New()
	r.POST("/targets", gw.CreateTarget)
	r.GET("/targets", gw.ListTargets)

	for i := 0; i < 2; i++ {
		body := `{"name":"vol","tenant":"acme","rbdImage":"nest-rbd-vol"}`
		req := httptest.NewRequest(http.MethodPost, "/targets", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}

	req := httptest.NewRequest(http.MethodGet, "/targets", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestGetTarget(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := newTestGateway()
	r := gin.New()
	r.POST("/targets", gw.CreateTarget)
	r.GET("/targets/:targetId", gw.GetTarget)

	// Create a target
	body := `{"name":"vol1","tenant":"acme","rbdImage":"nest-rbd-acme-vol1"}`
	req := httptest.NewRequest(http.MethodPost, "/targets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"].(string)

	// Get the target
	req = httptest.NewRequest(http.MethodGet, "/targets/"+id, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["id"] != id {
		t.Fatalf("expected target id %s, got %v", id, result["id"])
	}
	if result["name"] != "vol1" {
		t.Fatalf("expected name vol1, got %v", result["name"])
	}
}

func TestGetTargetNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := newTestGateway()
	r := gin.New()
	r.GET("/targets/:targetId", gw.GetTarget)

	req := httptest.NewRequest(http.MethodGet, "/targets/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCreateTargetInvalidJSON(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := newTestGateway()
	r := gin.New()
	r.POST("/targets", gw.CreateTarget)

	body := `{"name":"vol1","tenant":"acme"` // Missing closing brace
	req := httptest.NewRequest(http.MethodPost, "/targets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestCreateTargetMissingRequired(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := newTestGateway()
	r := gin.New()
	r.POST("/targets", gw.CreateTarget)

	body := `{"name":"vol1"}` // Missing tenant and rbdImage
	req := httptest.NewRequest(http.MethodPost, "/targets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestDeleteTargetNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := newTestGateway()
	r := gin.New()
	r.DELETE("/targets/:targetId", gw.DeleteTarget)

	req := httptest.NewRequest(http.MethodDelete, "/targets/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCreateTargetWithCustomPool(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw := newTestGateway()
	r := gin.New()
	r.POST("/targets", gw.CreateTarget)
	r.GET("/targets/:targetId", gw.GetTarget)

	body := `{"name":"vol1","tenant":"acme","rbdImage":"nest-rbd-acme-vol1","rbdPool":"custom-pool"}`
	req := httptest.NewRequest(http.MethodPost, "/targets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"].(string)

	// Verify the custom pool was stored
	req = httptest.NewRequest(http.MethodGet, "/targets/"+id, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var result map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &result)
	if result["rbdPool"] != "custom-pool" {
		t.Fatalf("expected rbdPool custom-pool, got %v", result["rbdPool"])
	}
}
