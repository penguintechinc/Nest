// Package main implements the Nest placement scheduler.
// It watches DataResource CRDs and HardwarePool CRDs, assigns resources
// to hardware pools using a filter+score pipeline (§3.2).
package main

import (
	"context"
	"flag"
	"os/signal"
	"syscall"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	ctrlzap "sigs.k8s.io/controller-runtime/pkg/log/zap"
	metricsserver "sigs.k8s.io/controller-runtime/pkg/metrics/server"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
	"github.com/penguintechinc/nest/services/scheduler/placement"
)

var scheme = runtime.NewScheme()

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(nestv1.AddToScheme(scheme))
}

func main() {
	var metricsAddr string
	var probeAddr string
	flag.StringVar(&metricsAddr, "metrics-bind-address", ":9091", "Metrics endpoint address")
	flag.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "Health probe address")
	flag.Parse()

	logger, _ := zap.NewProduction()
	defer logger.Sync()

	ctrl.SetLogger(ctrlzap.New(ctrlzap.UseDevMode(false)))

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer cancel()

	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
	})
	if err != nil {
		logger.Fatal("failed to create manager", zap.Error(err))
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		logger.Fatal("unable to add healthz check", zap.Error(err))
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		logger.Fatal("unable to add readyz check", zap.Error(err))
	}

	if err := (&placement.Scheduler{
		Client: mgr.GetClient(),
		Logger: logger,
	}).SetupWithManager(mgr); err != nil {
		logger.Fatal("failed to setup scheduler", zap.Error(err))
	}

	logger.Info("Nest scheduler starting")
	if err := mgr.Start(ctx); err != nil {
		if err != context.Canceled {
			logger.Fatal("manager exited with error", zap.Error(err))
		}
	}
}
