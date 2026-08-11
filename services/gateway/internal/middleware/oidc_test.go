package middleware

import (
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/penguintechinc/nest/services/gateway/internal/claims"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
)

var (
	testPrivKey *rsa.PrivateKey
	testJWKSURL string
)

func TestMain(m *testing.M) {
	// Generate RSA-2048 key pair
	privKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		panic(fmt.Sprintf("failed to generate RSA key: %v", err))
	}
	testPrivKey = privKey

	// Create JWKS endpoint serving the public key
	jwksHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Encode RSA public key to JWKS format
		nBytes := testPrivKey.PublicKey.N.Bytes()
		eBytes := big.NewInt(int64(testPrivKey.PublicKey.E)).Bytes()

		jwks := map[string]interface{}{
			"keys": []map[string]string{
				{
					"kty": "RSA",
					"kid": "test-key",
					"alg": "RS256",
					"use": "sig",
					"n":   base64.RawURLEncoding.EncodeToString(nBytes),
					"e":   base64.RawURLEncoding.EncodeToString(eBytes),
				},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(jwks)
	})

	server := httptest.NewServer(jwksHandler)
	testJWKSURL = server.URL

	// Run tests
	_ = m.Run()

	// Cleanup
	server.Close()
}

func TestOIDCUnaryInterceptor(t *testing.T) {
	logger := zap.NewNop()
	cfg := config.Config{OIDCJwksURL: testJWKSURL, OIDCAudience: "nest", OIDCIssuer: "https://test-issuer"}

	tests := []struct {
		name      string
		metadata  metadata.MD
		wantErr   bool
		errCode   codes.Code
		wantClaim bool
	}{
		{
			name:     "missing metadata",
			metadata: nil,
			wantErr:  true,
			errCode:  codes.Unauthenticated,
		},
		{
			name:     "missing authorization header",
			metadata: metadata.Pairs(),
			wantErr:  true,
			errCode:  codes.Unauthenticated,
		},
		{
			name:     "invalid authorization scheme",
			metadata: metadata.Pairs("authorization", "Basic user:pass"),
			wantErr:  true,
			errCode:  codes.Unauthenticated,
		},
		{
			name:     "valid token without tenant",
			metadata: metadata.Pairs("authorization", "Bearer "+makeValidToken("user1", "", 0)),
			wantErr:  true,
			errCode:  codes.Unauthenticated,
		},
		{
			name:      "valid token with tenant",
			metadata:  metadata.Pairs("authorization", "Bearer "+makeValidToken("user1", "tenant1", 0)),
			wantErr:   false,
			wantClaim: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims.ResetCacheForTesting()
			interceptor := OIDCUnaryInterceptor(cfg, logger)

			ctx := context.Background()
			if tt.metadata != nil {
				ctx = metadata.NewIncomingContext(ctx, tt.metadata)
			}

			handler := func(ctx context.Context, req interface{}) (interface{}, error) {
				return "response", nil
			}

			info := &grpc.UnaryServerInfo{FullMethod: "/test"}

			_, err := interceptor(ctx, nil, info, handler)

			if (err != nil) != tt.wantErr {
				t.Errorf("OIDCUnaryInterceptor() error = %v, wantErr %v", err, tt.wantErr)
			}

			if tt.wantErr && err != nil {
				st := status.Convert(err)
				if st.Code() != tt.errCode {
					t.Errorf("OIDCUnaryInterceptor() error code = %v, want %v", st.Code(), tt.errCode)
				}
			}
		})
	}
}

func TestOIDCHTTPMiddleware(t *testing.T) {
	logger := zap.NewNop()
	cfg := config.Config{OIDCJwksURL: testJWKSURL, OIDCAudience: "nest", OIDCIssuer: "https://test-issuer"}

	tests := []struct {
		name       string
		authHeader string
		wantStatus int
	}{
		{
			name:       "missing authorization header",
			authHeader: "",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "invalid authorization scheme",
			authHeader: "Basic user:pass",
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "valid token without tenant",
			authHeader: "Bearer " + makeValidToken("user1", "", 0),
			wantStatus: http.StatusUnauthorized,
		},
		{
			name:       "valid token with tenant",
			authHeader: "Bearer " + makeValidToken("user1", "tenant1", 0),
			wantStatus: http.StatusOK,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			claims.ResetCacheForTesting()
			nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(http.StatusOK)
				w.Write([]byte("ok"))
			})

			middleware := OIDCHTTPMiddleware(cfg, logger, nextHandler)

			req := httptest.NewRequest("GET", "/api/test", nil)
			if tt.authHeader != "" {
				req.Header.Set("Authorization", tt.authHeader)
			}

			w := httptest.NewRecorder()
			middleware.ServeHTTP(w, req)

			if w.Code != tt.wantStatus {
				t.Errorf("OIDCHTTPMiddleware() status = %d, want %d", w.Code, tt.wantStatus)
			}
		})
	}
}

func TestOIDCHTTPMiddleware_ContextClaims(t *testing.T) {
	claims.ResetCacheForTesting()
	logger := zap.NewNop()
	cfg := config.Config{OIDCJwksURL: testJWKSURL, OIDCAudience: "nest", OIDCIssuer: "https://test-issuer"}

	var receivedClaims *claims.Claims
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var ok bool
		receivedClaims, ok = claims.FromContext(r.Context())
		if !ok {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
	})

	middleware := OIDCHTTPMiddleware(cfg, logger, nextHandler)

	token := makeValidToken("user1", "tenant1", 0)
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", w.Code, http.StatusOK)
	}

	if receivedClaims == nil {
		t.Error("claims not set in context")
		return
	}

	if receivedClaims.Subject != "user1" || receivedClaims.Tenant != "tenant1" {
		t.Errorf("claims = %+v, want Subject=user1, Tenant=tenant1", receivedClaims)
	}
}

func TestOIDCUnaryInterceptor_InvalidToken(t *testing.T) {
	claims.ResetCacheForTesting()
	logger := zap.NewNop()
	cfg := config.Config{OIDCJwksURL: testJWKSURL, OIDCAudience: "nest", OIDCIssuer: "https://test-issuer"}
	interceptor := OIDCUnaryInterceptor(cfg, logger)

	// Expired token
	token := makeValidToken("user1", "tenant1", -1000)

	md := metadata.New(map[string]string{
		"authorization": "Bearer " + token,
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	})

	if err == nil {
		t.Error("expected error for expired token")
	}
	if st, ok := status.FromError(err); ok {
		if st.Code() != codes.Unauthenticated {
			t.Errorf("code = %v, want Unauthenticated", st.Code())
		}
	}
}

func TestOIDCHTTPMiddleware_InvalidToken(t *testing.T) {
	claims.ResetCacheForTesting()
	logger := zap.NewNop()
	cfg := config.Config{OIDCJwksURL: testJWKSURL, OIDCAudience: "nest", OIDCIssuer: "https://test-issuer"}

	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	middleware := OIDCHTTPMiddleware(cfg, logger, nextHandler)

	// Expired token
	token := makeValidToken("user1", "tenant1", -1000)
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if nextCalled {
		t.Error("next handler should not be called for invalid token")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

func TestOIDCUnaryInterceptor_MissingTenantClaim(t *testing.T) {
	claims.ResetCacheForTesting()
	logger := zap.NewNop()
	cfg := config.Config{OIDCJwksURL: testJWKSURL, OIDCAudience: "nest", OIDCIssuer: "https://test-issuer"}
	interceptor := OIDCUnaryInterceptor(cfg, logger)

	// Token with no tenant claim
	token := makeValidToken("user1", "", 3600)

	md := metadata.New(map[string]string{
		"authorization": "Bearer " + token,
	})
	ctx := metadata.NewIncomingContext(context.Background(), md)

	_, err := interceptor(ctx, nil, &grpc.UnaryServerInfo{}, func(ctx context.Context, req interface{}) (interface{}, error) {
		return "ok", nil
	})

	if err == nil {
		t.Error("expected error for missing tenant claim")
	}
	if st, ok := status.FromError(err); ok {
		if st.Code() != codes.Unauthenticated {
			t.Errorf("code = %v, want Unauthenticated", st.Code())
		}
	}
}

func TestOIDCHTTPMiddleware_MissingTenantClaim(t *testing.T) {
	claims.ResetCacheForTesting()
	logger := zap.NewNop()
	cfg := config.Config{OIDCJwksURL: testJWKSURL, OIDCAudience: "nest", OIDCIssuer: "https://test-issuer"}

	nextCalled := false
	nextHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		nextCalled = true
		w.WriteHeader(http.StatusOK)
	})

	middleware := OIDCHTTPMiddleware(cfg, logger, nextHandler)

	// Token with empty tenant
	token := makeValidToken("user1", "", 3600)
	req := httptest.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	middleware.ServeHTTP(w, req)

	if nextCalled {
		t.Error("next handler should not be called for missing tenant claim")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("status = %d, want 401", w.Code)
	}
}

// makeValidToken creates a properly signed RS256 JWT token with the given claims.
// expOffsetSecs is added to the current time for the exp claim.
func makeValidToken(sub, tenant string, expOffsetSecs int64) string {
	now := time.Now().Unix()
	// Offset 0 means "a comfortably valid token" (1h); negative offsets make an
	// expired token. Using exp==now would be flakily expired on a second boundary.
	if expOffsetSecs == 0 {
		expOffsetSecs = 3600
	}
	exp := now + expOffsetSecs

	// Create header
	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
		"kid": "test-key",
	}
	headerBytes, _ := json.Marshal(header)
	headerB64 := base64.RawURLEncoding.EncodeToString(headerBytes)

	// Create payload
	payload := map[string]interface{}{
		"sub":    sub,
		"tenant": tenant,
		"iat":    now,
		"exp":    exp,
		"aud":    "nest",
		"iss":    "https://test-issuer",
	}
	payloadBytes, _ := json.Marshal(payload)
	payloadB64 := base64.RawURLEncoding.EncodeToString(payloadBytes)

	// Sign: SHA256 hash of header.payload, signed with RSA private key
	message := headerB64 + "." + payloadB64
	hash := sha256.Sum256([]byte(message))
	sig, _ := rsa.SignPKCS1v15(rand.Reader, testPrivKey, crypto.SHA256, hash[:])
	sigB64 := base64.RawURLEncoding.EncodeToString(sig)

	return message + "." + sigB64
}
