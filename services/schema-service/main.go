package main

import (
	"context"
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
		addr = ":50054"
	}

	if err := run(context.Background(), addr); err != nil {
		logger.Error("run failed", zap.Error(err))
	}
}

// run initializes the schema service and runs the HTTP server.
// It handles graceful shutdown via context cancellation.
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

	// Initialize cache and introspector
	cache := NewSchemaCache(1 * time.Hour)
	introspector := NewIntrospector(logger)

	// Create HTTP server
	mux := NewMux(cache, introspector, logger, authMiddleware)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Setup signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start server in goroutine
	go func() {
		logger.Info("schema service listening", zap.String("addr", addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", zap.Error(err))
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	logger.Info("shutting down schema service")

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", zap.Error(err))
		return err
	}
	return nil
}
