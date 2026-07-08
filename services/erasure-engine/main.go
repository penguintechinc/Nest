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

func run(logger *zap.Logger, addr string, sigCh <-chan os.Signal) error {
	if addr == "" {
		addr = ":50056"
	}

	store := NewErasureStore(logger)
	mux := NewMux(store, logger)

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	go func() {
		logger.Info("starting erasure engine", zap.String("addr", addr))
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	<-sigCh

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return srv.Shutdown(ctx)
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	addr := os.Getenv("ADDR")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, syscall.SIGINT)

	if err := run(logger, addr, sigCh); err != nil {
		logger.Error("erasure engine shutdown with error", zap.Error(err))
		os.Exit(1)
	}
}
