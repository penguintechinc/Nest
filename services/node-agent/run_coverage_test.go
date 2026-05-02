package main

import (
	"context"
	"fmt"
	"net/http"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// TestAgentRun_HealthEndpoint verifies the /health endpoint responds 200.
func TestAgentRun_HealthEndpoint(t *testing.T) {
	logger := zap.NewNop()
	port := 39091
	cfg := AgentConfig{
		NodeName:     "health-run-node",
		ScanInterval: 10 * time.Second,
		MetricsAddr:  fmt.Sprintf(":%d", port),
		Logger:       logger,
	}
	agent := newTestAgent(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		done <- agent.Run(ctx)
	}()

	// Wait for server to start
	deadline := time.Now().Add(3 * time.Second)
	var resp *http.Response
	var err error
	for time.Now().Before(deadline) {
		resp, err = http.Get(fmt.Sprintf("http://localhost:%d/health", port))
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		cancel()
		t.Fatalf("health endpoint not reachable: %v", err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		cancel()
		t.Errorf("health endpoint returned %d, want 200", resp.StatusCode)
	}

	// Also hit /metrics
	mResp, mErr := http.Get(fmt.Sprintf("http://localhost:%d/metrics", port))
	if mErr == nil {
		mResp.Body.Close()
	}

	cancel()
	<-done
}

// TestAgentScan_PublisherNilNoPanic verifies scan runs without panic when publisher is nil.
func TestAgentScan_PublisherNilNoPanic(t *testing.T) {
	logger := zap.NewNop()
	agent := &Agent{
		cfg: AgentConfig{
			NodeName:     "nil-pub-node",
			ScanInterval: 1 * time.Minute,
			MetricsAddr:  ":0",
			Logger:       logger,
		},
		inventory:      NewInventoryCollector("nil-pub-node", logger),
		metrics:        newAgentMetricsWithRegistry(prometheus.NewRegistry()),
		publisher:      nil, // explicitly nil
		spindown:       NewSpindownTracker(30*time.Minute, logger),
		rma:            NewRMAManager("nil-pub-node", logger),
		prevBlockStats: make(map[string]*BlockStats),
	}
	// Should not panic
	agent.scan(context.Background())
}
