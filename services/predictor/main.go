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

const defaultAddr = ":50060"

func main() {
	logger := initLogger()
	slog.SetDefault(logger)

	addr := getAddr()
	ctx, cancel := setupSignalHandling()
	defer cancel()

	if err := run(ctx, addr); err != nil {
		logger.Error("run failed", "err", err)
		os.Exit(1)
	}
}

func initLogger() *slog.Logger {
	return slog.New(slog.NewJSONHandler(os.Stdout, nil))
}

func getAddr() string {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = defaultAddr
	}
	return addr
}

func setupSignalHandling() (context.Context, context.CancelFunc) {
	ctx, cancel := context.WithCancel(context.Background())
	sigch := make(chan os.Signal, 1)
	signal.Notify(sigch, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-sigch
		cancel()
	}()
	return ctx, cancel
}

func run(ctx context.Context, addr string) error {
	logger := slog.Default()

	lis, err := net.Listen("tcp", addr)
	if err != nil {
		logger.Error("listen failed", "addr", addr, "err", err)
		return err
	}
	defer lis.Close()

	srv, hsrv := createServer()

	serveDone := make(chan error, 1)
	go func() {
		logger.Info("server starting", "addr", addr)
		serveDone <- srv.Serve(lis)
	}()

	return handleShutdown(ctx, srv, hsrv, serveDone)
}

func handleShutdown(ctx context.Context, srv *grpc.Server, hsrv *health.Server, serveDone <-chan error) error {
	logger := slog.Default()

	select {
	case <-ctx.Done():
		logger.Info("shutdown signal received")
		hsrv.SetServingStatus("grpc.health.v1.Health", grpc_health_v1.HealthCheckResponse_NOT_SERVING)
		srv.GracefulStop()

		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer shutdownCancel()

		select {
		case <-shutdownCtx.Done():
			logger.Error("graceful shutdown timeout")
			srv.Stop()
		case <-serveDone:
		}
	case err := <-serveDone:
		if err != nil {
			logger.Error("serve failed", "err", err)
			return err
		}
	}

	logger.Info("shutdown complete")
	return nil
}

func createServer() (*grpc.Server, *health.Server) {
	forecaster := NewForecaster()
	_ = NewMux(forecaster, slog.Default())

	srv := grpc.NewServer()
	reflection.Register(srv)

	hsrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(srv, hsrv)
	hsrv.SetServingStatus("grpc.health.v1.Health", grpc_health_v1.HealthCheckResponse_SERVING)

	return srv, hsrv
}
