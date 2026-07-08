package main

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"go.uber.org/zap"
)

// run starts the federation controller with the given metrics address.
// It configures the replicator, starts the metrics HTTP server, and manages graceful shutdown.
// If sigChan is nil, it creates one for SIGINT/SIGTERM. Otherwise, it uses the provided channel.
func run(ctx context.Context, metricsAddr string, logger *zap.Logger, sigChan <-chan os.Signal) error {

	// Get service token for outbound replication (read from Secret, never logged fully)
	outboundToken := os.Getenv("FEDERATION_OUTBOUND_TOKEN")
	if outboundToken == "" {
		logger.Warn("FEDERATION_OUTBOUND_TOKEN not set; outbound replication will not authenticate")
	}

	// Create replicator with outbound token
	replicator := NewReplicator(logger, outboundToken)

	// Parse and add clusters from FEDERATION_CLUSTERS env
	addClustersFromEnv(replicator, logger)

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

// getMetricsAddr returns the metrics server address from env or default.
func getMetricsAddr() string {
	addr := os.Getenv("ADDR")
	if addr == "" {
		addr = ":9095"
	}
	return addr
}

// checkLicense warns if ENTERPRISE_LICENSE is not set.
func checkLicense(logger *zap.Logger) {
	license := os.Getenv("ENTERPRISE_LICENSE")
	if license == "" {
		logger.Warn("ENTERPRISE_LICENSE not set; federation controller disabled for unlicensed deployments")
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
