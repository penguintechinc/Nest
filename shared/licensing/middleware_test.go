package licensing

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
)

// Note: Tests that require actual network/database connections use mocked responses via httptest.NewServer.

// TestNewFeatureGate tests creating a new feature gate
func TestNewFeatureGate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/validate" {
			w.Header().Set("Content-Type", "application/json")
			resp := ValidationResponse{
				Valid:   true,
				Product: "testproduct",
				Features: []Feature{
					{Name: "feature1", Entitled: true},
					{Name: "feature2", Entitled: false},
				},
				Metadata: Metadata{ServerID: "srv_test"},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := NewClient("PENG-1234-5678-90AB-CDEF-TEST", "testproduct")
	client.BaseURL = server.URL

	fg := NewFeatureGate(client)

	if fg == nil {
		t.Errorf("Expected FeatureGate to be created, got nil")
	}
	if fg.client != client {
		t.Errorf("Expected client to be set in FeatureGate")
	}
	if len(fg.features) == 0 {
		t.Errorf("Expected features to be populated during initialization")
	}
	if !fg.features["feature1"] {
		t.Errorf("Expected feature1 to be enabled")
	}
	if fg.features["feature2"] {
		t.Errorf("Expected feature2 to be disabled")
	}
}

// TestHasFeatureCached tests HasFeature with cached feature
func TestHasFeatureCached(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/validate" {
			w.Header().Set("Content-Type", "application/json")
			resp := ValidationResponse{
				Valid:   true,
				Product: "testproduct",
				Features: []Feature{
					{Name: "cached_feature", Entitled: true},
				},
				Metadata: Metadata{ServerID: "srv_test"},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := NewClient("PENG-1234-5678-90AB-CDEF-TEST", "testproduct")
	client.BaseURL = server.URL

	fg := NewFeatureGate(client)

	// Feature should be in cache from initialization
	has := fg.HasFeature("cached_feature")
	if !has {
		t.Errorf("Expected cached feature to be available, got false")
	}
}

// TestHasFeatureNotCached tests HasFeature with feature not in cache
func TestHasFeatureNotCached(t *testing.T) {
	validateCalled := 0
	featureCalled := 0

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/api/v2/validate" {
			validateCalled++
			resp := ValidationResponse{
				Valid:    true,
				Product:  "testproduct",
				Features: []Feature{},
				Metadata: Metadata{ServerID: "srv_test"},
			}
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/api/v2/features" {
			featureCalled++
			resp := FeatureResponse{
				Valid:    true,
				Product:  "testproduct",
				Features: []Feature{{Name: "new_feature", Entitled: true}},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := NewClient("PENG-1234-5678-90AB-CDEF-TEST", "testproduct")
	client.BaseURL = server.URL

	fg := NewFeatureGate(client)

	// Request feature not in initial cache
	has := fg.HasFeature("new_feature")
	if !has {
		t.Errorf("Expected new feature to be available, got false")
	}
	if featureCalled == 0 {
		t.Errorf("Expected CheckFeature to be called for uncached feature")
	}

	// Check that it's now cached
	has2 := fg.HasFeature("new_feature")
	if !has2 {
		t.Errorf("Expected cached new feature to still be available")
	}
}

// TestHasFeatureError tests HasFeature when CheckFeature returns error
func TestHasFeatureError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/validate" {
			w.Header().Set("Content-Type", "application/json")
			resp := ValidationResponse{
				Valid:    true,
				Product:  "testproduct",
				Features: []Feature{},
				Metadata: Metadata{ServerID: "srv_test"},
			}
			json.NewEncoder(w).Encode(resp)
		} else if r.URL.Path == "/api/v2/features" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("error"))
		}
	}))
	defer server.Close()

	client := NewClient("PENG-1234-5678-90AB-CDEF-TEST", "testproduct")
	client.BaseURL = server.URL

	fg := NewFeatureGate(client)

	// Request feature that will trigger CheckFeature error
	has := fg.HasFeature("error_feature")
	if has {
		t.Errorf("Expected feature check to fail and return false, got true")
	}
}

// TestFeatureNotAvailableError tests FeatureNotAvailableError
func TestFeatureNotAvailableError(t *testing.T) {
	err := FeatureNotAvailableError{Feature: "premium"}
	expectedMsg := "feature 'premium' requires license upgrade"

	if err.Error() != expectedMsg {
		t.Errorf("Expected error message '%s', got '%s'", expectedMsg, err.Error())
	}
}

// TestRequireFeatureFunc tests RequireFeatureFunc returns proper function
func TestRequireFeatureFunc(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/validate" {
			w.Header().Set("Content-Type", "application/json")
			resp := ValidationResponse{
				Valid:   true,
				Product: "testproduct",
				Features: []Feature{
					{Name: "premium", Entitled: true},
					{Name: "basic", Entitled: false},
				},
				Metadata: Metadata{ServerID: "srv_test"},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := NewClient("PENG-1234-5678-90AB-CDEF-TEST", "testproduct")
	client.BaseURL = server.URL

	fg := NewFeatureGate(client)

	// Test enabled feature
	requirePremium := RequireFeatureFunc(fg, "premium")
	err := requirePremium()
	if err != nil {
		t.Errorf("Expected no error for enabled feature, got %v", err)
	}

	// Test disabled feature
	requireBasic := RequireFeatureFunc(fg, "basic")
	err = requireBasic()
	if err == nil {
		t.Errorf("Expected error for disabled feature, got nil")
	}
	if err.Error() != "feature 'basic' requires license upgrade" {
		t.Errorf("Expected specific error message, got '%s'", err.Error())
	}
}

// TestLicenseMiddleware tests the LicenseMiddleware
func TestLicenseMiddleware(t *testing.T) {
	client := NewClient("PENG-1234-5678-90AB-CDEF-TEST", "test")

	middleware := LicenseMiddleware(client)

	engine := gin.New()
	engine.Use(middleware)
	engine.GET("/test", func(c *gin.Context) {
		// Check that client is set in context
		if _, exists := c.Get("license_client"); !exists {
			t.Errorf("Expected license_client in context")
		}

		// Check that feature gate is set in context
		if _, exists := c.Get("feature_gate"); !exists {
			t.Errorf("Expected feature_gate in context")
		}

		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200, got %d", w.Code)
	}
}

// TestRequireFeatureMiddleware tests the RequireFeature middleware
func TestRequireFeatureMiddleware(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/validate" {
			w.Header().Set("Content-Type", "application/json")
			resp := ValidationResponse{
				Valid:   true,
				Product: "testproduct",
				Features: []Feature{
					{Name: "premium", Entitled: true},
				},
				Metadata: Metadata{ServerID: "srv_test"},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := NewClient("PENG-1234-5678-90AB-CDEF-TEST", "testproduct")
	client.BaseURL = server.URL

	fg := NewFeatureGate(client)

	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(fg.RequireFeature("premium"))
	engine.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("Expected status 200 for enabled feature, got %d", w.Code)
	}
}

// TestRequireFeatureMiddlewareFeatureDisabled tests RequireFeature when feature is disabled
func TestRequireFeatureMiddlewareFeatureDisabled(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/validate" {
			w.Header().Set("Content-Type", "application/json")
			resp := ValidationResponse{
				Valid:   true,
				Product: "testproduct",
				Features: []Feature{
					{Name: "premium", Entitled: false},
				},
				Metadata: Metadata{ServerID: "srv_test"},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := NewClient("PENG-1234-5678-90AB-CDEF-TEST", "testproduct")
	client.BaseURL = server.URL

	fg := NewFeatureGate(client)

	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.Use(fg.RequireFeature("premium"))
	engine.GET("/protected", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/protected", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("Expected status 403 for disabled feature, got %d", w.Code)
	}
}

// TestGetAllFeatures tests GetAllFeatures returns copy of features
func TestGetAllFeatures(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/validate" {
			w.Header().Set("Content-Type", "application/json")
			resp := ValidationResponse{
				Valid:   true,
				Product: "testproduct",
				Features: []Feature{
					{Name: "feature1", Entitled: true},
					{Name: "feature2", Entitled: false},
				},
				Metadata: Metadata{ServerID: "srv_test"},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := NewClient("PENG-1234-5678-90AB-CDEF-TEST", "testproduct")
	client.BaseURL = server.URL

	fg := NewFeatureGate(client)

	features := fg.GetAllFeatures()

	if len(features) != 2 {
		t.Errorf("Expected 2 features, got %d", len(features))
	}
	if !features["feature1"] {
		t.Errorf("Expected feature1 to be enabled")
	}
	if features["feature2"] {
		t.Errorf("Expected feature2 to be disabled")
	}

	// Verify it's a copy by modifying it
	features["feature1"] = false
	// Re-fetch and verify original is unchanged
	features2 := fg.GetAllFeatures()
	if !features2["feature1"] {
		t.Errorf("Expected feature1 to still be enabled in original cache")
	}
}

// TestGetFeatureGate tests GetFeatureGate extracts from context
func TestGetFeatureGate(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/validate" {
			w.Header().Set("Content-Type", "application/json")
			resp := ValidationResponse{
				Valid:   true,
				Product: "testproduct",
				Features: []Feature{},
				Metadata: Metadata{ServerID: "srv_test"},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := NewClient("PENG-1234-5678-90AB-CDEF-TEST", "testproduct")
	client.BaseURL = server.URL

	fg := NewFeatureGate(client)

	gin.SetMode(gin.ReleaseMode)
	engine := gin.New()
	engine.GET("/test", func(c *gin.Context) {
		c.Set("feature_gate", fg)
		retrievedGate, err := GetFeatureGate(c)
		if err != nil {
			t.Errorf("Expected no error, got %v", err)
		}
		if retrievedGate != fg {
			t.Errorf("Expected same feature gate instance")
		}
		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
}

// TestGetFeatureGateNotFound tests GetFeatureGate when not in context
func TestGetFeatureGateNotFound(t *testing.T) {
	engine := gin.New()
	engine.GET("/test", func(c *gin.Context) {
		_, err := GetFeatureGate(c)

		if err == nil {
			t.Errorf("Expected error when feature gate not in context, got nil")
		}
		if err.Error() != "feature gate not found in context" {
			t.Errorf("Expected specific error message, got '%s'", err.Error())
		}

		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
}

// TestGetFeatureGateWrongType tests GetFeatureGate with wrong type in context
func TestGetFeatureGateWrongType(t *testing.T) {
	engine := gin.New()
	engine.GET("/test", func(c *gin.Context) {
		c.Set("feature_gate", "not a feature gate")

		_, err := GetFeatureGate(c)

		if err == nil {
			t.Errorf("Expected error for wrong type, got nil")
		}
		if err.Error() != "invalid feature gate type in context" {
			t.Errorf("Expected specific error message, got '%s'", err.Error())
		}

		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
}

// TestGetLicenseClient tests GetLicenseClient extracts from context
func TestGetLicenseClient(t *testing.T) {
	client := NewClient("PENG-1234-5678-90AB-CDEF-TEST", "test")

	engine := gin.New()
	engine.GET("/test", func(c *gin.Context) {
		c.Set("license_client", client)

		retrievedClient, err := GetLicenseClient(c)

		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
		if retrievedClient != client {
			t.Errorf("Expected same client instance")
		}

		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
}

// TestGetLicenseClientNotFound tests GetLicenseClient when not in context
func TestGetLicenseClientNotFound(t *testing.T) {
	engine := gin.New()
	engine.GET("/test", func(c *gin.Context) {
		_, err := GetLicenseClient(c)

		if err == nil {
			t.Errorf("Expected error when client not in context, got nil")
		}
		if err.Error() != "license client not found in context" {
			t.Errorf("Expected specific error message, got '%s'", err.Error())
		}

		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
}

// TestGetLicenseClientWrongType tests GetLicenseClient with wrong type in context
func TestGetLicenseClientWrongType(t *testing.T) {
	engine := gin.New()
	engine.GET("/test", func(c *gin.Context) {
		c.Set("license_client", "not a client")

		_, err := GetLicenseClient(c)

		if err == nil {
			t.Errorf("Expected error for wrong type, got nil")
		}
		if err.Error() != "invalid license client type in context" {
			t.Errorf("Expected specific error message, got '%s'", err.Error())
		}

		c.JSON(http.StatusOK, gin.H{"message": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, req)
}

// TestCacheRefresh tests that cache is refreshed when TTL expires
func TestCacheRefresh(t *testing.T) {
	callCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/validate" {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			resp := ValidationResponse{
				Valid:   true,
				Product: "testproduct",
				Features: []Feature{
					{Name: "feature1", Entitled: callCount < 2},
				},
				Metadata: Metadata{ServerID: "srv_test"},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := NewClient("PENG-1234-5678-90AB-CDEF-TEST", "testproduct")
	client.BaseURL = server.URL

	fg := NewFeatureGate(client)
	fg.cacheTTL = 100 * time.Millisecond

	// First call should cache feature1 as enabled
	has1 := fg.HasFeature("feature1")
	if !has1 {
		t.Errorf("Expected feature1 to be enabled on first call")
	}

	// Wait for cache to expire
	time.Sleep(150 * time.Millisecond)

	// Second call should refresh cache (feature1 now disabled)
	has2 := fg.HasFeature("feature1")
	if has2 {
		t.Errorf("Expected feature1 to be disabled after cache refresh")
	}

	if callCount < 2 {
		t.Errorf("Expected Validate to be called at least twice")
	}
}

// TestConcurrentFeatureChecks tests concurrent access to feature gate
func TestConcurrentFeatureChecks(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/validate" {
			w.Header().Set("Content-Type", "application/json")
			resp := ValidationResponse{
				Valid:   true,
				Product: "testproduct",
				Features: []Feature{
					{Name: "feature1", Entitled: true},
					{Name: "feature2", Entitled: false},
				},
				Metadata: Metadata{ServerID: "srv_test"},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := NewClient("PENG-1234-5678-90AB-CDEF-TEST", "testproduct")
	client.BaseURL = server.URL

	fg := NewFeatureGate(client)

	// Perform concurrent reads
	done := make(chan bool, 10)
	for i := 0; i < 10; i++ {
		go func(featureName string) {
			_ = fg.HasFeature(featureName)
			done <- true
		}("feature1")
	}

	for i := 0; i < 10; i++ {
		<-done
	}

	// Verify data integrity
	features := fg.GetAllFeatures()
	if len(features) == 0 {
		t.Errorf("Expected features to be populated after concurrent access")
	}
}

// TestRefreshFeaturesValidationFails tests refreshFeatures when validation fails
func TestRefreshFeaturesValidationFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/validate" {
			w.WriteHeader(http.StatusInternalServerError)
			w.Write([]byte("error"))
		}
	}))
	defer server.Close()

	client := NewClient("PENG-1234-5678-90AB-CDEF-TEST", "testproduct")
	client.BaseURL = server.URL

	fg := NewFeatureGate(client)

	// Features should be empty when validation fails
	features := fg.GetAllFeatures()
	if len(features) != 0 {
		t.Errorf("Expected no features when validation fails, got %d", len(features))
	}
}

// TestRefreshFeaturesInvalidLicense tests refreshFeatures when license is invalid
func TestRefreshFeaturesInvalidLicense(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/v2/validate" {
			w.Header().Set("Content-Type", "application/json")
			resp := ValidationResponse{
				Valid:    false,
				Message:  "License not valid",
				Product:  "testproduct",
				Metadata: Metadata{ServerID: "srv_test"},
			}
			json.NewEncoder(w).Encode(resp)
		}
	}))
	defer server.Close()

	client := NewClient("PENG-1234-5678-90AB-CDEF-TEST", "testproduct")
	client.BaseURL = server.URL

	fg := NewFeatureGate(client)

	// Features should be empty when license is invalid
	features := fg.GetAllFeatures()
	if len(features) != 0 {
		t.Errorf("Expected no features when license is invalid, got %d", len(features))
	}
}
