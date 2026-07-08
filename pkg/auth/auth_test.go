package auth

import (
	"context"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestConfigVerify_ValidToken(t *testing.T) {
	config := &Config{
		Algorithm:    "HS256",
		SharedSecret: "test-secret",
		Issuer:       "test-issuer",
		Audience:     "test-audience",
	}

	// Create a valid token
	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":    "user123",
		"iss":    "test-issuer",
		"aud":    []string{"test-audience"},
		"iat":    now.Unix(),
		"exp":    now.Add(1 * time.Hour).Unix(),
		"tenant": "tenant-123",
		"scope":  "data:read data:write",
		"teams":  []string{"team1", "team2"},
		"roles":  []string{"admin"},
	})

	tokenString, err := token.SignedString([]byte("test-secret"))
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	claims, err := config.Verify(tokenString)
	if err != nil {
		t.Fatalf("failed to verify token: %v", err)
	}

	if claims.Sub != "user123" {
		t.Errorf("sub mismatch: got %q, want %q", claims.Sub, "user123")
	}
	if claims.Tenant != "tenant-123" {
		t.Errorf("tenant mismatch: got %q, want %q", claims.Tenant, "tenant-123")
	}
	if claims.Scope != "data:read data:write" {
		t.Errorf("scope mismatch: got %q, want %q", claims.Scope, "data:read data:write")
	}
	if len(claims.Teams) != 2 {
		t.Errorf("teams count mismatch: got %d, want 2", len(claims.Teams))
	}
}

func TestConfigVerify_ExpiredToken(t *testing.T) {
	config := &Config{
		Algorithm:    "HS256",
		SharedSecret: "test-secret",
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp":    now.Add(-1 * time.Hour).Unix(), // expired
		"tenant": "tenant-123",
	})

	tokenString, _ := token.SignedString([]byte("test-secret"))
	_, err := config.Verify(tokenString)
	if err == nil {
		t.Fatal("expected error for expired token")
	}
}

func TestConfigVerify_MissingExpClaim(t *testing.T) {
	config := &Config{
		Algorithm:    "HS256",
		SharedSecret: "test-secret",
	}

	// Token without exp claim
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"sub":    "user123",
		"tenant": "tenant-123",
		// no exp
	})

	tokenString, _ := token.SignedString([]byte("test-secret"))
	_, err := config.Verify(tokenString)
	if err == nil {
		t.Fatal("expected error for missing exp claim")
	}
}

func TestConfigVerify_WrongIssuer(t *testing.T) {
	config := &Config{
		Algorithm:    "HS256",
		SharedSecret: "test-secret",
		Issuer:       "expected-issuer",
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp":    now.Add(1 * time.Hour).Unix(),
		"iss":    "wrong-issuer",
		"tenant": "tenant-123",
	})

	tokenString, _ := token.SignedString([]byte("test-secret"))
	_, err := config.Verify(tokenString)
	if err == nil {
		t.Fatal("expected error for issuer mismatch")
	}
}

func TestConfigVerify_WrongAudience(t *testing.T) {
	config := &Config{
		Algorithm:    "HS256",
		SharedSecret: "test-secret",
		Audience:     "expected-audience",
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp":    now.Add(1 * time.Hour).Unix(),
		"aud":    []string{"wrong-audience"},
		"tenant": "tenant-123",
	})

	tokenString, _ := token.SignedString([]byte("test-secret"))
	_, err := config.Verify(tokenString)
	if err == nil {
		t.Fatal("expected error for audience mismatch")
	}
}

func TestConfigVerify_MissingTenant(t *testing.T) {
	config := &Config{
		Algorithm:    "HS256",
		SharedSecret: "test-secret",
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp": now.Add(1 * time.Hour).Unix(),
		// no tenant
	})

	tokenString, _ := token.SignedString([]byte("test-secret"))
	_, err := config.Verify(tokenString)
	if err == nil {
		t.Fatal("expected error for missing tenant claim")
	}
}

func TestConfigVerify_NotBefore(t *testing.T) {
	config := &Config{
		Algorithm:    "HS256",
		SharedSecret: "test-secret",
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp":    now.Add(1 * time.Hour).Unix(),
		"nbf":    now.Add(1 * time.Hour).Unix(), // not yet valid
		"tenant": "tenant-123",
	})

	tokenString, _ := token.SignedString([]byte("test-secret"))
	_, err := config.Verify(tokenString)
	if err == nil {
		t.Fatal("expected error for token not yet valid (nbf)")
	}
}

func TestConfigVerify_BadSignature(t *testing.T) {
	config := &Config{
		Algorithm:    "HS256",
		SharedSecret: "test-secret",
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp":    now.Add(1 * time.Hour).Unix(),
		"tenant": "tenant-123",
	})

	tokenString, _ := token.SignedString([]byte("wrong-secret"))

	// Verify with different secret
	_, err := config.Verify(tokenString)
	if err == nil {
		t.Fatal("expected error for bad signature")
	}
}

func TestClaimsFromContext_Present(t *testing.T) {
	ctx := contextWithClaims(context.Background(), &Claims{Sub: "user123", Tenant: "tenant-123"})
	claims := ClaimsFromContext(ctx)
	if claims == nil {
		t.Fatal("expected claims from context")
	}
	if claims.Sub != "user123" {
		t.Errorf("sub mismatch: got %q, want %q", claims.Sub, "user123")
	}
}

func TestClaimsFromContext_Absent(t *testing.T) {
	claims := ClaimsFromContext(context.Background())
	if claims != nil {
		t.Fatal("expected nil claims from empty context")
	}
}

func TestTenantFromContext_Present(t *testing.T) {
	ctx := contextWithClaims(context.Background(), &Claims{Tenant: "tenant-123"})
	tenant := TenantFromContext(ctx)
	if tenant != "tenant-123" {
		t.Errorf("tenant mismatch: got %q, want %q", tenant, "tenant-123")
	}
}

func TestTenantFromContext_Absent(t *testing.T) {
	tenant := TenantFromContext(context.Background())
	if tenant != "" {
		t.Errorf("tenant mismatch: got %q, want empty", tenant)
	}
}
