package provider

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/json"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

func init() { Register(&azureProvider{}) }

type azureProvider struct {
	mu          sync.Mutex
	cachedToken string
	tokenExpiry time.Time
}

func (p *azureProvider) Name() string          { return "azure" }
func (p *azureProvider) SupportsIndexing() bool { return true }

func (p *azureProvider) Validate(ctx context.Context, cfg ExternalProviderConfig) error {
	if cfg.ResourceID == "" {
		return fmt.Errorf("azure: resourceId (resource URI) is required")
	}
	return nil
}

func (p *azureProvider) getAccessToken(ctx context.Context, cfg ExternalProviderConfig) (string, error) {
	if token := cfg.Extra["access_token"]; token != "" {
		return token, nil
	}

	p.mu.Lock()
	defer p.mu.Unlock()

	if p.cachedToken != "" && time.Now().Before(p.tokenExpiry) {
		return p.cachedToken, nil
	}

	tenantID := cfg.Extra["tenant_id"]
	if tenantID == "" {
		return "", fmt.Errorf("azure: tenant_id required in Extra")
	}

	clientID := cfg.Extra["client_id"]
	if clientID == "" {
		return "", fmt.Errorf("azure: client_id required in Extra")
	}

	clientSecret := cfg.Extra["client_secret"]
	if clientSecret == "" {
		return "", fmt.Errorf("azure: client_secret required in Extra")
	}

	tokenURL := fmt.Sprintf("https://login.microsoftonline.com/%s/oauth2/v2.0/token", tenantID)

	data := url.Values{
		"grant_type":    {"client_credentials"},
		"client_id":     {clientID},
		"client_secret": {clientSecret},
		"scope":         {"https://management.azure.com/.default"},
	}

	req, err := http.NewRequestWithContext(ctx, "POST", tokenURL, strings.NewReader(data.Encode()))
	if err != nil {
		return "", fmt.Errorf("azure: create token request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("azure: token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("azure: read token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("azure: token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return "", fmt.Errorf("azure: parse token response: %w", err)
	}

	p.cachedToken = tokenResp.AccessToken
	p.tokenExpiry = time.Now().Add(time.Duration(tokenResp.ExpiresIn-600) * time.Second)

	return p.cachedToken, nil
}

func (p *azureProvider) Discover(ctx context.Context, cfg ExternalProviderConfig) (*ExternalResourceInfo, error) {
	info := &ExternalResourceInfo{Region: cfg.Region, EngineType: cfg.EngineType}

	token, err := p.getAccessToken(ctx, cfg)
	if err != nil {
		return p.discoverWithoutCredentials(cfg), nil
	}

	// Validate ResourceID to prevent SSRF: must start with / and not contain host-like patterns
	if !strings.HasPrefix(cfg.ResourceID, "/") || strings.Contains(cfg.ResourceID, "@") || strings.Contains(cfg.ResourceID, "://") {
		return p.discoverWithoutCredentials(cfg), nil
	}

	reqURL := fmt.Sprintf("https://management.azure.com%s?api-version=2023-06-01-preview", cfg.ResourceID)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return p.discoverWithoutCredentials(cfg), nil
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return p.discoverWithoutCredentials(cfg), nil
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return p.discoverWithoutCredentials(cfg), nil
	}

	if resp.StatusCode != http.StatusOK {
		return p.discoverWithoutCredentials(cfg), nil
	}

	switch cfg.EngineType {
	case "postgres":
		return p.discoverPostgres(body, info)
	case "redis", "keyvalue":
		return p.discoverRedis(body, info)
	default:
		return info, nil
	}
}

func (p *azureProvider) discoverWithoutCredentials(cfg ExternalProviderConfig) *ExternalResourceInfo {
	info := &ExternalResourceInfo{Region: cfg.Region, EngineType: cfg.EngineType, Endpoint: cfg.Endpoint}
	return info
}

func (p *azureProvider) discoverPostgres(body []byte, info *ExternalResourceInfo) (*ExternalResourceInfo, error) {
	var resp struct {
		Properties struct {
			FullyQualifiedDomainName string `json:"fullyQualifiedDomainName"`
			Version                  string `json:"version"`
		} `json:"properties"`
		SKU struct {
			Tier string `json:"tier"`
		} `json:"sku"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return info, nil
	}

	if resp.Properties.FullyQualifiedDomainName != "" {
		info.Endpoint = fmt.Sprintf("%s:5432", resp.Properties.FullyQualifiedDomainName)
	}
	if resp.Properties.Version != "" {
		info.EngineVersion = resp.Properties.Version
	}

	return info, nil
}

func (p *azureProvider) discoverRedis(body []byte, info *ExternalResourceInfo) (*ExternalResourceInfo, error) {
	var resp struct {
		Properties struct {
			HostName string `json:"hostName"`
			SSLPort  int    `json:"sslPort"`
		} `json:"properties"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return info, nil
	}

	if resp.Properties.HostName != "" {
		port := resp.Properties.SSLPort
		if port == 0 {
			port = 6380
		}
		info.Endpoint = fmt.Sprintf("%s:%d", resp.Properties.HostName, port)
	}

	return info, nil
}

func (p *azureProvider) SetupProxy(ctx context.Context, cfg ExternalProviderConfig) (*ProxyConfig, error) {
	info, err := p.Discover(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &ProxyConfig{Endpoint: info.Endpoint, TLSRequired: true, AuthType: "service-principal"}, nil
}

func (p *azureProvider) GetCostData(ctx context.Context, cfg ExternalProviderConfig) (*CostData, error) {
	subscriptionID := cfg.Extra["subscription_id"]
	if subscriptionID == "" {
		return nil, &ErrNotSupported{Provider: "azure", Capability: "GetCostData"}
	}

	token, err := p.getAccessToken(ctx, cfg)
	if err != nil {
		return nil, &ErrNotSupported{Provider: "azure", Capability: "GetCostData"}
	}

	queryURL := fmt.Sprintf("https://management.azure.com/subscriptions/%s/providers/Microsoft.CostManagement/query?api-version=2023-11-01", subscriptionID)

	queryBody := map[string]interface{}{
		"type":      "ActualCost",
		"timeframe": "MonthToDate",
		"dataSet": map[string]interface{}{
			"granularity": "None",
			"aggregation": map[string]interface{}{
				"totalCost": map[string]interface{}{
					"name":     "Cost",
					"function": "Sum",
				},
			},
			"filter": map[string]interface{}{
				"dimensions": map[string]interface{}{
					"name":     "ResourceId",
					"operator": "In",
					"values":   []string{cfg.ResourceID},
				},
			},
		},
	}

	queryBytes, err := json.Marshal(queryBody)
	if err != nil {
		return nil, fmt.Errorf("azure: marshal cost query: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", queryURL, bytes.NewReader(queryBytes))
	if err != nil {
		return nil, fmt.Errorf("azure: create cost request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("azure: cost request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("azure: read cost response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("azure: cost request failed with status %d", resp.StatusCode)
	}

	var costResp struct {
		Properties struct {
			Rows [][]interface{} `json:"rows"`
		} `json:"properties"`
	}

	if err := json.Unmarshal(body, &costResp); err != nil {
		return nil, fmt.Errorf("azure: parse cost response: %w", err)
	}

	var totalCost float64
	if len(costResp.Properties.Rows) > 0 && len(costResp.Properties.Rows[0]) > 0 {
		if cost, ok := costResp.Properties.Rows[0][0].(float64); ok {
			totalCost = cost
		}
	}

	now := time.Now()
	dayOfMonth := float64(now.Day())
	hourlyRate := totalCost / dayOfMonth / 24

	return &CostData{
		ProviderCostPerHour: hourlyRate,
		Currency:            "USD",
		BillingPeriod:       "",
	}, nil
}

func (p *azureProvider) CheckHealth(ctx context.Context, cfg ExternalProviderConfig) (*HealthResult, error) {
	token, err := p.getAccessToken(ctx, cfg)
	if err != nil {
		return p.checkHealthWithTCP(ctx, cfg)
	}

	reqURL := fmt.Sprintf("https://management.azure.com%s?api-version=2023-06-01-preview", cfg.ResourceID)

	req, err := http.NewRequestWithContext(ctx, "GET", reqURL, nil)
	if err != nil {
		return p.checkHealthWithTCP(ctx, cfg)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return p.checkHealthWithTCP(ctx, cfg)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return p.checkHealthWithTCP(ctx, cfg)
	}

	if resp.StatusCode != http.StatusOK {
		return p.checkHealthWithTCP(ctx, cfg)
	}

	switch cfg.EngineType {
	case "postgres":
		return p.checkHealthPostgres(body)
	case "redis", "keyvalue":
		return p.checkHealthRedis(body)
	default:
		return p.checkHealthWithTCP(ctx, cfg)
	}
}

func (p *azureProvider) checkHealthPostgres(body []byte) (*HealthResult, error) {
	var resp struct {
		Properties struct {
			State string `json:"state"`
		} `json:"properties"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return &HealthResult{State: "unknown", Message: "failed to parse response"}, nil
	}

	switch resp.Properties.State {
	case "Ready":
		return &HealthResult{State: "healthy", Message: "Server is ready"}, nil
	case "Starting", "Stopping", "Updating":
		return &HealthResult{State: "degraded", Message: fmt.Sprintf("Server is %s", resp.Properties.State)}, nil
	case "Stopped", "Dropping":
		return &HealthResult{State: "failed", Message: fmt.Sprintf("Server is %s", resp.Properties.State)}, nil
	default:
		return &HealthResult{State: "unknown", Message: fmt.Sprintf("Unknown state: %s", resp.Properties.State)}, nil
	}
}

func (p *azureProvider) checkHealthRedis(body []byte) (*HealthResult, error) {
	var resp struct {
		Properties struct {
			ProvisioningState string `json:"provisioningState"`
		} `json:"properties"`
	}

	if err := json.Unmarshal(body, &resp); err != nil {
		return &HealthResult{State: "unknown", Message: "failed to parse response"}, nil
	}

	switch resp.Properties.ProvisioningState {
	case "Succeeded":
		return &HealthResult{State: "healthy", Message: "Cache is provisioned"}, nil
	case "Creating", "Scaling", "Updating":
		return &HealthResult{State: "degraded", Message: fmt.Sprintf("Cache is %s", resp.Properties.ProvisioningState)}, nil
	case "Failed", "Deleting":
		return &HealthResult{State: "failed", Message: fmt.Sprintf("Cache provisioning %s", resp.Properties.ProvisioningState)}, nil
	default:
		return &HealthResult{State: "unknown", Message: fmt.Sprintf("Unknown state: %s", resp.Properties.ProvisioningState)}, nil
	}
}

func (p *azureProvider) checkHealthWithTCP(ctx context.Context, cfg ExternalProviderConfig) (*HealthResult, error) {
	dialer := net.Dialer{Timeout: 5 * time.Second}
	conn, err := dialer.DialContext(ctx, "tcp", cfg.Endpoint)
	if err != nil {
		return &HealthResult{State: "failed", Message: fmt.Sprintf("TCP dial failed: %v", err)}, nil
	}
	conn.Close()
	return &HealthResult{State: "healthy", Message: "TCP connection successful"}, nil
}

func (p *azureProvider) RotateCredential(ctx context.Context, cfg ExternalProviderConfig) (string, error) {
	token, err := p.getAccessToken(ctx, cfg)
	if err != nil {
		return "", &ErrNotSupported{Provider: "azure", Capability: "RotateCredential"}
	}

	switch cfg.EngineType {
	case "postgres":
		return p.rotatePostgresPassword(ctx, cfg, token)
	case "redis", "keyvalue":
		return p.rotateRedisKey(ctx, cfg, token)
	default:
		return "", &ErrNotSupported{Provider: "azure", Capability: "RotateCredential"}
	}
}

func (p *azureProvider) rotatePostgresPassword(ctx context.Context, cfg ExternalProviderConfig, token string) (string, error) {
	newPassword := generatePassword()

	updateBody := map[string]interface{}{
		"properties": map[string]interface{}{
			"administratorLoginPassword": newPassword,
		},
	}

	updateBytes, err := json.Marshal(updateBody)
	if err != nil {
		return "", fmt.Errorf("azure: marshal password update: %w", err)
	}

	reqURL := fmt.Sprintf("https://management.azure.com%s?api-version=2023-06-01-preview", cfg.ResourceID)

	req, err := http.NewRequestWithContext(ctx, "PATCH", reqURL, bytes.NewReader(updateBytes))
	if err != nil {
		return "", fmt.Errorf("azure: create password update request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("azure: password update request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("azure: read password update response: %w", err)
	}

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("azure: password update failed with status %d: %s", resp.StatusCode, string(body))
	}

	return newPassword, nil
}

func (p *azureProvider) rotateRedisKey(ctx context.Context, cfg ExternalProviderConfig, token string) (string, error) {
	keyURL := fmt.Sprintf("https://management.azure.com%s/regenerateKey?api-version=2023-08-01", cfg.ResourceID)

	keyBody := map[string]interface{}{
		"keyType": "Primary",
	}

	keyBytes, err := json.Marshal(keyBody)
	if err != nil {
		return "", fmt.Errorf("azure: marshal key regenerate: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, "POST", keyURL, bytes.NewReader(keyBytes))
	if err != nil {
		return "", fmt.Errorf("azure: create key regenerate request: %w", err)
	}
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("azure: key regenerate request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("azure: read key regenerate response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("azure: key regenerate failed with status %d", resp.StatusCode)
	}

	var keyResp struct {
		PrimaryKey string `json:"primaryKey"`
	}

	if err := json.Unmarshal(body, &keyResp); err != nil {
		return "", fmt.Errorf("azure: parse key regenerate response: %w", err)
	}

	if keyResp.PrimaryKey == "" {
		return "", fmt.Errorf("azure: no primary key in response")
	}

	return keyResp.PrimaryKey, nil
}

func generatePassword() string {
	const (
		upper   = "ABCDEFGHIJKLMNOPQRSTUVWXYZ"
		lower   = "abcdefghijklmnopqrstuvwxyz"
		digits  = "0123456789"
		symbols = "!@#$%^&*-_=+"
		all     = upper + lower + digits + symbols
	)

	length := 16
	password := make([]byte, length)

	password[0] = upper[randInt(len(upper))]
	password[1] = lower[randInt(len(lower))]
	password[2] = digits[randInt(len(digits))]
	password[3] = symbols[randInt(len(symbols))]

	for i := 4; i < length; i++ {
		password[i] = all[randInt(len(all))]
	}

	for i := len(password) - 1; i > 0; i-- {
		j := randInt(i + 1)
		password[i], password[j] = password[j], password[i]
	}

	return string(password)
}

func randInt(max int) int {
	n, err := rand.Int(rand.Reader, big.NewInt(int64(max)))
	if err != nil {
		panic(err)
	}
	return int(n.Int64())
}
