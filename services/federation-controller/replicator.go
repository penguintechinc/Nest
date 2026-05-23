package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"go.uber.org/zap"
)

// ClusterClient wraps an HTTP client for communicating with a remote Nest cluster.
type ClusterClient struct {
	Name     string
	Endpoint string
	client   *http.Client
}

// ReplicationEvent represents an event to be replicated to standby clusters.
type ReplicationEvent struct {
	Operation  string                 `json:"operation"` // create, update, delete
	Tenant     string                 `json:"tenant"`
	ResourceID string                 `json:"resourceId"`
	Type       string                 `json:"type"`
	Data       map[string]interface{} `json:"data,omitempty"`
	Timestamp  string                 `json:"timestamp"`
}

// Replicator watches the local API and replicates DataResource state to standby clusters.
type Replicator struct {
	mu       sync.RWMutex
	clusters []*ClusterClient
	logger   *zap.Logger
	// lagSecs tracks last known replication lag per cluster name
	lagSecs map[string]int64
}

// NewReplicator creates a new Replicator instance.
func NewReplicator(logger *zap.Logger) *Replicator {
	return &Replicator{
		clusters: make([]*ClusterClient, 0),
		logger:   logger,
		lagSecs:  make(map[string]int64),
	}
}

// AddCluster registers a standby cluster endpoint.
func (r *Replicator) AddCluster(name, endpoint string) {
	r.mu.Lock()
	defer r.mu.Unlock()

	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	r.clusters = append(r.clusters, &ClusterClient{
		Name:     name,
		Endpoint: endpoint,
		client:   client,
	})

	r.lagSecs[name] = 0
}

// Run starts the replication loop. Polls every 30s.
// In production: watches K8s events; for P8 stub: polls a local state snapshot.
func (r *Replicator) Run(ctx context.Context) error {
	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()

	r.logger.Info("Replicator started")

	for {
		select {
		case <-ctx.Done():
			r.logger.Info("Replicator context cancelled")
			return ctx.Err()
		case <-ticker.C:
			// Replication tick - in production, this would watch K8s events
			r.logger.Debug("replication tick", zap.Int("clusters", len(r.getClusters())))

			// Stub: no actual events to replicate in P8
			// In production, would:
			// 1. Query K8s for DataResource changes
			// 2. For each change, create ReplicationEvent
			// 3. Call ReplicateEvent(ctx, event)
		}
	}
}

// ReplicateEvent sends an event to all registered standbys.
// Event: JSON body with resource type, name, tenant, operation (create/update/delete).
// Idempotent: standbys accept re-delivery.
func (r *Replicator) ReplicateEvent(ctx context.Context, event ReplicationEvent) error {
	clusters := r.getClusters()
	if len(clusters) == 0 {
		r.logger.Debug("No clusters registered for replication")
		return nil
	}

	eventJSON, err := json.Marshal(event)
	if err != nil {
		r.logger.Error("Failed to marshal replication event", zap.Error(err))
		return err
	}

	var wg sync.WaitGroup
	for _, cluster := range clusters {
		wg.Add(1)
		go func(c *ClusterClient) {
			defer wg.Done()
			r.replicateToCluster(ctx, c, eventJSON)
		}(cluster)
	}

	wg.Wait()
	return nil
}

// replicateToCluster sends an event to a single cluster with timeout.
func (r *Replicator) replicateToCluster(ctx context.Context, cluster *ClusterClient, eventJSON []byte) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	url := fmt.Sprintf("%s/internal/v1/replicate", cluster.Endpoint)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(eventJSON))
	if err != nil {
		r.logger.Error("Failed to create replication request",
			zap.String("cluster", cluster.Name),
			zap.Error(err))
		r.setLag(cluster.Name, -1)
		return
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := cluster.client.Do(req)
	if err != nil {
		r.logger.Warn("Failed to replicate to cluster",
			zap.String("cluster", cluster.Name),
			zap.String("endpoint", cluster.Endpoint),
			zap.Error(err))
		r.setLag(cluster.Name, -1)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		r.logger.Debug("Successfully replicated to cluster",
			zap.String("cluster", cluster.Name))
		r.setLag(cluster.Name, 0)
	} else {
		r.logger.Warn("Cluster returned non-2xx status",
			zap.String("cluster", cluster.Name),
			zap.Int("status", resp.StatusCode))
		r.setLag(cluster.Name, -1)
	}
}

// LagSeconds returns the reported lag for each cluster.
func (r *Replicator) LagSeconds() map[string]int64 {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make(map[string]int64)
	for k, v := range r.lagSecs {
		result[k] = v
	}
	return result
}

// setLag updates the lag for a cluster (thread-safe).
func (r *Replicator) setLag(clusterName string, lag int64) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.lagSecs[clusterName] = lag
}

// getClusters returns a copy of the clusters list (thread-safe).
func (r *Replicator) getClusters() []*ClusterClient {
	r.mu.RLock()
	defer r.mu.RUnlock()

	result := make([]*ClusterClient, len(r.clusters))
	copy(result, r.clusters)
	return result
}
