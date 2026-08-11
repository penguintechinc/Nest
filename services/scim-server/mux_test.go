package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func newTestLogger() *zap.Logger {
	logger, _ := zap.NewDevelopment()
	return logger
}

func TestSCIMRoutes(t *testing.T) {
	t.Setenv("DB_TYPE", "sqlite")
	t.Setenv("DB_NAME", ":memory:")
	t.Setenv("OIDC_JWKS_URL", "test")
	t.Setenv("ENTERPRISE_LICENSE", "test-license")
	authHdr := "Bearer test-tenant:admin"

	logger := newTestLogger()
	dal := getTestDAL()
	store := NewSCIMStore(dal, logger)
	mux := NewMux(store, logger)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	t.Run("GET /healthz", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/healthz")
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /scim/v2/Users - success", func(t *testing.T) {
		user := map[string]interface{}{
			"userName": "alice",
			"active":   true,
		}
		body, _ := json.Marshal(user)
		req, _ := http.NewRequest("POST", srv.URL+"/scim/v2/Users", bytes.NewReader(body))
		req.Header.Set("Authorization", authHdr)
		req.Header.Set("Content-Type", "application/scim+json")
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusCreated {
			t.Errorf("expected 201, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /scim/v2/Users - success", func(t *testing.T) {
		req, _ := http.NewRequest("GET", srv.URL+"/scim/v2/Users", nil)
		req.Header.Set("Authorization", authHdr)
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})
}
