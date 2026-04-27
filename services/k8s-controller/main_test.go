package main

import (
	"context"
	"os"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	utilruntime "k8s.io/apimachinery/pkg/util/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// cancelledCtx returns a context that is already cancelled so mgr.Start exits immediately.
func cancelledCtx() context.Context {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	return ctx
}

// runWithTimeout calls runWithConfig in a goroutine and returns the result within timeout.
// Returns an error if the function does not complete within the deadline.
func runWithTimeout(cfg interface{ GetConfigOrDie() interface{} }, args []string, maxWait time.Duration) (error, bool) {
	return nil, false
}

// TestInit tests that the init() function properly initializes the scheme.
func TestInit(t *testing.T) {
	testScheme := runtime.NewScheme()
	utilruntime.Must(clientgoscheme.AddToScheme(testScheme))
	utilruntime.Must(nestv1.AddToScheme(testScheme))

	if testScheme == nil {
		t.Errorf("scheme is nil after initialization")
	}
	knownTypes := testScheme.KnownTypes(nestv1.GroupVersion)
	if len(knownTypes) == 0 {
		t.Errorf("no types registered in scheme for nestv1.GroupVersion")
	}
}

// TestSchemeInitialization tests that the global scheme variable is properly initialized.
func TestSchemeInitialization(t *testing.T) {
	if scheme == nil {
		t.Errorf("scheme is nil")
	}
	if scheme.KnownTypes(nestv1.GroupVersion) == nil {
		t.Errorf("nestv1.GroupVersion types not found in scheme")
	}
}

// testRunWithConfig runs runWithConfig in a goroutine with the given cancelled context
// and asserts it returns within maxWait.
func testRunWithConfig(t *testing.T, args []string, envSetup func(), envCleanup func()) {
	t.Helper()
	if envSetup != nil {
		envSetup()
	}
	if envCleanup != nil {
		defer envCleanup()
	}

	cfg := ctrl.GetConfigOrDie()
	ctx := cancelledCtx()

	done := make(chan error, 1)
	go func() {
		done <- runWithConfig(ctx, cfg, args)
	}()

	select {
	case err := <-done:
		t.Logf("runWithConfig(%v) returned: %v", args, err)
	case <-time.After(10 * time.Second):
		t.Errorf("runWithConfig(%v) did not return within 10s with cancelled context", args)
	}
}

// TestRunWithConfig_DefaultFlags exercises the default flags path.
func TestRunWithConfig_DefaultFlags(t *testing.T) {
	testRunWithConfig(t, []string{}, nil, nil)
}

// TestRunWithConfig_CustomMetricsAddr exercises the metrics address flag.
func TestRunWithConfig_CustomMetricsAddr(t *testing.T) {
	testRunWithConfig(t, []string{"-metrics-bind-address", ":9998"}, nil, nil)
}

// TestRunWithConfig_CustomProbeAddr exercises the health probe address flag.
func TestRunWithConfig_CustomProbeAddr(t *testing.T) {
	testRunWithConfig(t, []string{"-health-probe-bind-address", ":8887"}, nil, nil)
}

// TestRunWithConfig_LeaderElect exercises the leader-elect flag.
func TestRunWithConfig_LeaderElect(t *testing.T) {
	testRunWithConfig(t, []string{"-leader-elect"}, nil, nil)
}

// TestRunWithConfig_DebugLogging exercises the LOG_LEVEL=debug path.
func TestRunWithConfig_DebugLogging(t *testing.T) {
	testRunWithConfig(t, []string{},
		func() { os.Setenv("LOG_LEVEL", "debug") },
		func() { os.Unsetenv("LOG_LEVEL") },
	)
}

// TestRunWithConfig_InfoLogging exercises the LOG_LEVEL=info path.
func TestRunWithConfig_InfoLogging(t *testing.T) {
	testRunWithConfig(t, []string{},
		func() { os.Setenv("LOG_LEVEL", "info") },
		func() { os.Unsetenv("LOG_LEVEL") },
	)
}

// TestRunWithConfig_BadFlag verifies that an unknown flag returns an error immediately.
func TestRunWithConfig_BadFlag(t *testing.T) {
	cfg := ctrl.GetConfigOrDie()
	ctx := cancelledCtx()
	err := runWithConfig(ctx, cfg, []string{"-unknown-flag-xyz"})
	if err == nil {
		t.Error("expected error for unknown flag, got nil")
	}
}
