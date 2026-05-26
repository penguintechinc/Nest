package auth

import (
	"context"
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

// Claims holds the parsed JWT claims.
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

func base64urlDecodeNoPad(s string) ([]byte, error) {
	return base64.RawURLEncoding.DecodeString(s)
}

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

func getJWKS(jwksURL string) (*jwksResponse, error) {
	globalJWKSCache.mu.RLock()
	if globalJWKSCache.data != nil && time.Since(globalJWKSCache.fetchedAt) < 5*time.Minute {
		defer globalJWKSCache.mu.RUnlock()
		return globalJWKSCache.data, nil
	}
	globalJWKSCache.mu.RUnlock()

	jwks, err := fetchJWKS(jwksURL)
	if err != nil {
		return nil, err
	}

	globalJWKSCache.mu.Lock()
	globalJWKSCache.data = jwks
	globalJWKSCache.fetchedAt = time.Now()
	globalJWKSCache.mu.Unlock()

	return jwks, nil
}

func verifyRS256(header, payload, signature string, key jwksKey) error {
	nBytes, err := base64urlDecodeNoPad(key.N)
	if err != nil {
		return fmt.Errorf("decode RSA modulus: %w", err)
	}
	eBytes, err := base64urlDecodeNoPad(key.E)
	if err != nil {
		return fmt.Errorf("decode RSA exponent: %w", err)
	}

	var e int64
	for _, b := range eBytes {
		e = (e << 8) | int64(b)
	}

	n := new(big.Int).SetBytes(nBytes)
	pubKey := &rsa.PublicKey{N: n, E: int(e)}

	sigBytes, err := base64urlDecodeNoPad(signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}

	message := header + "." + payload
	hash := sha256.Sum256([]byte(message))

	if err := rsa.VerifyPKCS1v15(pubKey, 0, hash[:], sigBytes); err != nil {
		return fmt.Errorf("RS256 signature verification failed: %w", err)
	}

	return nil
}

func verifyES256(header, payload, signature string, key jwksKey) error {
	xBytes, err := base64urlDecodeNoPad(key.X)
	if err != nil {
		return fmt.Errorf("decode EC x: %w", err)
	}
	yBytes, err := base64urlDecodeNoPad(key.Y)
	if err != nil {
		return fmt.Errorf("decode EC y: %w", err)
	}

	x := new(big.Int).SetBytes(xBytes)
	y := new(big.Int).SetBytes(yBytes)
	pubKey := &ecdsa.PublicKey{
		Curve: elliptic.P256(),
		X:     x,
		Y:     y,
	}

	sigBytes, err := base64urlDecodeNoPad(signature)
	if err != nil {
		return fmt.Errorf("decode signature: %w", err)
	}
	if len(sigBytes) != 64 {
		return fmt.Errorf("invalid ES256 signature length: expected 64, got %d", len(sigBytes))
	}

	r := new(big.Int).SetBytes(sigBytes[:32])
	s := new(big.Int).SetBytes(sigBytes[32:])

	message := header + "." + payload
	hash := sha256.Sum256([]byte(message))

	if !ecdsa.Verify(pubKey, hash[:], r, s) {
		return fmt.Errorf("ES256 signature verification failed")
	}

	return nil
}

func verifySignature(header, payload, signature string, hdr jwtHeader, jwks *jwksResponse) error {
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
func ParseToken(token, jwksURL string) (*Claims, error) {
	if jwksURL == "" {
		return nil, fmt.Errorf("JWKS URL cannot be empty")
	}

	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return nil, fmt.Errorf("malformed JWT: expected 3 parts, got %d", len(parts))
	}

	header := parts[0]
	payload := parts[1]
	signature := parts[2]

	headerBytes, err := base64urlDecodeNoPad(header)
	if err != nil {
		return nil, fmt.Errorf("decode header: %w", err)
	}

	var hdr jwtHeader
	if err := json.Unmarshal(headerBytes, &hdr); err != nil {
		return nil, fmt.Errorf("unmarshal header: %w", err)
	}

	payloadBytes, err := base64urlDecodeNoPad(payload)
	if err != nil {
		return nil, fmt.Errorf("decode payload: %w", err)
	}

	jwks, err := getJWKS(jwksURL)
	if err != nil {
		return nil, fmt.Errorf("get JWKS: %w", err)
	}

	if err := verifySignature(header, payload, signature, hdr, jwks); err != nil {
		return nil, fmt.Errorf("verify signature: %w", err)
	}

	var raw map[string]interface{}
	if err := json.Unmarshal(payloadBytes, &raw); err != nil {
		return nil, fmt.Errorf("unmarshal payload: %w", err)
	}

	cl := &Claims{}
	if v, ok := raw["sub"].(string); ok {
		cl.Subject = v
	}
	if v, ok := raw["tenant"].(string); ok {
		cl.Tenant = v
	}
	if v, ok := raw["iat"].(float64); ok {
		cl.IssuedAt = int64(v)
		if cl.IssuedAt > time.Now().Unix()+30 {
			return nil, fmt.Errorf("token issued in the future")
		}
	}
	if v, ok := raw["exp"].(float64); ok {
		cl.Expires = int64(v)
		if cl.Expires > 0 && time.Now().Unix() > cl.Expires {
			return nil, fmt.Errorf("token expired")
		}
	}

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

func (c *Claims) HasScope(scope string) bool {
	for _, s := range c.Scopes {
		if s == scope {
			return true
		}
	}
	return false
}

func ResetCacheForTesting() {
	globalJWKSCache.mu.Lock()
	globalJWKSCache.data = nil
	globalJWKSCache.fetchedAt = time.Time{}
	globalJWKSCache.mu.Unlock()
}
