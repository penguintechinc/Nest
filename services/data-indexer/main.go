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

	catalog := NewCatalog()
	pipeline := NewPipeline(catalog, logger)
	mux := NewMux(catalog, pipeline, logger)

	server := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		logger.Info("starting server", zap.String("addr", addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	go pipeline.RunDailyScans(logger)

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	<-sigChan

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
