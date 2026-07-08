package provider

import (
	"bytes"
	"context"
	"crypto"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha256"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func init() { Register(&gcpProvider{}) }

type gcpProvider struct{}

func (p *gcpProvider) Name() string           { return "gcp" }
func (p *gcpProvider) SupportsIndexing() bool { return true }

func (p *gcpProvider) Validate(ctx context.Context, cfg ExternalProviderConfig) error {
	if cfg.ResourceID == "" {
		return fmt.Errorf("gcp: resourceId (self-link or project/instance) is required")
	}
	return nil
}

func (p *gcpProvider) Discover(ctx context.Context, cfg ExternalProviderConfig) (*ExternalResourceInfo, error) {
	info := &ExternalResourceInfo{Region: cfg.Region, EngineType: cfg.EngineType}

	// Try to obtain credentials and call GCP APIs
	token, err := p.getAccessToken(cfg)
	if err != nil || token == "" {
		// Fallback: use default endpoint construction
		return p.discoverFallback(cfg), nil
	}

	switch cfg.EngineType {
	case "postgres", "mysql":
		return p.discoverCloudSQL(ctx, cfg, token)
	case "redis", "keyvalue":
		return p.discoverMemorystore(ctx, cfg, token)
	default:
		// Fallback for unknown types
		info.Endpoint = cfg.Endpoint
		return info, nil
	}
}

func (p *gcpProvider) discoverFallback(cfg ExternalProviderConfig) *ExternalResourceInfo {
	info := &ExternalResourceInfo{Region: cfg.Region, EngineType: cfg.EngineType}
	switch cfg.EngineType {
	case "postgres", "mysql":
		info.Endpoint = fmt.Sprintf("%s.cloudsql.google.com:5432", cfg.Region)
	case "redis", "keyvalue":
		info.EngineType = "keyvalue"
		info.Endpoint = "redis.googleapis.com:6379"
	default:
		info.Endpoint = cfg.Endpoint
	}
	return info
}

func (p *gcpProvider) discoverCloudSQL(ctx context.Context, cfg ExternalProviderConfig, token string) (*ExternalResourceInfo, error) {
	info := &ExternalResourceInfo{Region: cfg.Region, EngineType: cfg.EngineType}

	resourceID := p.normalizeResourceID(cfg.ResourceID)
	url := fmt.Sprintf("https://sqladmin.googleapis.com/v1/%s", resourceID)

	resp, err := p.makeRequest(ctx, "GET", url, token, nil)
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return info, fmt.Errorf("gcp: CloudSQL API returned %d", resp.StatusCode)
	}

	var instance struct {
		IPAddresses []struct {
			IPAddress string `json:"ipAddress"`
		} `json:"ipAddresses"`
		DatabaseVersion string `json:"databaseVersion"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&instance); err != nil {
		return info, fmt.Errorf("gcp: failed to parse CloudSQL response: %w", err)
	}

	var port string
	if cfg.EngineType == "mysql" {
		port = "3306"
	} else {
		port = "5432"
	}

	if len(instance.IPAddresses) > 0 {
		info.Endpoint = fmt.Sprintf("%s:%s", instance.IPAddresses[0].IPAddress, port)
	}

	if instance.DatabaseVersion != "" {
		info.EngineVersion = instance.DatabaseVersion
	}

	return info, nil
}

func (p *gcpProvider) discoverMemorystore(ctx context.Context, cfg ExternalProviderConfig, token string) (*ExternalResourceInfo, error) {
	info := &ExternalResourceInfo{Region: cfg.Region, EngineType: "keyvalue"}

	resourceID := p.normalizeResourceID(cfg.ResourceID)
	url := fmt.Sprintf("https://redis.googleapis.com/v1/%s", resourceID)

	resp, err := p.makeRequest(ctx, "GET", url, token, nil)
	if err != nil {
		return info, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return info, fmt.Errorf("gcp: Memorystore API returned %d", resp.StatusCode)
	}

	var instance struct {
		Host string `json:"host"`
		Port int    `json:"port"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&instance); err != nil {
		return info, fmt.Errorf("gcp: failed to parse Memorystore response: %w", err)
	}

	if instance.Host != "" && instance.Port > 0 {
		info.Endpoint = fmt.Sprintf("%s:%d", instance.Host, instance.Port)
	}

	return info, nil
}

func (p *gcpProvider) SetupProxy(ctx context.Context, cfg ExternalProviderConfig) (*ProxyConfig, error) {
	info, err := p.Discover(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &ProxyConfig{Endpoint: info.Endpoint, TLSRequired: true, AuthType: "service-account"}, nil
}

func (p *gcpProvider) GetCostData(ctx context.Context, cfg ExternalProviderConfig) (*CostData, error) {
	token, err := p.getAccessToken(cfg)
	if err != nil || token == "" {
		return nil, &ErrNotSupported{Provider: "gcp", Capability: "GetCostData"}
	}

	projectID := p.extractProjectID(cfg)
	if projectID == "" {
		return nil, fmt.Errorf("gcp: unable to determine project ID")
	}

	url := fmt.Sprintf("https://cloudbilling.googleapis.com/v1/projects/%s/billingInfo", projectID)

	resp, err := p.makeRequest(ctx, "GET", url, token, nil)
	if err != nil {
		return nil, fmt.Errorf("gcp: billing info request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("gcp: billing API returned %d", resp.StatusCode)
	}

	var billingInfo struct {
		BillingAccountName string `json:"billingAccountName"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&billingInfo); err != nil {
		return nil, fmt.Errorf("gcp: failed to parse billing response: %w", err)
	}

	return &CostData{
		ProviderCostPerHour: 0,
		Currency:            "USD",
		BillingPeriod:       "monthly",
	}, nil
}

func (p *gcpProvider) CheckHealth(ctx context.Context, cfg ExternalProviderConfig) (*HealthResult, error) {
	token, err := p.getAccessToken(cfg)
	if err != nil || token == "" {
		// Fallback to TCP probe if no credentials
		return p.tcpHealthProbe(cfg)
	}

	resourceID := p.normalizeResourceID(cfg.ResourceID)

	switch cfg.EngineType {
	case "postgres", "mysql":
		return p.checkCloudSQLHealth(ctx, cfg, token, resourceID)
	case "redis", "keyvalue":
		return p.checkMemorystoreHealth(ctx, cfg, token, resourceID)
	default:
		// Fallback TCP probe for unknown types
		return p.tcpHealthProbe(cfg)
	}
}

func (p *gcpProvider) checkCloudSQLHealth(ctx context.Context, cfg ExternalProviderConfig, token, resourceID string) (*HealthResult, error) {
	url := fmt.Sprintf("https://sqladmin.googleapis.com/v1/%s", resourceID)

	resp, err := p.makeRequest(ctx, "GET", url, token, nil)
	if err != nil {
		return nil, fmt.Errorf("gcp: health check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &HealthResult{State: "failed", Message: "failed to reach GCP API"}, nil
	}

	var instance struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&instance); err != nil {
		return nil, fmt.Errorf("gcp: failed to parse health response: %w", err)
	}

	switch instance.State {
	case "RUNNABLE":
		return &HealthResult{State: "healthy", Message: "instance is running"}, nil
	case "SUSPENDED", "MAINTENANCE":
		return &HealthResult{State: "degraded", Message: fmt.Sprintf("instance is %s", instance.State)}, nil
	case "FAILED":
		return &HealthResult{State: "failed", Message: "instance has failed"}, nil
	default:
		return &HealthResult{State: "degraded", Message: fmt.Sprintf("unknown state: %s", instance.State)}, nil
	}
}

func (p *gcpProvider) checkMemorystoreHealth(ctx context.Context, cfg ExternalProviderConfig, token, resourceID string) (*HealthResult, error) {
	url := fmt.Sprintf("https://redis.googleapis.com/v1/%s", resourceID)

	resp, err := p.makeRequest(ctx, "GET", url, token, nil)
	if err != nil {
		return nil, fmt.Errorf("gcp: health check request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return &HealthResult{State: "failed", Message: "failed to reach GCP API"}, nil
	}

	var instance struct {
		State string `json:"state"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&instance); err != nil {
		return nil, fmt.Errorf("gcp: failed to parse health response: %w", err)
	}

	switch instance.State {
	case "READY":
		return &HealthResult{State: "healthy", Message: "instance is ready"}, nil
	case "CREATING", "UPDATING":
		return &HealthResult{State: "degraded", Message: fmt.Sprintf("instance is %s", instance.State)}, nil
	case "REPAIRING":
		return &HealthResult{State: "failed", Message: "instance is being repaired"}, nil
	default:
		return &HealthResult{State: "degraded", Message: fmt.Sprintf("unknown state: %s", instance.State)}, nil
	}
}

func (p *gcpProvider) tcpHealthProbe(cfg ExternalProviderConfig) (*HealthResult, error) {
	if cfg.Endpoint == "" {
		return &HealthResult{State: "degraded", Message: "no endpoint to probe"}, nil
	}

	conn, err := net.DialTimeout("tcp", cfg.Endpoint, 5*time.Second)
	if err != nil {
		return &HealthResult{State: "failed", Message: fmt.Sprintf("TCP probe failed: %v", err)}, nil
	}
	defer conn.Close()

	return &HealthResult{State: "healthy", Message: "TCP connection successful"}, nil
}

func (p *gcpProvider) RotateCredential(ctx context.Context, cfg ExternalProviderConfig) (string, error) {
	token, err := p.getAccessToken(cfg)
	if err != nil || token == "" {
		return "", fmt.Errorf("gcp: no credentials available for rotation")
	}

	resourceID := p.normalizeResourceID(cfg.ResourceID)

	switch cfg.EngineType {
	case "postgres", "mysql":
		return p.rotateCloudSQLPassword(ctx, cfg, token, resourceID)
	case "redis", "keyvalue":
		return p.rotateMemorystorePassword(ctx, cfg, token, resourceID)
	default:
		return "", &ErrNotSupported{Provider: "gcp", Capability: "RotateCredential"}
	}
}

func (p *gcpProvider) rotateCloudSQLPassword(ctx context.Context, cfg ExternalProviderConfig, token, resourceID string) (string, error) {
	username := "postgres"
	if u, ok := cfg.Extra["username"]; ok {
		username = u
	}

	newPassword := generateRandomPassword(20)

	url := fmt.Sprintf("https://sqladmin.googleapis.com/v1/%s/users/%s", resourceID, username)

	body := map[string]interface{}{
		"password": newPassword,
	}
	bodyBytes, _ := json.Marshal(body)

	resp, err := p.makeRequest(ctx, "PATCH", url, token, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("gcp: password rotation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("gcp: CloudSQL API returned %d", resp.StatusCode)
	}

	return newPassword, nil
}

func (p *gcpProvider) rotateMemorystorePassword(ctx context.Context, cfg ExternalProviderConfig, token, resourceID string) (string, error) {
	newPassword := generateRandomPassword(20)

	url := fmt.Sprintf("https://redis.googleapis.com/v1/%s", resourceID)

	body := map[string]interface{}{
		"authEnabled": true,
		"transitionalConfig": map[string]interface{}{
			"authString": newPassword,
		},
	}
	bodyBytes, _ := json.Marshal(body)

	resp, err := p.makeRequest(ctx, "PATCH", url, token, bytes.NewReader(bodyBytes))
	if err != nil {
		return "", fmt.Errorf("gcp: password rotation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusAccepted {
		return "", fmt.Errorf("gcp: Memorystore API returned %d", resp.StatusCode)
	}

	return newPassword, nil
}

// Helper functions

func (p *gcpProvider) getAccessToken(cfg ExternalProviderConfig) (string, error) {
	// Check for pre-obtained access token in Extra
	if token, ok := cfg.Extra["access_token"]; ok && token != "" {
		return token, nil
	}

	// Try to use service account credentials_json
	if credJSON, ok := cfg.Extra["credentials_json"]; ok && credJSON != "" {
		token, err := p.getTokenFromServiceAccount(credJSON)
		if err == nil && token != "" {
			return token, nil
		}
	}

	// Try GCP Metadata Server (for GCP-hosted environments)
	token, err := p.getTokenFromMetadataServer()
	if err == nil && token != "" {
		return token, nil
	}

	return "", fmt.Errorf("gcp: no credentials available")
}

func (p *gcpProvider) getTokenFromServiceAccount(credJSON string) (string, error) {
	var cred struct {
		ClientEmail  string `json:"client_email"`
		PrivateKeyID string `json:"private_key_id"`
		PrivateKey   string `json:"private_key"`
	}
	if err := json.Unmarshal([]byte(credJSON), &cred); err != nil {
		return "", fmt.Errorf("gcp: failed to parse credentials: %w", err)
	}

	// Parse private key
	block, _ := pem.Decode([]byte(cred.PrivateKey))
	if block == nil {
		return "", fmt.Errorf("gcp: failed to decode PEM block")
	}

	key, err := x509.ParsePKCS8PrivateKey(block.Bytes)
	if err != nil {
		return "", fmt.Errorf("gcp: failed to parse private key: %w", err)
	}

	rsaKey, ok := key.(*rsa.PrivateKey)
	if !ok {
		return "", fmt.Errorf("gcp: key is not RSA private key")
	}

	// Create JWT
	now := time.Now().Unix()
	claims := map[string]interface{}{
		"iss":   cred.ClientEmail,
		"scope": "https://www.googleapis.com/auth/cloud-platform",
		"aud":   "https://oauth2.googleapis.com/token",
		"exp":   now + 3600,
		"iat":   now,
	}

	header := map[string]string{
		"alg": "RS256",
		"typ": "JWT",
	}

	headerBytes, _ := json.Marshal(header)
	claimsBytes, _ := json.Marshal(claims)

	headerB64 := base64.RawURLEncoding.EncodeToString(headerBytes)
	claimsB64 := base64.RawURLEncoding.EncodeToString(claimsBytes)

	signInput := fmt.Sprintf("%s.%s", headerB64, claimsB64)

	// Sign with RS256
	hash := sha256.Sum256([]byte(signInput))
	signature, err := rsa.SignPKCS1v15(rand.Reader, rsaKey, crypto.SHA256, hash[:])
	if err != nil {
		return "", fmt.Errorf("gcp: failed to sign JWT: %w", err)
	}

	signatureB64 := base64.RawURLEncoding.EncodeToString(signature)
	jwt := fmt.Sprintf("%s.%s", signInput, signatureB64)

	// Exchange JWT for access token
	_ = fmt.Sprintf(
		"grant_type=%s&assertion=%s",
		base64.URLEncoding.EncodeToString([]byte("urn:ietf:params:oauth:grant-type:jwt-bearer")),
		jwt,
	)

	req, _ := http.NewRequest("POST", "https://oauth2.googleapis.com/token",
		bytes.NewReader([]byte(fmt.Sprintf(
			"grant_type=urn:ietf:params:oauth:grant-type:jwt-bearer&assertion=%s", jwt))))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("gcp: failed to request token: %w", err)
	}
	defer resp.Body.Close()

	var tokenResp struct {
		AccessToken string `json:"access_token"`
		ExpiresIn   int    `json:"expires_in"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", fmt.Errorf("gcp: failed to parse token response: %w", err)
	}

	if tokenResp.AccessToken == "" {
		return "", fmt.Errorf("gcp: no access token in response")
	}

	return tokenResp.AccessToken, nil
}

func (p *gcpProvider) getTokenFromMetadataServer() (string, error) {
	client := &http.Client{Timeout: 1 * time.Second}

	req, _ := http.NewRequest("GET", "http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token", nil)
	req.Header.Set("Metadata-Flavor", "Google")

	resp, err := client.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("metadata server returned %d", resp.StatusCode)
	}

	var tokenResp struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&tokenResp); err != nil {
		return "", err
	}

	return tokenResp.AccessToken, nil
}

func (p *gcpProvider) makeRequest(ctx context.Context, method, url, token string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, method, url, body)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", token))
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	client := &http.Client{Timeout: 30 * time.Second}
	return client.Do(req)
}

func (p *gcpProvider) normalizeResourceID(resourceID string) string {
	// Already in full format: "projects/{proj}/instances/{inst}"
	if strings.HasPrefix(resourceID, "projects/") {
		return resourceID
	}
	// Extract project from extra if needed, or assume format is OK
	return resourceID
}

func (p *gcpProvider) extractProjectID(cfg ExternalProviderConfig) string {
	// Check Extra first
	if proj, ok := cfg.Extra["project_id"]; ok && proj != "" {
		return proj
	}

	// Try to extract from ResourceID: "projects/{proj}/instances/{inst}"
	parts := strings.Split(cfg.ResourceID, "/")
	if len(parts) >= 2 && parts[0] == "projects" {
		return parts[1]
	}

	return ""
}
