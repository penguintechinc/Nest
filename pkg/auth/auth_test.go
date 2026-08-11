package auth

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

func TestConfigVerify_ValidToken(t *testing.T) {
	config := &Config{
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true, // HS256 now requires explicit opt-in
		Issuer:          "test-issuer",
		Audience:        "test-audience",
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
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
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
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
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
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
		Issuer:          "expected-issuer",
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
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
		Audience:        "expected-audience",
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
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
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
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
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
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: true,
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

// Test: HS256 rejected when AllowHS256Admin is false (default)
func TestConfigVerify_HS256RejectedWhenNotAllowed(t *testing.T) {
	config := &Config{
		Algorithm:       "HS256",
		SharedSecret:    "test-secret",
		AllowHS256Admin: false, // explicitly false or unset (default)
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, jwt.MapClaims{
		"exp":    now.Add(1 * time.Hour).Unix(),
		"tenant": "tenant-123",
	})

	tokenString, _ := token.SignedString([]byte("test-secret"))
	_, err := config.Verify(tokenString)
	if err == nil {
		t.Fatal("expected verification to fail when AllowHS256Admin is false")
	}
	if !strings.Contains(err.Error(), "HS256 is not enabled") {
		t.Errorf("unexpected error message: %v", err)
	}
}

// Test: alg: none is always rejected
func TestConfigVerify_AlgNoneRejected(t *testing.T) {
	config := &Config{
		Algorithm: "RS256",
		Issuer:    "test-issuer",
		Audience:  "test-audience",
	}

	now := time.Now()
	token := jwt.NewWithClaims(jwt.SigningMethodNone, jwt.MapClaims{
		"exp":    now.Add(1 * time.Hour).Unix(),
		"tenant": "tenant-123",
	})

	tokenString, _ := token.SignedString(jwt.UnsafeAllowNoneSignatureType)
	_, err := config.Verify(tokenString)
	if err == nil {
		t.Fatal("expected verification to fail for alg: none")
	}
	if !strings.Contains(err.Error(), "alg: none is not permitted") {
		t.Errorf("unexpected error: %v", err)
	}
}

// ============ JWKS Integration Tests ============

// createES256JWK converts an ECDSA public key to a JWK representation
func createES256JWK(pubKey *ecdsa.PublicKey, kid string) map[string]interface{} {
	return map[string]interface{}{
		"kty": "EC",
		"kid": kid,
		"crv": "P-256",
		"x":   base64.RawURLEncoding.EncodeToString(pubKey.X.Bytes()),
		"y":   base64.RawURLEncoding.EncodeToString(pubKey.Y.Bytes()),
		"alg": "ES256",
		"use": "sig",
	}
}

// createRS256JWK converts an RSA public key to a JWK representation
func createRS256JWK(pubKey *rsa.PublicKey, kid string) map[string]interface{} {
	return map[string]interface{}{
		"kty": "RSA",
		"kid": kid,
		"alg": "RS256",
		"use": "sig",
		"n":   base64.RawURLEncoding.EncodeToString(pubKey.N.Bytes()),
		"e":   "AQAB",
	}
}

// createMockJWKSServer returns a test HTTP server serving a JWKS endpoint
// Returns the server and a counter of how many times the endpoint was called
func createMockJWKSServer(keys []map[string]interface{}) (*httptest.Server, *int32) {
	callCount := int32(0)
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt32(&callCount, 1)
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]interface{}{"keys": keys})
	}))
	return server, &callCount
}

// TestVerifyES256WithJWKS: ES256 token signed with ECDSA key, verified via JWKS
func TestVerifyES256WithJWKS(t *testing.T) {
	// Generate ECDSA key pair
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate ECDSA key: %v", err)
	}

	// Create JWK for the public key
	jwk := createES256JWK(&privKey.PublicKey, "es256-key-1")
	server, _ := createMockJWKSServer([]map[string]interface{}{jwk})
	defer server.Close()

	// Create a valid ES256 token
	now := time.Now().Unix()
	claims := jwt.MapClaims{
		"sub":    "user123",
		"iss":    "test-issuer",
		"aud":    "test-audience",
		"iat":    now,
		"exp":    now + 3600,
		"tenant": "tenant1",
		"scope":  "read write",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodES256, claims)
	token.Header["kid"] = "es256-key-1"
	tokenString, err := token.SignedString(privKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	// Verify via JWKS
	config := &Config{
		JWKSEndpoint: server.URL,
		Algorithm:    "ES256",
		Issuer:       "test-issuer",
		Audience:     "test-audience",
	}

	verifiedClaims, err := config.Verify(tokenString)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	if verifiedClaims.Sub != "user123" {
		t.Errorf("expected sub user123, got %s", verifiedClaims.Sub)
	}
	if verifiedClaims.Tenant != "tenant1" {
		t.Errorf("expected tenant tenant1, got %s", verifiedClaims.Tenant)
	}
}

// TestVerifyRS256WithJWKS: RS256 token signed with RSA key, verified via JWKS
func TestVerifyRS256WithJWKS(t *testing.T) {
	// Generate RSA key pair
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	// Create JWK for the public key
	jwk := createRS256JWK(&privKey.PublicKey, "rs256-key-1")
	server, _ := createMockJWKSServer([]map[string]interface{}{jwk})
	defer server.Close()

	// Create a valid RS256 token
	now := time.Now().Unix()
	claims := jwt.MapClaims{
		"sub":    "user456",
		"iss":    "test-issuer",
		"aud":    "test-audience",
		"iat":    now,
		"exp":    now + 3600,
		"tenant": "tenant2",
		"scope":  "admin",
	}
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "rs256-key-1"
	tokenString, err := token.SignedString(privKey)
	if err != nil {
		t.Fatalf("failed to sign token: %v", err)
	}

	// Verify via JWKS
	config := &Config{
		JWKSEndpoint: server.URL,
		Algorithm:    "RS256",
		Issuer:       "test-issuer",
		Audience:     "test-audience",
	}

	verifiedClaims, err := config.Verify(tokenString)
	if err != nil {
		t.Fatalf("verification failed: %v", err)
	}

	if verifiedClaims.Sub != "user456" {
		t.Errorf("expected sub user456, got %s", verifiedClaims.Sub)
	}
	if verifiedClaims.Tenant != "tenant2" {
		t.Errorf("expected tenant tenant2, got %s", verifiedClaims.Tenant)
	}
}

// TestVerifyWrongKidNotInJWKS: token's kid doesn't match any key in JWKS
func TestVerifyWrongKidNotInJWKS(t *testing.T) {
	// Generate RSA key pair
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	// Create JWK with one kid
	jwk := createRS256JWK(&privKey.PublicKey, "correct-kid")
	server, _ := createMockJWKSServer([]map[string]interface{}{jwk})
	defer server.Close()

	// Create token with different kid
	now := time.Now().Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iat":    now,
		"exp":    now + 3600,
		"tenant": "tenant3",
	})
	token.Header["kid"] = "wrong-kid" // This kid is not in JWKS
	tokenString, _ := token.SignedString(privKey)

	config := &Config{
		JWKSEndpoint: server.URL,
		Algorithm:    "RS256",
	}

	_, err = config.Verify(tokenString)
	if err == nil {
		t.Fatal("expected verification to fail for wrong kid")
	}
	if !strings.Contains(err.Error(), "kid") {
		t.Errorf("expected kid-related error, got: %v", err)
	}
}

// TestVerifyJWKSEndpointUnreachable: JWKS endpoint is unreachable
func TestVerifyJWKSEndpointUnreachable(t *testing.T) {
	now := time.Now().Unix()
	claims := jwt.MapClaims{
		"iat":    now,
		"exp":    now + 3600,
		"tenant": "tenant4",
	}

	// Create a dummy token (won't actually be verified, but needed for format)
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, claims)
	token.Header["kid"] = "any-kid"
	tokenString, _ := token.SignedString([]byte("dummy"))

	config := &Config{
		JWKSEndpoint: "http://localhost:54321", // Port unlikely to be open
		Algorithm:    "RS256",
	}

	_, err := config.Verify(tokenString)
	if err == nil {
		t.Fatal("expected verification to fail for unreachable endpoint")
	}
	// Should fail gracefully with a network/connection error, not panic
	if strings.Contains(err.Error(), "panic") {
		t.Errorf("verification panicked: %v", err)
	}
}

// TestVerifyJWKSCaching: JWKS is cached, second call doesn't hit server
func TestVerifyJWKSCaching(t *testing.T) {
	// Generate RSA key pair
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate RSA key: %v", err)
	}

	jwk := createRS256JWK(&privKey.PublicKey, "cache-test-kid")
	server, callCount := createMockJWKSServer([]map[string]interface{}{jwk})
	defer server.Close()

	// Create two valid tokens
	now := time.Now().Unix()
	newToken := func() string {
		token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
			"iat":    now,
			"exp":    now + 3600,
			"tenant": "tenant5",
		})
		token.Header["kid"] = "cache-test-kid"
		tokenString, _ := token.SignedString(privKey)
		return tokenString
	}

	config := &Config{
		JWKSEndpoint: server.URL,
		Algorithm:    "RS256",
	}

	// First verify should hit the server
	token1 := newToken()
	_, err = config.Verify(token1)
	if err != nil {
		t.Fatalf("first verification failed: %v", err)
	}
	firstCount := atomic.LoadInt32(callCount)

	// Second verify within the 5-minute cache window should NOT hit server
	token2 := newToken()
	_, err = config.Verify(token2)
	if err != nil {
		t.Fatalf("second verification failed: %v", err)
	}
	secondCount := atomic.LoadInt32(callCount)

	if firstCount != 1 || secondCount != 1 {
		t.Errorf("JWKS cache not working: expected 1 server call total, got firstCount=%d, secondCount=%d", firstCount, secondCount)
	}
}

// TestVerifyES256SignatureTampering: token signed with different key than in JWKS
func TestVerifyES256SignatureTampering(t *testing.T) {
	// Generate TWO ECDSA key pairs
	correctKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate correct ECDSA key: %v", err)
	}

	tamperedKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("failed to generate tampered ECDSA key: %v", err)
	}

	// JWKS contains the correct key
	correctJWK := createES256JWK(&correctKey.PublicKey, "es256-sig-test")
	server, _ := createMockJWKSServer([]map[string]interface{}{correctJWK})
	defer server.Close()

	// Create a token SIGNED with the tampered key, but claim it's the correct key
	now := time.Now().Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodES256, jwt.MapClaims{
		"iat":    now,
		"exp":    now + 3600,
		"tenant": "tenant6",
	})
	token.Header["kid"] = "es256-sig-test"
	// Sign with tamperedKey, not correctKey
	tokenString, _ := token.SignedString(tamperedKey)

	config := &Config{
		JWKSEndpoint: server.URL,
		Algorithm:    "ES256",
	}

	_, err = config.Verify(tokenString)
	if err == nil {
		t.Fatal("expected verification to fail for tampered signature")
	}
	if !strings.Contains(err.Error(), "signature") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected signature/invalid error, got: %v", err)
	}
}

// TestVerifyRS256SignatureTampering: RS256 token signed with different key than in JWKS
func TestVerifyRS256SignatureTampering(t *testing.T) {
	// Generate TWO RSA key pairs
	correctKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate correct RSA key: %v", err)
	}

	tamperedKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("failed to generate tampered RSA key: %v", err)
	}

	// JWKS contains the correct key
	correctJWK := createRS256JWK(&correctKey.PublicKey, "rs256-sig-test")
	server, _ := createMockJWKSServer([]map[string]interface{}{correctJWK})
	defer server.Close()

	// Create a token SIGNED with the tampered key, but claim it's the correct key
	now := time.Now().Unix()
	token := jwt.NewWithClaims(jwt.SigningMethodRS256, jwt.MapClaims{
		"iat":    now,
		"exp":    now + 3600,
		"tenant": "tenant7",
	})
	token.Header["kid"] = "rs256-sig-test"
	// Sign with tamperedKey, not correctKey
	tokenString, _ := token.SignedString(tamperedKey)

	config := &Config{
		JWKSEndpoint: server.URL,
		Algorithm:    "RS256",
	}

	_, err = config.Verify(tokenString)
	if err == nil {
		t.Fatal("expected verification to fail for tampered signature")
	}
	if !strings.Contains(err.Error(), "signature") && !strings.Contains(err.Error(), "invalid") {
		t.Errorf("expected signature/invalid error, got: %v", err)
	}
}
