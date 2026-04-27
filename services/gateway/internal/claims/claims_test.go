package claims

import (
	"context"
	"encoding/base64"
	"testing"
	"time"
)

func TestParseToken(t *testing.T) {
	tests := []struct {
		name      string
		token     string
		audience  string
		wantErr   bool
		wantClaim *Claims
	}{
		{
			name:    "invalid token - not 3 parts",
			token:   "invalid.token",
			wantErr: true,
		},
		{
			name:    "invalid token - malformed payload",
			token:   "header." + base64.RawURLEncoding.EncodeToString([]byte("not-json")) + ".sig",
			wantErr: true,
		},
		{
			name:    "valid token - basic claims",
			token:   makeToken("user123", "tenant1", 0),
			wantErr: false,
			wantClaim: &Claims{
				Subject: "user123",
				Tenant:  "tenant1",
			},
		},
		{
			name:    "valid token - with scopes string",
			token:   makeTokenWithScopes("user1", "tenant1", "read write", false),
			wantErr: false,
			wantClaim: &Claims{
				Subject: "user1",
				Tenant:  "tenant1",
				Scopes:  []string{"read", "write"},
			},
		},
		{
			name:    "valid token - with scopes array",
			token:   makeTokenWithScopes("user2", "tenant2", "", true),
			wantErr: false,
			wantClaim: &Claims{
				Subject: "user2",
				Tenant:  "tenant2",
				Scopes:  []string{"admin", "write"},
			},
		},
		{
			name:    "token with roles",
			token:   makeTokenWithRoles("user3", "tenant3"),
			wantErr: false,
			wantClaim: &Claims{
				Subject: "user3",
				Tenant:  "tenant3",
				Roles:   []string{"Admin", "Maintainer"},
			},
		},
		{
			name:    "expired token",
			token:   makeToken("user4", "tenant4", -1000),
			wantErr: true,
		},
		{
			name:    "invalid base64",
			token:   "header.!!!invalid!!!.sig",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ParseToken(tt.token, tt.audience)
			if (err != nil) != tt.wantErr {
				t.Errorf("ParseToken() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && got != nil {
				if got.Subject != tt.wantClaim.Subject {
					t.Errorf("ParseToken() Subject = %q, want %q", got.Subject, tt.wantClaim.Subject)
				}
				if got.Tenant != tt.wantClaim.Tenant {
					t.Errorf("ParseToken() Tenant = %q, want %q", got.Tenant, tt.wantClaim.Tenant)
				}
				// Check scopes if expected
				if len(tt.wantClaim.Scopes) > 0 {
					if len(got.Scopes) != len(tt.wantClaim.Scopes) {
						t.Errorf("ParseToken() Scopes length = %d, want %d", len(got.Scopes), len(tt.wantClaim.Scopes))
					}
					for i, s := range tt.wantClaim.Scopes {
						if i < len(got.Scopes) && got.Scopes[i] != s {
							t.Errorf("ParseToken() Scopes[%d] = %q, want %q", i, got.Scopes[i], s)
						}
					}
				}
				// Check roles if expected
				if len(tt.wantClaim.Roles) > 0 {
					if len(got.Roles) != len(tt.wantClaim.Roles) {
						t.Errorf("ParseToken() Roles length = %d, want %d", len(got.Roles), len(tt.wantClaim.Roles))
					}
					for i, r := range tt.wantClaim.Roles {
						if i < len(got.Roles) && got.Roles[i] != r {
							t.Errorf("ParseToken() Roles[%d] = %q, want %q", i, got.Roles[i], r)
						}
					}
				}
			}
		})
	}
}

func TestWithClaims(t *testing.T) {
	ctx := context.Background()
	cl := &Claims{Subject: "user1", Tenant: "tenant1"}

	ctxWithClaims := WithClaims(ctx, cl)
	if ctxWithClaims == ctx {
		t.Error("WithClaims() should return a new context")
	}

	if _, ok := ctxWithClaims.Value(contextKey{}).(*Claims); !ok {
		t.Error("WithClaims() should store Claims in context")
	}
}

func TestFromContext(t *testing.T) {
	tests := []struct {
		name    string
		setup   func() context.Context
		wantOK  bool
		wantCl  *Claims
	}{
		{
			name: "claims present",
			setup: func() context.Context {
				cl := &Claims{Subject: "user1", Tenant: "tenant1"}
				return WithClaims(context.Background(), cl)
			},
			wantOK: true,
			wantCl: &Claims{Subject: "user1", Tenant: "tenant1"},
		},
		{
			name: "claims not present",
			setup: func() context.Context {
				return context.Background()
			},
			wantOK: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ctx := tt.setup()
			got, ok := FromContext(ctx)
			if ok != tt.wantOK {
				t.Errorf("FromContext() ok = %v, want %v", ok, tt.wantOK)
			}
			if ok && (got.Subject != tt.wantCl.Subject || got.Tenant != tt.wantCl.Tenant) {
				t.Errorf("FromContext() = %+v, want %+v", got, tt.wantCl)
			}
		})
	}
}

// Helper functions to create test tokens
func makeToken(sub, tenant string, expOffset int64) string {
	expTime := time.Now().Unix() + expOffset
	payload := `{"sub":"` + sub + `","tenant":"` + tenant + `","exp":` + itoa(expTime) + `}`
	header := `{"alg":"HS256","typ":"JWT"}`
	return base64.RawURLEncoding.EncodeToString([]byte(header)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".sig"
}

func makeTokenWithScopes(sub, tenant string, scopes string, isArray bool) string {
	payload := `{"sub":"` + sub + `","tenant":"` + tenant + `"`
	if scopes != "" && !isArray {
		payload += `,"scope":"` + scopes + `"`
	} else if isArray {
		payload += `,"scope":["admin","write"]`
	}
	payload += `,"exp":` + itoa(time.Now().Unix()+3600) + `}`

	header := `{"alg":"HS256","typ":"JWT"}`
	return base64.RawURLEncoding.EncodeToString([]byte(header)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".sig"
}

func makeTokenWithRoles(sub, tenant string) string {
	payload := `{"sub":"` + sub + `","tenant":"` + tenant + `","roles":["Admin","Maintainer"],"exp":` +
		itoa(time.Now().Unix()+3600) + `}`

	header := `{"alg":"HS256","typ":"JWT"}`
	return base64.RawURLEncoding.EncodeToString([]byte(header)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".sig"
}

func TestHasScope(t *testing.T) {
	cl := &Claims{
		Scopes: []string{"read", "write", "admin"},
	}

	tests := []struct {
		scope string
		want  bool
	}{
		{"read", true},
		{"write", true},
		{"admin", true},
		{"delete", false},
		{"", false},
		{"READ", false}, // case-sensitive
	}

	for _, tt := range tests {
		t.Run(tt.scope, func(t *testing.T) {
			got := cl.HasScope(tt.scope)
			if got != tt.want {
				t.Errorf("HasScope(%q) = %v, want %v", tt.scope, got, tt.want)
			}
		})
	}
}

func TestHasScope_EmptyScopes(t *testing.T) {
	cl := &Claims{Scopes: []string{}}
	if cl.HasScope("read") {
		t.Error("HasScope() = true for empty scopes, want false")
	}
}

func TestHasScope_NilScopes(t *testing.T) {
	cl := &Claims{}
	if cl.HasScope("read") {
		t.Error("HasScope() = true for nil scopes, want false")
	}
}

func itoa(n int64) string {
	if n == 0 {
		return "0"
	}
	negative := n < 0
	if negative {
		n = -n
	}
	var buf [20]byte
	i := len(buf) - 1
	for n > 0 {
		buf[i] = byte(n%10) + '0'
		i--
		n /= 10
	}
	if negative {
		buf[i] = '-'
		i--
	}
	return string(buf[i+1:])
}
