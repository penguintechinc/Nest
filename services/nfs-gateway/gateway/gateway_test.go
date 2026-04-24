package gateway_test

import (
	"bytes"
	"encoding/json"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/penguintechinc/nest/services/nfs-gateway/gateway"
)

func newTestGateway(t *testing.T) (*gateway.Gateway, string) {
	t.Helper()
	dir := t.TempDir()
	gw := gateway.New(gateway.Config{
		GaneshaConfigPath: dir,
		CephFSMount:       "/mnt/cephfs",
		Logger:            log.New(os.Stderr, "", 0),
	})
	return gw, dir
}

func TestCreateExport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, _ := newTestGateway(t)

	r := gin.New()
	r.POST("/exports", gw.CreateExport)

	body := `{"name":"test","tenant":"acme","path":"/cephfs/acme/test"}`
	req := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["id"] == "" {
		t.Fatal("expected export id in response")
	}
}

func TestListExports(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, _ := newTestGateway(t)

	r := gin.New()
	r.POST("/exports", gw.CreateExport)
	r.GET("/exports", gw.ListExports)

	for i := 0; i < 3; i++ {
		body := `{"name":"test","tenant":"acme","path":"/cephfs/acme/test"}`
		req := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}

	req := httptest.NewRequest(http.MethodGet, "/exports?tenant=acme", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	count, _ := resp["count"].(float64)
	if count != 3 {
		t.Fatalf("expected 3 exports, got %v", count)
	}
}

func TestDeleteExport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, _ := newTestGateway(t)

	r := gin.New()
	r.POST("/exports", gw.CreateExport)
	r.DELETE("/exports/:exportId", gw.DeleteExport)

	body := `{"name":"test","tenant":"acme","path":"/cephfs/acme/test"}`
	req := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var created map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created["id"].(string)

	req = httptest.NewRequest(http.MethodDelete, "/exports/"+id, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}
}

func TestDeleteExportNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, _ := newTestGateway(t)

	r := gin.New()
	r.DELETE("/exports/:exportId", gw.DeleteExport)

	req := httptest.NewRequest(http.MethodDelete, "/exports/nonexistent", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestGetExport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, _ := newTestGateway(t)

	r := gin.New()
	r.POST("/exports", gw.CreateExport)
	r.GET("/exports/:exportId", gw.GetExport)

	body := `{"name":"myexport","tenant":"acme","path":"/cephfs/acme/myexport","accessMode":"ro","clients":"10.0.0.0/8"}`
	req := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var created map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created["id"].(string)

	req = httptest.NewRequest(http.MethodGet, "/exports/"+id, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["id"] != id {
		t.Fatalf("expected id %s, got %v", id, resp["id"])
	}
	if resp["accessMode"] != "ro" {
		t.Fatalf("expected accessMode ro, got %v", resp["accessMode"])
	}
}

func TestGetExportNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, _ := newTestGateway(t)

	r := gin.New()
	r.GET("/exports/:exportId", gw.GetExport)

	req := httptest.NewRequest(http.MethodGet, "/exports/doesnotexist", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCreateExportMissingFields(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, _ := newTestGateway(t)

	r := gin.New()
	r.POST("/exports", gw.CreateExport)

	body := `{"name":"test"}`
	req := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body)
	}
}

func TestListExportsNoTenantFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, _ := newTestGateway(t)

	r := gin.New()
	r.POST("/exports", gw.CreateExport)
	r.GET("/exports", gw.ListExports)

	tenants := []string{"acme", "acme", "globex"}
	for _, tenant := range tenants {
		body := `{"name":"test","tenant":"` + tenant + `","path":"/cephfs/` + tenant + `/test"}`
		req := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	}

	req := httptest.NewRequest(http.MethodGet, "/exports", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	count, _ := resp["count"].(float64)
	if count != 3 {
		t.Fatalf("expected 3 total exports, got %v", count)
	}
}
