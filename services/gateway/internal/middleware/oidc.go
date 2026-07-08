package middleware

import (
	"context"
	"net/http"
	"strings"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/penguintechinc/nest/services/gateway/internal/claims"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
)

func OIDCUnaryInterceptor(cfg config.Config, log *zap.Logger) grpc.UnaryServerInterceptor {
	return func(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
		md, ok := metadata.FromIncomingContext(ctx)
		if !ok {
			return nil, status.Error(codes.Unauthenticated, "missing metadata")
		}
		authHdr := md.Get("authorization")
		if len(authHdr) == 0 {
			return nil, status.Error(codes.Unauthenticated, "missing authorization header")
		}
		token := strings.TrimPrefix(authHdr[0], "Bearer ")
		if token == authHdr[0] {
			return nil, status.Error(codes.Unauthenticated, "authorization header must use Bearer scheme")
		}
		cl, err := claims.ParseToken(token, cfg.OIDCJwksURL, cfg.OIDCAudience, cfg.OIDCIssuer)
		if err != nil {
			log.Warn("token parse failed", zap.Error(err))
			return nil, status.Errorf(codes.Unauthenticated, "invalid token: %v", err)
		}
		if cl.Tenant == "" {
			return nil, status.Error(codes.Unauthenticated, "token missing tenant claim")
		}
		ctx = claims.WithClaims(ctx, cl)
		return handler(ctx, req)
	}
}

func OIDCHTTPMiddleware(cfg config.Config, log *zap.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHdr := r.Header.Get("Authorization")
		token := strings.TrimPrefix(authHdr, "Bearer ")
		if token == "" || token == authHdr {
			http.Error(w, "missing or invalid authorization header", http.StatusUnauthorized)
			return
		}
		cl, err := claims.ParseToken(token, cfg.OIDCJwksURL, cfg.OIDCAudience, cfg.OIDCIssuer)
		if err != nil {
			http.Error(w, "invalid token", http.StatusUnauthorized)
			return
		}
		if cl.Tenant == "" {
			http.Error(w, "token missing tenant claim", http.StatusUnauthorized)
			return
		}
		r = r.WithContext(claims.WithClaims(r.Context(), cl))
		next.ServeHTTP(w, r)
	})
}
