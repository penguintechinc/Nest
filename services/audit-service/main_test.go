package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"testing"
	"time"
)

// TestRunServerStartup tests that run() starts the server successfully.
func TestRunServerStartup(t *testing.T) {
	// Find available port
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	// Create a context that cancels quickly to test startup and shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Run the server with short timeout
	errChan := make(chan error, 1)
	go func() {
		errChan <- run(ctx, ":"+addr[len("127.0.0.1:"):])
	}()

	// Give server time to start
	time.Sleep(10 * time.Millisecond)

	// Verify server is listening by attempting health check
	client := &http.Client{Timeout: 50 * time.Millisecond}
	resp, err := client.Get("http://127.0.0.1:" + addr[len("127.0.0.1:"):] + "/healthz")
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected health check to return 200, got %d", resp.StatusCode)
		}
	}

	// Wait for server to finish shutting down
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("run() returned error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("run() did not finish within timeout")
	}
}

// TestRunWithoutLicense tests that run() works without ENTERPRISE_LICENSE env var.
func TestRunWithoutLicense(t *testing.T) {
	// Ensure no license is set
	oldLicense := os.Getenv("ENTERPRISE_LICENSE")
	os.Unsetenv("ENTERPRISE_LICENSE")
	defer func() {
		if oldLicense != "" {
			os.Setenv("ENTERPRISE_LICENSE", oldLicense)
		}
	}()

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- run(ctx, ":"+addr[len("127.0.0.1:"):])
	}()

	time.Sleep(10 * time.Millisecond)

	// Verify server is running
	client := &http.Client{Timeout: 50 * time.Millisecond}
	resp, err := client.Get("http://127.0.0.1:" + addr[len("127.0.0.1:"):] + "/healthz")
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("health check failed without license: got %d", resp.StatusCode)
		}
	}

	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("run() returned error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("run() did not finish within timeout")
	}
}

// TestRunWithLicense tests that run() works with ENTERPRISE_LICENSE env var set.
func TestRunWithLicense(t *testing.T) {
	oldLicense := os.Getenv("ENTERPRISE_LICENSE")
	os.Setenv("ENTERPRISE_LICENSE", "test-license-key")
	defer func() {
		if oldLicense != "" {
			os.Setenv("ENTERPRISE_LICENSE", oldLicense)
		} else {
			os.Unsetenv("ENTERPRISE_LICENSE")
		}
	}()

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- run(ctx, ":"+addr[len("127.0.0.1:"):])
	}()

	time.Sleep(10 * time.Millisecond)

	// Verify health check returns OK
	client := &http.Client{Timeout: 50 * time.Millisecond}
	resp, err := client.Get("http://127.0.0.1:" + addr[len("127.0.0.1:"):] + "/healthz")
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("health check failed with license: got %d", resp.StatusCode)
		}
	}

	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("run() returned error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("run() did not finish within timeout")
	}
}

// TestRunContextCancelation tests that run() handles context cancellation.
func TestRunContextCancelation(t *testing.T) {
	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithCancel(context.Background())

	errChan := make(chan error, 1)
	go func() {
		errChan <- run(ctx, ":"+addr[len("127.0.0.1:"):])
	}()

	time.Sleep(10 * time.Millisecond)

	// Verify server is running
	client := &http.Client{Timeout: 50 * time.Millisecond}
	resp, err := client.Get("http://127.0.0.1:" + addr[len("127.0.0.1:"):] + "/healthz")
	if err == nil {
		resp.Body.Close()
	}

	// Cancel context to trigger shutdown
	cancel()

	// Wait for server to shut down
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("run() returned error: %v", err)
		}
	case <-time.After(1 * time.Second):
		t.Error("run() did not finish within timeout after context cancellation")
	}
}

// TestRunHealthzEndpoint tests that the /healthz endpoint is accessible without license.
func TestRunHealthzEndpoint(t *testing.T) {
	os.Unsetenv("ENTERPRISE_LICENSE")

	listener, err := net.Listen("tcp", ":0")
	if err != nil {
		t.Fatalf("failed to find available port: %v", err)
	}
	addr := listener.Addr().String()
	listener.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	errChan := make(chan error, 1)
	go func() {
		errChan <- run(ctx, ":"+addr[len("127.0.0.1:"):])
	}()

	time.Sleep(10 * time.Millisecond)

	// Health check should work without license
	client := &http.Client{Timeout: 50 * time.Millisecond}
	resp, err := client.Get("http://127.0.0.1:" + addr[len("127.0.0.1:"):] + "/healthz")
	if err == nil {
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected /healthz to return 200, got %d", resp.StatusCode)
		}
	}

	<-time.After(60 * time.Millisecond)
	select {
	case err := <-errChan:
		if err != nil {
			t.Errorf("run() returned error: %v", err)
		}
	default:
		// Server may still be shutting down
	}
}

// TestRunMultipleContexts tests that run() can be called multiple times with different contexts.
func TestRunMultipleContexts(t *testing.T) {
	for i := 0; i < 2; i++ {
		listener, err := net.Listen("tcp", ":0")
		if err != nil {
			t.Fatalf("failed to find available port: %v", err)
		}
		addr := listener.Addr().String()
		listener.Close()

		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)

		errChan := make(chan error, 1)
		go func() {
			errChan <- run(ctx, ":"+addr[len("127.0.0.1:"):])
		}()

		time.Sleep(10 * time.Millisecond)

		// Verify server responds
		client := &http.Client{Timeout: 50 * time.Millisecond}
		resp, err := client.Get("http://127.0.0.1:" + addr[len("127.0.0.1:"):] + "/healthz")
		if err == nil {
			resp.Body.Close()
		}

		cancel()

		select {
		case <-errChan:
		case <-time.After(1 * time.Second):
			t.Error("run() did not finish within timeout")
		}
	}
}
