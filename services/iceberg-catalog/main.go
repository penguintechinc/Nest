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

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8181"
	}

	catalog := NewCatalog(logger)
	srv := &http.Server{
		Addr:    addr,
		Handler: NewMux(catalog, logger),
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	logger.Info("iceberg-catalog starting", zap.String("addr", addr))

	go func() {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("server error", zap.Error(err))
		}
	}()

	<-ctx.Done()
	shutCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	srv.Shutdown(shutCtx)
	logger.Info("iceberg-catalog stopped")
}
