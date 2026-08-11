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

const (
	defaultAddr     = ":50061"
	defaultHTTPAddr = ":8092"
)

func run(ctx context.Context, addr string, logger *slog.Logger) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("listen failed", "addr", addr, "err", err)
		return err
	}

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

	detector := NewDetector()
	httpMux := NewMux(detector, os.Getenv("ENTERPRISE_LICENSE"), logger, authMiddleware)

	// HTTP server
	httpAddr := os.Getenv("HTTP_ADDR")
	if httpAddr == "" {
		httpAddr = defaultHTTPAddr
	}
	httpSrv := &http.Server{
		Addr:    httpAddr,
		Handler: httpMux,
	}

	// gRPC server
	srv := grpc.NewServer()
	reflection.Register(srv)

	hsrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, hsrv)
	hsrv.SetServingStatus("grpc.health.v1.Health", grpc_health_v1.HealthCheckResponse_SERVING)

	// gRPC server goroutine
	go func() {
		logger.Info("grpc server starting", "addr", addr)
		if err := srv.Serve(lis); err != nil {
			logger.Error("grpc serve failed", "err", err)
		}
	}()

	// HTTP server goroutine
	go func() {
		logger.Info("http server starting", "addr", httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("http serve failed", "err", err)
		}
	}()

	// Signal handling
	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, syscall.SIGINT, syscall.SIGTERM)

	// Wait for either context cancellation or shutdown signal
	select {
	case <-ctx.Done():
		logger.Info("context cancelled")
	case <-sigch:
		logger.Info("shutdown signal received")
	}

	// Graceful shutdown
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hsrv.SetServingStatus("grpc.health.v1.Health", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	srv.GracefulStop()

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

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	slog.SetDefault(logger)

	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = defaultAddr
	}

	// Create a context that cancels on signal
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := run(ctx, addr, logger); err != nil {
		logger.Error("run failed", "err", err)
		os.Exit(1)
	}
}
