package claims

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestParseToken_EmptyJWKSURL(t *testing.T) {
	// Reset cache to avoid cross-test pollution
	globalJWKSCache.mu.Lock()
	globalJWKSCache.data = nil
	globalJWKSCache.mu.Unlock()

	token := makeSimpleTokenString("user1", "tenant1", time.Now().Unix()+3600)
	_, err := ParseToken(token, "")
	if err == nil {
		t.Error("ParseToken() with empty JWKS URL should return error")
	}
}

func TestParseToken_MalformedJWT(t *testing.T) {
	tests := []struct {
		name  string
		token string
	}{
		{"not 3 parts", "invalid.token"},
		{"2 parts", "header.payload"},
		{"4 parts", "head.er.pay.load"},
		{"empty parts", ".."},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := ParseToken(tt.token, "http://example.com/.well-known/jwks.json")
			if err == nil {
				t.Errorf("ParseToken() with malformed JWT should return error")
			}
		})
	}
}

func TestParseToken_ValidRS256(t *testing.T) {
	// Generate test RSA key pair
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}
	pubKey := &privKey.PublicKey

	// Create JWKS response
	nBytes := pubKey.N.Bytes()
	eBytes := big.NewInt(int64(pubKey.E)).Bytes()

	jwksResp := jwksResponse{
		Keys: []jwksKey{
			{
				Kty: "RSA",
				Kid: "key1",
				Alg: "RS256",
				N:   base64.RawURLEncoding.EncodeToString(nBytes),
				E:   base64.RawURLEncoding.EncodeToString(eBytes),
			},
		},
	}

	// Start mock JWKS server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwksResp)
	}))
	defer server.Close()

	// Create and sign token
	token, err := createRS256Token(privKey, "user1", "tenant1", "key1", time.Now().Unix()+3600)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	// Parse and verify
	claims, err := ParseToken(token, server.URL)
	if err != nil {
		t.Errorf("ParseToken() with valid RS256 signature failed: %v", err)
		return
	}

	if claims.Subject != "user1" || claims.Tenant != "tenant1" {
		t.Errorf("ParseToken() extracted wrong claims: %+v", claims)
	}
}

func TestParseToken_ValidES256(t *testing.T) {
	// Reset cache to avoid cross-test pollution
	globalJWKSCache.mu.Lock()
	globalJWKSCache.data = nil
	globalJWKSCache.mu.Unlock()

	// Generate test EC key pair
	privKey, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("Failed to generate EC key: %v", err)
	}
	pubKey := &privKey.PublicKey

	// Create JWKS response
	xBytes := pubKey.X.Bytes()
	yBytes := pubKey.Y.Bytes()
	// Pad to 32 bytes for P-256
	xBytes = padTo32(xBytes)
	yBytes = padTo32(yBytes)

	jwksResp := jwksResponse{
		Keys: []jwksKey{
			{
				Kty: "EC",
				Kid: "key1",
				Alg: "ES256",
				Crv: "P-256",
				X:   base64.RawURLEncoding.EncodeToString(xBytes),
				Y:   base64.RawURLEncoding.EncodeToString(yBytes),
			},
		},
	}

	// Start mock JWKS server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwksResp)
	}))
	defer server.Close()

	// Create and sign token
	token, err := createES256Token(privKey, "user2", "tenant2", "key1", time.Now().Unix()+3600)
	if err != nil {
		t.Fatalf("Failed to create token: %v", err)
	}

	// Parse and verify
	claims, err := ParseToken(token, server.URL)
	if err != nil {
		t.Errorf("ParseToken() with valid ES256 signature failed: %v", err)
		return
	}

	if claims.Subject != "user2" || claims.Tenant != "tenant2" {
		t.Errorf("ParseToken() extracted wrong claims: %+v", claims)
	}
}

func TestParseToken_InvalidSignature(t *testing.T) {
	// Create a token signed with one key
	privKey1, _ := rsa.GenerateKey(rand.Reader, 2048)
	token, _ := createRS256Token(privKey1, "user1", "tenant1", "key1", time.Now().Unix()+3600)

	// But serve JWKS with different key
	privKey2, _ := rsa.GenerateKey(rand.Reader, 2048)
	pubKey2 := &privKey2.PublicKey

	jwksResp := jwksResponse{
		Keys: []jwksKey{
			{
				Kty: "RSA",
				Kid: "key1",
				Alg: "RS256",
				N:   base64.RawURLEncoding.EncodeToString(pubKey2.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pubKey2.E)).Bytes()),
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwksResp)
	}))
	defer server.Close()

	_, err := ParseToken(token, server.URL)
	if err == nil {
		t.Error("ParseToken() should reject token with invalid signature")
	}
}

func TestParseToken_ExpiredToken(t *testing.T) {
	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	pubKey := &privKey.PublicKey

	// Create token that expired 1 hour ago
	token, _ := createRS256Token(privKey, "user1", "tenant1", "key1", time.Now().Unix()-3600)

	jwksResp := jwksResponse{
		Keys: []jwksKey{
			{
				Kty: "RSA",
				Kid: "key1",
				Alg: "RS256",
				N:   base64.RawURLEncoding.EncodeToString(pubKey.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pubKey.E)).Bytes()),
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwksResp)
	}))
	defer server.Close()

	_, err := ParseToken(token, server.URL)
	if err == nil {
		t.Error("ParseToken() should reject expired token")
	}
}

func TestParseToken_FutureToken(t *testing.T) {
	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	pubKey := &privKey.PublicKey

	// Create token issued 2 hours in the future
	token, _ := createRS256Token(privKey, "user1", "tenant1", "key1", time.Now().Unix()+7200)
	// But set iat to future time
	parts := parseTokenParts(token)
	var payload map[string]interface{}
	json.Unmarshal(base64urlDecode(parts[1]), &payload)
	payload["iat"] = float64(time.Now().Unix() + 7200)
	payloadJSON, _ := json.Marshal(payload)
	newPayload := base64.RawURLEncoding.EncodeToString(payloadJSON)

	// Re-sign with correct signature
	message := parts[0] + "." + newPayload
	hash := sha256.Sum256([]byte(message))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, privKey, 0, hash[:])
	newSig := base64.RawURLEncoding.EncodeToString(sig)
	newToken := parts[0] + "." + newPayload + "." + newSig

	jwksResp := jwksResponse{
		Keys: []jwksKey{
			{
				Kty: "RSA",
				Kid: "key1",
				Alg: "RS256",
				N:   base64.RawURLEncoding.EncodeToString(pubKey.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pubKey.E)).Bytes()),
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwksResp)
	}))
	defer server.Close()

	_, err := ParseToken(newToken, server.URL)
	if err == nil {
		t.Error("ParseToken() should reject token issued in the future")
	}
}

func TestParseToken_JWKSCaching(t *testing.T) {
	// Reset cache to ensure clean state
	globalJWKSCache.mu.Lock()
	globalJWKSCache.data = nil
	globalJWKSCache.mu.Unlock()

	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)
	pubKey := &privKey.PublicKey

	fetchCount := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fetchCount++
		jwksResp := jwksResponse{
			Keys: []jwksKey{
				{
					Kty: "RSA",
					Kid: "key1",
					Alg: "RS256",
					N:   base64.RawURLEncoding.EncodeToString(pubKey.N.Bytes()),
					E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pubKey.E)).Bytes()),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwksResp)
	}))
	defer server.Close()

	// Create first token
	token1, _ := createRS256Token(privKey, "user1", "tenant1", "key1", time.Now().Unix()+3600)

	// Parse first token (should fetch JWKS)
	_, err1 := ParseToken(token1, server.URL)
	if err1 != nil {
		t.Fatalf("First ParseToken failed: %v", err1)
	}
	firstFetchCount := fetchCount

	// Create second token
	token2, _ := createRS256Token(privKey, "user2", "tenant2", "key1", time.Now().Unix()+3600)

	// Parse second token (should use cached JWKS)
	_, err2 := ParseToken(token2, server.URL)
	if err2 != nil {
		t.Fatalf("Second ParseToken failed: %v", err2)
	}
	secondFetchCount := fetchCount

	if firstFetchCount != 1 || secondFetchCount != 1 {
		t.Errorf("JWKS should be cached: fetch count went from %d to %d (expected 1 fetch total)", firstFetchCount, secondFetchCount)
	}
}

func TestParseToken_KeyIDNotFound(t *testing.T) {
	privKey, _ := rsa.GenerateKey(rand.Reader, 2048)

	// Create token with kid "key1"
	token, _ := createRS256Token(privKey, "user1", "tenant1", "key1", time.Now().Unix()+3600)

	// Serve JWKS with different kid
	pubKey := &privKey.PublicKey
	jwksResp := jwksResponse{
		Keys: []jwksKey{
			{
				Kty: "RSA",
				Kid: "different-key",
				Alg: "RS256",
				N:   base64.RawURLEncoding.EncodeToString(pubKey.N.Bytes()),
				E:   base64.RawURLEncoding.EncodeToString(big.NewInt(int64(pubKey.E)).Bytes()),
			},
		},
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwksResp)
	}))
	defer server.Close()

	_, err := ParseToken(token, server.URL)
	if err == nil {
		t.Error("ParseToken() should reject token with unknown key ID")
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

// Helper functions for test JWT creation
func padTo32(b []byte) []byte {
	if len(b) == 32 {
		return b
	}
	padded := make([]byte, 32)
	copy(padded[32-len(b):], b)
	return padded
}

func parseTokenParts(token string) []string {
	return strings.Split(token, ".")
}

func base64urlDecode(s string) []byte {
	// RawURLEncoding doesn't need padding
	b, _ := base64.RawURLEncoding.DecodeString(s)
	return b
}

func makeSimpleTokenString(sub, tenant string, exp int64) string {
	header := `{"alg":"HS256","typ":"JWT"}`
	payload := `{"sub":"` + sub + `","tenant":"` + tenant + `","exp":` + itoa(exp) + `}`
	return base64.RawURLEncoding.EncodeToString([]byte(header)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".sig"
}

func createRS256Token(privKey *rsa.PrivateKey, sub, tenant, kid string, exp int64) (string, error) {
	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
		"kid": kid,
	}
	payload := map[string]interface{}{
		"sub":    sub,
		"tenant": tenant,
		"exp":    float64(exp),
		"iat":    float64(time.Now().Unix()),
	}

	headerJSON, _ := json.Marshal(header)
	payloadJSON, _ := json.Marshal(payload)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	message := headerB64 + "." + payloadB64
	hash := sha256.Sum256([]byte(message))

	sig, err := rsa.SignPKCS1v15(rand.Reader, privKey, 0, hash[:])
	if err != nil {
		return "", err
	}

	sigB64 := base64.RawURLEncoding.EncodeToString(sig)
	return message + "." + sigB64, nil
}

func createES256Token(privKey *ecdsa.PrivateKey, sub, tenant, kid string, exp int64) (string, error) {
	header := map[string]string{
		"alg": "ES256",
		"typ": "JWT",
		"kid": kid,
	}
	payload := map[string]interface{}{
		"sub":    sub,
		"tenant": tenant,
		"exp":    float64(exp),
		"iat":    float64(time.Now().Unix()),
	}

	headerJSON, _ := json.Marshal(header)
	payloadJSON, _ := json.Marshal(payload)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerJSON)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadJSON)

	message := headerB64 + "." + payloadB64
	hash := sha256.Sum256([]byte(message))

	r, s, err := ecdsa.Sign(rand.Reader, privKey, hash[:])
	if err != nil {
		return "", err
	}

	// Encode r and s as 32-byte big-endian
	rBytes := r.Bytes()
	sBytes := s.Bytes()
	rBytes = padTo32(rBytes)
	sBytes = padTo32(sBytes)

	sigBytes := append(rBytes, sBytes...)
	sigB64 := base64.RawURLEncoding.EncodeToString(sigBytes)

	return message + "." + sigB64, nil
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
