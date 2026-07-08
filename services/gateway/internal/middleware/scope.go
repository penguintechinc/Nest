package middleware

import (
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/gateway/internal/claims"
)

// ScopeEnforcementMiddleware enforces JWT scope requirements based on HTTP method and path.
// Read endpoints (GET) require "read" scope, write/patch/post require "write" scope, delete requires "admin" scope.
// Scope format: "resource:action" (e.g., "databases:read", "databases:write", "databases:admin")
func ScopeEnforcementMiddleware(logger *zap.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cl, ok := claims.FromContext(r.Context())
		if !ok {
			http.Error(w, "missing claims in context", http.StatusUnauthorized)
			return
		}

		// Determine required scope based on HTTP method
		var requiredAction string
		switch r.Method {
		case http.MethodGet, http.MethodHead, http.MethodOptions:
			requiredAction = "read"
		case http.MethodPost, http.MethodPut, http.MethodPatch:
			requiredAction = "write"
		case http.MethodDelete:
			requiredAction = "admin"
		default:
			// Unknown method - deny by default
			http.Error(w, "forbidden: unsupported method", http.StatusForbidden)
			return
		}

		// Check if any of the token's scopes match the required action
		// Scopes are in format "resource:action" or can be wildcards like "*:write"
		hasRequiredScope := false
		for _, scope := range cl.Scopes {
			parts := strings.Split(scope, ":")
			if len(parts) == 2 {
				action := parts[1]
				// Match if action is "admin" (admin can do anything) or exact match
				if action == "admin" || action == requiredAction {
					hasRequiredScope = true
					break
				}
			}
		}

		if !hasRequiredScope {
			logger.Warn("scope check failed",
				zap.String("tenant", cl.Tenant),
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Strings("scopes", cl.Scopes),
				zap.String("required_action", requiredAction),
			)
			http.Error(w, "forbidden: insufficient scope", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
