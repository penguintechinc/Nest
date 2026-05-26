package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

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

	// Initialize database
	db, err := database.New(nil)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	dal := database.NewPenguinDAL(db.DB)

	store := NewPolicyStore(dal)
	mux := NewMux(store, logger)

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
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
