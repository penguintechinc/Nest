package gateway_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/penguintechinc/nest/services/nfs-gateway/gateway"
)

func newTestGateway(t *testing.T) (*gateway.Gateway, string, *gateway.FakeGaneshaReloader) {
	t.Helper()
	dir := t.TempDir()
	fakeReloader := gateway.NewFakeGaneshaReloader()
	gw := gateway.New(gateway.Config{
		GaneshaConfigPath: dir,
		CephFSMount:       "/mnt/cephfs",
		Logger:            log.New(os.Stderr, "", 0),
		Reloader:          fakeReloader,
	})
	return gw, dir, fakeReloader
}

func TestCreateExport(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, _, fakeReloader := newTestGateway(t)

	r := gin.New()
	r.POST("/exports", gw.CreateExport)

	body := `{"name":"test","tenant":"acme","path":"/volumes/acme/test"}`
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

	// Verify that reload was called exactly once
	if fakeReloader.ReloadCount() != 1 {
		t.Fatalf("expected 1 reload call, got %d", fakeReloader.ReloadCount())
	}
}

func TestListExports(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, _, _ := newTestGateway(t)

	r := gin.New()
	r.POST("/exports", gw.CreateExport)
	r.GET("/exports", gw.ListExports)

	for i := 0; i < 3; i++ {
		body := `{"name":"test","tenant":"acme","path":"/volumes/acme/test"}`
		req := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(body))
		req.Header.Set("Content-Type", "application/json")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
		if w.Code != http.StatusCreated {
			t.Fatalf("failed to create export: expected 201, got %d: %s", w.Code, w.Body)
		}
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
	gw, _, fakeReloader := newTestGateway(t)

	r := gin.New()
	r.POST("/exports", gw.CreateExport)
	r.DELETE("/exports/:exportId", gw.DeleteExport)

	body := `{"name":"test","tenant":"acme","path":"/volumes/acme/test"}`
	req := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create export: expected 201, got %d: %s", w.Code, w.Body)
	}

	var created map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, ok := created["id"].(string)
	if !ok || id == "" {
		t.Fatalf("failed to extract export ID from response: %v", created)
	}

	// Verify reload was called once for create
	if fakeReloader.ReloadCount() != 1 {
		t.Fatalf("expected 1 reload call after create, got %d", fakeReloader.ReloadCount())
	}

	req = httptest.NewRequest(http.MethodDelete, "/exports/"+id, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	// Verify reload was called again for delete (total 2)
	if fakeReloader.ReloadCount() != 2 {
		t.Fatalf("expected 2 reload calls (create + delete), got %d", fakeReloader.ReloadCount())
	}
}

func TestDeleteExportNotFound(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, _, _ := newTestGateway(t)

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
	gw, _, _ := newTestGateway(t)

	r := gin.New()
	r.POST("/exports", gw.CreateExport)
	r.GET("/exports/:exportId", gw.GetExport)

	body := `{"name":"myexport","tenant":"acme","path":"/volumes/acme/myexport","accessMode":"ro","clients":"10.0.0.0/8"}`
	req := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create export: expected 201, got %d: %s", w.Code, w.Body)
	}

	var created map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id, ok := created["id"].(string)
	if !ok || id == "" {
		t.Fatalf("failed to extract export ID from response: %v", created)
	}

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
	gw, _, _ := newTestGateway(t)

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
	gw, _, fakeReloader := newTestGateway(t)

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

	// Verify reload was never called for a failed request
	if fakeReloader.ReloadCount() != 0 {
		t.Fatalf("expected 0 reload calls for bad request, got %d", fakeReloader.ReloadCount())
	}
}

func TestListExportsNoTenantFilter(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, _, _ := newTestGateway(t)

	r := gin.New()
	r.POST("/exports", gw.CreateExport)
	r.GET("/exports", gw.ListExports)

	tenants := []string{"acme", "acme", "globex"}
	for _, tenant := range tenants {
		body := `{"name":"test","tenant":"` + tenant + `","path":"/volumes/` + tenant + `/test"}`
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

func TestCreateExportConfigWriteFails(t *testing.T) {
	gin.SetMode(gin.TestMode)
	fakeReloader := gateway.NewFakeGaneshaReloader()
	gw := gateway.New(gateway.Config{
		GaneshaConfigPath: "/invalid/nonexistent/path/that/cannot/be/created",
		CephFSMount:       "/mnt/cephfs",
		Logger:            log.New(os.Stderr, "", 0),
		Reloader:          fakeReloader,
	})

	r := gin.New()
	r.POST("/exports", gw.CreateExport)

	body := `{"name":"test","tenant":"acme","path":"/volumes/acme/test"}`
	req := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != "nest.nfs.config_write_failed" {
		t.Fatalf("expected code nest.nfs.config_write_failed, got %v", resp["code"])
	}

	// Verify reload was never called when config write fails
	if fakeReloader.ReloadCount() != 0 {
		t.Fatalf("expected 0 reload calls when config write fails, got %d", fakeReloader.ReloadCount())
	}
}

func TestCreateExportWithReadOnlyMode(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, _, _ := newTestGateway(t)

	r := gin.New()
	r.POST("/exports", gw.CreateExport)
	r.GET("/exports/:exportId", gw.GetExport)

	body := `{"name":"roexport","tenant":"acme","path":"/volumes/acme/roexport","accessMode":"ro","clients":"192.168.0.0/16"}`
	req := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body)
	}

	var created map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created["id"].(string)

	if created["accessMode"] != "ro" {
		t.Fatalf("expected accessMode ro in create response, got %v", created["accessMode"])
	}

	req = httptest.NewRequest(http.MethodGet, "/exports/"+id, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var getResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &getResp); err != nil {
		t.Fatal(err)
	}
	if getResp["accessMode"] != "ro" {
		t.Fatalf("expected accessMode ro in get response, got %v", getResp["accessMode"])
	}
	if getResp["clients"] != "192.168.0.0/16" {
		t.Fatalf("expected clients 192.168.0.0/16, got %v", getResp["clients"])
	}
}

func TestCreateExportDefaultValues(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, _, _ := newTestGateway(t)

	r := gin.New()
	r.POST("/exports", gw.CreateExport)
	r.GET("/exports/:exportId", gw.GetExport)

	// Create with minimal fields (no accessMode, no clients)
	body := `{"name":"minimal","tenant":"acme","path":"/volumes/acme/minimal"}`
	req := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body)
	}

	var created map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created["id"].(string)

	// Verify defaults were applied (secure defaults: rw access, localhost only)
	if created["accessMode"] != "rw" {
		t.Fatalf("expected accessMode rw (default), got %v", created["accessMode"])
	}
	if created["clients"] != "127.0.0.1" {
		t.Fatalf("expected clients 127.0.0.1 (secure default), got %v", created["clients"])
	}

	// Verify persistence through GET
	req = httptest.NewRequest(http.MethodGet, "/exports/"+id, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var getResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &getResp); err != nil {
		t.Fatal(err)
	}
	if getResp["accessMode"] != "rw" {
		t.Fatalf("expected accessMode rw in get response, got %v", getResp["accessMode"])
	}
	if getResp["clients"] != "127.0.0.1" {
		t.Fatalf("expected clients 127.0.0.1 in get response, got %v", getResp["clients"])
	}
}

func TestCreateExportReloadFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, dir, fakeReloader := newTestGateway(t)

	r := gin.New()
	r.POST("/exports", gw.CreateExport)
	r.GET("/exports", gw.ListExports)

	body := `{"name":"test","tenant":"acme","path":"/volumes/acme/test"}`
	req := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()

	// Configure fake reloader to fail
	fakeReloader.SetShouldFail(true, fmt.Errorf("simulated reload failure"))

	r.ServeHTTP(w, req)

	// CreateExport should return error when reload fails
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when reload fails, got %d: %s", w.Code, w.Body)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["code"] != "nest.nfs.reload_failed" {
		t.Fatalf("expected code nest.nfs.reload_failed, got %v", resp["code"])
	}

	// Verify that reload was attempted
	if fakeReloader.ReloadCount() != 1 {
		t.Fatalf("expected 1 reload attempt, got %d", fakeReloader.ReloadCount())
	}

	// Verify that the export was rolled back (not added to the list)
	req = httptest.NewRequest(http.MethodGet, "/exports?tenant=acme", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var listResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	count, _ := listResp["count"].(float64)
	if count != 0 {
		t.Fatalf("expected 0 exports after rollback, got %v", count)
	}

	// Verify that config file was removed
	files, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read config directory: %v", err)
	}
	if len(files) != 0 {
		t.Fatalf("expected 0 config files after rollback, got %d", len(files))
	}
}

func TestDeleteExportReloadFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, _, fakeReloader := newTestGateway(t)

	r := gin.New()
	r.POST("/exports", gw.CreateExport)
	r.DELETE("/exports/:exportId", gw.DeleteExport)
	r.GET("/exports", gw.ListExports)

	// Create export successfully
	body := `{"name":"test","tenant":"acme","path":"/volumes/acme/test"}`
	req := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("failed to create export: expected 201, got %d", w.Code)
	}

	var created map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &created); err != nil {
		t.Fatal(err)
	}
	id := created["id"].(string)

	// Clear reload count
	fakeReloader.ReloadCount() // reset

	// Configure reloader to fail on next reload (delete)
	fakeReloader.SetShouldFail(true, fmt.Errorf("simulated reload failure"))

	// Try to delete
	req = httptest.NewRequest(http.MethodDelete, "/exports/"+id, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// DeleteExport should return error when reload fails
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 when delete reload fails, got %d", w.Code)
	}

	// Verify export was restored to the list
	req = httptest.NewRequest(http.MethodGet, "/exports?tenant=acme", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var listResp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp); err != nil {
		t.Fatal(err)
	}
	count, _ := listResp["count"].(float64)
	if count != 1 {
		t.Fatalf("expected 1 export after rollback, got %v", count)
	}
}

func TestCreateExportIdempotentRecreate(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, _, fakeReloader := newTestGateway(t)

	r := gin.New()
	r.POST("/exports", gw.CreateExport)
	r.DELETE("/exports/:exportId", gw.DeleteExport)
	r.GET("/exports", gw.ListExports)

	body := `{"name":"idempotent-test","tenant":"acme","path":"/volumes/acme/idempotent-test"}`

	// Create first export
	req := httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("first create failed: expected 201, got %d", w.Code)
	}

	var created1 map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &created1); err != nil {
		t.Fatal(err)
	}
	id1 := created1["id"].(string)

	if fakeReloader.ReloadCount() != 1 {
		t.Fatalf("expected 1 reload after first create, got %d", fakeReloader.ReloadCount())
	}

	// Delete
	req = httptest.NewRequest(http.MethodDelete, "/exports/"+id1, nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("delete failed: expected 204, got %d", w.Code)
	}

	if fakeReloader.ReloadCount() != 2 {
		t.Fatalf("expected 2 reloads after delete, got %d", fakeReloader.ReloadCount())
	}

	// Verify export is gone
	req = httptest.NewRequest(http.MethodGet, "/exports?tenant=acme", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var listResp1 map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp1); err != nil {
		t.Fatal(err)
	}
	count1, _ := listResp1["count"].(float64)
	if count1 != 0 {
		t.Fatalf("expected 0 exports after delete, got %v", count1)
	}

	// Recreate with the same parameters (should get a new ID)
	req = httptest.NewRequest(http.MethodPost, "/exports", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("second create failed: expected 201, got %d", w.Code)
	}

	var created2 map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &created2); err != nil {
		t.Fatal(err)
	}
	id2 := created2["id"].(string)

	// IDs should be different (UUIDs)
	if id1 == id2 {
		t.Fatalf("expected different IDs after delete/recreate, got same ID: %s", id1)
	}

	if fakeReloader.ReloadCount() != 3 {
		t.Fatalf("expected 3 reloads after recreate, got %d", fakeReloader.ReloadCount())
	}

	// Verify export is back
	req = httptest.NewRequest(http.MethodGet, "/exports?tenant=acme", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var listResp2 map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &listResp2); err != nil {
		t.Fatal(err)
	}
	count2, _ := listResp2["count"].(float64)
	if count2 != 1 {
		t.Fatalf("expected 1 export after recreate, got %v", count2)
	}
}
