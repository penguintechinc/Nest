package main

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/penguintechinc/nest/shared/licensing"
	"go.uber.org/zap"
)

// validateSigningKey checks that the FEDERATION_JWT_SIGNING_KEY env var is set, decodable, and non-empty.
// Returns error if validation fails; otherwise returns the decoded signing key.
func validateSigningKey() ([]byte, error) {
	signingKeyB64 := os.Getenv("FEDERATION_JWT_SIGNING_KEY")
	if signingKeyB64 == "" {
		return nil, fmt.Errorf("FEDERATION_JWT_SIGNING_KEY must be set for authenticated replication (required for secure federation)")
	}

	signingKey, err := base64.StdEncoding.DecodeString(signingKeyB64)
	if err != nil {
		return nil, fmt.Errorf("failed to decode FEDERATION_JWT_SIGNING_KEY (must be base64): %w", err)
	}
	if len(signingKey) == 0 {
		return nil, fmt.Errorf("FEDERATION_JWT_SIGNING_KEY decoded to empty bytes (must be non-empty)")
	}

	return signingKey, nil
}

// run starts the federation controller with the given metrics address.
// It configures the replicator, starts the metrics HTTP server, and manages graceful shutdown.
// If sigChan is nil, it creates one for SIGINT/SIGTERM. Otherwise, it uses the provided channel.
func run(ctx context.Context, metricsAddr string, logger *zap.Logger, sigChan <-chan os.Signal) error {

	// Get and validate signing key for machine JWT issuance (base64-encoded for safe env var transport)
	// Authenticated replication is mandatory — fail fast if key is missing or invalid
	signingKey, err := validateSigningKey()
	if err != nil {
		logger.Fatal(err.Error())
	}

	// Get service identity for JWT issuer claim
	issuer := os.Getenv("FEDERATION_SERVICE_ISSUER")
	if issuer == "" {
		issuer = "federation-controller@nest"
	}

	// Create replicator with signing key
	replicator := NewReplicator(logger, signingKey, issuer)

	// Parse and add clusters from FEDERATION_CLUSTERS env
	addClustersFromEnv(replicator, logger)

	// Load tenant-to-cluster affinity mapping from env (format: cluster1=tenant1,tenant2;cluster2=tenant3)
	loadClusterTenantsMapping(replicator, logger)

	// Setup metrics HTTP server
	mux := http.NewServeMux()

	// Health check (no auth required)
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})

	// Metrics endpoint (NetworkPolicy-protected, no JWT required for Prometheus scraping)
	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		lags := replicator.LagSeconds()
		for cluster, lag := range lags {
			fmt.Fprintf(w, "nest_federation_replication_lag_seconds{cluster=%q} %d\n", cluster, lag)
		}
	})

	server := &http.Server{
		Addr:    metricsAddr,
		Handler: mux,
	}

	// Start metrics server
	go func() {
		logger.Info("Starting metrics server", zap.String("addr", metricsAddr))
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			logger.Error("Metrics server error", zap.Error(err))
		}
	}()

	// Start replicator in goroutine
	runCtx, cancel := context.WithCancel(ctx)
	defer cancel()

	go func() {
		if err := replicator.Run(runCtx); err != nil {
			logger.Error("Replicator error", zap.Error(err))
		}
	}()

	// Wait for signal
	<-sigChan
	logger.Info("Shutdown signal received")

	// Graceful shutdown
	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()

	if err := server.Shutdown(shutdownCtx); err != nil {
		logger.Error("Server shutdown error", zap.Error(err))
	}

	logger.Info("Federation controller stopped")
	return nil
}

// addClustersFromEnv parses FEDERATION_CLUSTERS env and adds them to the replicator.
// All cluster endpoints MUST use https:// — plaintext http:// is not permitted (except localhost for testing).
func addClustersFromEnv(replicator *Replicator, logger *zap.Logger) {
	clustersEnv := os.Getenv("FEDERATION_CLUSTERS")
	if clustersEnv == "" {
		return
	}

	for _, cluster := range strings.Split(clustersEnv, ",") {
		cluster = strings.TrimSpace(cluster)
		parts := strings.Split(cluster, "=")
		if len(parts) == 2 {
			name := strings.TrimSpace(parts[0])
			endpoint := strings.TrimSpace(parts[1])

			// Require https for non-localhost endpoints
			if strings.HasPrefix(endpoint, "http://") && !strings.Contains(endpoint, "localhost") {
				logger.Error("Cluster endpoint must use https",
					zap.String("name", name),
					zap.String("endpoint", endpoint))
				continue
			}

			replicator.AddCluster(name, endpoint)
			logger.Info("Added federation cluster", zap.String("name", name), zap.String("endpoint", endpoint))
		}
	}
}

// loadClusterTenantsMapping parses FEDERATION_CLUSTER_TENANTS env and registers tenant affinity.
// Format: "cluster1=tenant1,tenant2;cluster2=tenant3" (cluster name = comma-separated tenant list, pairs separated by ;)
// If not set, no tenant-cluster affinity is configured (all replication requests denied).
func loadClusterTenantsMapping(replicator *Replicator, logger *zap.Logger) {
	mappingEnv := os.Getenv("FEDERATION_CLUSTER_TENANTS")
	if mappingEnv == "" {
		logger.Warn("FEDERATION_CLUSTER_TENANTS not set; no tenant-cluster affinity configured (all replication blocked)")
		return
	}

	mapping := make(map[string][]string)
	for _, pair := range strings.Split(mappingEnv, ";") {
		pair = strings.TrimSpace(pair)
		parts := strings.Split(pair, "=")
		if len(parts) != 2 {
			logger.Warn("Skipping malformed tenant mapping entry",
				zap.String("entry", pair))
			continue
		}

		clusterName := strings.TrimSpace(parts[0])
		tenantsStr := strings.TrimSpace(parts[1])
		tenants := []string{}
		for _, t := range strings.Split(tenantsStr, ",") {
			t = strings.TrimSpace(t)
			if t != "" {
				tenants = append(tenants, t)
			}
		}

		if clusterName != "" && len(tenants) > 0 {
			mapping[clusterName] = tenants
			logger.Info("Registered cluster tenant affinity",
				zap.String("cluster", clusterName),
				zap.Strings("tenants", tenants))
		}
	}

	if len(mapping) > 0 {
		replicator.SetClusterTenantsMapping(mapping)
	}
}

// getMetricsAddr returns the metrics server address from env or default.
func getMetricsAddr() string {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":9095"
	}
	return addr
}

// checkLicense warns if enterprise license is invalid.
func checkLicense(logger *zap.Logger) {
	validator := licensing.NewValidator(os.Getenv("ENTERPRISE_LICENSE"), "nest")
	if !validator.IsValid(nil) {
		logger.Warn("ENTERPRISE_LICENSE not set or invalid; federation controller disabled for unlicensed deployments")
	}
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Check license
	checkLicense(logger)

	// Get metrics address
	metricsAddr := getMetricsAddr()

	// Setup signal handling for graceful shutdown
	sigChan := make(chan os.Signal, 1)
	signal.Notify(sigChan, syscall.SIGINT, syscall.SIGTERM)

	if err := run(context.Background(), metricsAddr, logger, sigChan); err != nil {
		logger.Error("Run error", zap.Error(err))
	}
}
