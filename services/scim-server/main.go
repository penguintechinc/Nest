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

func newServer(logger *zap.Logger) *http.Server {
	enterpriseLicense := os.Getenv("ENTERPRISE_LICENSE")
	if enterpriseLicense == "" {
		logger.Warn("ENTERPRISE_LICENSE not set; SCIM endpoints will be restricted")
	}

	// Initialize database
	db, err := database.New(nil)
	if err != nil {
		logger.Fatal("failed to connect to database", zap.Error(err))
	}

	dal := database.NewPenguinDAL(db.DB)

	store := NewSCIMStore(dal, logger)
	mux := NewMux(store, logger)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":8086"
	}

	return &http.Server{
		Addr:         addr,
		Handler:      mux,
		ReadTimeout:  15 * time.Second,
		WriteTimeout: 15 * time.Second,
		IdleTimeout:  60 * time.Second,
	}
}

func serveAndWait(logger *zap.Logger, server *http.Server) {
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	go func() {
		logger.Info("starting SCIM server", zap.String("addr", server.Addr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Fatal("listen error", zap.Error(err))
		}
	}()

	<-stop
	logger.Info("shutting down SCIM server")

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
