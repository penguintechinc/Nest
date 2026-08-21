package cloudprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"time"
)

func init() { Register(&vultrProvider{}) }

type vultrProvider struct{}

func (p *vultrProvider) Name() string           { return "vultr" }
func (p *vultrProvider) SupportsIndexing() bool { return true }

func (p *vultrProvider) Validate(ctx context.Context, cfg ExternalProviderConfig) error {
	if cfg.ResourceID == "" {
		return fmt.Errorf("vultr: resourceId (database UUID) is required")
	}
	return nil
}

func (p *vultrProvider) Discover(ctx context.Context, cfg ExternalProviderConfig) (*ExternalResourceInfo, error) {
	apiKey := cfg.Extra["api_key"]

	if apiKey == "" {
		return &ExternalResourceInfo{
			EngineType: cfg.EngineType,
			Endpoint:   cfg.Endpoint,
			Region:     cfg.Region,
		}, nil
	}

	dbInfo, err := getVultrDatabase(ctx, apiKey, cfg.ResourceID)
	if err != nil {
		return nil, fmt.Errorf("vultr discover: %w", err)
	}

	engineType := mapVultrEngineType(dbInfo.DatabaseEngine)
	endpoint := fmt.Sprintf("%s:%d", dbInfo.Host, dbInfo.Port)
	sizeGB := parseVultrPlanSize(dbInfo.PlanInfo)

	return &ExternalResourceInfo{
		EngineType: engineType,
		Endpoint:   endpoint,
		Region:     cfg.Region,
		SizeGB:     int64(sizeGB),
	}, nil
}

func (p *vultrProvider) SetupProxy(ctx context.Context, cfg ExternalProviderConfig) (*ProxyConfig, error) {
	return &ProxyConfig{Endpoint: cfg.Endpoint, TLSRequired: true, AuthType: "api-key"}, nil
}

func (p *vultrProvider) GetCostData(ctx context.Context, cfg ExternalProviderConfig) (*CostData, error) {
	apiKey := cfg.Extra["api_key"]

	if apiKey == "" {
		return nil, &ErrNotSupported{Provider: "vultr", Capability: "GetCostData"}
	}

	totalCost, err := getVultrMonthlyBilling(ctx, apiKey, cfg.ResourceID)
	if err != nil {
		return nil, fmt.Errorf("vultr cost data: %w", err)
	}

	return &CostData{
		ProviderCostPerHour: totalCost / 720.0,
		Currency:            "USD",
		BillingPeriod:       "monthly",
	}, nil
}

func (p *vultrProvider) CheckHealth(ctx context.Context, cfg ExternalProviderConfig) (*HealthResult, error) {
	apiKey := ""
	if cfg.Extra != nil {
		apiKey = cfg.Extra["api_key"]
	}

	if apiKey != "" {
		dbInfo, err := getVultrDatabase(ctx, apiKey, cfg.ResourceID)
		if err == nil {
			state, message := mapVultrStatus(dbInfo.Status)
			return &HealthResult{State: state, Message: message}, nil
		}
	}

	host, port, err := extractHostPort(cfg.Endpoint)
	if err != nil {
		return &HealthResult{State: "failed", Message: fmt.Sprintf("invalid endpoint: %v", err)}, nil
	}

	dialCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()

	addr := fmt.Sprintf("%s:%s", host, port)
	conn, err := (&net.Dialer{}).DialContext(dialCtx, "tcp", addr)
	if err != nil {
		return &HealthResult{State: "failed", Message: fmt.Sprintf("tcp connection failed: %v", err)}, nil
	}
	conn.Close()

	return &HealthResult{State: "healthy", Message: "tcp connection successful"}, nil
}

func (p *vultrProvider) RotateCredential(ctx context.Context, cfg ExternalProviderConfig) (string, error) {
	apiKey := ""
	if cfg.Extra != nil {
		apiKey = cfg.Extra["api_key"]
	}

	if apiKey == "" {
		return "", &ErrNotSupported{Provider: "vultr", Capability: "RotateCredential"}
	}

	username := "vultradmin"
	if cfg.Extra != nil {
		if u := cfg.Extra["username"]; u != "" {
			username = u
		}
	}

	newPassword := generateRandomPassword(20)

	err := updateVultrDatabaseUser(ctx, apiKey, cfg.ResourceID, username, newPassword)
	if err != nil {
		return "", fmt.Errorf("vultr rotate credential: %w", err)
	}

	return newPassword, nil
}

type vultrDatabaseResponse struct {
	Database vultrDatabase `json:"database"`
}

type vultrDatabase struct {
	ID                string    `json:"id"`
	Status            string    `json:"status"`
	Host              string    `json:"host"`
	Port              int       `json:"port"`
	DatabaseEngine    string    `json:"database_engine"`
	LatestRestoreTime string    `json:"latest_restore_time"`
	PlanInfo          vultrPlan `json:"plan"`
}

type vultrPlan struct {
	ID      string       `json:"id"`
	Name    string       `json:"name"`
	RamMb   int          `json:"ram"`
	DiskGb  int          `json:"disk"`
	VCpus   int          `json:"vcpus"`
	Pricing vultrPricing `json:"pricing"`
}

type vultrPricing struct {
	HourlyCost  float64 `json:"hourly"`
	MonthlyCost float64 `json:"monthly"`
}

type vultrBillingResponse struct {
	BillingHistory []vultrBillingItem `json:"billing_history"`
}

type vultrBillingItem struct {
	ID          string  `json:"id"`
	InvoiceID   string  `json:"invoice_id"`
	Description string  `json:"description"`
	Amount      float64 `json:"amount"`
	StartDate   string  `json:"start_date"`
	EndDate     string  `json:"end_date"`
}

func getVultrDatabase(ctx context.Context, apiKey, resourceID string) (*vultrDatabase, error) {
	url := fmt.Sprintf("https://api.vultr.com/v2/databases/%s", resourceID)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("vultr api error: status %d: %s", resp.StatusCode, string(body))
	}

	var result vultrDatabaseResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, err
	}

	return &result.Database, nil
}

func getVultrMonthlyBilling(ctx context.Context, apiKey, resourceID string) (float64, error) {
	url := "https://api.vultr.com/v2/billing/history"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return 0, err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return 0, fmt.Errorf("vultr api error: status %d: %s", resp.StatusCode, string(body))
	}

	var result vultrBillingResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}

	totalCost := 0.0
	for _, item := range result.BillingHistory {
		if strings.Contains(item.Description, resourceID) {
			totalCost += item.Amount
		}
	}

	return totalCost, nil
}

func updateVultrDatabaseUser(ctx context.Context, apiKey, resourceID, username, password string) error {
	url := fmt.Sprintf("https://api.vultr.com/v2/databases/%s/users/%s", resourceID, username)

	payload := map[string]string{"password": password}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, url, strings.NewReader(string(body)))
	if err != nil {
		return err
	}

	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", apiKey))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("vultr api error: status %d: %s", resp.StatusCode, string(body))
	}

	return nil
}

func extractHostPort(endpoint string) (string, string, error) {
	parts := strings.Split(endpoint, ":")
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid endpoint format: expected 'host:port', got '%s'", endpoint)
	}
	return parts[0], parts[1], nil
}

func mapVultrEngineType(engineStr string) string {
	switch strings.ToLower(engineStr) {
	case "postgresql":
		return "postgresql"
	case "mysql":
		return "mysql"
	case "redis":
		return "redis"
	case "kafka":
		return "kafka"
	default:
		return "unknown"
	}
}

func mapVultrStatus(status string) (string, string) {
	switch strings.ToLower(status) {
	case "running":
		return "healthy", "Database is running"
	case "rebuilding", "rebalancing":
		return "degraded", fmt.Sprintf("Database is %s", status)
	case "error":
		return "failed", "Database is in error state"
	default:
		return "failed", fmt.Sprintf("Database status: %s", status)
	}
}

func parseVultrPlanSize(plan vultrPlan) int {
	return plan.DiskGb
}
