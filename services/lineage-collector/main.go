package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
)

func run(ctx context.Context, addr string, logger *zap.Logger) error {
	store := NewLineageStore()
	mux := NewMux(store, logger)

	server := &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}

	// Start server in background
	go func() {
		logger.Info("starting lineage collector", zap.String("addr", addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", zap.Error(err))
		}
	}()

	// Wait for context cancellation or signal
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)

	select {
	case <-ctx.Done():
		// Context cancelled (test scenario)
	case <-sigChan:
		// Signal received (production scenario)
	}

	// Shutdown with timeout
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("shutdown error", zap.Error(err))
		return err
	}

	return ctx.Err()
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":5000"
	}

	if err := run(context.Background(), addr, logger); err != nil && err != context.Canceled {
		logger.Error("run error", zap.Error(err))
	}
}
