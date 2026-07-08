package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/gateway/internal/claims"
)

func TestScopeEnforcementMiddleware_ReadScopeAllowsGet(t *testing.T) {
	logger := zap.NewNop()

	// Handler that always returns 200
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := ScopeEnforcementMiddleware(logger, innerHandler)

	// Create a request with read scope
	req := httptest.NewRequest(http.MethodGet, "/api/v1/databases", nil)
	cl := &claims.Claims{
		Subject:  "user1",
		Tenant:   "tenant1",
		Scopes:   []string{"databases:read"},
		Roles:    []string{"viewer"},
		IssuedAt: 1234567890,
		Expires:  1234567890 + 3600,
	}
	req = req.WithContext(claims.WithClaims(context.Background(), cl))

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestScopeEnforcementMiddleware_ReadScopeDeniesPost(t *testing.T) {
	logger := zap.NewNop()

	// Handler that always returns 200
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := ScopeEnforcementMiddleware(logger, innerHandler)

	// Create a POST request with only read scope (viewer token)
	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases", nil)
	cl := &claims.Claims{
		Subject:  "user1",
		Tenant:   "tenant1",
		Scopes:   []string{"databases:read"}, // Only read scope
		Roles:    []string{"viewer"},
		IssuedAt: 1234567890,
		Expires:  1234567890 + 3600,
	}
	req = req.WithContext(claims.WithClaims(context.Background(), cl))

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}

func TestScopeEnforcementMiddleware_WriteScopeAllowsPost(t *testing.T) {
	logger := zap.NewNop()

	// Handler that always returns 200
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := ScopeEnforcementMiddleware(logger, innerHandler)

	// Create a POST request with write scope
	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases", nil)
	cl := &claims.Claims{
		Subject:  "user1",
		Tenant:   "tenant1",
		Scopes:   []string{"databases:write"}, // Write scope
		Roles:    []string{"maintainer"},
		IssuedAt: 1234567890,
		Expires:  1234567890 + 3600,
	}
	req = req.WithContext(claims.WithClaims(context.Background(), cl))

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestScopeEnforcementMiddleware_AdminScopeAllowsDelete(t *testing.T) {
	logger := zap.NewNop()

	// Handler that always returns 200
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := ScopeEnforcementMiddleware(logger, innerHandler)

	// Create a DELETE request with admin scope
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/databases/testdb", nil)
	cl := &claims.Claims{
		Subject:  "user1",
		Tenant:   "tenant1",
		Scopes:   []string{"databases:admin"}, // Admin scope
		Roles:    []string{"admin"},
		IssuedAt: 1234567890,
		Expires:  1234567890 + 3600,
	}
	req = req.WithContext(claims.WithClaims(context.Background(), cl))

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestScopeEnforcementMiddleware_WriteScopeDeniesDelete(t *testing.T) {
	logger := zap.NewNop()

	// Handler that always returns 200
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := ScopeEnforcementMiddleware(logger, innerHandler)

	// Create a DELETE request with only write scope
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/databases/testdb", nil)
	cl := &claims.Claims{
		Subject:  "user1",
		Tenant:   "tenant1",
		Scopes:   []string{"databases:write"}, // Only write scope, not admin
		Roles:    []string{"maintainer"},
		IssuedAt: 1234567890,
		Expires:  1234567890 + 3600,
	}
	req = req.WithContext(claims.WithClaims(context.Background(), cl))

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403, got %d", w.Code)
	}
}

func TestScopeEnforcementMiddleware_AdminActionPermitsAllActions(t *testing.T) {
	logger := zap.NewNop()

	// Handler that always returns 200
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := ScopeEnforcementMiddleware(logger, innerHandler)

	tests := []struct {
		name   string
		method string
	}{
		{"GET with admin scope", http.MethodGet},
		{"POST with admin scope", http.MethodPost},
		{"DELETE with admin scope", http.MethodDelete},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, "/api/v1/databases", nil)
			cl := &claims.Claims{
				Subject:  "user1",
				Tenant:   "tenant1",
				Scopes:   []string{"databases:admin"}, // Admin can do anything
				Roles:    []string{"admin"},
				IssuedAt: 1234567890,
				Expires:  1234567890 + 3600,
			}
			req = req.WithContext(claims.WithClaims(context.Background(), cl))

			w := httptest.NewRecorder()
			middleware.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("expected status 200, got %d", w.Code)
			}
		})
	}
}

func TestScopeEnforcementMiddleware_MissingClaimsDenies(t *testing.T) {
	logger := zap.NewNop()

	// Handler that always returns 200
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := ScopeEnforcementMiddleware(logger, innerHandler)

	// Create a request without claims in context
	req := httptest.NewRequest(http.MethodGet, "/api/v1/databases", nil)

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected status 401, got %d", w.Code)
	}
}
