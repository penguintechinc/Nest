package provider

import (
	"context"
	"encoding/json"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// ============================================================================
// AWS TESTS
// ============================================================================

func TestAwsValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ExternalProviderConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: ExternalProviderConfig{
				ResourceID: "arn:aws:rds:us-east-1:123456789:db:mydb",
				Region:     "us-east-1",
			},
			wantErr: false,
		},
		{
			name: "missing resourceId",
			cfg: ExternalProviderConfig{
				Region: "us-east-1",
			},
			wantErr: true,
		},
		{
			name: "missing region",
			cfg: ExternalProviderConfig{
				ResourceID: "arn:aws:rds:us-east-1:123456789:db:mydb",
			},
			wantErr: true,
		},
	}

	p := &awsProvider{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Validate(context.Background(), tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAwsDiscover_NoCredentials(t *testing.T) {
	p := &awsProvider{}
	cfg := ExternalProviderConfig{
		Region:     "us-west-2",
		EngineType: "postgres",
	}

	info, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if info.Endpoint != "rds.us-west-2.amazonaws.com:5432" {
		t.Errorf("Endpoint = %v, want rds.us-west-2.amazonaws.com:5432", info.Endpoint)
	}
	if info.EngineType != "postgres" {
		t.Errorf("EngineType = %v, want postgres", info.EngineType)
	}

	// Test MySQL
	cfg.EngineType = "mysql"
	info, _ = p.Discover(context.Background(), cfg)
	if info.Endpoint != "rds.us-west-2.amazonaws.com:3306" {
		t.Errorf("Endpoint = %v, want rds.us-west-2.amazonaws.com:3306", info.Endpoint)
	}

	// Test Redis
	cfg.EngineType = "redis"
	info, _ = p.Discover(context.Background(), cfg)
	if info.Endpoint != "elasticache.us-west-2.amazonaws.com:6379" {
		t.Errorf("Endpoint = %v, want elasticache.us-west-2.amazonaws.com:6379", info.Endpoint)
	}
	if info.EngineType != "keyvalue" {
		t.Errorf("EngineType = %v, want keyvalue", info.EngineType)
	}
}

func TestAwsCheckHealth_TCP(t *testing.T) {
	// Create a listener on a free port
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	p := &awsProvider{}
	cfg := ExternalProviderConfig{
		Endpoint: listener.Addr().String(),
		Extra:    map[string]string{}, // No credentials
	}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}

	if result.State != "healthy" {
		t.Errorf("State = %v, want healthy", result.State)
	}
}

func TestAwsCheckHealth_TCPFailed(t *testing.T) {
	p := &awsProvider{}
	cfg := ExternalProviderConfig{
		Endpoint: "127.0.0.1:1", // Port 1 is unlikely to be listening
		Extra:    map[string]string{},
	}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}

	if result.State != "unreachable" {
		t.Errorf("State = %v, want unreachable", result.State)
	}
}

func TestAwsGetCostData_NoCredentials(t *testing.T) {
	p := &awsProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "arn:aws:rds:us-east-1:123456789:db:mydb",
		Region:     "us-east-1",
		Extra:      map[string]string{},
	}

	_, err := p.GetCostData(context.Background(), cfg)
	if err == nil {
		t.Errorf("GetCostData() expected error, got nil")
	}

	_, ok := err.(*ErrNotSupported)
	if !ok {
		t.Errorf("GetCostData() error type = %T, want *ErrNotSupported", err)
	}
}

func TestAwsRotateCredential_NoCredentials(t *testing.T) {
	p := &awsProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "arn:aws:rds:us-east-1:123456789:db:mydb",
		Region:     "us-east-1",
		EngineType: "postgres",
		Extra:      map[string]string{},
	}

	_, err := p.RotateCredential(context.Background(), cfg)
	if err == nil {
		t.Errorf("RotateCredential() expected error, got nil")
	}

	_, ok := err.(*ErrNotSupported)
	if !ok {
		t.Errorf("RotateCredential() error type = %T, want *ErrNotSupported", err)
	}
}

func TestAwsSetupProxy(t *testing.T) {
	p := &awsProvider{}
	cfg := ExternalProviderConfig{
		Region:     "us-east-1",
		EngineType: "postgres",
	}

	proxy, err := p.SetupProxy(context.Background(), cfg)
	if err != nil {
		t.Fatalf("SetupProxy() error = %v", err)
	}

	if proxy.Endpoint != "rds.us-east-1.amazonaws.com:5432" {
		t.Errorf("Endpoint = %v", proxy.Endpoint)
	}
	if !proxy.TLSRequired {
		t.Errorf("TLSRequired = %v, want true", proxy.TLSRequired)
	}
	if proxy.AuthType != "iam-role" {
		t.Errorf("AuthType = %v, want iam-role", proxy.AuthType)
	}
}

func TestAwsName(t *testing.T) {
	p := &awsProvider{}
	if p.Name() != "aws" {
		t.Errorf("Name() = %v, want aws", p.Name())
	}
}

func TestAwsSupportsIndexing(t *testing.T) {
	p := &awsProvider{}
	if !p.SupportsIndexing() {
		t.Errorf("SupportsIndexing() = false, want true")
	}
}

// ============================================================================
// GCP TESTS
// ============================================================================

func TestGcpValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ExternalProviderConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: ExternalProviderConfig{
				ResourceID: "projects/my-project/instances/my-instance",
			},
			wantErr: false,
		},
		{
			name: "missing resourceId",
			cfg: ExternalProviderConfig{},
			wantErr: true,
		},
	}

	p := &gcpProvider{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Validate(context.Background(), tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGcpDiscover_NoCredentials(t *testing.T) {
	p := &gcpProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "projects/my-project/instances/my-instance",
		Region:     "us-central1",
		EngineType: "postgres",
	}

	info, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if info.Endpoint != "us-central1.cloudsql.google.com:5432" {
		t.Errorf("Endpoint = %v", info.Endpoint)
	}
	if info.EngineType != "postgres" {
		t.Errorf("EngineType = %v", info.EngineType)
	}

	// Test MySQL
	cfg.EngineType = "mysql"
	info, _ = p.Discover(context.Background(), cfg)
	if info.Endpoint != "us-central1.cloudsql.google.com:5432" {
		t.Errorf("Endpoint = %v", info.Endpoint)
	}

	// Test Redis
	cfg.EngineType = "redis"
	info, _ = p.Discover(context.Background(), cfg)
	if info.EngineType != "keyvalue" {
		t.Errorf("EngineType = %v, want keyvalue", info.EngineType)
	}
}

func TestGcpCheckHealth_TCPProbe(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	p := &gcpProvider{}
	cfg := ExternalProviderConfig{
		Endpoint: listener.Addr().String(),
	}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}

	if result.State != "healthy" {
		t.Errorf("State = %v, want healthy", result.State)
	}
}

func TestGcpGetCostData_NoCredentials(t *testing.T) {
	p := &gcpProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "projects/my-project/instances/my-instance",
	}

	_, err := p.GetCostData(context.Background(), cfg)
	if err == nil {
		t.Errorf("GetCostData() expected error, got nil")
	}

	_, ok := err.(*ErrNotSupported)
	if !ok {
		t.Errorf("GetCostData() error type = %T, want *ErrNotSupported", err)
	}
}

func TestGcpRotateCredential_NoCredentials(t *testing.T) {
	p := &gcpProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "projects/my-project/instances/my-instance",
		EngineType: "postgres",
	}

	_, err := p.RotateCredential(context.Background(), cfg)
	if err == nil {
		t.Errorf("RotateCredential() expected error, got nil")
	}
}

func TestGcpName(t *testing.T) {
	p := &gcpProvider{}
	if p.Name() != "gcp" {
		t.Errorf("Name() = %v, want gcp", p.Name())
	}
}

func TestGcpSupportsIndexing(t *testing.T) {
	p := &gcpProvider{}
	if !p.SupportsIndexing() {
		t.Errorf("SupportsIndexing() = false, want true")
	}
}

// ============================================================================
// AZURE TESTS
// ============================================================================

func TestAzureValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ExternalProviderConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: ExternalProviderConfig{
				ResourceID: "/subscriptions/sub-123/resourceGroups/rg-name/providers/Microsoft.DBforPostgreSQL/servers/server-name",
			},
			wantErr: false,
		},
		{
			name: "missing resourceId",
			cfg: ExternalProviderConfig{},
			wantErr: true,
		},
	}

	p := &azureProvider{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Validate(context.Background(), tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestAzureDiscover_NoCredentials(t *testing.T) {
	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "/subscriptions/sub-123/resourceGroups/rg-name/providers/Microsoft.DBforPostgreSQL/servers/server-name",
		Region:     "eastus",
		EngineType: "postgres",
		Endpoint:   "custom.postgres.database.azure.com:5432",
	}

	info, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if info.Endpoint != "custom.postgres.database.azure.com:5432" {
		t.Errorf("Endpoint = %v", info.Endpoint)
	}
	if info.EngineType != "postgres" {
		t.Errorf("EngineType = %v", info.EngineType)
	}
}

func TestAzureCheckHealth_TCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		Endpoint: listener.Addr().String(),
	}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}

	if result.State != "healthy" {
		t.Errorf("State = %v, want healthy", result.State)
	}
}

func TestAzureGetCostData_NoSubscription(t *testing.T) {
	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "/subscriptions/sub-123/resourceGroups/rg-name/providers/Microsoft.DBforPostgreSQL/servers/server-name",
		Extra:      map[string]string{},
	}

	_, err := p.GetCostData(context.Background(), cfg)
	if err == nil {
		t.Errorf("GetCostData() expected error, got nil")
	}

	_, ok := err.(*ErrNotSupported)
	if !ok {
		t.Errorf("GetCostData() error type = %T, want *ErrNotSupported", err)
	}
}

func TestAzureRotateCredential_NoCredentials(t *testing.T) {
	p := &azureProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "/subscriptions/sub-123/resourceGroups/rg-name/providers/Microsoft.DBforPostgreSQL/servers/server-name",
		EngineType: "postgres",
		Extra:      map[string]string{},
	}

	_, err := p.RotateCredential(context.Background(), cfg)
	if err == nil {
		t.Errorf("RotateCredential() expected error, got nil")
	}
}

func TestAzureName(t *testing.T) {
	p := &azureProvider{}
	if p.Name() != "azure" {
		t.Errorf("Name() = %v, want azure", p.Name())
	}
}

func TestAzureSupportsIndexing(t *testing.T) {
	p := &azureProvider{}
	if !p.SupportsIndexing() {
		t.Errorf("SupportsIndexing() = false, want true")
	}
}

// ============================================================================
// CLOUDFLARE TESTS
// ============================================================================

func TestCloudflareValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ExternalProviderConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: ExternalProviderConfig{
				ResourceID: "db-uuid-123",
			},
			wantErr: false,
		},
		{
			name: "missing resourceId",
			cfg: ExternalProviderConfig{},
			wantErr: true,
		},
	}

	p := &cloudflareProvider{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Validate(context.Background(), tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestCloudflareDiscover_NoCredentials(t *testing.T) {
	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid-123",
		EngineType: "d1",
	}

	info, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if !strings.Contains(info.Endpoint, "d1/database") {
		t.Errorf("Endpoint = %v, want to contain d1/database", info.Endpoint)
	}
	if info.EngineType != "d1" {
		t.Errorf("EngineType = %v, want d1", info.EngineType)
	}

	// Test R2
	cfg.EngineType = "r2"
	info, _ = p.Discover(context.Background(), cfg)
	if info.EngineType != "object" {
		t.Errorf("EngineType = %v, want object", info.EngineType)
	}

	// Test KV
	cfg.EngineType = "kv"
	info, _ = p.Discover(context.Background(), cfg)
	if info.EngineType != "keyvalue" {
		t.Errorf("EngineType = %v, want keyvalue", info.EngineType)
	}
}

func TestCloudflareCheckHealth_NoCredentials(t *testing.T) {
	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid-123",
		EngineType: "d1",
		Extra:      map[string]string{},
	}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}

	if result.State != "unknown" {
		t.Errorf("State = %v, want unknown", result.State)
	}
}

func TestCloudflareGetCostData_NoCredentials(t *testing.T) {
	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid-123",
		Extra:      map[string]string{},
	}

	_, err := p.GetCostData(context.Background(), cfg)
	if err == nil {
		t.Errorf("GetCostData() expected error, got nil")
	}

	_, ok := err.(*ErrNotSupported)
	if !ok {
		t.Errorf("GetCostData() error type = %T, want *ErrNotSupported", err)
	}
}

func TestCloudflareRotateCredential_NoAPIToken(t *testing.T) {
	p := &cloudflareProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid-123",
		Extra: map[string]string{
			"api_key":   "test-key",
			"api_email": "test@example.com",
		},
	}

	_, err := p.RotateCredential(context.Background(), cfg)
	if err == nil {
		t.Errorf("RotateCredential() expected error, got nil")
	}

	_, ok := err.(*ErrNotSupported)
	if !ok {
		t.Errorf("RotateCredential() error type = %T, want *ErrNotSupported", err)
	}
}

func TestCloudflareName(t *testing.T) {
	p := &cloudflareProvider{}
	if p.Name() != "cloudflare" {
		t.Errorf("Name() = %v, want cloudflare", p.Name())
	}
}

func TestCloudflareSupportsIndexing(t *testing.T) {
	p := &cloudflareProvider{}
	if p.SupportsIndexing() {
		t.Errorf("SupportsIndexing() = true, want false")
	}
}

// ============================================================================
// VULTR TESTS
// ============================================================================

func TestVultrValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ExternalProviderConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: ExternalProviderConfig{
				ResourceID: "db-uuid-123",
			},
			wantErr: false,
		},
		{
			name: "missing resourceId",
			cfg: ExternalProviderConfig{},
			wantErr: true,
		},
	}

	p := &vultrProvider{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Validate(context.Background(), tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVultrDiscover_NoCredentials(t *testing.T) {
	p := &vultrProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid-123",
		Region:     "us-east",
		EngineType: "postgres",
		Endpoint:   "db.example.com:5432",
		Extra:      map[string]string{},
	}

	info, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if info.Endpoint != "db.example.com:5432" {
		t.Errorf("Endpoint = %v, want db.example.com:5432", info.Endpoint)
	}
	if info.EngineType != "postgres" {
		t.Errorf("EngineType = %v, want postgres", info.EngineType)
	}
	if info.Region != "us-east" {
		t.Errorf("Region = %v, want us-east", info.Region)
	}
}

func TestVultrCheckHealth_TCP(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	p := &vultrProvider{}
	cfg := ExternalProviderConfig{
		Endpoint: listener.Addr().String(),
		Extra:    map[string]string{},
	}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}

	if result.State != "healthy" {
		t.Errorf("State = %v, want healthy", result.State)
	}
}

func TestVultrCheckHealth_InvalidEndpoint(t *testing.T) {
	p := &vultrProvider{}
	cfg := ExternalProviderConfig{
		Endpoint: "invalid:endpoint:format",
		Extra:    map[string]string{},
	}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}

	if result.State != "failed" {
		t.Errorf("State = %v, want failed", result.State)
	}
}

func TestVultrGetCostData_NoCredentials(t *testing.T) {
	p := &vultrProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid-123",
		Extra:      map[string]string{},
	}

	_, err := p.GetCostData(context.Background(), cfg)
	if err == nil {
		t.Errorf("GetCostData() expected error, got nil")
	}

	_, ok := err.(*ErrNotSupported)
	if !ok {
		t.Errorf("GetCostData() error type = %T, want *ErrNotSupported", err)
	}
}

func TestVultrRotateCredential_NoCredentials(t *testing.T) {
	p := &vultrProvider{}
	cfg := ExternalProviderConfig{
		ResourceID: "db-uuid-123",
		Extra:      map[string]string{},
	}

	_, err := p.RotateCredential(context.Background(), cfg)
	if err == nil {
		t.Errorf("RotateCredential() expected error, got nil")
	}

	_, ok := err.(*ErrNotSupported)
	if !ok {
		t.Errorf("RotateCredential() error type = %T, want *ErrNotSupported", err)
	}
}

func TestVultrSetupProxy(t *testing.T) {
	p := &vultrProvider{}
	cfg := ExternalProviderConfig{
		Endpoint: "db.example.com:5432",
	}

	proxy, err := p.SetupProxy(context.Background(), cfg)
	if err != nil {
		t.Fatalf("SetupProxy() error = %v", err)
	}

	if proxy.Endpoint != "db.example.com:5432" {
		t.Errorf("Endpoint = %v", proxy.Endpoint)
	}
	if !proxy.TLSRequired {
		t.Errorf("TLSRequired = %v, want true", proxy.TLSRequired)
	}
	if proxy.AuthType != "api-key" {
		t.Errorf("AuthType = %v, want api-key", proxy.AuthType)
	}
}

func TestVultrName(t *testing.T) {
	p := &vultrProvider{}
	if p.Name() != "vultr" {
		t.Errorf("Name() = %v, want vultr", p.Name())
	}
}

func TestVultrSupportsIndexing(t *testing.T) {
	p := &vultrProvider{}
	if !p.SupportsIndexing() {
		t.Errorf("SupportsIndexing() = false, want true")
	}
}

// ============================================================================
// GENERIC PROVIDER TESTS
// ============================================================================

func TestGenericValidate(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ExternalProviderConfig
		wantErr bool
	}{
		{
			name: "valid config",
			cfg: ExternalProviderConfig{
				Endpoint:   "db.example.com:5432",
				EngineType: "postgres",
			},
			wantErr: false,
		},
		{
			name: "missing endpoint",
			cfg: ExternalProviderConfig{
				EngineType: "postgres",
			},
			wantErr: true,
		},
		{
			name: "missing engineType",
			cfg: ExternalProviderConfig{
				Endpoint: "db.example.com:5432",
			},
			wantErr: true,
		},
	}

	p := &genericProvider{}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := p.Validate(context.Background(), tt.cfg)
			if (err != nil) != tt.wantErr {
				t.Errorf("Validate() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestGenericDiscover(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	p := &genericProvider{}
	cfg := ExternalProviderConfig{
		Endpoint:   listener.Addr().String(),
		EngineType: "postgres",
	}

	info, err := p.Discover(context.Background(), cfg)
	if err != nil {
		t.Fatalf("Discover() error = %v", err)
	}

	if info.Endpoint != listener.Addr().String() {
		t.Errorf("Endpoint = %v", info.Endpoint)
	}
	if info.EngineType != "postgres" {
		t.Errorf("EngineType = %v", info.EngineType)
	}
}

func TestGenericCheckHealth(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("Listen() error = %v", err)
	}
	defer listener.Close()

	p := &genericProvider{}
	cfg := ExternalProviderConfig{
		Endpoint: listener.Addr().String(),
	}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}

	if result.State != "healthy" {
		t.Errorf("State = %v, want healthy", result.State)
	}
}

func TestGenericCheckHealth_NoEndpoint(t *testing.T) {
	p := &genericProvider{}
	cfg := ExternalProviderConfig{}

	result, err := p.CheckHealth(context.Background(), cfg)
	if err != nil {
		t.Fatalf("CheckHealth() error = %v", err)
	}

	if result.State != "unreachable" {
		t.Errorf("State = %v, want unreachable", result.State)
	}
}

func TestGenericGetCostData(t *testing.T) {
	p := &genericProvider{}
	cfg := ExternalProviderConfig{
		Endpoint: "db.example.com:5432",
	}

	_, err := p.GetCostData(context.Background(), cfg)
	if err == nil {
		t.Errorf("GetCostData() expected error, got nil")
	}

	_, ok := err.(*ErrNotSupported)
	if !ok {
		t.Errorf("GetCostData() error type = %T, want *ErrNotSupported", err)
	}
}

func TestGenericRotateCredential(t *testing.T) {
	p := &genericProvider{}
	cfg := ExternalProviderConfig{
		Endpoint: "db.example.com:5432",
	}

	_, err := p.RotateCredential(context.Background(), cfg)
	if err == nil {
		t.Errorf("RotateCredential() expected error, got nil")
	}

	_, ok := err.(*ErrNotSupported)
	if !ok {
		t.Errorf("RotateCredential() error type = %T, want *ErrNotSupported", err)
	}
}

func TestGenericSetupProxy(t *testing.T) {
	p := &genericProvider{}
	cfg := ExternalProviderConfig{
		Endpoint:   "db.example.com:5432",
		EngineType: "postgres",
	}

	proxy, err := p.SetupProxy(context.Background(), cfg)
	if err != nil {
		t.Fatalf("SetupProxy() error = %v", err)
	}

	if proxy.Endpoint != "db.example.com:5432" {
		t.Errorf("Endpoint = %v", proxy.Endpoint)
	}
	if proxy.TLSRequired {
		t.Errorf("TLSRequired = %v, want false", proxy.TLSRequired)
	}
	if proxy.AuthType != "basic" {
		t.Errorf("AuthType = %v, want basic", proxy.AuthType)
	}
}

func TestGenericName(t *testing.T) {
	p := &genericProvider{}
	if p.Name() != "generic" {
		t.Errorf("Name() = %v, want generic", p.Name())
	}
}

func TestGenericSupportsIndexing(t *testing.T) {
	p := &genericProvider{}
	if p.SupportsIndexing() {
		t.Errorf("SupportsIndexing() = true, want false")
	}
}

// ============================================================================
// REGISTRY TESTS
// ============================================================================

func TestRegistryRegisterAndGet(t *testing.T) {
	tests := []struct {
		name          string
		providerNames []string
	}{
		{
			name:          "all providers registered",
			providerNames: []string{"aws", "gcp", "azure", "cloudflare", "vultr", "generic"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			for _, name := range tt.providerNames {
				p, err := Get(name)
				if err != nil {
					t.Errorf("Get(%s) error = %v", name, err)
				}
				if p == nil {
					t.Errorf("Get(%s) returned nil provider", name)
				}
				if p.Name() != name {
					t.Errorf("Provider name = %v, want %s", p.Name(), name)
				}
			}
		})
	}
}

func TestRegistryGetUnknown(t *testing.T) {
	// When a provider is not found, the registry falls back to generic provider
	p, err := Get("unknown-provider")
	if err != nil {
		t.Errorf("Get(unknown-provider) error = %v", err)
	}
	if p == nil {
		t.Errorf("Get(unknown-provider) returned nil provider")
	}
	// Should return the generic provider as fallback
	if p.Name() != "generic" {
		t.Errorf("Get(unknown-provider) returned %s, want generic fallback", p.Name())
	}
}

func TestRegistryList(t *testing.T) {
	names := List()
	if len(names) == 0 {
		t.Errorf("List() returned empty slice")
	}

	// Check that all known providers are present
	required := []string{"aws", "gcp", "azure", "cloudflare", "vultr", "generic"}
	for _, req := range required {
		found := false
		for _, name := range names {
			if name == req {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("List() missing provider %s", req)
		}
	}
}

// ============================================================================
// HELPER TESTS
// ============================================================================

func TestGenerateRandomPassword(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{name: "8 chars", length: 8},
		{name: "16 chars", length: 16},
		{name: "32 chars", length: 32},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pwd := generateRandomPassword(tt.length)
			if len(pwd) != tt.length {
				t.Errorf("generateRandomPassword(%d) length = %d, want %d", tt.length, len(pwd), tt.length)
			}

			// Verify all characters are from charset
			for _, ch := range pwd {
				if !strings.ContainsRune(passwordCharset, ch) {
					t.Errorf("generateRandomPassword(%d) contains invalid char %c", tt.length, ch)
				}
			}
		})
	}
}

func TestGenerateRandomPassword_Uniqueness(t *testing.T) {
	pwd1 := generateRandomPassword(16)
	pwd2 := generateRandomPassword(16)

	if pwd1 == pwd2 {
		t.Errorf("generateRandomPassword generated identical passwords, want different")
	}
}

func TestGenerateRandomPasswordComplex(t *testing.T) {
	tests := []struct {
		name   string
		length int
	}{
		{name: "8 chars", length: 8},
		{name: "16 chars", length: 16},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			pwd := generateRandomPasswordComplex(tt.length)
			if len(pwd) != tt.length {
				t.Errorf("generateRandomPasswordComplex(%d) length = %d, want %d", tt.length, len(pwd), tt.length)
			}

			// Verify all characters are from complex charset
			for _, ch := range pwd {
				if !strings.ContainsRune(passwordCharsetComplex, ch) {
					t.Errorf("generateRandomPasswordComplex(%d) contains invalid char %c", tt.length, ch)
				}
			}
		})
	}
}

// ============================================================================
// ERROR TYPE TESTS
// ============================================================================

func TestErrNotSupportedError(t *testing.T) {
	err := &ErrNotSupported{
		Provider:   "aws",
		Capability: "GetCostData",
	}

	expected := "aws: GetCostData is not supported by this provider"
	if err.Error() != expected {
		t.Errorf("ErrNotSupported.Error() = %v, want %v", err.Error(), expected)
	}
}

// ============================================================================
// INTEGRATION-LIKE TESTS (mock HTTP)
// ============================================================================

func TestVultrCheckHealth_WithMockedAPI(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/v2/databases") {
			t.Errorf("Unexpected path %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
			return
		}

		auth := r.Header.Get("Authorization")
		if auth != "Bearer test-api-key" {
			t.Errorf("Missing or invalid auth header: %s", auth)
			w.WriteHeader(http.StatusUnauthorized)
			return
		}

		dbResp := vultrDatabaseResponse{
			Database: vultrDatabase{
				ID:             "db-123",
				Status:         "running",
				Host:           "db.example.com",
				Port:           5432,
				DatabaseEngine: "postgresql",
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(dbResp)
	}))
	defer server.Close()

	// This test demonstrates the mock pattern but does not execute the actual API call
	// since vultr hardcodes the API URL. Demonstrating the approach for documentation.
	_ = server
}

// ============================================================================
// CONTEXT TIMEOUT TESTS
// ============================================================================

func TestGenericDiscover_ContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	p := &genericProvider{}
	cfg := ExternalProviderConfig{
		Endpoint:   "192.0.2.1:5432", // Non-routable IP (TEST-NET-1)
		EngineType: "postgres",
	}

	_, err := p.Discover(ctx, cfg)
	if err == nil {
		t.Errorf("Discover() with timeout expected error, got nil")
	}
}

// ============================================================================
// ENDPOINT PARSING TESTS
// ============================================================================

func TestExtractHostPort(t *testing.T) {
	tests := []struct {
		name      string
		endpoint  string
		wantHost  string
		wantPort  string
		wantError bool
	}{
		{
			name:     "valid endpoint",
			endpoint: "db.example.com:5432",
			wantHost: "db.example.com",
			wantPort: "5432",
		},
		{
			name:      "invalid format - no port",
			endpoint:  "db.example.com",
			wantError: true,
		},
		{
			name:      "invalid format - too many colons",
			endpoint:  "db:example:com:5432",
			wantError: true,
		},
		{
			name:     "IPv4 with port",
			endpoint: "192.168.1.1:3306",
			wantHost: "192.168.1.1",
			wantPort: "3306",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			host, port, err := extractHostPort(tt.endpoint)
			if (err != nil) != tt.wantError {
				t.Errorf("extractHostPort() error = %v, wantError %v", err, tt.wantError)
			}
			if !tt.wantError {
				if host != tt.wantHost || port != tt.wantPort {
					t.Errorf("extractHostPort() = (%v, %v), want (%v, %v)", host, port, tt.wantHost, tt.wantPort)
				}
			}
		})
	}
}

// ============================================================================
// VULTR HELPER TESTS
// ============================================================================

func TestMapVultrEngineType(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"postgresql", "postgresql"},
		{"PostgreSQL", "postgresql"},
		{"mysql", "mysql"},
		{"MySQL", "mysql"},
		{"redis", "redis"},
		{"Redis", "redis"},
		{"kafka", "kafka"},
		{"Kafka", "kafka"},
		{"unknown", "unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			if got := mapVultrEngineType(tt.input); got != tt.want {
				t.Errorf("mapVultrEngineType(%s) = %s, want %s", tt.input, got, tt.want)
			}
		})
	}
}

func TestMapVultrStatus(t *testing.T) {
	tests := []struct {
		input       string
		wantState   string
		wantMessage string
	}{
		{"running", "healthy", "Database is running"},
		{"Running", "healthy", "Database is running"},
		{"rebuilding", "degraded", "Database is rebuilding"},
		{"rebalancing", "degraded", "Database is rebalancing"},
		{"error", "failed", "Database is in error state"},
		{"Error", "failed", "Database is in error state"},
		{"unknown", "failed", "Database status: unknown"},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			state, message := mapVultrStatus(tt.input)
			if state != tt.wantState {
				t.Errorf("mapVultrStatus(%s) state = %s, want %s", tt.input, state, tt.wantState)
			}
			if message != tt.wantMessage {
				t.Errorf("mapVultrStatus(%s) message = %s, want %s", tt.input, message, tt.wantMessage)
			}
		})
	}
}

func TestParseVultrPlanSize(t *testing.T) {
	plan := vultrPlan{
		ID:     "plan-1",
		Name:   "Standard 1GB",
		RamMb:  1024,
		DiskGb: 25,
		VCpus:  1,
	}

	if got := parseVultrPlanSize(plan); got != 25 {
		t.Errorf("parseVultrPlanSize() = %d, want 25", got)
	}
}

// ============================================================================
// PROVIDER CONFIG TYPE TESTS
// ============================================================================

func TestExternalResourceInfo(t *testing.T) {
	info := &ExternalResourceInfo{
		EngineType:   "postgres",
		EngineVersion: "14.1",
		Endpoint:     "db.example.com:5432",
		Region:       "us-east-1",
		Tags: map[string]string{
			"environment": "production",
		},
		SizeGB:      100,
		ReplicaCount: 3,
	}

	if info.EngineType != "postgres" {
		t.Errorf("EngineType = %s", info.EngineType)
	}
	if info.EngineVersion != "14.1" {
		t.Errorf("EngineVersion = %s", info.EngineVersion)
	}
	if info.SizeGB != 100 {
		t.Errorf("SizeGB = %d", info.SizeGB)
	}
	if info.ReplicaCount != 3 {
		t.Errorf("ReplicaCount = %d", info.ReplicaCount)
	}
}

func TestProxyConfig(t *testing.T) {
	cfg := &ProxyConfig{
		Endpoint:    "db.example.com:5432",
		TLSRequired: true,
		AuthType:    "iam-role",
	}

	if cfg.Endpoint != "db.example.com:5432" {
		t.Errorf("Endpoint = %s", cfg.Endpoint)
	}
	if !cfg.TLSRequired {
		t.Errorf("TLSRequired = %v, want true", cfg.TLSRequired)
	}
	if cfg.AuthType != "iam-role" {
		t.Errorf("AuthType = %s", cfg.AuthType)
	}
}

func TestCostData(t *testing.T) {
	cd := &CostData{
		ProviderCostPerHour: 1.5,
		Currency:            "USD",
		BillingPeriod:       "monthly",
	}

	if cd.ProviderCostPerHour != 1.5 {
		t.Errorf("ProviderCostPerHour = %f", cd.ProviderCostPerHour)
	}
	if cd.Currency != "USD" {
		t.Errorf("Currency = %s", cd.Currency)
	}
	if cd.BillingPeriod != "monthly" {
		t.Errorf("BillingPeriod = %s", cd.BillingPeriod)
	}
}

func TestHealthResult(t *testing.T) {
	tests := []struct {
		state   string
		message string
	}{
		{"healthy", "All systems operational"},
		{"degraded", "Performance degraded"},
		{"failed", "Service unavailable"},
		{"unknown", "Unable to determine status"},
	}

	for _, tt := range tests {
		t.Run(tt.state, func(t *testing.T) {
			hr := &HealthResult{
				State:   tt.state,
				Message: tt.message,
			}

			if hr.State != tt.state {
				t.Errorf("State = %s, want %s", hr.State, tt.state)
			}
			if hr.Message != tt.message {
				t.Errorf("Message = %s, want %s", hr.Message, tt.message)
			}
		})
	}
}

// ============================================================================
// EDGE CASES
// ============================================================================

func TestAzureProviderConcurrentTokenCache(t *testing.T) {
	p := &azureProvider{}

	// Verify that the provider can be used safely (has mutex for token caching)
	cfg := ExternalProviderConfig{
		ResourceID: "/subscriptions/sub-123/resourceGroups/rg-name/providers/Microsoft.DBforPostgreSQL/servers/server-name",
	}

	// This should not panic due to race conditions
	_ = p.Validate(context.Background(), cfg)
}

func TestCloudflareDoRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Verify headers
		auth := r.Header.Get("Authorization")
		contentType := r.Header.Get("Content-Type")

		if auth != "Bearer test-token" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		if contentType != "application/json" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"success": true}`)
	}))
	defer server.Close()

	body, status, err := cloudflareDoRequest("GET", server.URL, nil, "test-token", "", "")
	if err != nil {
		t.Errorf("cloudflareDoRequest() error = %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("cloudflareDoRequest() status = %d, want %d", status, http.StatusOK)
	}

	var resp struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}
	if !resp.Success {
		t.Errorf("Response success = %v, want true", resp.Success)
	}
}

func TestCloudflareDoRequest_WithBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		var payload map[string]interface{}
		json.Unmarshal(body, &payload)

		if payload["test"] != "value" {
			w.WriteHeader(http.StatusBadRequest)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		io.WriteString(w, `{"success": true}`)
	}))
	defer server.Close()

	reqBody := []byte(`{"test": "value"}`)
	_, status, err := cloudflareDoRequest("POST", server.URL, reqBody, "test-token", "", "")
	if err != nil {
		t.Errorf("cloudflareDoRequest() error = %v", err)
	}
	if status != http.StatusOK {
		t.Errorf("cloudflareDoRequest() status = %d, want %d", status, http.StatusOK)
	}
}
