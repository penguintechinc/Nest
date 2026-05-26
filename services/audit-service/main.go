package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/penguintechinc/nest/shared/database"
	"go.uber.org/zap"
)

// run initializes and runs the audit service server.
func run(ctx context.Context, addr string) error {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Initialize database
	db, err := database.New(nil) // uses default config from env
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}
	defer db.Close()

	dal := database.NewPenguinDAL(db.DB)

	// Check enterprise license
	enterpriseLicense := os.Getenv("ENTERPRISE_LICENSE")
	if enterpriseLicense == "" {
		logger.Warn("ENTERPRISE_LICENSE not set; some endpoints will be restricted")
	}

	// Initialize audit logger
	auditLogger := NewAuditLogger(dal, logger)

	// Create HTTP server
	mux := NewMux(auditLogger, enterpriseLicense, logger)
	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Setup signal handling for graceful shutdown
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start server in goroutine
	go func() {
		logger.Info("audit service listening", zap.String("addr", addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", zap.Error(err))
		}
	}()

	// Wait for shutdown signal
	<-ctx.Done()
	logger.Info("shutting down audit service")

	// Graceful shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", zap.Error(err))
	}

	return nil
}

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8085"
	}

	if err := run(context.Background(), addr); err != nil {
		os.Exit(1)
	}
}
