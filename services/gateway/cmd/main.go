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
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/clientcmd"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
	"github.com/penguintechinc/nest/services/gateway/internal/config"
	"github.com/penguintechinc/nest/services/gateway/internal/handler"
	"github.com/penguintechinc/nest/services/gateway/internal/healthprobe"
	"github.com/penguintechinc/nest/services/gateway/internal/middleware"
	_ "github.com/penguintechinc/nest/services/gateway/internal/provider"
	"github.com/penguintechinc/nest/services/gateway/internal/server"
)

func newK8sClient(logger *zap.Logger) client.Client {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		kubeconfig := os.Getenv("KUBECONFIG")
		if kubeconfig == "" {
			if home, e := os.UserHomeDir(); e == nil {
				kubeconfig = home + "/.kube/config"
			}
		}
		cfg, err = clientcmd.BuildConfigFromFlags("", kubeconfig)
		if err != nil {
			logger.Warn("k8s client unavailable — DataResource CRUD will return 503", zap.Error(err))
			return nil
		}
	}

	scheme := runtime.NewScheme()
	if err := nestv1.AddToScheme(scheme); err != nil {
		logger.Warn("failed to register nest scheme", zap.Error(err))
		return nil
	}

	c, err := client.New(cfg, client.Options{Scheme: scheme})
	if err != nil {
		logger.Warn("failed to create k8s client", zap.Error(err))
		return nil
	}
	return c
}

func main() {
	var grpcAddr = flag.String("grpc-addr", ":50052", "gRPC listen address")
	var httpAddr = flag.String("http-addr", ":8082", "HTTP/REST listen address")
	flag.Parse()

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	cfg := config.FromEnv()
	cfg.K8sClient = newK8sClient(logger)

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
		logger.Info("gRPC gateway listening", zap.String("addr", *grpcAddr))
		if err := grpcServer.Serve(grpcLis); err != nil {
			logger.Error("gRPC serve error", zap.Error(err))
		}
	}()

	go func() {
		logger.Info("HTTP gateway listening", zap.String("addr", *httpAddr))
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("HTTP serve error", zap.Error(err))
		}
	}()

	<-ctx.Done()
	logger.Info("shutting down gateway")
	grpcServer.GracefulStop()
	httpSrv.Shutdown(context.Background())
}
