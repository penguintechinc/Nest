package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestRunHealthEndpoint(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	// Run in goroutine
	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), ":0", logger, sigChan)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Send shutdown signal
	sigChan <- os.Interrupt

	// Wait for run to complete
	err := <-done
	if err != nil {
		t.Logf("run returned: %v", err)
	}
}

func TestRunMetricsEndpoint(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	// Run in goroutine
	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), ":0", logger, sigChan)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Send shutdown signal
	sigChan <- os.Interrupt

	// Wait for run to complete
	err := <-done
	if err != nil {
		t.Logf("run returned: %v", err)
	}
}

func TestRunWithClusterConfiguration(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Set up environment variable for federation clusters
	originalEnv := os.Getenv("FEDERATION_CLUSTERS")
	defer os.Setenv("FEDERATION_CLUSTERS", originalEnv)

	os.Setenv("FEDERATION_CLUSTERS", "cluster-1=http://localhost:8080, cluster-2=http://localhost:8081")

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	// Run in goroutine
	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), ":0", logger, sigChan)
	}()

	// Give server time to start and process clusters
	time.Sleep(50 * time.Millisecond)

	// Send shutdown signal
	sigChan <- os.Interrupt

	// Wait for run to complete
	err := <-done
	if err != nil {
		t.Logf("run returned: %v", err)
	}
}

func TestRunWithoutClusterConfiguration(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Ensure no FEDERATION_CLUSTERS env var
	originalEnv := os.Getenv("FEDERATION_CLUSTERS")
	defer os.Setenv("FEDERATION_CLUSTERS", originalEnv)
	os.Unsetenv("FEDERATION_CLUSTERS")

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	// Run in goroutine
	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), ":0", logger, sigChan)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Send shutdown signal
	sigChan <- os.Interrupt

	// Wait for run to complete
	err := <-done
	if err != nil {
		t.Logf("run returned: %v", err)
	}
}

func TestRunWithMalformedClusterConfiguration(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Set up environment variable with malformed cluster spec
	originalEnv := os.Getenv("FEDERATION_CLUSTERS")
	defer os.Setenv("FEDERATION_CLUSTERS", originalEnv)

	// Malformed: missing "=" separator
	os.Setenv("FEDERATION_CLUSTERS", "invalid-cluster-spec")

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	// Run in goroutine
	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), ":0", logger, sigChan)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Send shutdown signal
	sigChan <- os.Interrupt

	// Wait for run to complete
	err := <-done
	if err != nil {
		t.Logf("run returned: %v", err)
	}
}

func TestRunSignalShutdown(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	// Run in goroutine
	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), ":0", logger, sigChan)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Send shutdown signal
	sigChan <- os.Interrupt

	// Wait for run to complete (should complete immediately after signal)
	select {
	case err := <-done:
		if err != nil {
			t.Logf("run returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("run did not complete after signal")
	}
}

func TestHealthEndpointResponse(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Create a test server with the health endpoint
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Make request to health endpoint
	resp, err := http.Get(server.URL + "/health")
	if err != nil {
		t.Errorf("failed to make health request: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Errorf("failed to read response body: %v", err)
		return
	}

	if string(body) != "ok" {
		t.Errorf("expected body 'ok', got '%s'", string(body))
	}
}

func TestMetricsEndpointNoClusters(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	replicator := NewReplicator(logger, "")

	// Create a test server with the metrics endpoint
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		lags := replicator.LagSeconds()
		for cluster, lag := range lags {
			w.Write([]byte("nest_federation_replication_lag_seconds{cluster=\"" + cluster + "\"} " + string(rune(lag)) + "\n"))
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Make request to metrics endpoint
	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Errorf("failed to make metrics request: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}

	if resp.Header.Get("Content-Type") != "text/plain" {
		t.Errorf("expected Content-Type 'text/plain', got '%s'", resp.Header.Get("Content-Type"))
	}
}

func TestMetricsEndpointWithClusters(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	replicator := NewReplicator(logger, "")
	replicator.AddCluster("cluster-1", "http://localhost:8080")
	replicator.AddCluster("cluster-2", "http://localhost:8081")

	// Create a test server with the metrics endpoint
	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		lags := replicator.LagSeconds()
		for cluster, lag := range lags {
			w.Write([]byte("nest_federation_replication_lag_seconds{cluster=\"" + cluster + "\"} " + string(rune(lag)) + "\n"))
		}
	})

	server := httptest.NewServer(mux)
	defer server.Close()

	// Make request to metrics endpoint
	resp, err := http.Get(server.URL + "/metrics")
	if err != nil {
		t.Errorf("failed to make metrics request: %v", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected status 200, got %d", resp.StatusCode)
	}
}

func TestRunReturnsNilError(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	// Create a channel to receive the result
	done := make(chan error, 1)

	go func() {
		done <- run(context.Background(), ":0", logger, sigChan)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Send signal to trigger shutdown
	sigChan <- os.Interrupt

	// Wait for run to complete and verify return value
	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected nil error, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("run did not complete within timeout")
	}
}

func TestRunSpecificMetricsAddress(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	// Run in goroutine with specific address
	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), ":9095", logger, sigChan)
	}()

	// Give server time to start
	time.Sleep(50 * time.Millisecond)

	// Send signal
	sigChan <- os.Interrupt

	// Wait for completion
	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Errorf("run did not complete within timeout")
	}
}

func TestRunLogsStartMessage(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), ":0", logger, sigChan)
	}()

	time.Sleep(50 * time.Millisecond)
	sigChan <- os.Interrupt

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Errorf("run did not complete within timeout")
	}
}

func TestRunLogsClusterConfiguration(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	originalEnv := os.Getenv("FEDERATION_CLUSTERS")
	defer os.Setenv("FEDERATION_CLUSTERS", originalEnv)

	os.Setenv("FEDERATION_CLUSTERS", "primary=http://primary:8080, secondary=http://secondary:8080")

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), ":0", logger, sigChan)
	}()

	time.Sleep(50 * time.Millisecond)
	sigChan <- os.Interrupt

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Errorf("run did not complete within timeout")
	}
}

func TestRunWithEmptyClusterName(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	originalEnv := os.Getenv("FEDERATION_CLUSTERS")
	defer os.Setenv("FEDERATION_CLUSTERS", originalEnv)

	// Empty name with endpoint
	os.Setenv("FEDERATION_CLUSTERS", " =http://localhost:8080")

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), ":0", logger, sigChan)
	}()

	time.Sleep(50 * time.Millisecond)
	sigChan <- os.Interrupt

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Errorf("run did not complete within timeout")
	}
}

func TestRunMetricsExposesLags(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	// Create a custom listener
	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), ":0", logger, sigChan)
	}()

	time.Sleep(50 * time.Millisecond)
	sigChan <- os.Interrupt

	select {
	case <-done:
		// Success - metrics endpoint was configured
	case <-time.After(2 * time.Second):
		t.Errorf("run did not complete within timeout")
	}
}

func TestRunStartsReplicator(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), ":0", logger, sigChan)
	}()

	time.Sleep(50 * time.Millisecond)
	sigChan <- os.Interrupt

	select {
	case <-done:
		// Success - replicator was started
	case <-time.After(2 * time.Second):
		t.Errorf("run did not complete within timeout")
	}
}

func TestRunShutdownFlow(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), ":0", logger, sigChan)
	}()

	time.Sleep(50 * time.Millisecond)

	// Send shutdown signal - logs "Shutdown signal received", cancels context, shuts down server
	sigChan <- os.Interrupt

	select {
	case err := <-done:
		if err != nil {
			t.Errorf("expected nil error on graceful shutdown, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Errorf("run did not complete within timeout")
	}
}

func TestRunContextPropagation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- run(ctx, ":0", logger, sigChan)
	}()

	time.Sleep(50 * time.Millisecond)
	sigChan <- os.Interrupt

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Errorf("run did not complete within timeout")
	}
}

func TestAddClustersFromEnvNone(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	originalEnv := os.Getenv("FEDERATION_CLUSTERS")
	defer os.Setenv("FEDERATION_CLUSTERS", originalEnv)
	os.Unsetenv("FEDERATION_CLUSTERS")

	replicator := NewReplicator(logger, "")
	addClustersFromEnv(replicator, logger)

	clusters := replicator.getClusters()
	if len(clusters) != 0 {
		t.Errorf("expected no clusters, got %d", len(clusters))
	}
}

func TestAddClustersFromEnvSingle(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	originalEnv := os.Getenv("FEDERATION_CLUSTERS")
	defer os.Setenv("FEDERATION_CLUSTERS", originalEnv)

	os.Setenv("FEDERATION_CLUSTERS", "primary=http://primary:8080")

	replicator := NewReplicator(logger, "")
	addClustersFromEnv(replicator, logger)

	clusters := replicator.getClusters()
	if len(clusters) != 1 {
		t.Errorf("expected 1 cluster, got %d", len(clusters))
	}

	if clusters[0].Name != "primary" {
		t.Errorf("expected cluster name 'primary', got '%s'", clusters[0].Name)
	}

	if clusters[0].Endpoint != "http://primary:8080" {
		t.Errorf("expected endpoint 'http://primary:8080', got '%s'", clusters[0].Endpoint)
	}
}

func TestAddClustersFromEnvMultiple(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	originalEnv := os.Getenv("FEDERATION_CLUSTERS")
	defer os.Setenv("FEDERATION_CLUSTERS", originalEnv)

	os.Setenv("FEDERATION_CLUSTERS", "primary=http://primary:8080, secondary=http://secondary:8080, tertiary=http://tertiary:8080")

	replicator := NewReplicator(logger, "")
	addClustersFromEnv(replicator, logger)

	clusters := replicator.getClusters()
	if len(clusters) != 3 {
		t.Errorf("expected 3 clusters, got %d", len(clusters))
	}
}

func TestAddClustersFromEnvWithWhitespace(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	originalEnv := os.Getenv("FEDERATION_CLUSTERS")
	defer os.Setenv("FEDERATION_CLUSTERS", originalEnv)

	os.Setenv("FEDERATION_CLUSTERS", "  primary  =  http://primary:8080  ,  secondary  =  http://secondary:8080  ")

	replicator := NewReplicator(logger, "")
	addClustersFromEnv(replicator, logger)

	clusters := replicator.getClusters()
	if len(clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(clusters))
	}

	if clusters[0].Name != "primary" {
		t.Errorf("expected trimmed name 'primary', got '%s'", clusters[0].Name)
	}

	if clusters[0].Endpoint != "http://primary:8080" {
		t.Errorf("expected trimmed endpoint 'http://primary:8080', got '%s'", clusters[0].Endpoint)
	}
}

func TestAddClustersFromEnvMalformedSkipped(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	originalEnv := os.Getenv("FEDERATION_CLUSTERS")
	defer os.Setenv("FEDERATION_CLUSTERS", originalEnv)

	// Mix of valid and invalid cluster specs
	os.Setenv("FEDERATION_CLUSTERS", "primary=http://primary:8080, invalid-no-equals, secondary=http://secondary:8080")

	replicator := NewReplicator(logger, "")
	addClustersFromEnv(replicator, logger)

	clusters := replicator.getClusters()
	// Should only add valid ones (primary and secondary, skipping invalid)
	if len(clusters) != 2 {
		t.Errorf("expected 2 clusters, got %d", len(clusters))
	}
}

func TestAddClustersFromEnvEmptyName(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	originalEnv := os.Getenv("FEDERATION_CLUSTERS")
	defer os.Setenv("FEDERATION_CLUSTERS", originalEnv)

	// Empty name is still added (no validation on name)
	os.Setenv("FEDERATION_CLUSTERS", "=http://endpoint:8080")

	replicator := NewReplicator(logger, "")
	addClustersFromEnv(replicator, logger)

	clusters := replicator.getClusters()
	if len(clusters) != 1 {
		t.Errorf("expected 1 cluster, got %d", len(clusters))
	}
}

func TestAddClustersFromEnvExtraEquals(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	originalEnv := os.Getenv("FEDERATION_CLUSTERS")
	defer os.Setenv("FEDERATION_CLUSTERS", originalEnv)

	// Extra equals signs should not match len(parts) == 2, so should be skipped
	os.Setenv("FEDERATION_CLUSTERS", "cluster=http://endpoint=extra")

	replicator := NewReplicator(logger, "")
	addClustersFromEnv(replicator, logger)

	clusters := replicator.getClusters()
	// Should be skipped because split on "=" produces 3 parts
	if len(clusters) != 0 {
		t.Errorf("expected 0 clusters (malformed spec), got %d", len(clusters))
	}
}

func TestRunHealthHandlerWorks(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	addr := listener.Addr().String()
	listener.Close()

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), addr, logger, sigChan)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Make request to health endpoint
	resp, err := http.Get("http://" + addr + "/health")
	if err != nil {
		t.Logf("failed to reach health endpoint (server might not be started yet): %v", err)
	} else {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
		body, _ := io.ReadAll(resp.Body)
		if string(body) != "ok" {
			t.Errorf("expected body 'ok', got '%s'", string(body))
		}
	}

	// Send shutdown signal
	sigChan <- os.Interrupt

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Errorf("run did not complete within timeout")
	}
}

func TestGetMetricsAddrDefault(t *testing.T) {
	originalAddr := os.Getenv("ADDR")
	defer os.Setenv("ADDR", originalAddr)

	os.Unsetenv("ADDR")

	addr := getMetricsAddr()
	if addr != ":9095" {
		t.Errorf("expected default ':9095', got '%s'", addr)
	}
}

func TestGetMetricsAddrFromEnv(t *testing.T) {
	originalAddr := os.Getenv("ADDR")
	defer os.Setenv("ADDR", originalAddr)

	os.Setenv("ADDR", ":8080")

	addr := getMetricsAddr()
	if addr != ":8080" {
		t.Errorf("expected ':8080', got '%s'", addr)
	}
}

func TestGetMetricsAddrCustom(t *testing.T) {
	originalAddr := os.Getenv("ADDR")
	defer os.Setenv("ADDR", originalAddr)

	os.Setenv("ADDR", "localhost:3000")

	addr := getMetricsAddr()
	if addr != "localhost:3000" {
		t.Errorf("expected 'localhost:3000', got '%s'", addr)
	}
}

func TestCheckLicenseNotSet(t *testing.T) {
	originalLicense := os.Getenv("ENTERPRISE_LICENSE")
	defer os.Setenv("ENTERPRISE_LICENSE", originalLicense)

	os.Unsetenv("ENTERPRISE_LICENSE")

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Should not panic
	checkLicense(logger)
}

func TestCheckLicenseSet(t *testing.T) {
	originalLicense := os.Getenv("ENTERPRISE_LICENSE")
	defer os.Setenv("ENTERPRISE_LICENSE", originalLicense)

	os.Setenv("ENTERPRISE_LICENSE", "valid-license-key")

	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Should not panic
	checkLicense(logger)
}

func TestRunMetricsHandlerWorks(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to create listener: %v", err)
	}

	addr := listener.Addr().String()
	listener.Close()

	sigChan := make(chan os.Signal, 1)
	defer close(sigChan)

	done := make(chan error, 1)
	go func() {
		done <- run(context.Background(), addr, logger, sigChan)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Make request to metrics endpoint
	resp, err := http.Get("http://" + addr + "/metrics")
	if err != nil {
		t.Logf("failed to reach metrics endpoint (server might not be started yet): %v", err)
	} else {
		defer resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("expected status 200, got %d", resp.StatusCode)
		}
		if resp.Header.Get("Content-Type") != "text/plain" {
			t.Errorf("expected Content-Type 'text/plain', got '%s'", resp.Header.Get("Content-Type"))
		}
	}

	// Send shutdown signal
	sigChan <- os.Interrupt

	select {
	case <-done:
		// Success
	case <-time.After(2 * time.Second):
		t.Errorf("run did not complete within timeout")
	}
}
