package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/penguintechinc/nest/pkg/auth"
	"go.uber.org/zap"
)

func shutdownServer(server *http.Server, logger *zap.Logger) error {
	logger.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", zap.Error(err))
		return err
	}
	return nil
}

func run(ctx context.Context, addr string) error {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

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
		return err
	}

	catalog := NewCatalog()
	pipeline := NewPipeline(catalog, logger)
	mux := NewMux(catalog, pipeline, logger, authMiddleware)

	server := &http.Server{
		Addr:        addr,
		Handler:     mux,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}

	srvErr := make(chan error, 1)
	go func() {
		logger.Info("starting server", zap.String("addr", addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			srvErr <- err
		}
		close(srvErr)
	}()

	go pipeline.RunDailyScans(logger)

	select {
	case err := <-srvErr:
		return err
	case <-ctx.Done():
	}

	return shutdownServer(server, logger)
}

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8090"
	}

	ctx := context.Background()
	if err := run(ctx, addr); err != nil {
		os.Exit(1)
	}
}
