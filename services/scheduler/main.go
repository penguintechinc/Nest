// Package main implements the Nest placement scheduler.
// It watches DataResource CRDs and HardwarePool CRDs, assigns resources
// to hardware pools using a filter+score pipeline (§3.2).
package main

import (
	"context"
	"flag"
	"os/signal"
	"strings"
	"syscall"
	"time"

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

// runOnce builds and starts the manager once. Returns nil on clean shutdown,
// or an error (possibly transient) if startup fails.
func runOnce(ctx context.Context, metricsAddr, probeAddr string, logger *zap.Logger) error {
	mgr, err := ctrl.NewManager(ctrl.GetConfigOrDie(), ctrl.Options{
		Scheme:                 scheme,
		Metrics:                metricsserver.Options{BindAddress: metricsAddr},
		HealthProbeBindAddress: probeAddr,
	})
	if err != nil {
		return err
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		return err
	}

	if err := (&placement.Scheduler{
		Client: mgr.GetClient(),
		Logger: logger,
	}).SetupWithManager(mgr); err != nil {
		return err
	}

	logger.Info("Nest scheduler starting")
	if err := mgr.Start(ctx); err != nil && err != context.Canceled {
		return err
	}
	return nil
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

	const maxRetries = 10
	for attempt := 0; ; attempt++ {
		err := runOnce(ctx, metricsAddr, probeAddr, logger)
		if err == nil || ctx.Err() != nil {
			return
		}
		s := err.Error()
		isTransient := strings.Contains(s, "timed out waiting for cache") ||
			strings.Contains(s, "i/o timeout") ||
			strings.Contains(s, "failed to wait for")
		if !isTransient || attempt >= maxRetries {
			logger.Fatal("manager exited with error", zap.Error(err))
		}
		wait := time.Duration(30*(attempt+1)) * time.Second
		if wait > 5*time.Minute {
			wait = 5 * time.Minute
		}
		logger.Warn("transient startup error, retrying", zap.Error(err), zap.Int("attempt", attempt+1), zap.Duration("backoff", wait))
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}
