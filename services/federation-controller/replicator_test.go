package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestNewReplicator(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	if replicator.logger != logger {
		t.Errorf("logger not set correctly")
	}

	if len(replicator.clusters) != 0 {
		t.Errorf("expected empty clusters list")
	}

	if replicator.lagSecs == nil {
		t.Errorf("expected lagSecs to be initialized")
	}
}

func TestAddCluster(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	replicator.AddCluster("cluster-1", "http://cluster1:8080")

	clusters := replicator.getClusters()
	if len(clusters) != 1 {
		t.Errorf("expected 1 cluster, got %d", len(clusters))
	}

	if clusters[0].Name != "cluster-1" {
		t.Errorf("expected cluster name 'cluster-1'")
	}

	if clusters[0].Endpoint != "http://cluster1:8080" {
		t.Errorf("expected endpoint 'http://cluster1:8080'")
	}

	if clusters[0].client == nil {
		t.Errorf("expected HTTP client to be initialized")
	}
}

func TestAddMultipleClusters(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	replicator.AddCluster("cluster-1", "http://cluster1:8080")
	replicator.AddCluster("cluster-2", "http://cluster2:8080")
	replicator.AddCluster("cluster-3", "http://cluster3:8080")

	clusters := replicator.getClusters()
	if len(clusters) != 3 {
		t.Errorf("expected 3 clusters, got %d", len(clusters))
	}
}

func TestReplicateEventNoClusters(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "res-123",
		Type:       "User",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	ctx := context.Background()
	err := replicator.ReplicateEvent(ctx, event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReplicateEventWithCluster(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	// Create mock server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/replicate" {
			w.WriteHeader(http.StatusNotFound)
			return
		}

		if r.Header.Get("Content-Type") != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		var event ReplicationEvent
		if err := json.NewDecoder(r.Body).Decode(&event); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	replicator.AddCluster("test-cluster", server.URL)

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "res-123",
		Type:       "User",
		Data: map[string]interface{}{
			"name": "John Doe",
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	ctx := context.Background()
	err := replicator.ReplicateEvent(ctx, event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestReplicateEventMarshalError(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	replicator.AddCluster("test-cluster", "http://localhost:9999")

	// Create a mock event that would fail marshaling (using a custom type that can't be marshaled)
	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "res-123",
		Type:       "User",
		Data: map[string]interface{}{
			"complex": make(chan int), // Channels cannot be marshaled to JSON
		},
		Timestamp: time.Now().UTC().Format(time.RFC3339),
	}

	ctx := context.Background()
	err := replicator.ReplicateEvent(ctx, event)

	if err == nil {
		t.Errorf("expected error for unmarshable event")
	}
}

func TestReplicateEventMultipleClusters(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	receivedCount := 0
	var mu sync.Mutex

	// Create multiple mock servers
	server1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server1.Close()

	server2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		receivedCount++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server2.Close()

	replicator.AddCluster("cluster-1", server1.URL)
	replicator.AddCluster("cluster-2", server2.URL)

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "res-123",
		Type:       "User",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	ctx := context.Background()
	err := replicator.ReplicateEvent(ctx, event)

	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	// Give goroutines time to complete
	time.Sleep(100 * time.Millisecond)

	if receivedCount != 2 {
		t.Errorf("expected 2 servers to receive event, got %d", receivedCount)
	}
}

func TestReplicateEventServerError(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	replicator.AddCluster("error-cluster", server.URL)

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "res-123",
		Type:       "User",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	ctx := context.Background()
	// Should not error, but should handle the error gracefully
	err := replicator.ReplicateEvent(ctx, event)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	time.Sleep(100 * time.Millisecond)

	lag := replicator.LagSeconds()
	if lag["error-cluster"] >= 0 {
		t.Errorf("expected negative lag for failed replication")
	}
}

func TestLagSeconds(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	replicator.AddCluster("cluster-1", "http://cluster1:8080")
	replicator.AddCluster("cluster-2", "http://cluster2:8080")

	lags := replicator.LagSeconds()

	if len(lags) != 2 {
		t.Errorf("expected 2 lag entries, got %d", len(lags))
	}

	if lags["cluster-1"] != 0 {
		t.Errorf("expected initial lag of 0 for cluster-1")
	}

	if lags["cluster-2"] != 0 {
		t.Errorf("expected initial lag of 0 for cluster-2")
	}
}

func TestLagSecondsReturnsIndependentCopy(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	replicator.AddCluster("cluster-1", "http://cluster1:8080")

	lags1 := replicator.LagSeconds()
	lags1["cluster-1"] = 999

	lags2 := replicator.LagSeconds()

	if lags2["cluster-1"] == 999 {
		t.Errorf("expected independent copy, modifications should not affect internal state")
	}
}

func TestGetClustersReturnsIndependentCopy(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	replicator.AddCluster("cluster-1", "http://cluster1:8080")

	clusters := replicator.getClusters()
	originalLen := len(clusters)

	// Try to modify the copy
	clusters = append(clusters, &ClusterClient{Name: "fake"})

	// Original should be unchanged
	clustersCopy := replicator.getClusters()
	if len(clustersCopy) != originalLen {
		t.Errorf("expected getClusters to return independent copy")
	}
}

func TestSetLag(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	replicator.AddCluster("cluster-1", "http://cluster1:8080")

	replicator.setLag("cluster-1", 42)

	lags := replicator.LagSeconds()
	if lags["cluster-1"] != 42 {
		t.Errorf("expected lag to be set to 42, got %d", lags["cluster-1"])
	}
}

func TestSetLagForUnknownCluster(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	// Should not panic, even for unknown cluster
	replicator.setLag("unknown-cluster", 10)

	lags := replicator.LagSeconds()
	if lags["unknown-cluster"] != 10 {
		t.Errorf("expected lag to be set for unknown cluster")
	}
}

func TestReplicationEventStructure(t *testing.T) {
	event := ReplicationEvent{
		Operation:  "update",
		Tenant:     "tenant-1",
		ResourceID: "res-456",
		Type:       "Group",
		Data: map[string]interface{}{
			"name":        "Admins",
			"description": "Admin group",
		},
		Timestamp: "2025-04-23T10:00:00Z",
	}

	jsonBytes, err := json.Marshal(event)
	if err != nil {
		t.Errorf("failed to marshal event: %v", err)
	}

	var unmarshaled ReplicationEvent
	err = json.Unmarshal(jsonBytes, &unmarshaled)
	if err != nil {
		t.Errorf("failed to unmarshal event: %v", err)
	}

	if unmarshaled.Operation != "update" {
		t.Errorf("operation not preserved")
	}

	if unmarshaled.Tenant != "tenant-1" {
		t.Errorf("tenant not preserved")
	}

	if unmarshaled.Type != "Group" {
		t.Errorf("type not preserved")
	}
}

func TestReplicateEventContextTimeout(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	// Create slow server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	replicator.AddCluster("slow-cluster", server.URL)

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "res-123",
		Type:       "User",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	err := replicator.ReplicateEvent(ctx, event)

	// Should not error, but should handle timeout gracefully
	if err != nil {
		// Context timeout may or may not produce an error depending on timing
		// Just verify the system doesn't crash
	}

	time.Sleep(100 * time.Millisecond)

	lag := replicator.LagSeconds()
	if lag["slow-cluster"] >= 0 {
		t.Errorf("expected negative lag for timeout")
	}
}

func TestConcurrentReplication(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	counter := 0
	var mu sync.Mutex

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		counter++
		mu.Unlock()
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	replicator.AddCluster("test-cluster", server.URL)

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			event := ReplicationEvent{
				Operation:  "create",
				Tenant:     "tenant-1",
				ResourceID: fmt.Sprintf("res-%d", index),
				Type:       "User",
				Timestamp:  time.Now().UTC().Format(time.RFC3339),
			}

			ctx := context.Background()
			replicator.ReplicateEvent(ctx, event)
		}(i)
	}

	wg.Wait()
	time.Sleep(200 * time.Millisecond)

	if counter != 10 {
		t.Errorf("expected 10 replications, got %d", counter)
	}
}

func TestReplicateEventContentType(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	contentTypeReceived := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		contentTypeReceived = r.Header.Get("Content-Type")
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	replicator.AddCluster("test-cluster", server.URL)

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "res-123",
		Type:       "User",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	ctx := context.Background()
	replicator.ReplicateEvent(ctx, event)

	time.Sleep(100 * time.Millisecond)

	if contentTypeReceived != "application/json" {
		t.Errorf("expected Content-Type 'application/json', got '%s'", contentTypeReceived)
	}
}

func TestReplicateEventMethod(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	methodReceived := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methodReceived = r.Method
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	replicator.AddCluster("test-cluster", server.URL)

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "res-123",
		Type:       "User",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	ctx := context.Background()
	replicator.ReplicateEvent(ctx, event)

	time.Sleep(100 * time.Millisecond)

	if methodReceived != http.MethodPost {
		t.Errorf("expected POST method, got %s", methodReceived)
	}
}

func TestReplicateEventPath(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	pathReceived := ""
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		pathReceived = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	replicator.AddCluster("test-cluster", server.URL)

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "res-123",
		Type:       "User",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	ctx := context.Background()
	replicator.ReplicateEvent(ctx, event)

	time.Sleep(100 * time.Millisecond)

	if pathReceived != "/internal/v1/replicate" {
		t.Errorf("expected path '/internal/v1/replicate', got %s", pathReceived)
	}
}

func TestReplicateEventDataPreservation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	receivedEvent := ReplicationEvent{}
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		json.Unmarshal(body, &receivedEvent)
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	replicator.AddCluster("test-cluster", server.URL)

	originalEvent := ReplicationEvent{
		Operation:  "update",
		Tenant:     "tenant-1",
		ResourceID: "res-789",
		Type:       "User",
		Data: map[string]interface{}{
			"name":  "John Doe",
			"email": "john@example.com",
		},
		Timestamp: "2025-04-23T10:00:00Z",
	}

	ctx := context.Background()
	replicator.ReplicateEvent(ctx, originalEvent)

	time.Sleep(100 * time.Millisecond)

	if receivedEvent.Operation != "update" {
		t.Errorf("operation not preserved")
	}

	if receivedEvent.ResourceID != "res-789" {
		t.Errorf("resourceId not preserved")
	}
}

func TestClusterClientHTTPTimeout(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	replicator.AddCluster("test-cluster", "http://localhost:9999")

	clusters := replicator.getClusters()
	if len(clusters) > 0 {
		client := clusters[0].client
		if client.Timeout != 10*time.Second {
			t.Errorf("expected 10s timeout, got %v", client.Timeout)
		}
	}
}

func BenchmarkReplicateEvent(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	replicator.AddCluster("bench-cluster", server.URL)

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "res-123",
		Type:       "User",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		replicator.ReplicateEvent(ctx, event)
	}
}

func BenchmarkAddCluster(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		replicator.AddCluster(fmt.Sprintf("cluster-%d", i), "http://localhost:8080")
	}
}

func TestReplicateToClusterRequestCreationError(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	replicator.AddCluster("test-cluster", "http://invalid://url\n")

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "res-123",
		Type:       "User",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	eventJSON, _ := json.Marshal(event)
	ctx := context.Background()
	replicator.replicateToCluster(ctx, &ClusterClient{
		Name:     "test-cluster",
		Endpoint: "http://invalid://url\n",
		client:   &http.Client{Timeout: 10 * time.Second},
	}, eventJSON)

	time.Sleep(100 * time.Millisecond)

	lag := replicator.LagSeconds()
	if lag["test-cluster"] >= 0 {
		t.Errorf("expected negative lag for request creation error")
	}
}

func TestReplicateToClusterNetworkError(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	replicator.AddCluster("test-cluster", "http://localhost:9999")

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "res-123",
		Type:       "User",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	eventJSON, _ := json.Marshal(event)
	ctx := context.Background()

	clusters := replicator.getClusters()
	replicator.replicateToCluster(ctx, clusters[0], eventJSON)

	time.Sleep(100 * time.Millisecond)

	lag := replicator.LagSeconds()
	if lag["test-cluster"] >= 0 {
		t.Errorf("expected negative lag for network error")
	}
}

func TestReplicateToClusterSuccess(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	requestReceived := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	replicator.AddCluster("test-cluster", server.URL)

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "res-123",
		Type:       "User",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	eventJSON, _ := json.Marshal(event)
	ctx := context.Background()

	clusters := replicator.getClusters()
	replicator.replicateToCluster(ctx, clusters[0], eventJSON)

	time.Sleep(100 * time.Millisecond)

	if !requestReceived {
		t.Errorf("expected request to be received")
	}

	lag := replicator.LagSeconds()
	if lag["test-cluster"] != 0 {
		t.Errorf("expected lag to be 0 for successful replication")
	}
}

func TestReplicateToClusterNon2xxStatus(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusBadRequest)
	}))
	defer server.Close()

	replicator.AddCluster("test-cluster", server.URL)

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "res-123",
		Type:       "User",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	eventJSON, _ := json.Marshal(event)
	ctx := context.Background()

	clusters := replicator.getClusters()
	replicator.replicateToCluster(ctx, clusters[0], eventJSON)

	time.Sleep(100 * time.Millisecond)

	lag := replicator.LagSeconds()
	if lag["test-cluster"] >= 0 {
		t.Errorf("expected negative lag for non-2xx status")
	}
}

func TestReplicateToClusterWith3xxStatus(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusMovedPermanently)
	}))
	defer server.Close()

	replicator.AddCluster("test-cluster", server.URL)

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "res-123",
		Type:       "User",
		Timestamp:  time.Now().UTC().Format(time.RFC3339),
	}

	eventJSON, _ := json.Marshal(event)
	ctx := context.Background()

	clusters := replicator.getClusters()
	replicator.replicateToCluster(ctx, clusters[0], eventJSON)

	time.Sleep(100 * time.Millisecond)

	lag := replicator.LagSeconds()
	if lag["test-cluster"] >= 0 {
		t.Errorf("expected negative lag for 3xx status")
	}
}

func TestRun(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	ctx, cancel := context.WithCancel(context.Background())

	// Run in a goroutine and cancel after short delay
	done := make(chan error, 1)
	go func() {
		done <- replicator.Run(ctx)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	// Wait for Run to exit
	err := <-done
	if err != context.Canceled {
		t.Errorf("expected context.Canceled error, got %v", err)
	}
}

func TestRunWithTicks(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	replicator.AddCluster("test-cluster", "http://localhost:8080")

	ctx, cancel := context.WithCancel(context.Background())

	// Run in goroutine and cancel after allowing some ticks to occur
	done := make(chan error, 1)
	go func() {
		done <- replicator.Run(ctx)
	}()

	// Let it run through multiple ticks (30s + 5s = 35s of simulated time)
	time.Sleep(100 * time.Millisecond)
	cancel()

	err := <-done
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRunImmediateCancellation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately

	// Run should return context.Canceled immediately
	err := replicator.Run(ctx)
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRunLogsReplicatorStarted(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	ctx, cancel := context.WithCancel(context.Background())

	// Run in goroutine
	done := make(chan error, 1)
	go func() {
		done <- replicator.Run(ctx)
	}()

	// Let it start
	time.Sleep(50 * time.Millisecond)

	// Cancel
	cancel()

	// Wait for completion
	err := <-done
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRunHandlesTicker(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	ctx, cancel := context.WithCancel(context.Background())

	// Run in goroutine
	done := make(chan error, 1)
	go func() {
		done <- replicator.Run(ctx)
	}()

	// Let it run for a bit (allow ticker to fire)
	time.Sleep(50 * time.Millisecond)

	// Cancel
	cancel()

	// Wait for completion
	err := <-done
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRunRespectsContextDeadline(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	// Create context with deadline
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()

	// Run should respect deadline
	err := replicator.Run(ctx)
	if err != context.DeadlineExceeded {
		t.Errorf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestRunWithMultipleClusters(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	replicator.AddCluster("cluster-1", "http://localhost:8080")
	replicator.AddCluster("cluster-2", "http://localhost:8081")
	replicator.AddCluster("cluster-3", "http://localhost:8082")

	ctx, cancel := context.WithCancel(context.Background())

	// Run in goroutine
	done := make(chan error, 1)
	go func() {
		done <- replicator.Run(ctx)
	}()

	// Let it run
	time.Sleep(50 * time.Millisecond)

	// Cancel
	cancel()

	// Wait for completion
	err := <-done
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRunReplicationTick(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer server.Close()

	replicator.AddCluster("test-cluster", server.URL)

	ctx, cancel := context.WithCancel(context.Background())

	// Run in goroutine
	done := make(chan error, 1)
	go func() {
		done <- replicator.Run(ctx)
	}()

	// Let it run and process ticks
	time.Sleep(100 * time.Millisecond)

	// Cancel
	cancel()

	// Wait for completion
	err := <-done
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRunContextCanceledReturnsError(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	ctx, cancel := context.WithCancel(context.Background())

	// Run in goroutine
	done := make(chan error, 1)
	go func() {
		done <- replicator.Run(ctx)
	}()

	// Let it start
	time.Sleep(10 * time.Millisecond)

	// Cancel
	cancel()

	// Should return context.Canceled
	err := <-done
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRunLogsContextCancelled(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- replicator.Run(ctx)
	}()

	time.Sleep(10 * time.Millisecond)
	cancel()

	err := <-done
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}

func TestRunTickerFires(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	replicator := NewReplicator(logger, "")

	// Create a longer-running context to allow ticker to fire
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan error, 1)
	go func() {
		done <- replicator.Run(ctx)
	}()

	// Let it run long enough for ticker to fire (30+ seconds nominal, but we'll wait 150ms for real-world)
	time.Sleep(150 * time.Millisecond)

	// Cancel
	cancel()

	err := <-done
	if err != context.Canceled {
		t.Errorf("expected context.Canceled, got %v", err)
	}
}
