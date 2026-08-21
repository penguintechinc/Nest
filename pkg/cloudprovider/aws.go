package cloudprovider

import (
	"bytes"
	"context"
	"encoding/json"
	"encoding/xml"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/penguintechinc/nest/pkg/sigv4"
)

func init() { Register(&awsProvider{}) }

type awsProvider struct{}

func (p *awsProvider) Name() string           { return "aws" }
func (p *awsProvider) SupportsIndexing() bool { return true }

func (p *awsProvider) Validate(ctx context.Context, cfg ExternalProviderConfig) error {
	if cfg.ResourceID == "" {
		return fmt.Errorf("aws: resourceId (ARN) is required")
	}
	if cfg.Region == "" {
		return fmt.Errorf("aws: region is required")
	}
	return nil
}

func (p *awsProvider) Discover(ctx context.Context, cfg ExternalProviderConfig) (*ExternalResourceInfo, error) {
	info := &ExternalResourceInfo{
		Region: cfg.Region,
	}
	switch cfg.EngineType {
	case "postgres":
		info.EngineType = "postgres"
		info.Endpoint = fmt.Sprintf("rds.%s.amazonaws.com:5432", cfg.Region)
	case "mysql":
		info.EngineType = "mysql"
		info.Endpoint = fmt.Sprintf("rds.%s.amazonaws.com:3306", cfg.Region)
	case "redis", "keyvalue":
		info.EngineType = "keyvalue"
		info.Endpoint = fmt.Sprintf("elasticache.%s.amazonaws.com:6379", cfg.Region)
	default:
		info.EngineType = cfg.EngineType
		info.Endpoint = cfg.Endpoint
	}
	return info, nil
}

func (p *awsProvider) SetupProxy(ctx context.Context, cfg ExternalProviderConfig) (*ProxyConfig, error) {
	info, err := p.Discover(ctx, cfg)
	if err != nil {
		return nil, err
	}
	return &ProxyConfig{
		Endpoint:    info.Endpoint,
		TLSRequired: true,
		AuthType:    "iam-role",
	}, nil
}

func (p *awsProvider) CheckHealth(ctx context.Context, cfg ExternalProviderConfig) (*HealthResult, error) {
	accessKey, _ := cfg.Extra["access_key_id"]
	secretKey, _ := cfg.Extra["secret_access_key"]
	sessionToken, _ := cfg.Extra["session_token"]

	if accessKey == "" || secretKey == "" {
		return p.tcpProbeHealth(cfg)
	}

	switch cfg.EngineType {
	case "postgres", "mysql":
		return p.checkRDSHealth(ctx, cfg, accessKey, secretKey, sessionToken)
	case "redis", "keyvalue":
		return p.checkElastiCacheHealth(ctx, cfg, accessKey, secretKey, sessionToken)
	case "s3", "object":
		return p.checkS3Health(ctx, cfg, accessKey, secretKey, sessionToken)
	default:
		return p.tcpProbeHealth(cfg)
	}
}

func (p *awsProvider) GetCostData(ctx context.Context, cfg ExternalProviderConfig) (*CostData, error) {
	accessKey, _ := cfg.Extra["access_key_id"]
	secretKey, _ := cfg.Extra["secret_access_key"]
	sessionToken, _ := cfg.Extra["session_token"]

	if accessKey == "" || secretKey == "" {
		return nil, &ErrNotSupported{Provider: "aws", Capability: "GetCostData"}
	}

	now := time.Now().UTC()
	endDate := now.Format("2006-01-02")
	startDate := now.AddDate(0, 0, -30).Format("2006-01-02")

	body := map[string]interface{}{
		"TimePeriod": map[string]string{
			"Start": startDate,
			"End":   endDate,
		},
		"Granularity": "MONTHLY",
		"Metrics":     []string{"AmortizedCost"},
		"Filter": map[string]interface{}{
			"Dimensions": map[string]interface{}{
				"Key":    "RESOURCE_ID",
				"Values": []string{cfg.ResourceID},
			},
		},
	}

	bodyBytes, _ := json.Marshal(body)

	req, _ := http.NewRequestWithContext(ctx, "POST", "https://ce.us-east-1.amazonaws.com/", bytes.NewBuffer(bodyBytes))
	req.Header.Set("Content-Type", "application/x-amz-json-1.1")
	req.Header.Set("X-Amz-Target", "AWSInsightsIndexService.GetCostAndUsage")

	_ = sigv4.Sign(req, "ce", "us-east-1", accessKey, secretKey, sessionToken, bodyBytes)

	client := &http.Client{Timeout: 15 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("aws cost api request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("aws cost api returned %d: %s", resp.StatusCode, string(body))
	}

	var costResp struct {
		ResultsByTime []struct {
			Total struct {
				AmortizedCost struct {
					Amount string `json:"Amount"`
					Unit   string `json:"Unit"`
				} `json:"AmortizedCost"`
			} `json:"Total"`
		} `json:"ResultsByTime"`
	}

	_ = json.NewDecoder(resp.Body).Decode(&costResp)

	costPerMonth := 0.0
	if len(costResp.ResultsByTime) > 0 {
		fmt.Sscanf(costResp.ResultsByTime[0].Total.AmortizedCost.Amount, "%f", &costPerMonth)
	}

	costPerHour := costPerMonth / 30.0 / 24.0

	return &CostData{
		ProviderCostPerHour: costPerHour,
		Currency:            "USD",
		BillingPeriod:       "monthly",
	}, nil
}

func (p *awsProvider) RotateCredential(ctx context.Context, cfg ExternalProviderConfig) (string, error) {
	accessKey, _ := cfg.Extra["access_key_id"]
	secretKey, _ := cfg.Extra["secret_access_key"]
	sessionToken, _ := cfg.Extra["session_token"]

	if accessKey == "" || secretKey == "" {
		return "", &ErrNotSupported{Provider: "aws", Capability: "RotateCredential"}
	}

	switch cfg.EngineType {
	case "postgres", "mysql":
		return p.rotateRDSPassword(ctx, cfg, accessKey, secretKey, sessionToken)
	case "redis", "keyvalue":
		return "", &ErrNotSupported{Provider: "aws", Capability: "RotateCredential for ElastiCache"}
	default:
		return "", &ErrNotSupported{Provider: "aws", Capability: "RotateCredential"}
	}
}

func (p *awsProvider) tcpProbeHealth(cfg ExternalProviderConfig) (*HealthResult, error) {
	conn, err := net.DialTimeout("tcp", cfg.Endpoint, 5*time.Second)
	if err != nil {
		return &HealthResult{State: "unreachable", Message: err.Error()}, nil
	}
	defer conn.Close()
	return &HealthResult{State: "healthy", Message: "TCP connection successful"}, nil
}

func (p *awsProvider) checkRDSHealth(ctx context.Context, cfg ExternalProviderConfig, accessKey, secretKey, sessionToken string) (*HealthResult, error) {
	dbInstanceID := extractDBInstanceID(cfg.ResourceID)

	params := url.Values{}
	params.Set("Action", "DescribeDBInstances")
	params.Set("DBInstanceIdentifier", dbInstanceID)
	params.Set("Version", "2014-10-31")

	endpoint := fmt.Sprintf("https://rds.%s.amazonaws.com/", cfg.Region)
	req, _ := http.NewRequestWithContext(ctx, "GET", endpoint+"?"+params.Encode(), nil)

	_ = sigv4.Sign(req, "rds", cfg.Region, accessKey, secretKey, sessionToken, nil)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &HealthResult{State: "unreachable", Message: err.Error()}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var rdsResp struct {
		DescribeDBInstancesResult struct {
			DBInstances []struct {
				DBInstanceStatus string `xml:"DBInstanceStatus"`
			} `xml:"DBInstances>DBInstance"`
		} `xml:"DescribeDBInstancesResult"`
	}

	_ = xml.Unmarshal(body, &rdsResp)

	if len(rdsResp.DescribeDBInstancesResult.DBInstances) > 0 {
		status := rdsResp.DescribeDBInstancesResult.DBInstances[0].DBInstanceStatus
		if status == "available" {
			return &HealthResult{State: "healthy", Message: "RDS instance available"}, nil
		}
		return &HealthResult{State: "degraded", Message: fmt.Sprintf("RDS instance status: %s", status)}, nil
	}

	return &HealthResult{State: "failed", Message: "RDS instance not found"}, nil
}

func (p *awsProvider) checkElastiCacheHealth(ctx context.Context, cfg ExternalProviderConfig, accessKey, secretKey, sessionToken string) (*HealthResult, error) {
	clusterID := extractClusterID(cfg.ResourceID)

	params := url.Values{}
	params.Set("Action", "DescribeCacheClusters")
	params.Set("CacheClusterId", clusterID)
	params.Set("Version", "2015-02-02")

	endpoint := fmt.Sprintf("https://elasticache.%s.amazonaws.com/", cfg.Region)
	req, _ := http.NewRequestWithContext(ctx, "GET", endpoint+"?"+params.Encode(), nil)

	_ = sigv4.Sign(req, "elasticache", cfg.Region, accessKey, secretKey, sessionToken, nil)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &HealthResult{State: "unreachable", Message: err.Error()}, nil
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)

	var cacheResp struct {
		DescribeCacheClustersResult struct {
			CacheClusters []struct {
				CacheClusterStatus string `xml:"CacheClusterStatus"`
			} `xml:"CacheClusters>CacheCluster"`
		} `xml:"DescribeCacheClustersResult"`
	}

	_ = xml.Unmarshal(body, &cacheResp)

	if len(cacheResp.DescribeCacheClustersResult.CacheClusters) > 0 {
		status := cacheResp.DescribeCacheClustersResult.CacheClusters[0].CacheClusterStatus
		if status == "available" {
			return &HealthResult{State: "healthy", Message: "ElastiCache cluster available"}, nil
		}
		return &HealthResult{State: "degraded", Message: fmt.Sprintf("ElastiCache cluster status: %s", status)}, nil
	}

	return &HealthResult{State: "failed", Message: "ElastiCache cluster not found"}, nil
}

func (p *awsProvider) checkS3Health(ctx context.Context, cfg ExternalProviderConfig, accessKey, secretKey, sessionToken string) (*HealthResult, error) {
	bucketName := cfg.ResourceID

	endpoint := fmt.Sprintf("https://s3.%s.amazonaws.com/%s", cfg.Region, bucketName)
	req, _ := http.NewRequestWithContext(ctx, "HEAD", endpoint, nil)

	_ = sigv4.Sign(req, "s3", cfg.Region, accessKey, secretKey, sessionToken, nil)

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return &HealthResult{State: "unreachable", Message: err.Error()}, nil
	}
	resp.Body.Close()

	if resp.StatusCode == 200 || resp.StatusCode == 403 {
		return &HealthResult{State: "healthy", Message: "S3 bucket accessible"}, nil
	}
	if resp.StatusCode == 404 {
		return &HealthResult{State: "failed", Message: "S3 bucket not found"}, nil
	}

	return &HealthResult{State: "degraded", Message: fmt.Sprintf("S3 returned status %d", resp.StatusCode)}, nil
}

func (p *awsProvider) rotateRDSPassword(ctx context.Context, cfg ExternalProviderConfig, accessKey, secretKey, sessionToken string) (string, error) {
	dbInstanceID := extractDBInstanceID(cfg.ResourceID)
	newPassword := generateRandomPassword(24)

	params := url.Values{}
	params.Set("Action", "ModifyDBInstance")
	params.Set("DBInstanceIdentifier", dbInstanceID)
	params.Set("MasterUserPassword", newPassword)
	params.Set("ApplyImmediately", "true")
	params.Set("Version", "2014-10-31")

	endpoint := fmt.Sprintf("https://rds.%s.amazonaws.com/", cfg.Region)
	req, _ := http.NewRequestWithContext(ctx, "POST", endpoint, strings.NewReader(params.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	_ = sigv4.Sign(req, "rds", cfg.Region, accessKey, secretKey, sessionToken, []byte(params.Encode()))

	client := &http.Client{Timeout: 30 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("rds password rotation request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("rds password rotation returned %d: %s", resp.StatusCode, string(body))
	}

	return newPassword, nil
}

func extractDBInstanceID(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return arn
}

func extractClusterID(arn string) string {
	parts := strings.Split(arn, ":")
	if len(parts) > 0 {
		return parts[len(parts)-1]
	}
	return arn
}
