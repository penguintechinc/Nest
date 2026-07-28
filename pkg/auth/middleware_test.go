package auth

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func createValidToken(secret, tenant string) string {
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":    "user123",
		"exp":    now.Add(1 * time.Hour).Unix(),
		"tenant": tenant,
		"scope":  "admin:write policy:read",
	})
	tokenString, _ := token.SignedString([]byte(secret))
	return tokenString
}

func TestNewMiddleware_NoConfig(t *testing.T) {
	_, err := NewMiddleware(nil)
	if err == nil {
		t.Fatal("expected error for nil config")
	}
}

func TestNewMiddleware_NoSharedSecretOrJWKS(t *testing.T) {
	config := &Config{
		Algorithm: "HS256",
		// neither SharedSecret nor JWKSEndpoint
	}
	_, err := NewMiddleware(config)
	if err == nil {
		t.Fatal("expected error for missing auth config")
	}
}

func TestNewMiddleware_Valid(t *testing.T) {
	config := &Config{
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
	}
	_, err := NewMiddleware(config)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRequireAuth_ValidToken(t *testing.T) {
	config := &Config{
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
	}
	middleware, _ := NewMiddleware(config)

	tokenString := createValidToken("test-secret", "tenant-123")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromContext(r.Context())
		if claims == nil {
			http.Error(w, "no claims", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.RequireAuth(next)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireAuth_MissingToken(t *testing.T) {
	config := &Config{
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
	}
	middleware, _ := NewMiddleware(config)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.RequireAuth(next)

	req := httptest.NewRequest("GET", "/", nil)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireAuth_InvalidToken(t *testing.T) {
	config := &Config{
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
	}
	middleware, _ := NewMiddleware(config)

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	handler := middleware.RequireAuth(next)

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestRequireTenant_Present(t *testing.T) {
	config := &Config{
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
	}
	middleware, _ := NewMiddleware(config)

	tokenString := createValidToken("test-secret", "tenant-123")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tenant := TenantFromContext(r.Context())
		if tenant != "tenant-123" {
			http.Error(w, "tenant mismatch", http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	// Chain: RequireAuth -> RequireTenant -> next
	handler := middleware.RequireAuth(middleware.RequireTenant(next))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireScope_Present(t *testing.T) {
	config := &Config{
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
	}
	middleware, _ := NewMiddleware(config)

	tokenString := createValidToken("test-secret", "tenant-123")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Require a scope that's present in the token
	handler := middleware.RequireAuth(middleware.RequireScope("admin:write")(next))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireScope_Missing(t *testing.T) {
	config := &Config{
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
	}
	middleware, _ := NewMiddleware(config)

	tokenString := createValidToken("test-secret", "tenant-123")

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Require a scope that's NOT in the token
	handler := middleware.RequireAuth(middleware.RequireScope("delete:all")(next))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRequireScope_WildcardResource(t *testing.T) {
	config := &Config{
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
	}
	middleware, _ := NewMiddleware(config)

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":    "user123",
		"exp":    now.Add(1 * time.Hour).Unix(),
		"tenant": "tenant-123",
		"scope":  "*:write", // Wildcard resource
	})
	tokenString, _ := token.SignedString([]byte("test-secret"))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Require a specific resource:action; should be satisfied by *:write
	handler := middleware.RequireAuth(middleware.RequireScope("databases:write")(next))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireScope_AdminAction(t *testing.T) {
	config := &Config{
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
	}
	middleware, _ := NewMiddleware(config)

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":    "user123",
		"exp":    now.Add(1 * time.Hour).Unix(),
		"tenant": "tenant-123",
		"scope":  "databases:admin", // Admin action
	})
	tokenString, _ := token.SignedString([]byte("test-secret"))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Require a read/write action; should be satisfied by admin
	handler := middleware.RequireAuth(middleware.RequireScope("databases:read")(next))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRequireScope_WrongResource(t *testing.T) {
	config := &Config{
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
	}
	middleware, _ := NewMiddleware(config)

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":    "user123",
		"exp":    now.Add(1 * time.Hour).Unix(),
		"tenant": "tenant-123",
		"scope":  "analytics:write", // Wrong resource
	})
	tokenString, _ := token.SignedString([]byte("test-secret"))

	next := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Require databases:write; should NOT be satisfied by analytics:write
	handler := middleware.RequireAuth(middleware.RequireScope("databases:write")(next))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer "+tokenString)
	w := httptest.NewRecorder()

	handler.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestAssertTenantMatch_Match(t *testing.T) {
	config := &Config{
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
	}
	middleware, _ := NewMiddleware(config)

	err := middleware.AssertTenantMatch("tenant-123", "tenant-123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestAssertTenantMatch_Mismatch(t *testing.T) {
	config := &Config{
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
	}
	middleware, _ := NewMiddleware(config)

	err := middleware.AssertTenantMatch("tenant-123", "tenant-456")
	if err == nil {
		t.Fatal("expected error for tenant mismatch")
	}
}
