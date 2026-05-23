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

func run(ctx context.Context, addr string) error {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	calc := NewCalculator()
	mux := NewMux(calc, logger)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	// Start daily aggregation
	go calc.RunDailyAggregation(logger)

	// Graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-sigChan
		logger.Info("shutdown signal received")
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		server.Shutdown(shutdownCtx)
	}()

	logger.Info("cost-calculator listening", zap.String("addr", addr))
	if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		return err
	}
	return nil
}

func main() {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8091"
	}

	ctx := context.Background()
	if err := run(ctx, addr); err != nil {
		os.Exit(1)
	}
}
