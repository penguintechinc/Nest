package main

import (
	"bytes"
	"encoding/json"

	"net/http"
	"net/http/httptest"
	"os"
	"testing"

	"github.com/penguintechinc/nest/pkg/auth"
	"go.uber.org/zap"
)

func createTestAuthMiddleware() *auth.Middleware {
	authMiddleware, _ := auth.NewMiddleware(&auth.Config{
		Algorithm:       "HS256",
		AllowHS256Admin: true,
		SharedSecret:    "test-secret",
	})
	return authMiddleware
}

func TestAuditServiceRoutes(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	tmpDir := t.TempDir()
	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("DB_NAME", tmpDir+"/test.db")
	defer os.Unsetenv("DB_TYPE")
	defer os.Unsetenv("DB_NAME")

	auditLogger, err := NewAuditLogger(getTestDAL(), logger)
	if err != nil {
		t.Fatalf("failed to create audit logger: %v", err)
	}
	defer auditLogger.Close()

	authMiddleware := createTestAuthMiddleware()
	srv := httptest.NewServer(NewMux(auditLogger, "test-license", logger, authMiddleware))
	defer srv.Close()

	t.Run("GET /healthz - no license required", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/audit/events - without license", func(t *testing.T) {
		noLicenseSrv := httptest.NewServer(NewMux(auditLogger, "", logger, authMiddleware))
		defer noLicenseSrv.Close()

		body, _ := json.Marshal(map[string]interface{}{
			"tenant":   "test-tenant",
			"actor":    "user-1",
			"action":   "read",
			"resource": "dataset-1",
			"outcome":  "success",
		})
		resp, err := http.Post(noLicenseSrv.URL+"/api/v1/audit/events", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /notfound - unrecognized path", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/notfound")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 404, got %d", resp.StatusCode)
		}
	})
}
