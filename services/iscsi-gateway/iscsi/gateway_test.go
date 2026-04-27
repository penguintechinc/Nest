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

func newTestGateway() *iscsi.Gateway {
	return iscsi.New(iscsi.Config{
		CephISCSIEndpoint: "http://localhost:5000",
		Logger:            log.New(os.Stderr, "", 0),
	})
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
