package iscsi_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/penguintechinc/nest/services/iscsi-gateway/iscsi"
)

type mockAPIRequest struct {
	Method string
	Path   string
	Body   map[string]interface{}
}

func newTestGatewayWithMockAPI(t *testing.T) (*iscsi.Gateway, *httptest.Server, *[]mockAPIRequest) {
	// Create a mock Ceph-iSCSI API server
	var requests []mockAPIRequest

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Record the request
		body, _ := io.ReadAll(r.Body)
		var bodyMap map[string]interface{}
		if len(body) > 0 {
			json.Unmarshal(body, &bodyMap)
		}
		requests = append(requests, mockAPIRequest{
			Method: r.Method,
			Path:   r.URL.Path,
			Body:   bodyMap,
		})

		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/target":
			var req map[string]string
			json.NewDecoder(bytes.NewReader(body)).Decode(&req)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"target_iqn": req["target_iqn"],
				"status":     "created",
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/disk"):
			// Disk attachment
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "attached",
			})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/api/client/"):
			// Client ACL registration
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "acl_applied",
			})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/api/clientauth/"):
			// CHAP auth application
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"status": "auth_applied",
			})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/api/target/"):
			// Status polling
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"target_iqn": "iqn.test",
				"status":     "ready",
			})
		case r.Method == http.MethodDelete:
			// Delete target
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
		Logger:            log.New(io.Discard, "", 0), // Discard logs during tests
	})

	return gw, mockServer, &requests
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

	body := `{"name":"vol1","tenant":"acme","rbdImage":"rbd/nest-acme-vol1"}`
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

	body := `{"name":"vol1","tenant":"acme","rbdImage":"rbd/nest-acme-vol1"}`
	req := httptest.NewRequest(http.MethodPost, "/targets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id, ok := created["id"].(string)
	if !ok || id == "" {
		t.Fatalf("failed to extract target ID from response: %v", created)
	}

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
		body := `{"name":"vol","tenant":"acme","rbdImage":"rbd/nest-vol"}`
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
	body := `{"name":"vol1","tenant":"acme","rbdImage":"rbd/nest-acme-vol1"}`
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

	body := `{"name":"vol1","tenant":"acme","rbdImage":"custom-pool/nest-acme-vol1","rbdPool":"custom-pool"}`
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

func TestCreateTargetWithInitiatorAndCHAP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, server, requests := newTestGatewayWithMockAPI(t)
	defer server.Close()

	r := gin.New()
	r.POST("/targets", gw.CreateTarget)

	body := `{
		"name":"vol1",
		"tenant":"acme",
		"rbdImage":"rbd/nest-acme-vol1",
		"initiatorIqn":"iqn.1991-05.com.example:storage.disk1",
		"chapUsername":"user123",
		"chapPassword":"secret_password_12345"
	}`
	req := httptest.NewRequest(http.MethodPost, "/targets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)

	// Verify ACL and CHAP endpoints were called
	clientACLFound := false
	chapAuthFound := false
	for _, req := range *requests {
		if req.Method == http.MethodPut && strings.Contains(req.Path, "/api/client/") && !strings.Contains(req.Path, "clientauth") {
			clientACLFound = true
		}
		if req.Method == http.MethodPut && strings.Contains(req.Path, "/api/clientauth/") {
			chapAuthFound = true
		}
	}

	if !clientACLFound {
		t.Fatal("expected client ACL endpoint to be called")
	}
	if !chapAuthFound {
		t.Fatal("expected CHAP auth endpoint to be called")
	}

	// Wait a moment for status polling to start
	time.Sleep(100 * time.Millisecond)
}

func TestCreateTargetWithInitiatorNoCHAP(t *testing.T) {
	gin.SetMode(gin.TestMode)
	gw, server, requests := newTestGatewayWithMockAPI(t)
	defer server.Close()

	r := gin.New()
	r.POST("/targets", gw.CreateTarget)

	body := `{
		"name":"vol1",
		"tenant":"acme",
		"rbdImage":"rbd/nest-acme-vol1",
		"initiatorIqn":"iqn.1991-05.com.example:storage.disk1"
	}`
	req := httptest.NewRequest(http.MethodPost, "/targets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body)
	}

	// Verify ACL endpoint was called but not CHAP
	clientACLFound := false
	chapAuthFound := false
	for _, req := range *requests {
		if req.Method == http.MethodPut && strings.Contains(req.Path, "/api/client/") && !strings.Contains(req.Path, "clientauth") {
			clientACLFound = true
		}
		if req.Method == http.MethodPut && strings.Contains(req.Path, "/api/clientauth/") {
			chapAuthFound = true
		}
	}

	if !clientACLFound {
		t.Fatal("expected client ACL endpoint to be called")
	}
	if chapAuthFound {
		t.Fatal("expected CHAP auth endpoint to NOT be called when no credentials provided")
	}
}

func TestCreateTargetACLFailure(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a mock server that fails on ACL endpoint
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/target":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "created"})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/disk"):
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "attached"})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/api/client/"):
			// Simulate ACL failure
			w.WriteHeader(http.StatusInternalServerError)
			json.NewEncoder(w).Encode(map[string]string{"error": "ACL application failed"})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	gw := iscsi.New(iscsi.Config{
		CephISCSIEndpoint: mockServer.URL,
		Logger:            log.New(io.Discard, "", 0),
	})

	r := gin.New()
	r.POST("/targets", gw.CreateTarget)

	body := `{
		"name":"vol1",
		"tenant":"acme",
		"rbdImage":"rbd/nest-acme-vol1",
		"initiatorIqn":"iqn.1991-05.com.example:storage.disk1"
	}`
	req := httptest.NewRequest(http.MethodPost, "/targets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Should fail due to ACL error
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500 on ACL failure, got %d: %s", w.Code, w.Body)
	}

	var errResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &errResp)
	if !strings.Contains(errResp["message"].(string), "ACL") {
		t.Fatalf("expected ACL error message, got %v", errResp["message"])
	}
}

func TestCHAPSecretNeverLogged(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Capture logs to verify CHAP secret is never logged
	logBuffer := &bytes.Buffer{}
	logger := log.New(logBuffer, "", 0)

	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/target":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "created"})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/disk"):
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "attached"})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/api/client/"):
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "acl_applied"})
		case r.Method == http.MethodPut && strings.Contains(r.URL.Path, "/api/clientauth/"):
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "auth_applied"})
		case r.Method == http.MethodGet:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "ready"})
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	gw := iscsi.New(iscsi.Config{
		CephISCSIEndpoint: mockServer.URL,
		Logger:            logger,
	})

	r := gin.New()
	r.POST("/targets", gw.CreateTarget)

	body := `{
		"name":"vol1",
		"tenant":"acme",
		"rbdImage":"rbd/nest-acme-vol1",
		"initiatorIqn":"iqn.1991-05.com.example:storage.disk1",
		"chapUsername":"testuser",
		"chapPassword":"my_super_secret_password_123"
	}`
	req := httptest.NewRequest(http.MethodPost, "/targets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	logs := logBuffer.String()

	// Verify that the secret password never appears in logs
	if strings.Contains(logs, "my_super_secret_password_123") {
		t.Fatal("CHAP password found in logs - security violation!")
	}

	// Verify that masked password appears
	if !strings.Contains(logs, "****") {
		t.Fatal("expected masked password (****) in logs")
	}

	// Verify username appears but password is masked
	if !strings.Contains(logs, "testuser") {
		t.Fatal("expected username in logs")
	}
}

func TestStatusPolling(t *testing.T) {
	gin.SetMode(gin.TestMode)

	// Create a mock server that returns different statuses
	statusCallCount := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")

		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/target":
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{
				"target_iqn": "iqn.test",
				"status":     "created",
			})
		case r.Method == http.MethodPost && strings.Contains(r.URL.Path, "/disk"):
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "attached"})
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/api/target/"):
			statusCallCount++
			w.WriteHeader(http.StatusOK)
			// Return provisioning first, then ready
			if statusCallCount <= 1 {
				json.NewEncoder(w).Encode(map[string]string{
					"target_iqn": "iqn.test",
					"status":     "provisioning",
				})
			} else {
				json.NewEncoder(w).Encode(map[string]string{
					"target_iqn": "iqn.test",
					"status":     "ready",
				})
			}
		case r.Method == http.MethodDelete:
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{"status": "deleted"})
		default:
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer mockServer.Close()

	gw := iscsi.New(iscsi.Config{
		CephISCSIEndpoint: mockServer.URL,
		Logger:            log.New(io.Discard, "", 0),
	})

	r := gin.New()
	r.POST("/targets", gw.CreateTarget)
	r.GET("/targets/:targetId", gw.GetTarget)

	// Create target
	body := `{"name":"vol1","tenant":"acme","rbdImage":"rbd/nest-acme-vol1"}`
	req := httptest.NewRequest(http.MethodPost, "/targets", bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	var created map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &created)
	id := created["id"].(string)

	// Initial status should be provisioning
	if created["status"] != "provisioning" {
		t.Fatalf("expected initial status provisioning, got %v", created["status"])
	}

	// Poll a few times - status should eventually become active
	// First poll should have already happened in the background
	for i := 0; i < 20; i++ {
		req := httptest.NewRequest(http.MethodGet, "/targets/"+id, nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		var result map[string]interface{}
		json.Unmarshal(w.Body.Bytes(), &result)
		status := result["status"].(string)

		// Should eventually transition to active
		if status == "active" {
			return
		}

		time.Sleep(200 * time.Millisecond)
	}

	t.Fatal("status never transitioned to active")
}
