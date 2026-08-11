package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/golang-jwt/jwt/v5"
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
// Replication is tenant-scoped: events only replicate to clusters authorized for that tenant.
type Replicator struct {
	mu             sync.RWMutex
	clusters       []*ClusterClient
	logger         *zap.Logger
	signingKey     []byte              // HMAC signing key for issuing machine JWTs (never logged)
	issuer         string              // JWT issuer claim (service identity)
	clusterTenants map[string][]string // cluster name -> list of authorized tenants
	lagSecs        map[string]int64    // lagSecs tracks last known replication lag per cluster name
}

// NewReplicator creates a new Replicator instance with outbound auth signing key.
// signingKey: HMAC secret for issuing machine JWTs (base64-decoded or raw bytes).
// issuer: JWT issuer claim (e.g., "federation-controller@nest").
func NewReplicator(logger *zap.Logger, signingKey []byte, issuer string) *Replicator {
	return &Replicator{
		clusters:       make([]*ClusterClient, 0),
		logger:         logger,
		signingKey:     signingKey,
		issuer:         issuer,
		clusterTenants: make(map[string][]string),
		lagSecs:        make(map[string]int64),
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

// SetClusterTenantsMapping registers which tenants are authorized to replicate to each cluster.
// clusterToTenants maps cluster name to a list of tenant IDs.
// If a tenant is not listed for a cluster, replication to that cluster is blocked for that tenant.
func (r *Replicator) SetClusterTenantsMapping(clusterToTenants map[string][]string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.clusterTenants = clusterToTenants
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

// ReplicateEvent sends an event to clusters authorized for the event's tenant.
// Event: JSON body with resource type, name, tenant, operation (create/update/delete).
// Idempotent: standbys accept re-delivery.
// Only replicates to clusters configured for this event's tenant.
func (r *Replicator) ReplicateEvent(ctx context.Context, event ReplicationEvent) error {
	clusters := r.getClustersForTenant(event.Tenant)
	if len(clusters) == 0 {
		r.logger.Debug("No clusters authorized for replication", zap.String("tenant", event.Tenant))
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

// issueMachineJWT generates a short-lived machine JWT for outbound service-to-service authentication.
// Expires in 5 minutes. Subject is the issuer (service identity).
func (r *Replicator) issueMachineJWT() (string, error) {
	if len(r.signingKey) == 0 {
		return "", fmt.Errorf("signing key not configured")
	}

	now := time.Now()
	claims := jwt.MapClaims{
		"sub": r.issuer,                        // Subject: this service
		"iss": r.issuer,                        // Issuer
		"iat": now.Unix(),                      // Issued at
		"exp": now.Add(5 * time.Minute).Unix(), // Expires in 5 minutes
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString(r.signingKey)
	if err != nil {
		return "", fmt.Errorf("sign JWT: %w", err)
	}

	return tokenString, nil
}

// replicateToCluster sends an event to a single cluster with timeout.
// Attaches a short-lived machine JWT for authentication (issued fresh per request).
// Fails closed: refuses to send unauthenticated requests to prevent auth bypass.
func (r *Replicator) replicateToCluster(ctx context.Context, cluster *ClusterClient, eventJSON []byte) {
	// Defense in depth: ensure signing key is configured before proceeding
	// (main.go enforces this at startup, but check here too in case someone constructs Replicator directly)
	if len(r.signingKey) == 0 {
		r.logger.Error("Cannot replicate: signing key not configured (mandatory for authenticated federation)",
			zap.String("cluster", cluster.Name))
		r.setLag(cluster.Name, -1)
		return
	}

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

	// Issue fresh machine JWT for this request (guaranteed to succeed due to check above)
	token, err := r.issueMachineJWT()
	if err != nil {
		r.logger.Error("Failed to issue machine JWT",
			zap.String("cluster", cluster.Name),
			zap.Error(err))
		r.setLag(cluster.Name, -1)
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)

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

// getClustersForTenant returns clusters authorized for a specific tenant.
// Only clusters that have this tenant in their authorized tenant list are returned.
// If no tenant-cluster mapping is configured, no clusters are returned for any tenant.
func (r *Replicator) getClustersForTenant(tenant string) []*ClusterClient {
	r.mu.RLock()
	defer r.mu.RUnlock()

	// If no mapping is configured, deny replication (zero clusters)
	if len(r.clusterTenants) == 0 {
		return []*ClusterClient{}
	}

	var result []*ClusterClient
	for _, cluster := range r.clusters {
		// Check if this cluster is authorized for the given tenant
		authorizedTenants, ok := r.clusterTenants[cluster.Name]
		if !ok {
			// Cluster not in mapping; deny replication
			continue
		}

		// Check if tenant is in the authorized list for this cluster
		for _, t := range authorizedTenants {
			if t == tenant {
				result = append(result, cluster)
				break
			}
		}
	}

	return result
}
