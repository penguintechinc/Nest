package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/penguintechinc/nest/pkg/auth"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8181"
	}

	// Initialize JWT auth middleware (FAIL-CLOSED if not configured)
	authConfig := &auth.Config{
		Algorithm:    os.Getenv("JWT_ALGORITHM"),
		SharedSecret: os.Getenv("JWT_SHARED_SECRET"),
		JWKSEndpoint: os.Getenv("JWT_JWKS_ENDPOINT"),
		Issuer:       os.Getenv("JWT_ISSUER"),
		Audience:     os.Getenv("JWT_AUDIENCE"),
	}
	authMiddleware, err := auth.NewMiddleware(authConfig)
	if err != nil {
		logger.Error("failed to initialize auth middleware", zap.Error(err))
		os.Exit(1)
	}

	catalog := NewCatalog(logger)
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	srv := &http.Server{
		Addr:        addr,
		Handler:     NewMux(catalog, logger, authMiddleware),
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}

	logger.Info("iceberg-catalog starting", zap.String("addr", addr))

	srvErr := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			srvErr <- err
		}
		close(srvErr)
	}()

	select {
	case err := <-srvErr:
		if err != nil {
			logger.Fatal("server error", zap.Error(err))
		}
	case <-ctx.Done():
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)
	logger.Info("iceberg-catalog stopped")
}
