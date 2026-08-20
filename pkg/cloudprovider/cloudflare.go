package cloudprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

func init() { Register(&cloudflareProvider{}) }

type cloudflareProvider struct{}

func (p *cloudflareProvider) Name() string           { return "cloudflare" }
func (p *cloudflareProvider) SupportsIndexing() bool { return false }

func (p *cloudflareProvider) Validate(ctx context.Context, cfg ExternalProviderConfig) error {
	if cfg.ResourceID == "" {
		return fmt.Errorf("cloudflare: resourceId (database/bucket/KV namespace ID) is required")
	}
	return nil
}

func (p *cloudflareProvider) Discover(ctx context.Context, cfg ExternalProviderConfig) (*ExternalResourceInfo, error) {
	info := &ExternalResourceInfo{EngineType: cfg.EngineType}

	apiToken, _ := cfg.Extra["api_token"]
	apiKey, _ := cfg.Extra["api_key"]
	apiEmail, _ := cfg.Extra["api_email"]
	accountID, _ := cfg.Extra["account_id"]

	// If no credentials, return default endpoints
	if (apiToken == "" && (apiKey == "" || apiEmail == "")) || accountID == "" {
		switch cfg.EngineType {
		case "d1":
			info.Endpoint = "https://api.cloudflare.com/client/v4/accounts/{accountId}/d1/database/{dbId}/query"
		case "r2", "object":
			info.EngineType = "object"
			info.Endpoint = "https://{accountId}.r2.cloudflarestorage.com"
		case "kv", "keyvalue":
			info.EngineType = "keyvalue"
			info.Endpoint = "https://api.cloudflare.com/client/v4/accounts/{accountId}/storage/kv/namespaces/{namespaceId}"
		default:
			info.Endpoint = cfg.Endpoint
		}
		return info, nil
	}

	// Real API calls with credentials
	switch cfg.EngineType {
	case "d1":
		return p.discoverD1(ctx, cfg, apiToken, apiKey, apiEmail, accountID)
	case "r2", "object":
		return p.discoverR2(ctx, cfg, apiToken, apiKey, apiEmail, accountID)
	case "kv", "keyvalue":
		return p.discoverKV(ctx, cfg, apiToken, apiKey, apiEmail, accountID)
	default:
		info.Endpoint = cfg.Endpoint
		return info, nil
	}
}

func (p *cloudflareProvider) discoverD1(ctx context.Context, cfg ExternalProviderConfig, apiToken, apiKey, apiEmail, accountID string) (*ExternalResourceInfo, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/d1/database/%s", accountID, cfg.ResourceID)
	body, _, err := cloudflareDoRequest("GET", url, nil, apiToken, apiKey, apiEmail)
	if err != nil {
		return nil, fmt.Errorf("cloudflare d1 discover: %w", err)
	}

	var resp struct {
		Success bool `json:"success"`
		Result  struct {
			Name      string `json:"name"`
			CreatedAt string `json:"created_at"`
			FileSize  int64  `json:"file_size"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("cloudflare d1 discover parse: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("cloudflare d1 discover: API returned success=false")
	}

	info := &ExternalResourceInfo{
		EngineType: "d1",
		Endpoint:   fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/d1/database/%s/query", accountID, cfg.ResourceID),
	}
	return info, nil
}

func (p *cloudflareProvider) discoverR2(ctx context.Context, cfg ExternalProviderConfig, apiToken, apiKey, apiEmail, accountID string) (*ExternalResourceInfo, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/r2/buckets/%s", accountID, cfg.ResourceID)
	body, _, err := cloudflareDoRequest("GET", url, nil, apiToken, apiKey, apiEmail)
	if err != nil {
		return nil, fmt.Errorf("cloudflare r2 discover: %w", err)
	}

	var resp struct {
		Success bool `json:"success"`
		Result  struct {
			Name      string `json:"name"`
			CreatedAt string `json:"creation_date"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("cloudflare r2 discover parse: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("cloudflare r2 discover: API returned success=false")
	}

	info := &ExternalResourceInfo{
		EngineType: "object",
		Endpoint:   fmt.Sprintf("https://%s.r2.cloudflarestorage.com/%s", accountID, cfg.ResourceID),
	}
	return info, nil
}

func (p *cloudflareProvider) discoverKV(ctx context.Context, cfg ExternalProviderConfig, apiToken, apiKey, apiEmail, accountID string) (*ExternalResourceInfo, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s", accountID, cfg.ResourceID)
	body, _, err := cloudflareDoRequest("GET", url, nil, apiToken, apiKey, apiEmail)
	if err != nil {
		return nil, fmt.Errorf("cloudflare kv discover: %w", err)
	}

	var resp struct {
		Success bool `json:"success"`
		Result  struct {
			ID    string `json:"id"`
			Title string `json:"title"`
		} `json:"result"`
	}
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("cloudflare kv discover parse: %w", err)
	}

	if !resp.Success {
		return nil, fmt.Errorf("cloudflare kv discover: API returned success=false")
	}

	info := &ExternalResourceInfo{
		EngineType: "keyvalue",
		Endpoint:   fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s", accountID, cfg.ResourceID),
	}
	return info, nil
}

func (p *cloudflareProvider) SetupProxy(ctx context.Context, cfg ExternalProviderConfig) (*ProxyConfig, error) {
	info, err := p.Discover(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &ProxyConfig{Endpoint: info.Endpoint, TLSRequired: true, AuthType: "api-token"}, nil
}

func (p *cloudflareProvider) GetCostData(ctx context.Context, cfg ExternalProviderConfig) (*CostData, error) {
	apiToken, _ := cfg.Extra["api_token"]
	apiKey, _ := cfg.Extra["api_key"]
	apiEmail, _ := cfg.Extra["api_email"]
	accountID, _ := cfg.Extra["account_id"]

	// If no credentials, return not supported
	if (apiToken == "" && (apiKey == "" || apiEmail == "")) || accountID == "" {
		return nil, &ErrNotSupported{Provider: "cloudflare", Capability: "GetCostData"}
	}

	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/billing/profile", accountID)
	body, status, err := cloudflareDoRequest("GET", url, nil, apiToken, apiKey, apiEmail)
	if err != nil || status != http.StatusOK {
		// Fall back gracefully if billing endpoint unavailable
		return &CostData{
			ProviderCostPerHour: 0,
			Currency:            "USD",
			BillingPeriod:       "monthly",
		}, nil
	}

	var resp struct {
		Success bool `json:"success"`
	}
	if err := json.Unmarshal(body, &resp); err == nil && resp.Success {
		return &CostData{
			ProviderCostPerHour: 0,
			Currency:            "USD",
			BillingPeriod:       "monthly",
		}, nil
	}

	return nil, &ErrNotSupported{Provider: "cloudflare", Capability: "GetCostData"}
}

func (p *cloudflareProvider) CheckHealth(ctx context.Context, cfg ExternalProviderConfig) (*HealthResult, error) {
	apiToken, _ := cfg.Extra["api_token"]
	apiKey, _ := cfg.Extra["api_key"]
	apiEmail, _ := cfg.Extra["api_email"]
	accountID, _ := cfg.Extra["account_id"]

	// If no credentials, return unknown
	if (apiToken == "" && (apiKey == "" || apiEmail == "")) || accountID == "" {
		return &HealthResult{
			State:   "unknown",
			Message: "no credentials provided for health check",
		}, nil
	}

	switch cfg.EngineType {
	case "d1":
		return p.checkHealthD1(ctx, cfg, apiToken, apiKey, apiEmail, accountID)
	case "r2", "object":
		return p.checkHealthR2(ctx, cfg, apiToken, apiKey, apiEmail, accountID)
	case "kv", "keyvalue":
		return p.checkHealthKV(ctx, cfg, apiToken, apiKey, apiEmail, accountID)
	default:
		return &HealthResult{
			State:   "unknown",
			Message: fmt.Sprintf("unsupported engine type: %s", cfg.EngineType),
		}, nil
	}
}

func (p *cloudflareProvider) checkHealthD1(ctx context.Context, cfg ExternalProviderConfig, apiToken, apiKey, apiEmail, accountID string) (*HealthResult, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/d1/database/%s", accountID, cfg.ResourceID)
	_, status, err := cloudflareDoRequest("GET", url, nil, apiToken, apiKey, apiEmail)

	if err != nil {
		return &HealthResult{
			State:   "degraded",
			Message: fmt.Sprintf("health check failed: %v", err),
		}, nil
	}

	switch status {
	case http.StatusOK:
		return &HealthResult{
			State:   "healthy",
			Message: "D1 database is reachable and operational",
		}, nil
	case http.StatusNotFound:
		return &HealthResult{
			State:   "failed",
			Message: "D1 database not found (404)",
		}, nil
	default:
		return &HealthResult{
			State:   "degraded",
			Message: fmt.Sprintf("D1 API returned status %d", status),
		}, nil
	}
}

func (p *cloudflareProvider) checkHealthR2(ctx context.Context, cfg ExternalProviderConfig, apiToken, apiKey, apiEmail, accountID string) (*HealthResult, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/r2/buckets/%s", accountID, cfg.ResourceID)
	_, status, err := cloudflareDoRequest("GET", url, nil, apiToken, apiKey, apiEmail)

	if err != nil {
		return &HealthResult{
			State:   "degraded",
			Message: fmt.Sprintf("health check failed: %v", err),
		}, nil
	}

	switch status {
	case http.StatusOK:
		return &HealthResult{
			State:   "healthy",
			Message: "R2 bucket is reachable and operational",
		}, nil
	case http.StatusNotFound:
		return &HealthResult{
			State:   "failed",
			Message: "R2 bucket not found (404)",
		}, nil
	default:
		return &HealthResult{
			State:   "degraded",
			Message: fmt.Sprintf("R2 API returned status %d", status),
		}, nil
	}
}

func (p *cloudflareProvider) checkHealthKV(ctx context.Context, cfg ExternalProviderConfig, apiToken, apiKey, apiEmail, accountID string) (*HealthResult, error) {
	url := fmt.Sprintf("https://api.cloudflare.com/client/v4/accounts/%s/storage/kv/namespaces/%s", accountID, cfg.ResourceID)
	_, status, err := cloudflareDoRequest("GET", url, nil, apiToken, apiKey, apiEmail)

	if err != nil {
		return &HealthResult{
			State:   "degraded",
			Message: fmt.Sprintf("health check failed: %v", err),
		}, nil
	}

	switch status {
	case http.StatusOK:
		return &HealthResult{
			State:   "healthy",
			Message: "KV namespace is reachable and operational",
		}, nil
	case http.StatusNotFound:
		return &HealthResult{
			State:   "failed",
			Message: "KV namespace not found (404)",
		}, nil
	default:
		return &HealthResult{
			State:   "degraded",
			Message: fmt.Sprintf("KV API returned status %d", status),
		}, nil
	}
}

func (p *cloudflareProvider) RotateCredential(ctx context.Context, cfg ExternalProviderConfig) (string, error) {
	apiToken, _ := cfg.Extra["api_token"]
	apiKey, _ := cfg.Extra["api_key"]
	apiEmail, _ := cfg.Extra["api_email"]

	// Rotation only supported with API token auth
	if apiToken == "" {
		return "", &ErrNotSupported{Provider: "cloudflare", Capability: "RotateCredential"}
	}

	// Create new scoped API token for D1 access
	tokenPayload := map[string]interface{}{
		"name": fmt.Sprintf("nest-d1-%s", cfg.ResourceID),
		"policies": []map[string]interface{}{
			{
				"effect": "allow",
				"resources": map[string]interface{}{
					"com.cloudflare.api.account.*": "*",
				},
				"permission_groups": []map[string]interface{}{
					{
						"id":   "c1fde68c7bcc44588cbe523cf028a315",
						"name": "D1 Read",
					},
				},
			},
		},
		"not_before": nil,
		"expires_on": nil,
	}

	bodyBytes, err := json.Marshal(tokenPayload)
	if err != nil {
		return "", fmt.Errorf("cloudflare token rotation: marshal request: %w", err)
	}

	respBody, status, err := cloudflareDoRequest("POST", "https://api.cloudflare.com/client/v4/user/tokens", bodyBytes, apiToken, apiKey, apiEmail)
	if err != nil {
		return "", fmt.Errorf("cloudflare token rotation: %w", err)
	}

	if status != http.StatusOK && status != http.StatusCreated {
		return "", fmt.Errorf("cloudflare token rotation: API returned status %d", status)
	}

	var resp struct {
		Success bool `json:"success"`
		Result  struct {
			Token string `json:"token"`
		} `json:"result"`
		Errors []map[string]interface{} `json:"errors"`
	}

	if err := json.Unmarshal(respBody, &resp); err != nil {
		return "", fmt.Errorf("cloudflare token rotation: parse response: %w", err)
	}

	if !resp.Success {
		errMsg := "unknown error"
		if len(resp.Errors) > 0 {
			errMsg = fmt.Sprintf("%v", resp.Errors[0])
		}
		return "", fmt.Errorf("cloudflare token rotation: API error: %s", errMsg)
	}

	return resp.Result.Token, nil
}

// cloudflareDoRequest makes an HTTP request to the Cloudflare API with appropriate authentication.
// Returns response body, HTTP status code, and error (if any).
// Supports either API token (Bearer) or API key + email authentication.
func cloudflareDoRequest(method, url string, body []byte, apiToken, apiKey, apiEmail string) ([]byte, int, error) {
	client := &http.Client{
		Timeout: 10 * time.Second,
	}

	var req *http.Request
	var err error

	if body != nil {
		req, err = http.NewRequest(method, url, bytes.NewReader(body))
	} else {
		req, err = http.NewRequest(method, url, nil)
	}

	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	// Set authentication headers
	if apiToken != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiToken))
	} else if apiKey != "" && apiEmail != "" {
		req.Header.Set("X-Auth-Key", apiKey)
		req.Header.Set("X-Auth-Email", apiEmail)
	}

	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("do request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}

	return respBody, resp.StatusCode, nil
}
