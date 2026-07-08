package main

import (
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const (
	testJWTAlgorithm    = "HS256"
	testJWTSharedSecret = "test-secret-key-for-jwt-signing"
	testJWTIssuer       = "test-issuer"
	testJWTAudience     = "test-audience"
)

// generateTestJWT creates a valid HS256 JWT token for testing.
// Includes required claims: sub, iss, aud, iat, exp, scope, tenant.
func generateTestJWT(subject, tenant, scope string) (string, error) {
	now := time.Now()
	claims := jwt.MapClaims{
		"sub":    subject,
		"iss":    testJWTIssuer,
		"aud":    []string{testJWTAudience},
		"iat":    now.Unix(),
		"exp":    now.Add(1 * time.Hour).Unix(),
		"tenant": tenant,
		"scope":  scope,
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	return token.SignedString([]byte(testJWTSharedSecret))
}
