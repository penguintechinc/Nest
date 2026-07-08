package auth

import (
	"context"
	"fmt"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// Claims represents the standard JWT claims extracted from the token.
type Claims struct {
	Sub   string   `json:"sub"`             // Subject (user ID)
	Iss   string   `json:"iss"`             // Issuer
	Aud   []string `json:"aud"`             // Audience
	Iat   int64    `json:"iat"`             // Issued at
	Exp   int64    `json:"exp"`             // Expiration time
	Nbf   int64    `json:"nbf,omitempty"`   // Not before
	Scope string   `json:"scope,omitempty"` // OAuth2 scope (space-delimited)
	Tenant string   `json:"tenant"`         // Tenant ID
	Teams  []string `json:"teams,omitempty"` // Team IDs
	Roles  []string `json:"roles,omitempty"` // Roles (informational only)
}

// contextKey is a private type for context keys.
type contextKey string

const (
	claimsKey  contextKey = "auth.claims"
	tenantKey  contextKey = "auth.tenant"
)

// Config holds JWT verification configuration.
type Config struct {
	// JWKS endpoint for RS256 verification (takes precedence)
	JWKSEndpoint string
	// Static shared secret for HS256 verification (fallback if JWKS not set)
	SharedSecret string
	// Expected issuer
	Issuer string
	// Expected audience
	Audience string
	// Algorithm: either "RS256" or "HS256"
	Algorithm string
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

		// TODO: For RS256 with JWKS endpoint, fetch and use appropriate key
		// For now, only support HS256 with static secret
		if expectedAlg == "HS256" {
			if c.SharedSecret == "" {
				return nil, fmt.Errorf("no shared secret configured for HS256")
			}
			return []byte(c.SharedSecret), nil
		}

		return nil, fmt.Errorf("algorithm %s not yet implemented", expectedAlg)
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
