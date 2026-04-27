package claims

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type contextKey struct{}

// Claims holds the parsed JWT claims relevant to the gateway.
type Claims struct {
	Subject  string   `json:"sub"`
	Tenant   string   `json:"tenant"`
	Scopes   []string `json:"scope"`
	Roles    []string `json:"roles"`
	IssuedAt int64    `json:"iat"`
	Expires  int64    `json:"exp"`
}

// ParseToken extracts claims from a JWT without signature verification.
// TODO(P2): implement real JWKS signature validation.
func ParseToken(token, _ string) (*Claims, error) {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed JWT: expected 3 parts, got %d", len(parts))
	}

	payload, err := base64.RawURLEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(payload, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	cl := &Claims{}
	if v, ok := raw["sub"].(string); ok {
		cl.Subject = v
	}
	if v, ok := raw["tenant"].(string); ok {
		cl.Tenant = v
	}
	if v, ok := raw["exp"].(float64); ok {
		cl.Expires = int64(v)
		if cl.Expires > 0 && time.Now().Unix() > cl.Expires {
			return nil, fmt.Errorf("token expired")
		}
	}
	// Parse scope: may be space-separated string or array
	switch v := raw["scope"].(type) {
	case string:
		cl.Scopes = strings.Fields(v)
	case []interface{}:
		for _, s := range v {
			if str, ok := s.(string); ok {
				cl.Scopes = append(cl.Scopes, str)
			}
		}
	}
	if v, ok := raw["roles"].([]interface{}); ok {
		for _, r := range v {
			if str, ok := r.(string); ok {
				cl.Roles = append(cl.Roles, str)
			}
		}
	}

	return cl, nil
}

func WithClaims(ctx context.Context, cl *Claims) context.Context {
	return context.WithValue(ctx, contextKey{}, cl)
}

func FromContext(ctx context.Context) (*Claims, bool) {
	cl, ok := ctx.Value(contextKey{}).(*Claims)
	return cl, ok && cl != nil
}

// HasScope checks if the claims contain a specific scope.
func (c *Claims) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}
