package claims

import (
	"context"
	"crypto"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net/http"
	"strings"
	"sync"
	"time"
)

type contextKey struct{}

// Claims holds the parsed JWT claims relevant to the data proxy.
type Claims struct {
	Subject  string   `json:"sub"`
	Tenant   string   `json:"tenant"`
	Scopes   []string `json:"scope"`
	Roles    []string `json:"roles"`
	IssuedAt int64    `json:"iat"`
	Expires  int64    `json:"exp"`
}

// JWKS caching with 5-minute TTL
type jwksCache struct {
	data      *jwksResponse
	fetchedAt time.Time
	mu        sync.RWMutex
}

var globalJWKSCache = &jwksCache{}

type jwksKey struct {
	Kty string `json:"kty"`
	Kid string `json:"kid"`
	Use string `json:"use"`
	Alg string `json:"alg"`
	N   string `json:"n"`   // RSA modulus (base64url)
	E   string `json:"e"`   // RSA exponent (base64url)
	X   string `json:"x"`   // EC x (base64url)
	Y   string `json:"y"`   // EC y (base64url)
	Crv string `json:"crv"` // EC curve
}

type jwksResponse struct {
	Keys []jwksKey `json:"keys"`
}

type jwtHeader struct {
	Alg string `json:"alg"`
	Kid string `json:"kid"`
	Typ string `json:"typ"`
}

// base64urlDecodeNoPad decodes a base64url string without padding.
func base64urlDecodeNoPad(s string) ([]byte, error) {
	// RawURLEncoding doesn't support padding, so decode as-is
	// Padding is optional for base64url
	return base64.RawURLEncoding.DecodeString(s)
}

// fetchJWKS fetches JWKS from the endpoint with 30s timeout.
func fetchJWKS(jwksURL string) (*jwksResponse, error) {
	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Get(jwksURL)
	if err != nil {
		return nil, fmt.Errorf("fetch JWKS: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("JWKS endpoint returned %d: %s", resp.StatusCode, body)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read JWKS response: %w", err)
	}

	var jwks jwksResponse
	if err := json.Unmarshal(body, &jwks); err != nil {
		return nil, fmt.Errorf("unmarshal JWKS: %w", err)
	}

	return &jwks, nil
}

// getJWKS returns cached JWKS or fetches fresh data if cache expired.
func getJWKS(jwksURL string) (*jwksResponse, error) {
	globalJWKSCache.mu.RLock()
	if globalJWKSCache.data != nil && time.Since(globalJWKSCache.fetchedAt) < 5*time.Minute {
		defer globalJWKSCache.mu.RUnlock()
		return globalJWKSCache.data, nil
	}
	globalJWKSCache.mu.RUnlock()

	// Fetch fresh data
	jwks, err := fetchJWKS(jwksURL)
	if err != nil {
		return nil, err
	}

	// Update cache
	globalJWKSCache.mu.Lock()
	globalJWKSCache.data = jwks
	globalJWKSCache.fetchedAt = time.Now()
	globalJWKSCache.mu.Unlock()

	return jwks, nil
}

// verifyRS256 verifies RS256 signature using RSA public key.
func verifyRS256(header, payload, signature string, key jwksKey) error {
	// Validate key type
	if key.Kty != "RSA" {
		return fmt.Errorf("RS256 requires RSA key, got %s", key.Kty)
	}

	// Decode modulus and exponent
	nBytes, err := base64urlDecodeNoPad(key.N)
	if err != nil {
		return fmt.Errorf("decode RSA modulus: %w", err)
	}
	eBytes, err := base64urlDecodeNoPad(key.E)
	if err != nil {
		return fmt.Errorf("decode RSA exponent: %w", err)
	}

	// Convert exponent bytes to int
	var e int64
	for _, b := range eBytes {
		e = (e << 8) | int64(b)
	}

	// Construct RSA public key
	n := new(big.Int).SetBytes(nBytes)
	pubKey := &rsa.PublicKey{N: n, E: int(e)}

	// Decode signature
	sigBytes, err := base64urlDecodeNoPad(signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	// Create message to verify
	message := header + "." + payload
	hash := sha256.Sum256([]byte(message))

	// Verify signature using PKCS#1 v1.5 with SHA256
	// (use crypto.SHA256 instead of 0 to ensure proper digest binding)
	if err := rsa.VerifyPKCS1v15(pubKey, crypto.SHA256, hash[:], sigBytes); err != nil {
		return fmt.Errorf("RS256 signature verification failed: %w", err)
	}

	return nil
}

// verifyES256 verifies ES256 signature using EC public key.
func verifyES256(header, payload, signature string, key jwksKey) error {
	// Decode x and y coordinates
	xBytes, err := base64urlDecodeNoPad(key.X)
	if err != nil {
		return fmt.Errorf("decode EC x: %w", err)
	}
	yBytes, err := base64urlDecodeNoPad(key.Y)
	if err != nil {
		return fmt.Errorf("decode EC y: %w", err)
	}

	// Construct EC public key
	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)
	pubKey := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}

	// Decode signature (r and s, each 32 bytes for P-256)
	sigBytes, err := base64urlDecodeNoPad(signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if len(sigBytes) != 64 {
		return fmt.Errorf("invalid ES256 signature length: expected 64, got %d", len(sigBytes))
	}

	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:])

	// Create message hash
	message := header + "." + payload
	hash := sha256.Sum256([]byte(message))

	// Verify signature
	if !ecdsa.Verify(pubKey, hash[:], r, s) {
		return fmt.Errorf("ES256 signature verification failed")
	}

	return nil
}

// verifySignature verifies JWT signature against the appropriate key.
func verifySignature(header, payload, signature string, hdr jwtHeader, jwks *jwksResponse) error {
	// Find key by kid
	var key *jwksKey
	for i := range jwks.Keys {
		if jwks.Keys[i].Kid == hdr.Kid {
			key = &jwks.Keys[i]
			break
		}
	}
	if key == nil {
		return fmt.Errorf("key ID %q not found in JWKS", hdr.Kid)
	}

	switch hdr.Alg {
	case "RS256":
		if key.Kty != "RSA" {
			return fmt.Errorf("RS256 requires RSA key, got %s", key.Kty)
		}
		return verifyRS256(header, payload, signature, *key)
	case "ES256":
		if key.Kty != "EC" {
			return fmt.Errorf("ES256 requires EC key, got %s", key.Kty)
		}
		return verifyES256(header, payload, signature, *key)
	default:
		return fmt.Errorf("unsupported algorithm: %s", hdr.Alg)
	}
}

// ParseToken parses and validates a JWT against the JWKS endpoint.
// jwksURL is the /.well-known/jwks.json URL of the auth server.
// expectedAud is the expected audience claim (e.g., "my-service").
// expectedIss is the expected issuer claim (e.g., "https://auth.example.com").
// Returns Claims only if signature is valid and all required claims are present and correct.
func ParseToken(token, jwksURL, expectedAud, expectedIss string) (*Claims, error) {
	if jwksURL == "" {
		return nil, fmt.Errorf("JWKS URL cannot be empty")
	}
	if expectedAud == "" {
		return nil, fmt.Errorf("expected audience cannot be empty")
	}
	if expectedIss == "" {
		return nil, fmt.Errorf("expected issuer cannot be empty")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed JWT: expected 3 parts, got %d", len(parts))
	}

	header := parts[0]
	payload := parts[1]
	signature := parts[2]

	// Decode and parse header
	headerBytes, err := base64urlDecodeNoPad(header)
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}

	var hdr jwtHeader
	if err := json.Unmarshal(headerBytes, &hdr); err != nil {
		return nil, fmt.Errorf("unmarshal header: %w", err)
	}

	// Reject "none" algorithm and enforce RS256/ES256
	if hdr.Alg == "none" {
		return nil, fmt.Errorf("algorithm 'none' is not allowed")
	}
	if hdr.Alg != "RS256" && hdr.Alg != "ES256" {
		return nil, fmt.Errorf("algorithm %q is not supported", hdr.Alg)
	}

	// Decode payload
	payloadBytes, err := base64urlDecodeNoPad(payload)
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	// Fetch JWKS and verify signature
	jwks, err := getJWKS(jwksURL)
	if err != nil {
		return nil, fmt.Errorf("get JWKS: %w", err)
	}

	if err := verifySignature(header, payload, signature, hdr, jwks); err != nil {
		return nil, fmt.Errorf("verify signature: %w", err)
	}

	// Parse claims
	var raw map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	// Validate issuer (iss)
	iss, ok := raw["iss"].(string)
	if !ok || iss == "" {
		return nil, fmt.Errorf("token missing required 'iss' claim")
	}
	if iss != expectedIss {
		return nil, fmt.Errorf("token issuer %q does not match expected %q", iss, expectedIss)
	}

	// Validate audience (aud)
	var aud string
	switch v := raw["aud"].(type) {
	case string:
		aud = v
	case []interface{}:
		// aud can be an array; check if expectedAud is in it
		for _, a := range v {
			if audStr, ok := a.(string); ok && audStr == expectedAud {
				aud = expectedAud
				break
			}
		}
	}
	if aud == "" || aud != expectedAud {
		return nil, fmt.Errorf("token audience does not match expected %q", expectedAud)
	}

	// Validate expiration (exp) — REQUIRED
	expV, ok := raw["exp"].(float64)
	if !ok {
		return nil, fmt.Errorf("token missing required 'exp' claim")
	}
	exp := int64(expV)
	if time.Now().Unix() > exp {
		return nil, fmt.Errorf("token expired")
	}

	// Validate not-before (nbf) if present
	if nbfV, ok := raw["nbf"].(float64); ok {
		nbf := int64(nbfV)
		if time.Now().Unix() < nbf {
			return nil, fmt.Errorf("token not valid yet (nbf claim)")
		}
	}

	// Validate issued-at (iat) — reject tokens issued in the future by >30s
	cl := &Claims{}
	if v, ok := raw["iat"].(float64); ok {
		cl.IssuedAt = int64(v)
		if cl.IssuedAt > time.Now().Unix()+30 {
			return nil, fmt.Errorf("token issued in the future")
		}
	}

	// Extract standard claims
	if v, ok := raw["sub"].(string); ok {
		cl.Subject = v
	}
	if v, ok := raw["tenant"].(string); ok {
		cl.Tenant = v
	}
	cl.Expires = exp

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
