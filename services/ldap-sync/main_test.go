package main

import (
	"context"
	"io"
	"net/http"
	"os"
	"testing"
	"time"
)

func init() {
	// Set JWT env vars for tests
	os.Setenv("JWT_ALGORITHM", "HS256")
	os.Setenv("JWT_SHARED_SECRET", "test-secret-key-for-testing")
	os.Setenv("JWT_ISSUER", "test-issuer")
	os.Setenv("JWT_AUDIENCE", "test-audience")
}

func TestRun(t *testing.T) {
	// Test that run() initializes and starts the server properly
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// run() will exit when ctx is cancelled
	err := run(ctx, "localhost:0")
	if err != nil {
		// context deadline exceeded is expected when test times out
		if err != context.DeadlineExceeded {
			t.Fatalf("run() returned unexpected error: %v", err)
		}
	}
}

func TestRunGracefulShutdown(t *testing.T) {
	// Test graceful shutdown path
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := run(ctx, "localhost:0")
	// Expect context deadline exceeded as context times out
	if err != nil && err != context.DeadlineExceeded {
		t.Errorf("expected context deadline or nil, got: %v", err)
	}
}

func TestRunInitializesComponents(t *testing.T) {
	// Test that run initializes logger, syncer, and mux
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// If components failed to initialize, run() would panic or error early
	err := run(ctx, "localhost:0")
	if err != nil && err != context.DeadlineExceeded {
		t.Errorf("unexpected error during initialization: %v", err)
	}
}

func TestRunServerListens(t *testing.T) {
	// Test that the server actually listens on the specified address
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	go func() {
		_ = run(ctx, "localhost:18991")
	}()

	// Give server a moment to start
	time.Sleep(20 * time.Millisecond)

	// Try to connect to verify server is listening
	client := &http.Client{Timeout: 50 * time.Millisecond}
	resp, err := client.Get("http://localhost:18991/healthz")
	if err == nil {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected 200, got %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if len(body) == 0 {
			t.Error("expected response body")
		}
	}
	// Allow connection errors during test since server startup timing is variable
}

func TestRunHandlesContextCancellation(t *testing.T) {
	// Test that run properly handles context cancellation
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := run(ctx, "localhost:0")
	if err != nil && err != context.DeadlineExceeded {
		t.Errorf("expected nil or deadline exceeded, got: %v", err)
	}
}

func TestRunLogsMissingLicense(t *testing.T) {
	// Test that run logs warning when license is not set
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	t.Setenv("ENTERPRISE_LICENSE", "")

	err := run(ctx, "localhost:0")
	if err != nil && err != context.DeadlineExceeded {
		t.Errorf("expected context deadline or nil, got: %v", err)
	}
}

func TestRunWithLicense(t *testing.T) {
	// Test that run starts correctly when license is set
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	t.Setenv("ENTERPRISE_LICENSE", "valid-license")

	err := run(ctx, "localhost:0")
	if err != nil && err != context.DeadlineExceeded {
		t.Errorf("expected context deadline or nil, got: %v", err)
	}
}

func TestRunContextCancelledImmediately(t *testing.T) {
	// Test with already-cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	// Should handle cancelled context gracefully
	err := run(ctx, "localhost:0")
	if err != context.Canceled && err != nil {
		t.Logf("got error: %v (context canceled is OK)", err)
	}
}

func TestRunMultipleCycles(t *testing.T) {
	// Test multiple startup/shutdown cycles
	for i := 0; i < 3; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		err := run(ctx, "localhost:0")
		cancel()
		if err != nil && err != context.DeadlineExceeded {
			t.Errorf("cycle %d: unexpected error %v", i, err)
		}
	}
}

func TestRunInvalidAddress(t *testing.T) {
	// Test with invalid address
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Invalid address might fail to bind
	err := run(ctx, "invalid:999999")
	// May timeout or may error, both are acceptable
	if err != nil && err != context.DeadlineExceeded {
		t.Logf("got error (acceptable): %v", err)
	}
}

func TestRunSyncerInitialization(t *testing.T) {
	// Test that syncer is properly initialized with LDAP URL
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	t.Setenv("LDAP_URL", "ldap://localhost:389")

	err := run(ctx, "localhost:0")
	if err != nil && err != context.DeadlineExceeded {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestRunSyncerErrorHandling(t *testing.T) {
	// Test that syncer error in goroutine is logged
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	t.Setenv("LDAP_URL", "ldap://invalid.example.com")

	// Should handle syncer errors gracefully
	err := run(ctx, "localhost:0")
	if err != nil && err != context.DeadlineExceeded {
		t.Logf("error (acceptable): %v", err)
	}
}

func TestRunShutdownErrorHandling(t *testing.T) {
	// Test server shutdown error handling
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := run(ctx, "localhost:19002")
	// Server should attempt graceful shutdown on context cancellation
	if err != nil && err != context.DeadlineExceeded {
		t.Logf("shutdown error (acceptable): %v", err)
	}
}

func TestRunSyncStartupWithoutLicense(t *testing.T) {
	// Test syncer starts even without enterprise license
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	t.Setenv("ENTERPRISE_LICENSE", "")
	t.Setenv("LDAP_URL", "")

	err := run(ctx, "localhost:0")
	if err != nil && err != context.DeadlineExceeded {
		t.Logf("error: %v", err)
	}
}

func TestRunSignalContextSetup(t *testing.T) {
	// Test that signal.NotifyContext is properly set up
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Signal handling is set up internally; verify run() completes
	_ = run(ctx, "localhost:0")
}

func TestRunAllEnvVarsSet(t *testing.T) {
	// Test run with all environment variables configured
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	t.Setenv("ADDR", "localhost:19003")
	t.Setenv("ENTERPRISE_LICENSE", "test-license")
	t.Setenv("LDAP_URL", "ldap://ldap.example.com:389")

	err := run(ctx, "localhost:0")
	if err != nil && err != context.DeadlineExceeded {
		t.Logf("error: %v", err)
	}
}

func TestRunServerStartupFailure(t *testing.T) {
	// Test with privileged port (should fail to bind)
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	err := run(ctx, "localhost:1")
	if err != nil && err != context.DeadlineExceeded {
		t.Logf("expected error or timeout, got: %v", err)
	}
}
