package auth

import (
	"errors"
	"net/http"
	"strings"
)

// Middleware provides HTTP middleware functions for JWT verification and scope checking.
type Middleware struct {
	config *Config
}

// NewMiddleware creates a new middleware instance.
// Returns error if config is not properly initialized (FAIL-CLOSED).
func NewMiddleware(config *Config) (*Middleware, error) {
	if config == nil {
		return nil, errors.New("config is required")
	}

	// FAIL-CLOSED: both JWKS and shared secret must not be empty
	if config.JWKSEndpoint == "" && config.SharedSecret == "" {
		return nil, errors.New("neither JWKS endpoint nor shared secret is configured; cannot verify JWTs")
	}

	if config.Algorithm == "" {
		config.Algorithm = "HS256" // default to HS256
	}

	if config.Algorithm != "RS256" && config.Algorithm != "HS256" {
		return nil, errors.New("algorithm must be RS256 or HS256")
	}

	return &Middleware{config: config}, nil
}

// RequireAuth verifies the JWT token and injects claims into the request context.
// Returns 401 if token is missing or invalid.
func (m *Middleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		token := extractBearerToken(r)
		if token == "" {
			http.Error(w, `{"error": "missing or invalid authorization header"}`, http.StatusUnauthorized)
			return
		}

		claims, err := m.config.Verify(token)
		if err != nil {
			http.Error(w, `{"error": "`+err.Error()+`"}`, http.StatusUnauthorized)
			return
		}

		// Attach claims to context
		ctx := contextWithClaims(r.Context(), claims)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RequireTenant ensures the token has a tenant claim.
// Returns 403 if tenant is missing.
func (m *Middleware) RequireTenant(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		claims := ClaimsFromContext(r.Context())
		if claims == nil || claims.Tenant == "" {
			http.Error(w, `{"error": "tenant is required"}`, http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireScope ensures the token has a scope that satisfies the required scope.
// scope should be in "resource:action" format.
// Matching semantics:
// - Exact match: resource:action == required resource:action
// - Wildcard resource: *:action satisfies resource:action if action matches or is admin
// - Admin action: admin action satisfies any action (write satisfies read, etc.)
// Returns 403 if no scope satisfies the requirement.
func (m *Middleware) RequireScope(scope string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			claims := ClaimsFromContext(r.Context())
			if claims == nil || claims.Scope == "" {
				http.Error(w, `{"error": "insufficient scope"}`, http.StatusForbidden)
				return
			}

			// Split scope by space and check if any scope satisfies the requirement
			scopes := strings.Fields(claims.Scope)
			found := false
			for _, s := range scopes {
				if m.scopeSatisfies(s, scope) {
					found = true
					break
				}
			}

			if !found {
				http.Error(w, `{"error": "insufficient scope"}`, http.StatusForbidden)
				return
			}

			next.ServeHTTP(w, r)
		})
	}
}

// scopeSatisfies checks if a single scope satisfies the required scope.
func (m *Middleware) scopeSatisfies(scope, required string) bool {
	// Parse scope and required into resource and action
	scopeParts := strings.Split(scope, ":")
	requiredParts := strings.Split(required, ":")

	if len(scopeParts) != 2 || len(requiredParts) != 2 {
		return false
	}

	scopeResource, scopeAction := scopeParts[0], scopeParts[1]
	requiredResource, requiredAction := requiredParts[0], requiredParts[1]

	// Resource matching: exact match or wildcard
	resourceMatches := scopeResource == requiredResource || scopeResource == "*"
	if !resourceMatches {
		return false
	}

	// Action matching: exact match or admin action covers everything
	actionMatches := scopeAction == requiredAction || scopeAction == "admin"
	return actionMatches
}

// AssertTenantMatch ensures the provided tenant matches the token's tenant.
// Returns 403 if they don't match.
func (m *Middleware) AssertTenantMatch(tokenTenant, providedTenant string) error {
	if tokenTenant == "" || providedTenant == "" {
		return errors.New("tenants must not be empty")
	}
	if tokenTenant != providedTenant {
		return errors.New("tenant mismatch")
	}
	return nil
}

// extractBearerToken extracts the JWT from the Authorization header.
func extractBearerToken(r *http.Request) string {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return ""
	}

	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ""
	}

	return parts[1]
}
