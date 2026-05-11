package main

import (
	"context"
	"flag"
	"os"
	"strings"
	"time"

	"go.uber.org/zap/zapcore"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/healthz"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
	"github.com/penguintechinc/nest/services/k8s-controller/controllers"
)

var (
	scheme   = runtime.NewScheme()
	setupLog = ctrl.Log.WithName("setup")
)

func init() {
	utilruntime.Must(clientgoscheme.AddToScheme(scheme))
	utilruntime.Must(nestv1.AddToScheme(scheme))
}

// runWithConfig sets up the controller manager using the provided rest.Config and context.
// Accepts args for flag parsing (allows tests to pass empty args without flag redefinition).
// ctx controls the manager lifetime; tests pass a pre-cancelled context to exit immediately.
func runWithConfig(ctx context.Context, cfg *rest.Config, args []string) error {
	fs := flag.NewFlagSet("k8s-controller", flag.ContinueOnError)
	var metricsAddr string
	var probeAddr string
	var leaderElect bool
	fs.StringVar(&metricsAddr, "metrics-bind-address", ":9090", "Metrics endpoint address")
	fs.StringVar(&probeAddr, "health-probe-bind-address", ":8081", "Health probe address")
	fs.BoolVar(&leaderElect, "leader-elect", false, "Enable leader election")
	if err := fs.Parse(args); err != nil {
		return err
	}

	opts := zap.Options{
		Development:     os.Getenv("LOG_LEVEL") == "debug",
		StacktraceLevel: zapcore.DPanicLevel,
	}
	ctrl.SetLogger(zap.New(zap.UseFlagOptions(&opts)))

	mgr, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme:                 scheme,
		HealthProbeBindAddress: probeAddr,
		LeaderElection:         leaderElect,
		LeaderElectionID:       "nest-controller.penguintech.io",
	})
	if err != nil {
		setupLog.Error(err, "unable to create manager")
		return err
	}

	if err := (&controllers.DataResourceReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create DataResource controller")
		return err
	}

	if err := (&controllers.TenantReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create Tenant controller")
		return err
	}

	if err := (&controllers.DarkDriveReconciler{
		Client: mgr.GetClient(),
		Scheme: mgr.GetScheme(),
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create DarkDrive controller")
		return err
	}

	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		setupLog.Error(err, "unable to create dynamic client")
		return err
	}

	if err := (&controllers.DataProtectionPolicyReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		DynClient: dynClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create DataProtectionPolicy controller")
		return err
	}

	if err := (&controllers.SearchPoolReconciler{
		Client:    mgr.GetClient(),
		Scheme:    mgr.GetScheme(),
		DynClient: dynClient,
	}).SetupWithManager(mgr); err != nil {
		setupLog.Error(err, "unable to create SearchPool controller")
		return err
	}

	if err := mgr.AddHealthzCheck("healthz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to add healthz check")
		return err
	}
	if err := mgr.AddReadyzCheck("readyz", healthz.Ping); err != nil {
		setupLog.Error(err, "unable to add readyz check")
		return err
	}

	setupLog.Info("starting Nest k8s-controller")
	if err := mgr.Start(ctx); err != nil {
		setupLog.Error(err, "controller manager exited")
		return err
	}
	return nil
}

// isTransientStartupError returns true for API connectivity errors that occur
// during initial cache sync and are safe to retry (e.g. brief Cilium BPF map
// recalculations after endpoint churn).
func isTransientStartupError(err error) bool {
	s := err.Error()
	return strings.Contains(s, "timed out waiting for cache") ||
		strings.Contains(s, "i/o timeout") ||
		strings.Contains(s, "failed to wait for")
}

// run retries on transient API connectivity errors so that a brief ClusterIP
// outage during startup does not escalate into an exponential-backoff crash loop.
func run() error {
	ctx := ctrl.SetupSignalHandler()
	cfg := ctrl.GetConfigOrDie()
	const maxRetries = 10
	for attempt := 0; ; attempt++ {
		err := runWithConfig(ctx, cfg, os.Args[1:])
		if err == nil || ctx.Err() != nil {
			return nil
		}
		if !isTransientStartupError(err) || attempt >= maxRetries {
			return err
		}
		wait := time.Duration(30*(attempt+1)) * time.Second
		if wait > 5*time.Minute {
			wait = 5 * time.Minute
		}
		setupLog.Error(err, "transient startup error, retrying", "attempt", attempt+1, "backoff", wait)
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}
	}
}

func main() {
	if err := run(); err != nil {
		os.Exit(1)
	}
}
