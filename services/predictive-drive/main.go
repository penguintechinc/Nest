package main

import (
	"context"
	"log/slog"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"
)

const defaultAddr = ":50059"

func run(ctx context.Context, addr string, logger *slog.Logger) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("listen failed", "addr", addr, "err", err)
		return err
	}
	defer lis.Close()

	predictor := NewDrivePredictor()
	_ = NewMux(predictor, logger)

	srv := grpc.NewServer()
	reflection.Register(srv)

	hsrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, hsrv)
	hsrv.SetServingStatus("grpc.health.v1.Health", grpc_health_v1.HealthCheckResponse_SERVING)

	go func() {
		logger.Info("server starting", "addr", addr)
		if err := srv.Serve(lis); err != nil {
			logger.Error("serve failed", "err", err)
		}
	}()

	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, syscall.SIGINT, syscall.SIGTERM)

	select {
	case <-ctx.Done():
		logger.Info("context cancelled, initiating shutdown")
	case <-sigch:
		logger.Info("shutdown signal received")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	hsrv.SetServingStatus("grpc.health.v1.Health", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
	srv.GracefulStop()

	select {
	case <-ctx.Done():
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

	if err := run(context.Background(), addr, logger); err != nil {
		os.Exit(1)
	}
}
