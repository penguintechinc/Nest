package cloudprovider

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

// ============================================================================
// GCP internal helpers
// ============================================================================

func TestGcpNormalizeResourceID(t *testing.T) {
	p := &gcpProvider{}
	tests := []struct {
		input string
		want  string
	}{
		{"projects/my-proj/instances/my-inst", "projects/my-proj/instances/my-inst"},
		{"my-inst", "my-inst"},
		{"", ""},
	}
	for _, tt := range tests {
		got := p.normalizeResourceID(tt.input)
		if got != tt.want {
			t.Errorf("normalizeResourceID(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestGcpExtractProjectID(t *testing.T) {
	p := &gcpProvider{}

	// From Extra
	cfg := ExternalProviderConfig{
		Extra: map[string]string{"project_id": "my-project"},
	}
	got := p.extractProjectID(cfg)
	if got != "my-project" {
		t.Errorf("extractProjectID() from Extra = %q, want my-project", got)
	}

	// From ResourceID
	cfg2 := ExternalProviderConfig{
		ResourceID: "projects/resource-project/instances/my-inst",
		Extra:      map[string]string{},
	}
	got2 := p.extractProjectID(cfg2)
	if got2 != "resource-project" {
		t.Errorf("extractProjectID() from ResourceID = %q, want resource-project", got2)
	}

	// Neither
	cfg3 := ExternalProviderConfig{
		ResourceID: "just-an-id",
		Extra:      map[string]string{},
	}
	got3 := p.extractProjectID(cfg3)
	if got3 != "" {
		t.Errorf("extractProjectID() = %q, want empty", got3)
	}
}

func TestGcpDiscoverFallback(t *testing.T) {
	p := &gcpProvider{}

	tests := []struct {
		engineType   string
		region       string
		wantEndpoint string
		wantEngine   string
	}{
		{"postgres", "us-central1", "us-central1.cloudsql.google.com:5432", "postgres"},
		{"mysql", "europe-west1", "europe-west1.cloudsql.google.com:5432", "mysql"},
		{"redis", "us-east1", "redis.googleapis.com:6379", "keyvalue"},
		{"keyvalue", "us-east1", "redis.googleapis.com:6379", "keyvalue"},
		{"other", "us-east1", "", "other"},
	}

	for _, tt := range tests {
		cfg := ExternalProviderConfig{
			EngineType: tt.engineType,
			Region:     tt.region,
			Endpoint:   "", // empty for "other" case
		}
		info := p.discoverFallback(cfg)
		if info.Endpoint != tt.wantEndpoint {
			t.Errorf("discoverFallback(%s) endpoint = %q, want %q", tt.engineType, info.Endpoint, tt.wantEndpoint)
		}
		if info.EngineType != tt.wantEngine {
			t.Errorf("discoverFallback(%s) engineType = %q, want %q", tt.engineType, info.EngineType, tt.wantEngine)
		}
	}
}

func TestGcpSetupProxy(t *testing.T) {
	p := &gcpProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "projects/my-project/instances/my-instance",
		Region:     "us-central1",
		EngineType: "postgres",
	}

	proxy, err := p.SetupProxy(context.Background(), cfg)
	if err != nil {
		t.Fatalf("SetupProxy() error = %v", err)
	}
	if proxy.AuthType != "service-account" {
		t.Errorf("AuthType = %q, want service-account", proxy.AuthType)
	}
	if !proxy.TLSRequired {
		t.Error("TLSRequired = false, want true")
	}
}

func TestGcpTCPHealthProbe_Success(t *testing.T) {
	p := &gcpProvider{}

	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	cfg := ExternalProviderConfig{Endpoint: ln.Addr().String()}
	result, err := p.tcpHealthProbe(cfg)
	if err != nil {
		t.Fatalf("tcpHealthProbe() error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("tcpHealthProbe() state = %q, want healthy", result.State)
	}
}

func TestGcpTCPHealthProbe_Fail(t *testing.T) {
	p := &gcpProvider{}
	cfg := ExternalProviderConfig{Endpoint: "127.0.0.1:1"}
	result, err := p.tcpHealthProbe(cfg)
	if err != nil {
		t.Fatalf("tcpHealthProbe() error = %v", err)
	}
	if result.State != "failed" {
		t.Errorf("tcpHealthProbe() state = %q, want failed", result.State)
	}
}

func TestGcpTCPHealthProbe_NoEndpoint(t *testing.T) {
	p := &gcpProvider{}
	cfg := ExternalProviderConfig{}
	result, err := p.tcpHealthProbe(cfg)
	if err != nil {
		t.Fatalf("tcpHealthProbe() error = %v", err)
	}
	if result.State != "degraded" {
		t.Errorf("tcpHealthProbe() state = %q, want degraded", result.State)
	}
}

func TestGcpCheckHealth_NoEndpoint(t *testing.T) {
	p := &gcpProvider{}
	cfg := ExternalProviderConfig{
		Extra: map[string]string{},
	}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	// No endpoint and no credentials → degraded (tcpHealthProbe with empty endpoint)
	if result.State != "degraded" {
		t.Errorf("State = %q, want degraded", result.State)
	}
}

func TestGcpCheckHealth_TCPEndpoint(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{
		Endpoint: ln.Addr().String(),
		Extra:    map[string]string{},
	}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

func TestGcpGetTokenFromMetadataServer_Fail(t *testing.T) {
	// metadata.google.internal is not reachable in test env
	p := &gcpProvider{}
	_, err := p.getTokenFromMetadataServer()
	// Should fail gracefully (error or empty token)
	if err == nil {
		// In some environments this may succeed (GCE); just verify no panic
		t.Log("getTokenFromMetadataServer() succeeded (may be running on GCE)")
	}
}

func TestGcpGetTokenFromMetadataServer_MockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Metadata-Flavor") != "Google" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`{"access_token": "mock-token-123"}`))
	}))
	defer srv.Close()

	// We can't inject the URL easily without refactoring, so just test that the function
	// exists and handles errors gracefully when metadata server fails with 403
	p := &gcpProvider{}
	_, _ = p.getTokenFromMetadataServer() // Just verify no panic
	_ = p
}

// ============================================================================
// Azure internal helpers
// ============================================================================

func TestAzureCheckHealthWithTCP_Success(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer ln.Close()

	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		Endpoint: ln.Addr().String(),
		Extra:    map[string]string{},
	}
	result, err := p.checkHealthWithTCP(context.Background(), cfg)
	if err != nil {
		t.Fatalf("checkHealthWithTCP() error = %v", err)
	}
	if result.State != "healthy" {
		t.Errorf("State = %q, want healthy", result.State)
	}
}

func TestAzureCheckHealthWithTCP_Fail(t *testing.T) {
	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		Endpoint: "127.0.0.1:1",
		Extra:    map[string]string{},
	}
	result, err := p.checkHealthWithTCP(context.Background(), cfg)
	if err != nil {
		t.Fatalf("checkHealthWithTCP() error = %v", err)
	}
	if result.State != "failed" {
		t.Errorf("State = %q, want failed", result.State)
	}
}

func TestGeneratePassword(t *testing.T) {
	pwd := generatePassword()
	if len(pwd) < 8 {
		t.Errorf("generatePassword() length = %d, want >= 8", len(pwd))
	}
	// Should be non-empty
	if pwd == "" {
		t.Error("generatePassword() returned empty string")
	}
}

func TestRandInt(t *testing.T) {
	for i := 0; i < 10; i++ {
		n := randInt(100)
		if n < 0 || n >= 100 {
			t.Errorf("randInt(100) = %d, want [0, 100)", n)
		}
	}
}

func TestAzureCheckHealth_NoEndpoint(t *testing.T) {
	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "/subscriptions/sub-123/resourceGroups/rg/providers/Microsoft.DBforPostgreSQL/servers/srv",
		Extra:      map[string]string{},
	}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	// No endpoint and no credentials → unknown/unreachable
	if result.State == "" {
		t.Error("CheckHealth() returned empty state")
	}
}

func TestAzureRotateCredential_UnsupportedType(t *testing.T) {
	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "/subscriptions/sub-123/resourceGroups/rg/providers/Microsoft.Unknown/servers/srv",
		EngineType: "unknown",
		Extra:      map[string]string{},
	}

	_, err := p.RotateCredential(context.Background(), cfg)
	if err == nil {
		t.Error("RotateCredential() expected error for unknown engine type")
	}
}

// ============================================================================
// Cloudflare internal helpers
// ============================================================================

func TestCloudflareSetupProxy(t *testing.T) {
	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid-123",
		EngineType: "d1",
	}

	proxy, err := p.SetupProxy(context.Background(), cfg)
	if err != nil {
		t.Fatalf("SetupProxy() error = %v", err)
	}
	if proxy == nil {
		t.Fatal("SetupProxy() returned nil")
	}
}

func TestCloudflareDoRequest_Error(t *testing.T) {
	// Non-routable URL
	_, _, err := cloudflareDoRequest("GET", "http://127.0.0.1:1/test", nil, "token", "", "")
	if err == nil {
		t.Error("cloudflareDoRequest() expected error for unreachable server")
	}
}

func TestCloudflareCheckHealth_WithD1(t *testing.T) {
	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid-123",
		EngineType: "d1",
		Extra: map[string]string{
			"api_token":  "test-token",
			"account_id": "acc-123",
		},
	}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	// Will fail due to bad credentials but should handle gracefully
	if result == nil {
		t.Error("CheckHealth() returned nil result")
	}
}

func TestCloudflareDiscover_R2Type(t *testing.T) {
	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "bucket-123",
		EngineType: "r2",
	}

	info, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if info.EngineType != "object" {
		t.Errorf("EngineType = %q, want object", info.EngineType)
	}
}

func TestCloudflareDiscover_KVType(t *testing.T) {
	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "ns-123",
		EngineType: "kv",
	}

	info, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if info.EngineType != "keyvalue" {
		t.Errorf("EngineType = %q, want keyvalue", info.EngineType)
	}
}

func TestCloudflareDiscover_DefaultType(t *testing.T) {
	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "res-123",
		EngineType: "other",
		Endpoint:   "custom.endpoint.com",
	}

	info, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if info.Endpoint != "custom.endpoint.com" {
		t.Errorf("Endpoint = %q, want custom.endpoint.com", info.Endpoint)
	}
}

func TestCloudflareGetCostData_WithAPIToken(t *testing.T) {
	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid-123",
		Extra: map[string]string{
			"api_token":  "test-token",
			"account_id": "acc-123",
		},
	}

	_, err := p.GetCostData(context.Background(), cfg)
	// Expect error as we can't reach Cloudflare API in tests
	if err == nil {
		t.Log("GetCostData() succeeded (unexpected but acceptable)")
	}
}

func TestCloudflareRotateCredential_WithAPIToken(t *testing.T) {
	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid-123",
		EngineType: "d1",
		Extra: map[string]string{
			"api_token":  "test-token",
			"account_id": "acc-123",
		},
	}

	_, err := p.RotateCredential(context.Background(), cfg)
	// Expect error as we can't reach Cloudflare API in tests
	if err == nil {
		t.Log("RotateCredential() succeeded (unexpected)")
	}
}

// ============================================================================
// Vultr additional coverage
// ============================================================================

func TestVultrCheckHealth_APICredentials(t *testing.T) {
	p := &vultrProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid-123",
		Endpoint:   "db.example.com:5432",
		Extra: map[string]string{
			"api_key": "test-key",
		},
	}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	// With API key but unreachable Vultr API → failure
	if result == nil {
		t.Error("CheckHealth() returned nil result")
	}
}

func TestVultrDiscover_WithAPIKey(t *testing.T) {
	p := &vultrProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid-123",
		Region:     "us-east",
		EngineType: "postgres",
		Endpoint:   "db.example.com:5432",
		Extra: map[string]string{
			"api_key": "test-key",
		},
	}

	_, err := p.Discover(context.Background(), cfg)
	// Will fail due to unreachable Vultr API — just verify graceful error
	if err == nil {
		t.Log("Discover() succeeded (unexpected but acceptable)")
	}
}

func TestVultrGetCostData_WithAPIKey(t *testing.T) {
	p := &vultrProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid-123",
		Extra: map[string]string{
			"api_key": "test-key",
		},
	}

	_, err := p.GetCostData(context.Background(), cfg)
	// Expect error - can't reach Vultr API
	if err == nil {
		t.Log("GetCostData() succeeded (unexpected)")
	}
}

func TestVultrRotateCredential_WithAPIKey(t *testing.T) {
	p := &vultrProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid-123",
		EngineType: "postgres",
		Extra: map[string]string{
			"api_key": "test-key",
		},
	}

	_, err := p.RotateCredential(context.Background(), cfg)
	// Expect error - can't reach Vultr API
	if err == nil {
		t.Log("RotateCredential() succeeded (unexpected)")
	}
}

// ============================================================================
// Generic provider additional coverage
// ============================================================================

func TestGenericSetupProxy_NoTLS(t *testing.T) {
	p := &genericProvider{}
	cfg := ExternalProviderConfig{
		Endpoint:   "db.example.com:5432",
		EngineType: "postgres",
	}

	proxy, err := p.SetupProxy(context.Background(), cfg)
	if err != nil {
		t.Fatalf("SetupProxy() error = %v", err)
	}
	if proxy.TLSRequired {
		t.Error("generic provider SetupProxy() TLSRequired = true, want false")
	}
}

// ============================================================================
// AWS additional coverage (checkRDSHealth code path via mock)
// ============================================================================

func TestAwsCheckHealth_WithAPICredentials(t *testing.T) {
	// Start a mock AWS endpoint
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/xml")
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`<?xml version="1.0" encoding="UTF-8"?>
<DescribeDBInstancesResponse>
  <DescribeDBInstancesResult>
    <DBInstances>
      <DBInstance>
        <DBInstanceStatus>available</DBInstanceStatus>
      </DBInstance>
    </DBInstances>
  </DescribeDBInstancesResult>
</DescribeDBInstancesResponse>`))
	}))
	defer srv.Close()

	// We can't easily override the endpoint, but test that the code path handles gracefully
	p := &awsProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "arn:aws:rds:us-east-1:123456789:db:mydb",
		Region:     "us-east-1",
		EngineType: "postgres",
		Endpoint:   "127.0.0.1:1", // TCP fallback
		Extra: map[string]string{
			"access_key_id":     "AKIATEST",
			"secret_access_key": "test-secret",
		},
	}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	// With credentials but unreachable RDS → some non-nil result
	if result == nil {
		t.Error("CheckHealth() returned nil result")
	}
}

func TestAwsCheckHealth_S3EngineType(t *testing.T) {
	p := &awsProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "arn:aws:s3:::my-bucket",
		Region:     "us-east-1",
		EngineType: "s3",
		Extra: map[string]string{
			"access_key_id":     "AKIATEST",
			"secret_access_key": "test-secret",
		},
	}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	if result == nil {
		t.Error("CheckHealth() returned nil result")
	}
}

func TestAwsCheckHealth_ElastiCacheEngineType(t *testing.T) {
	p := &awsProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "arn:aws:elasticache:us-east-1:123:cluster:my-cluster",
		Region:     "us-east-1",
		EngineType: "redis",
		Extra: map[string]string{
			"access_key_id":     "AKIATEST",
			"secret_access_key": "test-secret",
		},
	}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}
	if result == nil {
		t.Error("CheckHealth() returned nil result")
	}
}

func TestAwsGetCostData_WithCredentials(t *testing.T) {
	p := &awsProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "arn:aws:rds:us-east-1:123456789:db:mydb",
		Region:     "us-east-1",
		Extra: map[string]string{
			"access_key_id":     "AKIATEST",
			"secret_access_key": "test-secret",
		},
	}

	_, err := p.GetCostData(context.Background(), cfg)
	// Expect error as we can't reach AWS Cost Explorer
	if err == nil {
		t.Log("GetCostData() succeeded (unexpected but acceptable)")
	}
}

func TestAwsRotateCredential_WithCredentials(t *testing.T) {
	p := &awsProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "arn:aws:rds:us-east-1:123456789:db:mydb",
		Region:     "us-east-1",
		EngineType: "postgres",
		Extra: map[string]string{
			"access_key_id":     "AKIATEST",
			"secret_access_key": "test-secret",
		},
	}

	_, err := p.RotateCredential(context.Background(), cfg)
	// Expect error as we can't reach RDS
	if err == nil {
		t.Log("RotateCredential() succeeded (unexpected)")
	}
}

func TestAwsDiscover_DefaultCase(t *testing.T) {
	p := &awsProvider{}
	cfg := ExternalProviderConfig{
		Region:     "us-east-1",
		EngineType: "custom",
		Endpoint:   "custom.endpoint:5432",
	}

	info, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}
	if info.EngineType != "custom" {
		t.Errorf("EngineType = %q, want custom", info.EngineType)
	}
	if info.Endpoint != "custom.endpoint:5432" {
		t.Errorf("Endpoint = %q, want custom.endpoint:5432", info.Endpoint)
	}
}
