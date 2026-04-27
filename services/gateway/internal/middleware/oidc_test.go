package middleware

import (
	"context"
	"encoding/base64"
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

func TestOIDCUnaryInterceptor(t *testing.T) {
	logger := zap.NewNop()
	cfg := config.Config{OIDCAudience: "test"}

	tests := []struct {
		name      string
		metadata  metadata.MD
		wantErr   bool
		errCode   codes.Code
		wantClaim bool
	}{
		{
			name:    "missing metadata",
			metadata: nil,
			wantErr: true,
			errCode: codes.Unauthenticated,
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
	cfg := config.Config{OIDCAudience: "test"}

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
	logger := zap.NewNop()
	cfg := config.Config{OIDCAudience: "test"}

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
	logger := zap.NewNop()
	cfg := config.Config{OIDCAudience: "test"}
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
	logger := zap.NewNop()
	cfg := config.Config{OIDCAudience: "test"}

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
	logger := zap.NewNop()
	cfg := config.Config{OIDCAudience: "test"}
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
	logger := zap.NewNop()
	cfg := config.Config{OIDCAudience: "test"}

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

// Helper to create a valid JWT token for testing
func makeValidToken(sub, tenant string, expOffset int64) string {
	expTime := time.Now().Unix() + expOffset
	payload := `{"sub":"` + sub + `","tenant":"` + tenant + `","exp":` + itoa64(expTime) + `}`
	header := `{"alg":"HS256","typ":"JWT"}`
	return base64.RawURLEncoding.EncodeToString([]byte(header)) + "." +
		base64.RawURLEncoding.EncodeToString([]byte(payload)) + ".sig"
}

func itoa64(n int64) string {
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
