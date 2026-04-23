package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func init() {
	// Set Gin to test mode
	gin.SetMode(gin.TestMode)
}

// TestTenantMiddlewareValidToken tests valid token passes through
func TestTenantMiddlewareValidToken(t *testing.T) {
	router := gin.New()
	router.Use(TenantMiddleware())

	// Track if Next() was called
	var nextCalled bool
	router.GET("/test", func(c *gin.Context) {
		nextCalled = true
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer sub:tenant-a:admin")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if !nextCalled {
		t.Error("expected Next() to be called, but it wasn't")
	}
}

// TestTenantMiddlewareMissingHeader tests missing Authorization header
func TestTenantMiddlewareMissingHeader(t *testing.T) {
	router := gin.New()
	router.Use(TenantMiddleware())

	var nextCalled bool
	router.GET("/test", func(c *gin.Context) {
		nextCalled = true
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	// No Authorization header

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}

	if nextCalled {
		t.Error("expected Next() not to be called")
	}

	// Verify error response structure
	if !strings.Contains(w.Body.String(), "nest.auth.missing_token") {
		t.Errorf("expected error code nest.auth.missing_token in response, got: %s", w.Body.String())
	}
}

// TestTenantMiddlewareMissingBearerPrefix tests malformed Authorization header
func TestTenantMiddlewareMissingBearerPrefix(t *testing.T) {
	router := gin.New()
	router.Use(TenantMiddleware())

	var nextCalled bool
	router.GET("/test", func(c *gin.Context) {
		nextCalled = true
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz") // Not Bearer

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}

	if nextCalled {
		t.Error("expected Next() not to be called")
	}
}

// TestTenantMiddlewareMissingTenant tests token without tenant claim
func TestTenantMiddlewareMissingTenant(t *testing.T) {
	router := gin.New()
	router.Use(TenantMiddleware())

	var nextCalled bool
	router.GET("/test", func(c *gin.Context) {
		nextCalled = true
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer subject-only") // No tenant

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}

	if nextCalled {
		t.Error("expected Next() not to be called")
	}

	// Verify error response structure
	if !strings.Contains(w.Body.String(), "nest.auth.missing_tenant") {
		t.Errorf("expected error code nest.auth.missing_tenant in response, got: %s", w.Body.String())
	}
}

// TestTenantMiddlewareSetContext tests that context is properly set
func TestTenantMiddlewareSetContext(t *testing.T) {
	router := gin.New()
	router.Use(TenantMiddleware())

	var capturedTenant string
	var capturedClaims *Claims

	router.GET("/test", func(c *gin.Context) {
		capturedTenant = GetTenant(c)
		capturedClaims = GetClaims(c)
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer subject-123:customer-tenant:admin")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if capturedTenant != "customer-tenant" {
		t.Errorf("expected tenant 'customer-tenant', got '%s'", capturedTenant)
	}

	if capturedClaims == nil {
		t.Fatal("expected claims to be set, got nil")
	}

	if capturedClaims.Tenant != "customer-tenant" {
		t.Errorf("expected claims.Tenant 'customer-tenant', got '%s'", capturedClaims.Tenant)
	}

	if capturedClaims.Sub != "subject-123" {
		t.Errorf("expected claims.Sub 'subject-123', got '%s'", capturedClaims.Sub)
	}
}

// TestGetTenantFromContext tests GetTenant extraction
func TestGetTenantFromContext(t *testing.T) {
	c := &gin.Context{}
	c.Set(TenantKey, "test-tenant")

	tenant := GetTenant(c)
	if tenant != "test-tenant" {
		t.Errorf("expected 'test-tenant', got '%s'", tenant)
	}
}

// TestGetTenantFromContextEmpty tests GetTenant with no value set
func TestGetTenantFromContextEmpty(t *testing.T) {
	c := &gin.Context{}

	tenant := GetTenant(c)
	if tenant != "" {
		t.Errorf("expected empty string, got '%s'", tenant)
	}
}

// TestGetClaimsFromContext tests GetClaims extraction
func TestGetClaimsFromContext(t *testing.T) {
	c := &gin.Context{}
	claims := &Claims{
		Sub:    "user-123",
		Tenant: "tenant-a",
		Scopes: []string{"read", "write"},
	}
	c.Set(ClaimsKey, claims)

	retrieved := GetClaims(c)
	if retrieved == nil {
		t.Fatal("expected claims, got nil")
	}

	if retrieved.Sub != "user-123" {
		t.Errorf("expected Sub 'user-123', got '%s'", retrieved.Sub)
	}

	if retrieved.Tenant != "tenant-a" {
		t.Errorf("expected Tenant 'tenant-a', got '%s'", retrieved.Tenant)
	}
}

// TestGetClaimsFromContextEmpty tests GetClaims with no value set
func TestGetClaimsFromContextEmpty(t *testing.T) {
	c := &gin.Context{}

	claims := GetClaims(c)
	if claims != nil {
		t.Errorf("expected nil, got %v", claims)
	}
}

// TestRequireScopeValidScope tests RequireScope with valid scope
func TestRequireScopeValidScope(t *testing.T) {
	router := gin.New()
	router.Use(TenantMiddleware())
	router.Use(RequireScope("read"))

	var nextCalled bool
	router.GET("/test", func(c *gin.Context) {
		nextCalled = true
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer subject:tenant:admin")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if !nextCalled {
		t.Error("expected Next() to be called")
	}
}

// TestRequireScopeAdminWildcard tests RequireScope with admin wildcard
func TestRequireScopeAdminWildcard(t *testing.T) {
	router := gin.New()
	router.Use(TenantMiddleware())
	router.Use(RequireScope("restricted-scope"))

	var nextCalled bool
	router.GET("/test", func(c *gin.Context) {
		nextCalled = true
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	// Token with admin wildcard should pass any scope check
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer subject:tenant:admin")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	if !nextCalled {
		t.Error("expected Next() to be called with admin scope")
	}
}

// TestRequireScopeMissingScope tests RequireScope when token lacks scope
func TestRequireScopeMissingScope(t *testing.T) {
	router := gin.New()

	// Set up endpoint with scope requirement
	router.GET("/test-scope-missing",
		func(c *gin.Context) {
			// Inject claims with limited scope
			claims := &Claims{
				Sub:    "user",
				Tenant: "tenant-a",
				Scopes: []string{"read"},
			}
			c.Set(ClaimsKey, claims)
			c.Next()
		},
		RequireScope("admin"),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		},
	)

	req := httptest.NewRequest("GET", "/test-scope-missing", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}

	if !strings.Contains(w.Body.String(), "nest.auth.scope_denied") {
		t.Errorf("expected error code nest.auth.scope_denied in response, got: %s", w.Body.String())
	}
}

// TestRequireScopeMissingClaims tests RequireScope when claims are nil
func TestRequireScopeMissingClaims(t *testing.T) {
	router := gin.New()

	router.GET("/test-scope-missing-claims",
		RequireScope("read"),
		func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"status": "ok"})
		},
	)

	req := httptest.NewRequest("GET", "/test-scope-missing-claims", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}

// TestParseTokenValid tests parseToken with valid format
func TestParseTokenValid(t *testing.T) {
	tests := []struct {
		name        string
		token       string
		expectedSub string
		expectedTen string
		expectedTier string
	}{
		{
			name:        "subject and tenant",
			token:       "user-123:tenant-a",
			expectedSub: "user-123",
			expectedTen: "tenant-a",
			expectedTier: "free",
		},
		{
			name:        "subject, tenant, and tier",
			token:       "user-456:tenant-b:premium",
			expectedSub: "user-456",
			expectedTen: "tenant-b",
			expectedTier: "premium",
		},
		{
			name:        "subject only",
			token:       "user-789",
			expectedSub: "user-789",
			expectedTen: "",
			expectedTier: "free",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims, err := parseToken(tt.token)
			if err != nil {
				t.Errorf("parseToken failed: %v", err)
			}

			if claims.Sub != tt.expectedSub {
				t.Errorf("expected Sub %q, got %q", tt.expectedSub, claims.Sub)
			}

			if claims.Tenant != tt.expectedTen {
				t.Errorf("expected Tenant %q, got %q", tt.expectedTen, claims.Tenant)
			}

			if claims.Tier != tt.expectedTier {
				t.Errorf("expected Tier %q, got %q", tt.expectedTier, claims.Tier)
			}

			// P1 always grants admin scope
			if len(claims.Scopes) != 1 || claims.Scopes[0] != "nest:*:admin" {
				t.Errorf("expected admin scope, got %v", claims.Scopes)
			}
		})
	}
}

// TestParseTokenEmpty tests parseToken with empty token
func TestParseTokenEmpty(t *testing.T) {
	_, err := parseToken("")
	if err == nil {
		t.Error("expected error for empty token, got nil")
	}
}

// TestNestError tests nestError response format
func TestNestError(t *testing.T) {
	result := nestError("test.error.code", "Test message", "req-12345")

	code, ok := result["code"]
	if !ok || code != "test.error.code" {
		t.Errorf("expected code 'test.error.code', got %v", code)
	}

	message, ok := result["message"]
	if !ok || message != "Test message" {
		t.Errorf("expected message 'Test message', got %v", message)
	}

	requestId, ok := result["requestId"]
	if !ok || requestId != "req-12345" {
		t.Errorf("expected requestId 'req-12345', got %v", requestId)
	}

	docsUrl, ok := result["docsUrl"]
	if !ok {
		t.Error("expected docsUrl field")
	}
	if !strings.Contains(docsUrl.(string), "test.error.code") {
		t.Errorf("expected docsUrl to contain error code, got %s", docsUrl)
	}
}

// TestTenantMiddlewareEmptyToken tests token with only Bearer prefix
func TestTenantMiddlewareEmptyToken(t *testing.T) {
	router := gin.New()
	router.Use(TenantMiddleware())

	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer ")

	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	// Should fail with missing tenant error (not missing token)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}

// TestIntegrationFullFlow tests complete auth flow with multiple middleware
func TestIntegrationFullFlow(t *testing.T) {
	router := gin.New()
	router.Use(TenantMiddleware())

	// Public endpoint (no scope check)
	router.GET("/public", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"tenant": GetTenant(c)})
	})

	// Protected endpoint (requires read scope)
	router.GET("/protected", RequireScope("read"), func(c *gin.Context) {
		claims := GetClaims(c)
		c.JSON(http.StatusOK, gin.H{
			"tenant": GetTenant(c),
			"sub":    claims.Sub,
			"scopes": claims.Scopes,
		})
	})

	t.Run("public endpoint with valid token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/public", nil)
		req.Header.Set("Authorization", "Bearer user:tenant-a")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		if !strings.Contains(w.Body.String(), "tenant-a") {
			t.Errorf("expected tenant-a in response, got: %s", w.Body.String())
		}
	})

	t.Run("protected endpoint with valid token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/protected", nil)
		req.Header.Set("Authorization", "Bearer user:tenant-a")

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected status 200, got %d", w.Code)
		}

		if !strings.Contains(w.Body.String(), "nest:*:admin") {
			t.Errorf("expected admin scope in response, got: %s", w.Body.String())
		}
	})

	t.Run("public endpoint without token", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/public", nil)

		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if w.Code != http.StatusUnauthorized {
			t.Errorf("expected status 401, got %d", w.Code)
		}
	})
}

// BenchmarkTenantMiddleware benchmarks middleware overhead
func BenchmarkTenantMiddleware(b *testing.B) {
	router := gin.New()
	router.Use(TenantMiddleware())

	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer user:tenant:admin")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}

// BenchmarkRequireScope benchmarks scope checking overhead
func BenchmarkRequireScope(b *testing.B) {
	router := gin.New()
	router.Use(TenantMiddleware())
	router.Use(RequireScope("read"))

	router.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})

	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer user:tenant:admin")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)
	}
}
