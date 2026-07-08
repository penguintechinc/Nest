package middleware

import (
	"net/http"
	"strings"

	"go.uber.org/zap"

	"github.com/penguintechinc/nest/services/gateway/internal/claims"
)

// extractResourceFromPath extracts the resource type from the request path.
// Examples:
//
//	/api/v1/query → query
//	/api/v1/objects/... → objects
//	/api/v1/tenants/{tid}/databases → databases
//	/api/v1/tenants/{tid}/dataresources → dataresources
//	/api/v1/tenants/{tid}/indexer/... → indexer
//	/api/v1/tenants/{tid}/policy/... → policy
func extractResourceFromPath(path string) string {
	// Remove leading /api/v1
	path = strings.TrimPrefix(path, "/api/v1/")

	// Split by /
	parts := strings.Split(path, "/")

	// Skip "tenants" and the {tid} part if present
	if len(parts) > 0 && parts[0] == "tenants" {
		if len(parts) > 2 {
			// Return the resource after tenants/{tid}/
			return parts[2]
		}
	}

	// For top-level resources like /query, /objects, etc.
	if len(parts) > 0 {
		return parts[0]
	}

	return ""
}

// ScopeEnforcementMiddleware enforces JWT scope requirements based on HTTP method, path, and resource.
// Scope format: "resource:action" where:
//   - resource: specific resource name (databases, warehouses, etc.) or "*" (wildcard)
//   - action: read, write, or admin
//
// A token scope satisfies the requirement if:
//   - The action matches (requiredAction OR admin)
//   - The resource matches (requiredResource OR "*")
//
// Examples:
//   - Token scope "*:write" satisfies POST to any resource (standard bundle)
//   - Token scope "databases:write" satisfies POST to databases only
//   - Token scope "analytics:write" does NOT satisfy POST to databases (cross-resource escalation blocked)
func ScopeEnforcementMiddleware(logger *zap.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cl, ok := claims.FromContext(r.Context())
		if !ok {
			http.Error(w, "missing claims in context", http.StatusUnauthorized)
			return
		}

		// Determine required action based on HTTP method
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

		// Extract required resource from the path
		requiredResource := extractResourceFromPath(r.URL.Path)
		if requiredResource == "" {
			http.Error(w, "forbidden: unable to determine resource", http.StatusForbidden)
			return
		}

		// Check if any of the token's scopes satisfy the requirement
		// Scope format: "resource:action"
		hasRequiredScope := false
		for _, scope := range cl.Scopes {
			parts := strings.Split(scope, ":")
			if len(parts) != 2 {
				continue
			}

			resource := parts[0]
			action := parts[1]

			// Check action match: exact match or admin (admin can do anything)
			actionMatch := action == requiredAction || action == "admin"
			if !actionMatch {
				continue
			}

			// Check resource match: exact match or wildcard (wildcard is for standard bundles)
			resourceMatch := resource == requiredResource || resource == "*"
			if !resourceMatch {
				continue
			}

			// Both resource and action match
			hasRequiredScope = true
			break
		}

		if !hasRequiredScope {
			logger.Warn("scope check failed",
				zap.String("tenant", cl.Tenant),
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.String("required_resource", requiredResource),
				zap.String("required_action", requiredAction),
				zap.Strings("scopes", cl.Scopes),
			)
			http.Error(w, "forbidden: insufficient scope", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	})
}
