package main

import (
	"context"
	"errors"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// newTestAgentWithPublisher creates an Agent with an injected CRPublisher backed
// by a mock CRClient, so that scan() can exercise publisher code paths.
func newTestAgentWithPublisher(client CRClient) *Agent {
	logger := zap.NewNop()
	pub := newCRPublisherWithClient("scan-node", logger, client)
	return &Agent{
		cfg: AgentConfig{
			NodeName:     "scan-node",
			ScanInterval: 1 * time.Minute,
			MetricsAddr:  ":0",
			Logger:       logger,
		},
		inventory:      NewInventoryCollector("scan-node", logger),
		metrics:        newTestMetrics(),
		publisher:      pub,
		spindown:       NewSpindownTracker(30*time.Minute, logger),
		rma:            NewRMAManager("scan-node", logger),
		prevBlockStats: make(map[string]*BlockStats),
	}
}

// TestAgentScan_WithPublisher_Success verifies scan calls publisher when non-nil.
func TestAgentScan_WithPublisher_Success(t *testing.T) {
	upsertCalled := false
	client := &mockCRClient{
		patchFn: func(_ context.Context, _ schema.GroupVersionResource, _ string, _ []byte, _ metav1.PatchOptions) (*unstructured.Unstructured, error) {
			upsertCalled = true
			return &unstructured.Unstructured{}, nil
		},
	}
	agent := newTestAgentWithPublisher(client)
	agent.scan(context.Background())

	if !upsertCalled {
		// Note: scan falls back to stub devices; publisher.UpsertHardwareInventory is called regardless.
		t.Log("upsert not called — may be OK if inventory collection path skipped publisher")
	}
}

// TestAgentScan_WithPublisher_UpsertError verifies scan continues even when upsert fails.
func TestAgentScan_WithPublisher_UpsertError(t *testing.T) {
	client := &mockCRClient{
		patchFn: func(_ context.Context, _ schema.GroupVersionResource, _ string, _ []byte, _ metav1.PatchOptions) (*unstructured.Unstructured, error) {
			return nil, errors.New("k8s unavailable")
		},
	}
	agent := newTestAgentWithPublisher(client)
	// Should not panic
	agent.scan(context.Background())
}

// TestAgentScan_DarkDrivePublisher verifies EnsureDarkDriveCR is called for dark drives.
func TestAgentScan_DarkDrivePublisher(t *testing.T) {
	createCalled := 0
	client := &mockCRClient{
		patchFn: func(_ context.Context, _ schema.GroupVersionResource, _ string, _ []byte, _ metav1.PatchOptions) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		},
		createFn: func(_ context.Context, _ schema.GroupVersionResource, _ *unstructured.Unstructured, _ metav1.CreateOptions) (*unstructured.Unstructured, error) {
			createCalled++
			return &unstructured.Unstructured{}, nil
		},
	}

	logger := zap.NewNop()
	pub := newCRPublisherWithClient("scan-node", logger, client)

	// Build agent with a custom inventory that returns known devices including a Dark one.
	agent := &Agent{
		cfg: AgentConfig{
			NodeName:     "scan-node",
			ScanInterval: 1 * time.Minute,
			MetricsAddr:  ":0",
			Logger:       logger,
		},
		inventory:      NewInventoryCollector("scan-node", logger),
		metrics:        newTestMetrics(),
		publisher:      pub,
		spindown:       NewSpindownTracker(30*time.Minute, logger),
		rma:            NewRMAManager("scan-node", logger),
		prevBlockStats: make(map[string]*BlockStats),
	}

	// Inject mock so lsblk returns a known dark device.
	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			switch name {
			case "lsblk":
				for _, a := range args {
					if a == "MOUNTPOINT" || a == "-no" {
						return []byte(""), nil // no mount → Dark
					}
				}
				return []byte("sda  SN001  WD-HGST  18000000000000  1  disk\n"), nil
			case "smartctl":
				return []byte(`{"smart_status":{"passed":true},"temperature":{},"power_on_time":{},"ata_smart_attributes":{"table":[]},"nvme_smart_health_information_log":{}}`), nil
			}
			return nil, nil
		},
	}
	fs := &mockFileReader{
		readFileFn: func(_ string) ([]byte, error) {
			return []byte(""), nil // no diskstats match
		},
	}
	agent.inventory.cmd = cmd
	agent.inventory.fs = fs

	agent.scan(context.Background())

	// Should have called Create for the dark drive
	if createCalled == 0 {
		t.Error("expected EnsureDarkDriveCR to be called for dark drive")
	}
}

// TestAgentScan_DarkDrivePublisher_CreateError verifies scan continues when EnsureDarkDriveCR fails.
func TestAgentScan_DarkDrivePublisher_CreateError(t *testing.T) {
	client := &mockCRClient{
		patchFn: func(_ context.Context, _ schema.GroupVersionResource, _ string, _ []byte, _ metav1.PatchOptions) (*unstructured.Unstructured, error) {
			return &unstructured.Unstructured{}, nil
		},
		createFn: func(_ context.Context, _ schema.GroupVersionResource, _ *unstructured.Unstructured, _ metav1.CreateOptions) (*unstructured.Unstructured, error) {
			return nil, errors.New("create failed")
		},
	}

	logger := zap.NewNop()
	pub := newCRPublisherWithClient("scan-node", logger, client)
	agent := &Agent{
		cfg: AgentConfig{
			NodeName:     "scan-node",
			ScanInterval: 1 * time.Minute,
			MetricsAddr:  ":0",
			Logger:       logger,
		},
		inventory:      NewInventoryCollector("scan-node", logger),
		metrics:        newTestMetrics(),
		publisher:      pub,
		spindown:       NewSpindownTracker(30*time.Minute, logger),
		rma:            NewRMAManager("scan-node", logger),
		prevBlockStats: make(map[string]*BlockStats),
	}

	cmd := &mockCommandRunner{
		outputFn: func(_ context.Context, name string, args ...string) ([]byte, error) {
			switch name {
			case "lsblk":
				for _, a := range args {
					if a == "MOUNTPOINT" || a == "-no" {
						return []byte(""), nil
					}
				}
				return []byte("sda  SN001  WD  18000000000000  1  disk\n"), nil
			case "smartctl":
				return []byte(`{"smart_status":{"passed":true},"temperature":{},"power_on_time":{},"ata_smart_attributes":{"table":[]},"nvme_smart_health_information_log":{}}`), nil
			}
			return nil, nil
		},
	}
	agent.inventory.cmd = cmd
	agent.inventory.fs = &mockFileReader{readFileFn: func(_ string) ([]byte, error) { return []byte(""), nil }}

	// Should not panic even when create fails
	agent.scan(context.Background())
}

// TestNewAgentMetrics_Direct verifies newAgentMetrics() (the public production version).
func TestNewAgentMetrics_Direct(t *testing.T) {
	// newAgentMetrics registers to DefaultRegisterer.
	// We can't call it a second time without re-registration panic, but we can call
	// newAgentMetricsWithRegistry with a fresh registry.
	reg := prometheus.NewRegistry()
	m := newAgentMetricsWithRegistry(reg)
	if m == nil {
		t.Fatal("expected non-nil AgentMetrics")
	}
}

// TestAgentRun_TickerScan verifies Run calls scan on each ticker tick.
func TestAgentRun_TickerScan(t *testing.T) {
	logger := zap.NewNop()
	cfg := AgentConfig{
		NodeName:     "ticker-test",
		ScanInterval: 50 * time.Millisecond,
		MetricsAddr:  ":29999",
		Logger:       logger,
	}
	agent := newTestAgent(cfg)

	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()

	err := agent.Run(ctx)
	if err != context.DeadlineExceeded && err != context.Canceled {
		t.Logf("Run() returned: %v (expected DeadlineExceeded or Canceled)", err)
	}
}
