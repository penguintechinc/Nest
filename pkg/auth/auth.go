package auth

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/MicahParks/keyfunc/v3"
	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the standard JWT claims extracted from the token.
type Claims struct {
	Sub    string   `json:"sub"`             // Subject (user ID)
	Iss    string   `json:"iss"`             // Issuer
	Aud    []string `json:"aud"`             // Audience
	Iat    int64    `json:"iat"`             // Issued at
	Exp    int64    `json:"exp"`             // Expiration time
	Nbf    int64    `json:"nbf,omitempty"`   // Not before
	Scope  string   `json:"scope,omitempty"` // OAuth2 scope (space-delimited)
	Tenant string   `json:"tenant"`          // Tenant ID
	Teams  []string `json:"teams,omitempty"` // Team IDs
	Roles  []string `json:"roles,omitempty"` // Roles (informational only)
}

// contextKey is a private type for context keys.
type contextKey string

const (
	claimsKey contextKey = "auth.claims"
	tenantKey contextKey = "auth.tenant"
)

// jwksCache holds cached JWKS keyfunc with TTL.
type jwksCache struct {
	keyfunc   keyfunc.Keyfunc
	expiresAt time.Time
	mu        sync.RWMutex
}

// Config holds JWT verification configuration.
type Config struct {
	// JWKS endpoint for RS256/ES256 verification
	JWKSEndpoint string
	// Static shared secret for HS256 verification (manual admin use only)
	SharedSecret string
	// Allow HS256 verification (manual admin use only; requires explicit opt-in)
	AllowHS256Admin bool
	// Expected issuer
	Issuer string
	// Expected audience
	Audience string
	// Algorithm: "RS256", "ES256", or "HS256"
	Algorithm string

	// Internal JWKS cache (initialized lazily)
	jwksCache *jwksCache
}

// fetchJWKS fetches and caches JWKS from the endpoint (5-minute TTL).
func (c *Config) fetchJWKS(ctx context.Context) (keyfunc.Keyfunc, error) {
	if c.jwksCache == nil {
		c.jwksCache = &jwksCache{}
	}

	c.jwksCache.mu.RLock()
	if time.Now().Before(c.jwksCache.expiresAt) && c.jwksCache.keyfunc != nil {
		defer c.jwksCache.mu.RUnlock()
		return c.jwksCache.keyfunc, nil
	}
	c.jwksCache.mu.RUnlock()

	// Use keyfunc to fetch and parse JWKS
	kf, err := keyfunc.NewDefaultCtx(ctx, []string{c.JWKSEndpoint})
	if err != nil {
		return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
	}

	c.jwksCache.mu.Lock()
	c.jwksCache.keyfunc = kf
	c.jwksCache.expiresAt = time.Now().Add(5 * time.Minute)
	c.jwksCache.mu.Unlock()

	return kf, nil
}

// Verify verifies a JWT token and extracts claims.
// Returns error if:
// - Algorithm is "none"
// - Signature cannot be verified
// - Required claims are missing or invalid
// - Token is expired
func (c *Config) Verify(tokenString string) (*Claims, error) {
	// Parse token as MapClaims to extract all fields
	token, err := jwt.ParseWithClaims(tokenString, jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
		// Reject alg: none
		if token.Method.Alg() == "none" {
			return nil, fmt.Errorf("alg: none is not permitted")
		}

		// Verify algorithm matches config
		expectedAlg := c.Algorithm
		if expectedAlg == "" {
			expectedAlg = "RS256"
		}
		if token.Method.Alg() != expectedAlg {
			return nil, fmt.Errorf("unexpected signing algorithm: %s (expected %s)", token.Method.Alg(), expectedAlg)
		}

		// Handle HS256 with explicit opt-in requirement
		if expectedAlg == "HS256" {
			if !c.AllowHS256Admin {
				return nil, fmt.Errorf("HS256 is not enabled; set AllowHS256Admin=true for manual admin use only")
			}
			if c.SharedSecret == "" {
				return nil, fmt.Errorf("no shared secret configured for HS256")
			}
			return []byte(c.SharedSecret), nil
		}

		// Handle RS256 and ES256 via JWKS
		if expectedAlg == "RS256" || expectedAlg == "ES256" {
			if c.JWKSEndpoint == "" {
				return nil, fmt.Errorf("JWKS endpoint not configured for %s", expectedAlg)
			}

			kf, err := c.fetchJWKS(context.Background())
			if err != nil {
				return nil, fmt.Errorf("failed to fetch JWKS: %w", err)
			}

			// Use keyfunc to get the signing key
			key, err := kf.Keyfunc(token)
			if err != nil {
				return nil, fmt.Errorf("failed to get signing key from JWKS: %w", err)
			}

			return key, nil
		}

		return nil, fmt.Errorf("algorithm %s is not supported", expectedAlg)
	})

	if err != nil {
		return nil, fmt.Errorf("failed to parse token: %w", err)
	}

	if !token.Valid {
		return nil, fmt.Errorf("token is invalid")
	}

	// Extract all claims as MapClaims
	mapClaims, ok := token.Claims.(jwt.MapClaims)
	if !ok {
		return nil, fmt.Errorf("invalid claims type")
	}

	// Extract standard registered claims
	var exp, iat, nbf int64
	if expVal, ok := mapClaims["exp"].(float64); ok {
		exp = int64(expVal)
	} else {
		return nil, fmt.Errorf("exp claim is required")
	}

	if iatVal, ok := mapClaims["iat"].(float64); ok {
		iat = int64(iatVal)
	}

	if nbfVal, ok := mapClaims["nbf"].(float64); ok {
		nbf = int64(nbfVal)
	}

	// Verify token is not expired
	if exp <= 0 {
		return nil, fmt.Errorf("exp claim is invalid")
	}
	if time.Now().Unix() > exp {
		return nil, fmt.Errorf("token is expired")
	}

	// Verify nbf if present
	if nbf > 0 && time.Now().Unix() < nbf {
		return nil, fmt.Errorf("token is not yet valid (nbf)")
	}

	// Extract subject
	sub := ""
	if subVal, ok := mapClaims["sub"].(string); ok {
		sub = subVal
	}

	// Extract issuer
	iss := ""
	if issVal, ok := mapClaims["iss"].(string); ok {
		iss = issVal
	}

	// Verify issuer
	if c.Issuer != "" && iss != c.Issuer {
		return nil, fmt.Errorf("issuer mismatch: got %q, expected %q", iss, c.Issuer)
	}

	// Extract audience
	var aud []string
	if audVal, ok := mapClaims["aud"]; ok {
		switch audTyped := audVal.(type) {
		case []interface{}:
			for _, a := range audTyped {
				if audStr, ok := a.(string); ok {
					aud = append(aud, audStr)
				}
			}
		case string:
			aud = []string{audTyped}
		}
	}

	// Verify audience
	if c.Audience != "" {
		found := false
		for _, a := range aud {
			if a == c.Audience {
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("audience mismatch: got %v, expected %q", aud, c.Audience)
		}
	}

	// Extract tenant (required)
	tenant := ""
	if tenantVal, ok := mapClaims["tenant"].(string); ok {
		tenant = tenantVal
	} else {
		return nil, fmt.Errorf("tenant claim is required")
	}

	// Extract scope (optional)
	scope := ""
	if scopeVal, ok := mapClaims["scope"].(string); ok {
		scope = scopeVal
	}

	// Extract teams (optional)
	var teams []string
	if teamsVal, ok := mapClaims["teams"].([]interface{}); ok {
		for _, t := range teamsVal {
			if team, ok := t.(string); ok {
				teams = append(teams, team)
			}
		}
	}

	// Extract roles (optional)
	var roles []string
	if rolesVal, ok := mapClaims["roles"].([]interface{}); ok {
		for _, r := range rolesVal {
			if role, ok := r.(string); ok {
				roles = append(roles, role)
			}
		}
	}

	// Build Claims struct
	claims := &Claims{
		Sub:    sub,
		Iss:    iss,
		Aud:    aud,
		Iat:    iat,
		Exp:    exp,
		Nbf:    nbf,
		Tenant: tenant,
		Scope:  scope,
		Teams:  teams,
		Roles:  roles,
	}

	return claims, nil
}

// ClaimsFromContext extracts claims from the request context.
func ClaimsFromContext(ctx context.Context) *Claims {
	claims, ok := ctx.Value(claimsKey).(*Claims)
	if !ok {
		return nil
	}
	return claims
}

// TenantFromContext extracts tenant from the request context.
func TenantFromContext(ctx context.Context) string {
	tenant, ok := ctx.Value(tenantKey).(string)
	if !ok {
		return ""
	}
	return tenant
}

// contextWithClaims returns a new context with claims attached.
func contextWithClaims(ctx context.Context, claims *Claims) context.Context {
	ctx = context.WithValue(ctx, claimsKey, claims)
	ctx = context.WithValue(ctx, tenantKey, claims.Tenant)
	return ctx
}
