package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
	"go.uber.org/zap"
)

func TestIssueMachineJWT_GeneratesFreshToken(t *testing.T) {
	logger := zap.NewNop()
	signingKey := []byte("test-signing-key-at-least-32-bytes-long-for-hs256")
	replicator := NewReplicator(logger, signingKey, "test-issuer@nest")

	// Issue two tokens and verify both are valid and carry proper claims
	token1, err := replicator.issueMachineJWT()
	if err != nil {
		t.Fatalf("Failed to issue first JWT: %v", err)
	}

	time.Sleep(1050 * time.Millisecond) // Sleep >1s to ensure different iat times

	token2, err := replicator.issueMachineJWT()
	if err != nil {
		t.Fatalf("Failed to issue second JWT: %v", err)
	}

	// Verify tokens are different (after >1s sleep, iat should differ)
	if token1 == token2 {
		t.Fatal("Expected different JWTs after 1+ second delay, but got identical tokens")
	}

	// Verify both tokens are valid
	for _, tokenStr := range []string{token1, token2} {
		token, err := jwt.ParseWithClaims(tokenStr, jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
			return signingKey, nil
		})
		if err != nil {
			t.Errorf("Failed to parse token: %v", err)
			continue
		}

		claims, ok := token.Claims.(jwt.MapClaims)
		if !ok {
			t.Error("Invalid claims type")
			continue
		}

		if claims["sub"] != "test-issuer@nest" {
			t.Errorf("Expected sub='test-issuer@nest', got %v", claims["sub"])
		}
		if claims["iss"] != "test-issuer@nest" {
			t.Errorf("Expected iss='test-issuer@nest', got %v", claims["iss"])
		}
		if _, ok := claims["iat"].(float64); !ok {
			t.Error("Missing or invalid iat claim")
		}
		if _, ok := claims["exp"].(float64); !ok {
			t.Error("Missing or invalid exp claim")
		}
	}
}

func TestMachineJWT_Expiration(t *testing.T) {
	logger := zap.NewNop()
	signingKey := []byte("test-signing-key-at-least-32-bytes-long-for-hs256")
	replicator := NewReplicator(logger, signingKey, "test-issuer@nest")

	token, err := replicator.issueMachineJWT()
	if err != nil {
		t.Fatalf("Failed to issue JWT: %v", err)
	}

	parsed, err := jwt.ParseWithClaims(token, jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
		return signingKey, nil
	})
	if err != nil {
		t.Fatalf("Failed to parse token: %v", err)
	}

	claims := parsed.Claims.(jwt.MapClaims)
	expTime := int64(claims["exp"].(float64))
	now := time.Now().Unix()

	// Token should expire in approximately 5 minutes (allow 10-second window for test timing)
	expectedExp := now + 300 // 5 minutes
	if expTime < expectedExp-10 || expTime > expectedExp+10 {
		t.Errorf("Token expiration should be ~5 minutes from now, got %d (expected ~%d)", expTime, expectedExp)
	}
}

func TestReplicateToCluster_AttachesMachineJWT(t *testing.T) {
	logger := zap.NewNop()
	signingKey := []byte("test-signing-key-at-least-32-bytes-long-for-hs256")
	replicator := NewReplicator(logger, signingKey, "test-issuer@nest")

	// Mock server to capture the Authorization header
	var capturedAuth string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	cluster := &ClusterClient{
		Name:     "test-cluster",
		Endpoint: mockServer.URL,
		client:   mockServer.Client(),
	}

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "resource-1",
		Type:       "postgres",
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	eventJSON, _ := json.Marshal(event)
	replicator.replicateToCluster(context.Background(), cluster, eventJSON)

	// Verify Authorization header was attached
	if capturedAuth == "" {
		t.Fatal("Expected Authorization header to be set")
	}

	if !strings.HasPrefix(capturedAuth, "Bearer ") {
		t.Errorf("Expected 'Bearer ' prefix, got: %s", capturedAuth)
	}

	// Extract and verify the token
	tokenStr := strings.TrimPrefix(capturedAuth, "Bearer ")
	token, err := jwt.ParseWithClaims(tokenStr, jwt.MapClaims{}, func(token *jwt.Token) (interface{}, error) {
		return signingKey, nil
	})
	if err != nil {
		t.Errorf("Failed to parse attached JWT: %v", err)
	}

	if !token.Valid {
		t.Error("Attached JWT is invalid")
	}
}

func TestReplicateToCluster_NoSigningKey_UnauthenticatedRequest(t *testing.T) {
	logger := zap.NewNop()
	replicator := NewReplicator(logger, []byte{}, "") // No signing key

	var capturedAuth string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	cluster := &ClusterClient{
		Name:     "test-cluster",
		Endpoint: mockServer.URL,
		client:   mockServer.Client(),
	}

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "resource-1",
		Type:       "postgres",
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	eventJSON, _ := json.Marshal(event)
	replicator.replicateToCluster(context.Background(), cluster, eventJSON)

	// Verify no Authorization header was attached (graceful degradation)
	if capturedAuth != "" {
		t.Errorf("Expected no Authorization header when signing key is empty, got: %s", capturedAuth)
	}
}

func TestGetClustersForTenant_FiltersByTenantAffinity(t *testing.T) {
	logger := zap.NewNop()
	replicator := NewReplicator(logger, []byte("key"), "issuer")

	// Add clusters
	replicator.AddCluster("cluster-a", "https://cluster-a.example.com")
	replicator.AddCluster("cluster-b", "https://cluster-b.example.com")
	replicator.AddCluster("cluster-c", "https://cluster-c.example.com")

	// Set tenant affinity: cluster-a serves tenant-1 and tenant-2, cluster-b serves tenant-2 and tenant-3
	// cluster-c has no affinity (not in mapping)
	mapping := map[string][]string{
		"cluster-a": {"tenant-1", "tenant-2"},
		"cluster-b": {"tenant-2", "tenant-3"},
	}
	replicator.SetClusterTenantsMapping(mapping)

	tests := []struct {
		tenant          string
		expectedCluster []string
	}{
		{
			tenant:          "tenant-1",
			expectedCluster: []string{"cluster-a"},
		},
		{
			tenant:          "tenant-2",
			expectedCluster: []string{"cluster-a", "cluster-b"},
		},
		{
			tenant:          "tenant-3",
			expectedCluster: []string{"cluster-b"},
		},
		{
			tenant:          "tenant-4", // Not mapped to any cluster
			expectedCluster: []string{},
		},
	}

	for _, tc := range tests {
		clusters := replicator.getClustersForTenant(tc.tenant)
		clusterNames := make([]string, len(clusters))
		for i, c := range clusters {
			clusterNames[i] = c.Name
		}

		if len(clusterNames) != len(tc.expectedCluster) {
			t.Errorf("Tenant %s: expected %d clusters, got %d: %v",
				tc.tenant, len(tc.expectedCluster), len(clusterNames), clusterNames)
			continue
		}

		// Verify cluster names match (order may vary)
		for _, expected := range tc.expectedCluster {
			found := false
			for _, actual := range clusterNames {
				if actual == expected {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("Tenant %s: expected cluster %s not found in %v",
					tc.tenant, expected, clusterNames)
			}
		}
	}
}

func TestGetClustersForTenant_NoMappingConfigured_DeniesAllReplication(t *testing.T) {
	logger := zap.NewNop()
	replicator := NewReplicator(logger, []byte("key"), "issuer")

	// Add clusters but don't set any tenant affinity mapping
	replicator.AddCluster("cluster-a", "https://cluster-a.example.com")
	replicator.AddCluster("cluster-b", "https://cluster-b.example.com")

	// With no mapping, all tenants should be denied
	for _, tenant := range []string{"tenant-1", "tenant-2", "any-tenant"} {
		clusters := replicator.getClustersForTenant(tenant)
		if len(clusters) != 0 {
			t.Errorf("Tenant %s: expected 0 clusters with no mapping configured, got %d",
				tenant, len(clusters))
		}
	}
}

func TestReplicateEvent_RespectsTenantAffinity(t *testing.T) {
	logger := zap.NewNop()
	replicator := NewReplicator(logger, []byte("key"), "issuer")

	var requestsReceived []string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsReceived = append(requestsReceived, r.URL.String())
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	// Add clusters
	replicator.AddCluster("cluster-a", mockServer.URL)
	replicator.AddCluster("cluster-b", mockServer.URL)

	// Set affinity: tenant-1 -> only cluster-a
	mapping := map[string][]string{
		"cluster-a": {"tenant-1"},
		"cluster-b": {"tenant-2"},
	}
	replicator.SetClusterTenantsMapping(mapping)

	// Replicate event for tenant-1
	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "resource-1",
		Type:       "postgres",
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	err := replicator.ReplicateEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("ReplicateEvent failed: %v", err)
	}

	// Should have received exactly 1 request (cluster-a only)
	if len(requestsReceived) != 1 {
		t.Errorf("Expected 1 replication request for tenant-1, got %d", len(requestsReceived))
	}
}

func TestReplicateEvent_BlocksUnmappedTenant(t *testing.T) {
	logger := zap.NewNop()
	replicator := NewReplicator(logger, []byte("key"), "issuer")

	var requestsReceived []string
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsReceived = append(requestsReceived, r.URL.String())
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	// Add clusters
	replicator.AddCluster("cluster-a", mockServer.URL)

	// Set affinity: only tenant-1 is authorized
	mapping := map[string][]string{
		"cluster-a": {"tenant-1"},
	}
	replicator.SetClusterTenantsMapping(mapping)

	// Try to replicate event for unmapped tenant-999
	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-999",
		ResourceID: "resource-1",
		Type:       "postgres",
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	err := replicator.ReplicateEvent(context.Background(), event)
	if err != nil {
		t.Fatalf("ReplicateEvent should not fail for unmapped tenant: %v", err)
	}

	// Should have received 0 requests (tenant-999 blocked)
	if len(requestsReceived) != 0 {
		t.Errorf("Expected 0 replication requests for unmapped tenant, got %d", len(requestsReceived))
	}
}

func TestReplicateToCluster_RefusesUnauthenticatedSend(t *testing.T) {
	logger := zap.NewNop()
	// Create replicator with NO signing key (simulates defense-in-depth: empty key somehow gets past main)
	replicator := NewReplicator(logger, []byte{}, "test-issuer@nest")

	// Mock server tracks whether it receives any requests
	requestReceived := false
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestReceived = true
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	cluster := &ClusterClient{
		Name:     "test-cluster",
		Endpoint: mockServer.URL,
		client:   mockServer.Client(),
	}

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "resource-1",
		Type:       "postgres",
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	eventJSON, _ := json.Marshal(event)
	replicator.replicateToCluster(context.Background(), cluster, eventJSON)

	// CRITICAL: Verify the request was NEVER sent to the mock server
	if requestReceived {
		t.Fatal("Expected NO HTTP request when signing key is empty (fail-closed), but request was sent")
	}

	// Verify lag was marked as failed
	lags := replicator.LagSeconds()
	if lag, ok := lags["test-cluster"]; !ok || lag != -1 {
		t.Errorf("Expected lag -1 (error state), got %d", lag)
	}
}

func TestReplicateEvent_NoAuthSendBlocked(t *testing.T) {
	logger := zap.NewNop()
	replicator := NewReplicator(logger, []byte{}, "test-issuer@nest") // No signing key

	requestsReceived := 0
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsReceived++
		w.WriteHeader(http.StatusOK)
	}))
	defer mockServer.Close()

	replicator.AddCluster("cluster-a", mockServer.URL)
	mapping := map[string][]string{"cluster-a": {"tenant-1"}}
	replicator.SetClusterTenantsMapping(mapping)

	event := ReplicationEvent{
		Operation:  "create",
		Tenant:     "tenant-1",
		ResourceID: "resource-1",
		Type:       "postgres",
		Timestamp:  time.Now().Format(time.RFC3339),
	}

	replicator.ReplicateEvent(context.Background(), event)

	// Verify NO requests were sent (fail-closed)
	if requestsReceived != 0 {
		t.Errorf("Expected 0 requests with no signing key, got %d", requestsReceived)
	}
}
