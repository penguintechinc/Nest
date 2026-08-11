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
	"github.com/penguintechinc/nest/shared/database"
	"go.uber.org/zap"
)

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	if err := run(ctx, logger); err != nil {
		logger.Error("policy-engine error", zap.Error(err))
		os.Exit(1)
	}
}

// run starts the HTTP server and blocks until ctx is cancelled or a fatal error occurs.
// Separating ctx from signal handling allows tests to drive shutdown without sending OS signals.
func run(ctx context.Context, logger *zap.Logger) error {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":50058"
	}

	// Initialize database with PenguinDAL
	db, err := database.New(nil)
	if err != nil {
		logger.Error("failed to connect to database", zap.Error(err))
		return err
	}
	defer db.Close()

	dal := database.NewPenguinDAL(db.DB)

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

	store := NewPolicyStore(dal)
	mux := NewMux(store, logger, authMiddleware)

	srv := &http.Server{
		Addr:        addr,
		Handler:     mux,
		BaseContext: func(_ net.Listener) context.Context { return ctx },
	}

	srvErr := make(chan error, 1)
	go func() {
		logger.Info("starting policy-engine", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			srvErr <- err
		}
		close(srvErr)
	}()

	select {
	case err := <-srvErr:
		return err
	case <-ctx.Done():
	}

	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutCtx); err != nil {
		logger.Error("shutdown error", zap.Error(err))
		return err
	}
	logger.Info("policy-engine stopped")
	return nil
}
