package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/penguintechinc/nest/shared/go_libs/auth"
	"go.uber.org/zap"
)

// AuthMiddleware enforces OIDC token validation and injects claims into context.
func AuthMiddleware(jwksURL string, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			authHdr := r.Header.Get("Authorization")
			if authHdr == "" {
				http.Error(w, "missing authorization header", http.StatusUnauthorized)
				return
			}

			token := strings.TrimPrefix(authHdr, "Bearer ")
			if token == authHdr {
				http.Error(w, "invalid authorization scheme", http.StatusUnauthorized)
				return
			}

			var cl *auth.Claims
			var err error

			if jwksURL == "test" {
				// Bypass for tests: dummy token format "test-tenant:test-sub"
				parts := strings.Split(token, ":")
				if len(parts) == 2 {
					cl = &auth.Claims{
						Tenant:  parts[0],
						Subject: parts[1],
					}
				} else {
					err = fmt.Errorf("invalid test token")
				}
			} else {
				cl, err = auth.ParseToken(token, jwksURL)
			}

			if err != nil {
				logger.Warn("token validation failed", zap.Error(err))
				http.Error(w, fmt.Sprintf("invalid token: %v", err), http.StatusUnauthorized)
				return
			}

			if cl.Tenant == "" {
				http.Error(w, "token missing tenant claim", http.StatusUnauthorized)
				return
			}

			// Inject claims into context
			ctx := auth.WithClaims(r.Context(), cl)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// TenantFilter enforces that the requested tenant matches the authenticated tenant.
func TenantFilter(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		cl, ok := auth.FromContext(r.Context())
		if !ok {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Check tenant in URL path
		tid := r.PathValue("tid")
		if tid == "" {
			tid = r.PathValue("tenantId")
		}
		if tid != "" && tid != cl.Tenant {
			http.Error(w, "tenant mismatch", http.StatusForbidden)
			return
		}

		// Check tenant in query param
		queryTid := r.URL.Query().Get("tenant")
		if queryTid != "" && queryTid != cl.Tenant {
			http.Error(w, "tenant mismatch", http.StatusForbidden)
			return
		}

		next.ServeHTTP(w, r)
	}
}
