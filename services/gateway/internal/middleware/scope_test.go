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

func TestScopeEnforcementMiddleware_CrossResourceEscalation_WriteDenied(t *testing.T) {
	logger := zap.NewNop()

	// Handler that always returns 200
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := ScopeEnforcementMiddleware(logger, innerHandler)

	// Create a POST to databases with only analytics:write scope
	// This should be DENIED because analytics:write doesn't grant write on databases
	req := httptest.NewRequest(http.MethodPost, "/api/v1/databases", nil)
	cl := &claims.Claims{
		Subject:  "user1",
		Tenant:   "tenant1",
		Scopes:   []string{"analytics:write"}, // write on analytics, not databases
		Roles:    []string{"maintainer"},
		IssuedAt: 1234567890,
		Expires:  1234567890 + 3600,
	}
	req = req.WithContext(claims.WithClaims(context.Background(), cl))

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403 (cross-resource escalation blocked), got %d", w.Code)
	}
}

func TestScopeEnforcementMiddleware_CrossResourceEscalation_AdminDenied(t *testing.T) {
	logger := zap.NewNop()

	// Handler that always returns 200
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := ScopeEnforcementMiddleware(logger, innerHandler)

	// Create a DELETE to databases with only analytics:admin scope
	// This should be DENIED because analytics:admin doesn't grant admin on databases
	req := httptest.NewRequest(http.MethodDelete, "/api/v1/databases/testdb", nil)
	cl := &claims.Claims{
		Subject:  "user1",
		Tenant:   "tenant1",
		Scopes:   []string{"analytics:admin"}, // admin on analytics, not databases
		Roles:    []string{"admin"},
		IssuedAt: 1234567890,
		Expires:  1234567890 + 3600,
	}
	req = req.WithContext(claims.WithClaims(context.Background(), cl))

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected status 403 (cross-resource escalation blocked), got %d", w.Code)
	}
}

func TestScopeEnforcementMiddleware_WildcardScopesWork(t *testing.T) {
	logger := zap.NewNop()

	// Handler that always returns 200
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := ScopeEnforcementMiddleware(logger, innerHandler)

	tests := []struct {
		name     string
		method   string
		path     string
		scopes   []string
		wantCode int
	}{
		// Wildcard scopes (standard bundles) should work on all resources
		{"wildcard:read on GET databases", http.MethodGet, "/api/v1/databases", []string{"*:read"}, http.StatusOK},
		{"wildcard:write on POST databases", http.MethodPost, "/api/v1/databases", []string{"*:write"}, http.StatusOK},
		{"wildcard:write on PUT objects", http.MethodPut, "/api/v1/objects/res/bucket/key", []string{"*:write"}, http.StatusOK},
		{"wildcard:admin on DELETE databases", http.MethodDelete, "/api/v1/databases/testdb", []string{"*:admin"}, http.StatusOK},

		// Standard bundle combinations (viewer, maintainer, admin)
		{"viewer bundle on GET", http.MethodGet, "/api/v1/databases", []string{"*:read"}, http.StatusOK},
		{"maintainer bundle on POST", http.MethodPost, "/api/v1/databases", []string{"*:read", "*:write"}, http.StatusOK},
		{"admin bundle on DELETE", http.MethodDelete, "/api/v1/databases/testdb", []string{"*:read", "*:write", "*:admin"}, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			cl := &claims.Claims{
				Subject:  "user1",
				Tenant:   "tenant1",
				Scopes:   tt.scopes,
				IssuedAt: 1234567890,
				Expires:  1234567890 + 3600,
			}
			req = req.WithContext(claims.WithClaims(context.Background(), cl))

			w := httptest.NewRecorder()
			middleware.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("expected status %d, got %d", tt.wantCode, w.Code)
			}
		})
	}
}

func TestScopeEnforcementMiddleware_ResourceSpecificScopes(t *testing.T) {
	logger := zap.NewNop()

	// Handler that always returns 200
	innerHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	middleware := ScopeEnforcementMiddleware(logger, innerHandler)

	tests := []struct {
		name     string
		method   string
		path     string
		scopes   []string
		wantCode int
	}{
		// Resource-specific scopes work on correct resource
		{"databases:read on GET databases", http.MethodGet, "/api/v1/databases", []string{"databases:read"}, http.StatusOK},
		{"databases:write on POST databases", http.MethodPost, "/api/v1/databases", []string{"databases:write"}, http.StatusOK},
		{"warehouses:write on POST warehouses", http.MethodPost, "/api/v1/tenants/t1/warehouses", []string{"warehouses:write"}, http.StatusOK},
		{"dataresources:admin on DELETE dataresources", http.MethodDelete, "/api/v1/tenants/t1/dataresources/res1", []string{"dataresources:admin"}, http.StatusOK},

		// Resource-specific scopes don't work on other resources
		{"databases:write on POST warehouses (should fail)", http.MethodPost, "/api/v1/tenants/t1/warehouses", []string{"databases:write"}, http.StatusForbidden},
		{"warehouses:admin on DELETE databases (should fail)", http.MethodDelete, "/api/v1/databases/db1", []string{"warehouses:admin"}, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			req := httptest.NewRequest(tt.method, tt.path, nil)
			cl := &claims.Claims{
				Subject:  "user1",
				Tenant:   "tenant1",
				Scopes:   tt.scopes,
				IssuedAt: 1234567890,
				Expires:  1234567890 + 3600,
			}
			req = req.WithContext(claims.WithClaims(context.Background(), cl))

			w := httptest.NewRecorder()
			middleware.ServeHTTP(w, req)

			if w.Code != tt.wantCode {
				t.Errorf("expected status %d, got %d", tt.wantCode, w.Code)
			}
		})
	}
}
