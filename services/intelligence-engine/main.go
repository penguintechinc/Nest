package main

import (
	"context"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/penguintechinc/nest/pkg/auth"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

const defaultAddr = ":50057"

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = defaultAddr
	}

	if err := run(context.Background(), addr); err != nil {
		logger.Error("run failed", "err", err)
		os.Exit(1)
	}
}

// runConfig allows dependency injection for testing
type runConfig struct {
	sigChan        chan os.Signal
	shutdownTimeMs int64 // milliseconds; 0 = use default 10s
}

func run(ctx context.Context, addr string) error {
	return runWithConfig(ctx, addr, &runConfig{
		sigChan:        make(chan os.Signal, 1),
		shutdownTimeMs: 10000, // 10 seconds default
	})
}

func runWithConfig(ctx context.Context, addr string, cfg *runConfig) error {
	logger := slog.Default()

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("listen failed", "addr", addr, "err", err)
		return err
	}
	defer lis.Close()

	// Initialize JWT auth middleware (FAIL-CLOSED if not configured)
	authConfig := &auth.Config{
		Algorithm:    os.Getenv("JWT_ALGORITHM"),
		SharedSecret: os.Getenv("JWT_SHARED_SECRET"),
		JWKSEndpoint: os.Getenv("JWT_JWKS_ENDPOINT"),
		Issuer:       os.Getenv("JWT_ISSUER"),
		Audience:     os.Getenv("JWT_AUDIENCE"),
	}
	authMiddleware, err := auth.NewMiddleware(authConfig)
	if err != nil {
		logger.Error("failed to initialize auth middleware", "err", err)
		return err
	}

	classifier := NewClassifier()
	httpMux := NewMux(classifier, logger, authMiddleware)

	// HTTP server for REST API
	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = ":8093"
	}
	httpSrv := &http.Server{
		Addr:    httpAddr,
		Handler: httpMux,
	}

	// Start HTTP server in goroutine
	go func() {
		logger.Info("http server starting", "addr", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http serve failed", "err", err)
		}
	}()

	srv := grpc.NewServer()
	reflection.Register(srv)

	hsrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, hsrv)
	hsrv.SetServingStatus("grpc.health.v1.Health", grpc_health_v1.HealthCheckResponse_SERVING)

	go func() {
		logger.Info("grpc server starting", "addr", addr)
		if err := srv.Serve(lis); err != nil {
			logger.Error("grpc serve failed", "err", err)
		}
	}()

	sigch := cfg.sigChan
	signal.Notify(sigch, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-sigch:
		logger.Info("shutdown signal received")
	case <-ctx.Done():
		logger.Info("context cancelled")
	}

	shutdownTime := time.Duration(cfg.shutdownTimeMs) * time.Millisecond
	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTime)
	defer cancel()

	hsrv.SetServingStatus("grpc.health.v1.Health", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	srv.GracefulStop()

	// Shutdown HTTP server
	if err := httpSrv.Shutdown(shutdownCtx); err != nil {
		logger.Error("http shutdown failed", "err", err)
	}

	select {
	case <-shutdownCtx.Done():
		logger.Error("graceful shutdown timeout")
		srv.Stop()
	default:
	}

	logger.Info("shutdown complete")
	return nil
}
