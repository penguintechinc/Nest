package main

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"github.com/penguintechinc/nest/pkg/auth"
)

func mintToken(secret, tenant, scope string) string {
	claims := jwt.MapClaims{
		"sub":    "test-user",
		"iss":    "test-issuer",
		"aud":    []string{"test-audience"},
		"iat":    time.Now().Unix(),
		"exp":    time.Now().Add(1 * time.Hour).Unix(),
		"tenant": tenant,
		"scope":  scope,
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, _ := token.SignedString([]byte(secret))
	return tokenString
}

func TestIntelligenceEngineRoutes(t *testing.T) {
	logger := slog.New(slog.NewTextHandler(os.Stderr, nil))
	classifier := NewClassifier()

	// Create test auth middleware
	testSecret := "test-secret-key-must-be-at-least-32-chars!!!!"
	authConfig := &auth.Config{
		Algorithm:    "HS256",
		SharedSecret: testSecret,
	}
	authMiddleware, err := auth.NewMiddleware(authConfig)
	if err != nil {
		t.Fatalf("failed to create auth middleware: %v", err)
	}

	srv := httptest.NewServer(NewMux(classifier, logger, authMiddleware))
	defer srv.Close()

	validToken := mintToken(testSecret, "test-tenant", "*:admin")

	doRequest := func(method, url string, body *bytes.Buffer, token string) *http.Response {
		var req *http.Request
		if body != nil {
			req, _ = http.NewRequest(method, url, body)
		} else {
			req, _ = http.NewRequest(method, url, nil)
		}
		if token != "" {
			req.Header.Set("Authorization", "Bearer "+token)
		}
		req.Header.Set("Content-Type", "application/json")
		resp, _ := http.DefaultClient.Do(req)
		return resp
	}

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

	t.Run("POST /api/v1/intelligence/classify - without auth", func(t *testing.T) {
		body, _ := json.Marshal(map[string]interface{}{
			"cpuUsage":    45.5,
			"memoryUsage": 60.2,
		})
		resp, err := http.Post(srv.URL+"/api/v1/intelligence/classify", "application/json", bytes.NewReader(body))
		if err != nil {
			t.Fatalf("request failed: %v", err)
		}
		defer resp.Body.Close()
		// Now requires auth
		if resp.StatusCode != http.StatusUnauthorized {
			t.Errorf("expected 401, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/intelligence/classify - with license", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		body, _ := json.Marshal(map[string]interface{}{
			"cpuUsage":    45.5,
			"memoryUsage": 60.2,
		})
		resp := doRequest("POST", srv.URL+"/api/v1/intelligence/classify", bytes.NewBuffer(body), validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/intelligence/classify - invalid request", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp := doRequest("POST", srv.URL+"/api/v1/intelligence/classify", bytes.NewBuffer([]byte("invalid")), validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Errorf("expected 400, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations - without license", func(t *testing.T) {
		os.Unsetenv("ENTERPRISE_LICENSE")
		os.Unsetenv("WADDLEAI_ENABLED")

		resp := doRequest("GET", srv.URL+"/api/v1/intelligence/recommendations", nil, validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations - with license", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp := doRequest("GET", srv.URL+"/api/v1/intelligence/recommendations", nil, validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if _, ok := result["recommendations"]; !ok {
			t.Error("expected recommendations in response")
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations - filter by tenant", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp := doRequest("GET", srv.URL+"/api/v1/intelligence/recommendations?tenant=test-tenant", nil, validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations/{resourceId} - without license", func(t *testing.T) {
		os.Unsetenv("ENTERPRISE_LICENSE")
		os.Unsetenv("WADDLEAI_ENABLED")

		resp := doRequest("GET", srv.URL+"/api/v1/intelligence/recommendations/res-1", nil, validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations/{resourceId} - with license", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp := doRequest("GET", srv.URL+"/api/v1/intelligence/recommendations/res-1", nil, validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNotFound {
			t.Errorf("expected 200 or 404, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/intelligence/classify - only ENTERPRISE_LICENSE set", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Unsetenv("WADDLEAI_ENABLED")
		defer os.Unsetenv("ENTERPRISE_LICENSE")

		body, _ := json.Marshal(map[string]interface{}{
			"cpuUsage":    45.5,
			"memoryUsage": 60.2,
		})
		resp := doRequest("POST", srv.URL+"/api/v1/intelligence/classify", bytes.NewBuffer(body), validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402 (WADDLEAI_ENABLED missing), got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/intelligence/classify - only WADDLEAI_ENABLED set", func(t *testing.T) {
		os.Unsetenv("ENTERPRISE_LICENSE")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		body, _ := json.Marshal(map[string]interface{}{
			"cpuUsage":    45.5,
			"memoryUsage": 60.2,
		})
		resp := doRequest("POST", srv.URL+"/api/v1/intelligence/classify", bytes.NewBuffer(body), validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402 (ENTERPRISE_LICENSE missing), got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations - only WADDLEAI_ENABLED set", func(t *testing.T) {
		os.Unsetenv("ENTERPRISE_LICENSE")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		resp := doRequest("GET", srv.URL+"/api/v1/intelligence/recommendations", nil, validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations/{resourceId} - only ENTERPRISE_LICENSE set", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Unsetenv("WADDLEAI_ENABLED")
		defer os.Unsetenv("ENTERPRISE_LICENSE")

		resp := doRequest("GET", srv.URL+"/api/v1/intelligence/recommendations/res-1", nil, validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusPaymentRequired {
			t.Errorf("expected 402, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/intelligence/classify - valid metrics", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		body, _ := json.Marshal(map[string]interface{}{
			"resourceId":   "test-res",
			"tenant":       "test-tenant",
			"readRps":      100.0,
			"writeRps":     50.0,
			"avgLatencyMs": 10.0,
			"p99LatencyMs": 50.0,
			"dataSizeGb":   1000.0,
			"scanRatio":    0.8,
		})
		resp := doRequest("POST", srv.URL+"/api/v1/intelligence/classify", bytes.NewBuffer(body), validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if _, ok := result["workloadType"]; !ok {
			t.Error("expected workloadType in response")
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations - with results", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		// First, classify something
		body, _ := json.Marshal(map[string]interface{}{
			"resourceId":   "test-res-2",
			"tenant":       "test-tenant",
			"readRps":      100.0,
			"writeRps":     50.0,
			"avgLatencyMs": 10.0,
			"p99LatencyMs": 50.0,
			"dataSizeGb":   1000.0,
			"scanRatio":    0.8,
		})
		doRequest("POST", srv.URL+"/api/v1/intelligence/classify", bytes.NewBuffer(body), validToken)

		// Then list
		resp := doRequest("GET", srv.URL+"/api/v1/intelligence/recommendations?tenant=all", nil, validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("POST /api/v1/intelligence/classify - OLTP classification", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		body, _ := json.Marshal(map[string]interface{}{
			"resourceId":   "oltp-res",
			"tenant":       "test-tenant",
			"readRps":      5000.0,
			"writeRps":     5000.0,
			"avgLatencyMs": 2.0,
			"p99LatencyMs": 2.0,
			"dataSizeGb":   100.0,
			"scanRatio":    0.1,
		})
		resp := doRequest("POST", srv.URL+"/api/v1/intelligence/classify", bytes.NewBuffer(body), validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
	})

	t.Run("GET /api/v1/intelligence/recommendations/{resourceId} - found resource", func(t *testing.T) {
		os.Setenv("ENTERPRISE_LICENSE", "test-key")
		os.Setenv("WADDLEAI_ENABLED", "true")
		defer os.Unsetenv("ENTERPRISE_LICENSE")
		defer os.Unsetenv("WADDLEAI_ENABLED")

		// First, classify something
		body, _ := json.Marshal(map[string]interface{}{
			"resourceId":   "found-res",
			"tenant":       "test-tenant",
			"readRps":      100.0,
			"writeRps":     50.0,
			"avgLatencyMs": 10.0,
			"p99LatencyMs": 50.0,
			"dataSizeGb":   1000.0,
			"scanRatio":    0.8,
		})
		doRequest("POST", srv.URL+"/api/v1/intelligence/classify", bytes.NewBuffer(body), validToken)

		// Then get specific resource
		resp := doRequest("GET", srv.URL+"/api/v1/intelligence/recommendations/found-res", nil, validToken)
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}

		var result map[string]interface{}
		json.NewDecoder(resp.Body).Decode(&result)
		if _, ok := result["resourceId"]; !ok {
			t.Error("expected resourceId in response")
		}
	})
}
