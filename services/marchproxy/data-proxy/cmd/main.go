package main

import (
	"context"
	"flag"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	"google.golang.org/grpc/health/grpc_health_v1"
	"google.golang.org/grpc/reflection"

	"github.com/penguintechinc/nest/services/marchproxy/data-proxy/internal/config"
	"github.com/penguintechinc/nest/services/marchproxy/data-proxy/internal/handler"
	"github.com/penguintechinc/nest/services/marchproxy/data-proxy/internal/healthprobe"
	"github.com/penguintechinc/nest/services/marchproxy/data-proxy/internal/middleware"
	_ "github.com/penguintechinc/nest/services/marchproxy/data-proxy/internal/protocol"
	_ "github.com/penguintechinc/nest/services/marchproxy/data-proxy/internal/provider"
	"github.com/penguintechinc/nest/services/marchproxy/data-proxy/internal/server"
)

func main() {
	var grpcAddr = flag.String("grpc-addr", ":50053", "gRPC listen address")
	var httpAddr = flag.String("http-addr", ":8083", "HTTP/REST listen address")
	flag.Parse()

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := config.FromEnv()

	// gRPC server
	grpcServer := grpc.NewServer(
		grpc.ChainUnaryInterceptor(middleware.OIDCUnaryInterceptor(cfg, logger)),
	)

	healthSrv := health.NewServer()
	grpc_health_v1.RegisterHealthServer(grpcServer, healthSrv)
	healthSrv.SetServingStatus("", grpc_health_v1.HealthCheckResponse_SERVING)
	reflection.Register(grpcServer)

	svcServer := server.New(cfg, logger)
	svcServer.Register(grpcServer)

	grpcLis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		logger.Fatal("grpc listen failed", zap.Error(err))
	}

	// HTTP server
	mux := handler.NewMux(cfg, logger)
	httpSrv := &http.Server{Addr: *httpAddr, Handler: mux}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Start health prober for imported/external resources
	probeMetrics := healthprobe.NewProbeMetrics()
	sloTracker := healthprobe.NewSLOTracker()
	prober := healthprobe.NewProber(30*time.Second, func(r healthprobe.HealthResult) {
		probeMetrics.Record(r)
		sloTracker.Record(r)
		logger.Info("resource probe",
			zap.String("tenant", r.Tenant),
			zap.String("resource", r.Name),
			zap.String("state", r.State),
		)
	}, logger)
	go prober.Run(ctx)

	go func() {
		logger.Info("gRPC data-proxy listening", zap.String("addr", *grpcAddr))
		if err := grpcServer.Serve(grpcLis); err != nil {
			logger.Error("gRPC serve error", zap.Error(err))
		}
	}()

	go func() {
		logger.Info("HTTP data-proxy listening", zap.String("addr", *httpAddr))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP serve error", zap.Error(err))
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down data-proxy")
	grpcServer.GracefulStop()
	httpSrv.Shutdown(context.Background())
}
