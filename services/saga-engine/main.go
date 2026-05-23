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

func newServer(logger *zap.Logger) *http.Server {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":50055"
	}

	store := NewSagaStore(logger)
	mux := NewMux(store, logger)

	return &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 60 * time.Second,
	}
}

func serveAndWait(logger *zap.Logger, server *http.Server) {
	go func() {
		logger.Info("starting saga engine", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("server error", zap.Error(err))
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGTERM, syscall.SIGINT)
	<-sigChan

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", zap.Error(err))
	}
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	server := newServer(logger)
	serveAndWait(logger, server)
}
