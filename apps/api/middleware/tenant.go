package middleware

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

const TenantKey = "nest_tenant"
const ClaimsKey = "nest_claims"

// Claims represents the JWT claims subset Nest cares about
type Claims struct {
	Sub    string   `json:"sub"`
	Tenant string   `json:"tenant"`
	Scopes []string `json:"scopes"`
	Tier   string   `json:"tier"`
}

// TenantMiddleware enforces presence of tenant claim on every request.
// In P1 this validates a simple Bearer token; full OIDC wired in P2+.
func TenantMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		auth := c.GetHeader("Authorization")
		if !strings.HasPrefix(auth, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, nestError(
				"nest.auth.missing_token",
				"Authorization header with Bearer token required",
				c.Request.Header.Get("X-Request-ID"),
			))
			return
		}

		// TODO(P2): validate JWT signature via OIDC JWKS
		claims, err := parseToken(strings.TrimPrefix(auth, "Bearer "))
		if err != nil || claims.Tenant == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, nestError(
				"nest.auth.missing_tenant",
				"JWT missing mandatory tenant claim",
				c.Request.Header.Get("X-Request-ID"),
			))
			return
		}

		c.Set(TenantKey, claims.Tenant)
		c.Set(ClaimsKey, claims)
		c.Next()
	}
}

// GetTenant extracts the validated tenant from context
func GetTenant(c *gin.Context) string {
	t, _ := c.Get(TenantKey)
	s, _ := t.(string)
	return s
}

// GetClaims extracts the validated claims from context
func GetClaims(c *gin.Context) *Claims {
	v, _ := c.Get(ClaimsKey)
	cl, _ := v.(*Claims)
	return cl
}

// RequireScope aborts if the token lacks the given scope
func RequireScope(scope string) gin.HandlerFunc {
	return func(c *gin.Context) {
		claims := GetClaims(c)
		if claims == nil {
			c.AbortWithStatusJSON(http.StatusForbidden, nestError(
				"nest.auth.scope_denied",
				"Authentication required",
				c.Request.Header.Get("X-Request-ID"),
			))
			return
		}
		for _, s := range claims.Scopes {
			if s == scope || s == "nest:*:admin" {
				c.Next()
				return
			}
		}
		c.AbortWithStatusJSON(http.StatusForbidden, nestError(
			"nest.auth.scope_denied",
			"Insufficient scope: "+scope+" required",
			c.Request.Header.Get("X-Request-ID"),
		))
	}
}

// parseToken is a stub for P1. In P2+ this validates via OIDC JWKS.
func parseToken(token string) (*Claims, error) {
	// P1: accept any token with format "tenant:scopes" for local dev
	// This is replaced by real OIDC validation in P2
	if token == "" {
		return nil, fmt.Errorf("empty token")
	}
	parts := strings.SplitN(token, ":", 3)
	claims := &Claims{
		Sub:    parts[0],
		Tenant: "",
		Scopes: []string{"nest:*:admin"},
		Tier:   "free",
	}
	if len(parts) >= 2 {
		claims.Tenant = parts[1]
	}
	if len(parts) >= 3 {
		claims.Tier = parts[2]
	}
	return claims, nil
}

func nestError(code, message, requestID string) gin.H {
	return gin.H{
		"code":      code,
		"message":   message,
		"requestId": requestID,
		"docsUrl":   "https://docs.nest.penguintech.io/errors/" + code,
	}
}
