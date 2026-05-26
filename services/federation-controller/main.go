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

	"github.com/penguintechinc/nest/shared/licensing"
	"go.uber.org/zap"
)

// run starts the federation controller with the given metrics address.
// It configures the replicator, starts the metrics HTTP server, and manages graceful shutdown.
// If sigChan is nil, it creates one for SIGINT/SIGTERM. Otherwise, it uses the provided channel.
func run(ctx context.Context, metricsAddr string, logger *zap.Logger, sigChan <-chan os.Signal) error {
	// Create replicator
	replicator := NewReplicator(logger)

	// Parse and add clusters from FEDERATION_CLUSTERS env
	addClustersFromEnv(replicator, logger)

	// Setup metrics HTTP server
	mux := http.NewServeMux()
	mux.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("ok"))
	})
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

