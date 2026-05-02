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
	enterpriseLicense := os.Getenv("ENTERPRISE_LICENSE")
	if enterpriseLicense == "" {
		logger.Warn("ENTERPRISE_LICENSE not set - SCIM endpoints will require license")
	}

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8087"
	}

	store := NewSCIMStore(logger)
	mux := NewMux(store, logger)

	return &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func serveAndWait(logger *zap.Logger, server *http.Server) {
	go func() {
		logger.Info("starting SCIM server", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigChan

	logger.Info("received signal", zap.String("signal", sig.String()))

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := server.Shutdown(ctx); err != nil {
		logger.Error("shutdown error", zap.Error(err))
		os.Exit(1)
	}

	logger.Info("server stopped gracefully")
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	server := newServer(logger)
	serveAndWait(logger, server)
}
