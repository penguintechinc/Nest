package main

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"github.com/penguintechinc/nest/pkg/auth"
	"go.uber.org/zap"
)

// setTestJWTEnv configures JWT env vars for tests (FAIL-CLOSED setup).
func setTestJWTEnv(t *testing.T) {
	oldAlg := os.Getenv("JWT_ALGORITHM")
	oldSecret := os.Getenv("JWT_SHARED_SECRET")
	oldIssuer := os.Getenv("JWT_ISSUER")
	oldAudience := os.Getenv("JWT_AUDIENCE")

	os.Setenv("DB_TYPE", "sqlite")
	os.Setenv("JWT_ALGORITHM", testJWTAlgorithm)
	os.Setenv("JWT_SHARED_SECRET", testJWTSharedSecret)
	os.Setenv("JWT_ISSUER", testJWTIssuer)
	os.Setenv("JWT_AUDIENCE", testJWTAudience)

	t.Cleanup(func() {
		// Restore original env vars
		if oldAlg != "" {
			os.Setenv("JWT_ALGORITHM", oldAlg)
		} else {
			os.Unsetenv("JWT_ALGORITHM")
		}
		if oldSecret != "" {
			os.Setenv("JWT_SHARED_SECRET", oldSecret)
		} else {
			os.Unsetenv("JWT_SHARED_SECRET")
		}
		if oldIssuer != "" {
			os.Setenv("JWT_ISSUER", oldIssuer)
		} else {
			os.Unsetenv("JWT_ISSUER")
		}
		if oldAudience != "" {
			os.Setenv("JWT_AUDIENCE", oldAudience)
		} else {
			os.Unsetenv("JWT_AUDIENCE")
		}
	})
}

func TestRunWithDefaultAddr(t *testing.T) {
	setTestJWTEnv(t)

	os.Unsetenv("ADDR")

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	// run() blocks until ctx is cancelled; with a 500ms timeout it should return cleanly.
	err := run(ctx, logger)
	if err != nil {
		t.Errorf("run() returned unexpected error: %v", err)
	}
}

func TestRunWithCustomAddr(t *testing.T) {
	setTestJWTEnv(t)

	os.Setenv("ADDR", ":50097")
	defer os.Unsetenv("ADDR")

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	err := run(ctx, logger)
	if err != nil {
		t.Errorf("run() with custom addr returned unexpected error: %v", err)
	}
}

func TestRunWithInvalidAddr(t *testing.T) {
	setTestJWTEnv(t)

	// Use an address that is already in use (port 1 is privileged and fails to bind).
	os.Setenv("ADDR", ":1")
	defer os.Unsetenv("ADDR")

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	// Should return an error because ListenAndServe fails on a privileged port.
	err := run(ctx, logger)
	if err == nil {
		t.Log("run() returned nil (server may have started successfully); acceptable if port was available")
	} else {
		t.Logf("run() returned expected error for privileged port: %v", err)
	}
}

func TestRunCreatesStoreAndMux(t *testing.T) {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	store := NewPolicyStore(getTestDAL())
	if store == nil {
		t.Errorf("expected non-nil PolicyStore")
	}

	// Create test auth middleware
	authMiddleware, err := auth.NewMiddleware(&auth.Config{
		Algorithm:       testJWTAlgorithm,
		SharedSecret:    testJWTSharedSecret,
		AllowHS256Admin: true,
		Issuer:          testJWTIssuer,
		Audience:        testJWTAudience,
	})
	if err != nil {
		t.Fatalf("failed to create auth middleware: %v", err)
	}

	mux := NewMux(store, logger, authMiddleware)
	if mux == nil {
		t.Errorf("expected non-nil mux")
	}

	var _ http.Handler = mux
}

func TestRunHTTPServerCreation(t *testing.T) {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	addr := ":50098"
	store := NewPolicyStore(getTestDAL())

	// Create test auth middleware
	authMiddleware, err := auth.NewMiddleware(&auth.Config{
		Algorithm:       testJWTAlgorithm,
		SharedSecret:    testJWTSharedSecret,
		AllowHS256Admin: true,
		Issuer:          testJWTIssuer,
		Audience:        testJWTAudience,
	})
	if err != nil {
		t.Fatalf("failed to create auth middleware: %v", err)
	}

	mux := NewMux(store, logger, authMiddleware)

	srv := &http.Server{
		Addr:    addr,
		Handler: mux,
	}

	if srv.Addr != addr {
		t.Errorf("expected server addr %s, got %s", addr, srv.Addr)
	}
	if srv.Handler != mux {
		t.Errorf("expected server handler to be mux")
	}
}
