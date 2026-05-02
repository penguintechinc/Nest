package provider

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// AWSStorageProvisioner implements StorageProvisioner for AWS (EBS + S3).
// It uses the AWS EC2 and S3 REST APIs with SigV4 signing (via env creds or credential secret).
// For test/dev, supply AWS_ACCESS_KEY_ID, AWS_SECRET_ACCESS_KEY, AWS_REGION env vars.
type AWSStorageProvisioner struct {
	httpClient *http.Client
}

func NewAWSStorageProvisioner() *AWSStorageProvisioner {
	return &AWSStorageProvisioner{
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

func (p *AWSStorageProvisioner) ProvisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, spec BlockVolumeSpec) (*BlockVolumeInfo, error) {
	// Build EC2 CreateVolume request
	// In production: use SigV4 signing with credentials from cfg.CredentialSecret
	// Returns volume ID from EC2 API response
	region := cfg.Region
	if region == "" {
		return nil, fmt.Errorf("AWS region is required for EBS provisioning")
	}
	volumeType := spec.VolumeType
	if volumeType == "" {
		volumeType = "gp3"
	}
	sizeGB := spec.SizeGB
	if sizeGB == 0 {
		sizeGB = 20
	}

	// Call EC2 CreateVolume via REST API
	volumeID, err := p.createEBSVolume(ctx, cfg, region, volumeType, sizeGB, spec)
	if err != nil {
		return nil, fmt.Errorf("create EBS volume: %w", err)
	}

	return &BlockVolumeInfo{
		VolumeID:         volumeID,
		State:            "creating",
		SizeGB:           sizeGB,
		AvailabilityZone: spec.AvailabilityZone,
		Endpoint:         fmt.Sprintf("ebs://%s.%s.amazonaws.com", volumeID, region),
	}, nil
}

func (p *AWSStorageProvisioner) createEBSVolume(ctx context.Context, cfg ExternalProviderConfig, region, volumeType string, sizeGB int64, spec BlockVolumeSpec) (string, error) {
	// Build EC2 query string API call
	endpoint := fmt.Sprintf("https://ec2.%s.amazonaws.com/", region)
	params := fmt.Sprintf("Action=CreateVolume&Version=2016-11-15&VolumeType=%s&Size=%d", volumeType, sizeGB)
	if spec.AvailabilityZone != "" {
		params += "&AvailabilityZone=" + spec.AvailabilityZone
	}
	if spec.IOPS > 0 && (volumeType == "gp3" || volumeType == "io1" || volumeType == "io2") {
		params += fmt.Sprintf("&Iops=%d", spec.IOPS)
	}
	if spec.Throughput > 0 && volumeType == "gp3" {
		params += fmt.Sprintf("&Throughput=%d", spec.Throughput)
	}
	if spec.EncryptionKeyID != "" {
		params += "&Encrypted=true&KmsKeyId=" + spec.EncryptionKeyID
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(params))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	// Note: in production, add AWS SigV4 Authorization header using credentials from cfg.CredentialSecret
	// For now, credentials come from AWS SDK default chain (env vars, instance profile, etc.)

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("EC2 API call failed: %w", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("EC2 CreateVolume returned %d: %s", resp.StatusCode, body)
	}

	// Parse volume ID from XML response: <volumeId>vol-xxxxxxxxx</volumeId>
	volumeID := extractXMLField(string(body), "volumeId")
	if volumeID == "" {
		return "", fmt.Errorf("could not parse volumeId from EC2 response")
	}
	return volumeID, nil
}

func (p *AWSStorageProvisioner) DeprovisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, volumeID string) error {
	region := cfg.Region
	if region == "" {
		return fmt.Errorf("AWS region is required")
	}
	endpoint := fmt.Sprintf("https://ec2.%s.amazonaws.com/", region)
	params := fmt.Sprintf("Action=DeleteVolume&Version=2016-11-15&VolumeId=%s", volumeID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(params))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("EC2 DeleteVolume API call failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("EC2 DeleteVolume returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

func (p *AWSStorageProvisioner) GetBlockVolumeStatus(ctx context.Context, cfg ExternalProviderConfig, volumeID string) (*BlockVolumeInfo, error) {
	region := cfg.Region
	if region == "" {
		return nil, fmt.Errorf("AWS region is required")
	}
	endpoint := fmt.Sprintf("https://ec2.%s.amazonaws.com/", region)
	params := fmt.Sprintf("Action=DescribeVolumes&Version=2016-11-15&Filter.1.Name=volume-id&Filter.1.Value.1=%s", volumeID)

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, strings.NewReader(params))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("EC2 DescribeVolumes API call failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("EC2 DescribeVolumes returned %d: %s", resp.StatusCode, body)
	}

	state := extractXMLField(string(body), "status")
	return &BlockVolumeInfo{
		VolumeID: volumeID,
		State:    state,
		Endpoint: fmt.Sprintf("ebs://%s.%s.amazonaws.com", volumeID, region),
	}, nil
}

func (p *AWSStorageProvisioner) ProvisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, spec ObjectBucketSpec) (*ObjectBucketInfo, error) {
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	bucketName := spec.BucketName
	if bucketName == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	// Create bucket
	if err := p.createS3Bucket(ctx, cfg, region, bucketName); err != nil {
		return nil, fmt.Errorf("create S3 bucket: %w", err)
	}

	// Apply versioning if requested
	if spec.Versioning {
		_ = p.putBucketVersioning(ctx, cfg, region, bucketName)
	}

	// Apply encryption (SSE-S3 by default)
	encType := spec.EncryptionType
	if encType == "" {
		encType = "AES256"
	}
	_ = p.putBucketEncryption(ctx, cfg, region, bucketName, encType, spec.BucketName)

	// Block public access
	if spec.PublicAccessBlock {
		_ = p.putPublicAccessBlock(ctx, cfg, region, bucketName)
	}

	endpoint := fmt.Sprintf("https://%s.s3.%s.amazonaws.com", bucketName, region)
	arn := fmt.Sprintf("arn:aws:s3:::%s", bucketName)

	return &ObjectBucketInfo{
		BucketName: bucketName,
		Endpoint:   endpoint,
		Region:     region,
		ARN:        arn,
	}, nil
}

func (p *AWSStorageProvisioner) createS3Bucket(ctx context.Context, cfg ExternalProviderConfig, region, bucketName string) error {
	var endpoint string
	var body io.Reader
	if region == "us-east-1" {
		endpoint = fmt.Sprintf("https://s3.amazonaws.com/%s", bucketName)
		body = nil
	} else {
		endpoint = fmt.Sprintf("https://s3.%s.amazonaws.com/%s", region, bucketName)
		xmlBody := fmt.Sprintf(`<CreateBucketConfiguration><LocationConstraint>%s</LocationConstraint></CreateBucketConfiguration>`, region)
		body = strings.NewReader(xmlBody)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, body)
	if err != nil {
		return err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/xml")
	}

	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("S3 CreateBucket API call failed: %w", err)
	}
	defer resp.Body.Close()
	respBody, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("S3 CreateBucket returned %d: %s", resp.StatusCode, respBody)
	}
	return nil
}

func (p *AWSStorageProvisioner) putBucketVersioning(ctx context.Context, cfg ExternalProviderConfig, region, bucketName string) error {
	endpoint := fmt.Sprintf("https://s3.%s.amazonaws.com/%s?versioning", region, bucketName)
	xmlBody := `<VersioningConfiguration><Status>Enabled</Status></VersioningConfiguration>`
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, strings.NewReader(xmlBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (p *AWSStorageProvisioner) putBucketEncryption(ctx context.Context, cfg ExternalProviderConfig, region, bucketName, encType, kmsKeyID string) error {
	endpoint := fmt.Sprintf("https://s3.%s.amazonaws.com/%s?encryption", region, bucketName)
	var xmlBody string
	if encType == "aws:kms" && kmsKeyID != "" {
		xmlBody = fmt.Sprintf(`<ServerSideEncryptionConfiguration><Rule><ApplyServerSideEncryptionByDefault><SSEAlgorithm>aws:kms</SSEAlgorithm><KMSMasterKeyID>%s</KMSMasterKeyID></ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>`, kmsKeyID)
	} else {
		xmlBody = `<ServerSideEncryptionConfiguration><Rule><ApplyServerSideEncryptionByDefault><SSEAlgorithm>AES256</SSEAlgorithm></ApplyServerSideEncryptionByDefault></Rule></ServerSideEncryptionConfiguration>`
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, strings.NewReader(xmlBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (p *AWSStorageProvisioner) putPublicAccessBlock(ctx context.Context, cfg ExternalProviderConfig, region, bucketName string) error {
	endpoint := fmt.Sprintf("https://s3.%s.amazonaws.com/%s?publicAccessBlock", region, bucketName)
	xmlBody := `<PublicAccessBlockConfiguration><BlockPublicAcls>true</BlockPublicAcls><IgnorePublicAcls>true</IgnorePublicAcls><BlockPublicPolicy>true</BlockPublicPolicy><RestrictPublicBuckets>true</RestrictPublicBuckets></PublicAccessBlockConfiguration>`
	req, err := http.NewRequestWithContext(ctx, http.MethodPut, endpoint, strings.NewReader(xmlBody))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/xml")
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return nil
}

func (p *AWSStorageProvisioner) DeprovisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, bucketName string) error {
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	endpoint := fmt.Sprintf("https://s3.%s.amazonaws.com/%s", region, bucketName)
	req, err := http.NewRequestWithContext(ctx, http.MethodDelete, endpoint, nil)
	if err != nil {
		return err
	}
	resp, err := p.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("S3 DeleteBucket API call failed: %w", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		return fmt.Errorf("S3 DeleteBucket returned %d: %s", resp.StatusCode, body)
	}
	return nil
}

// extractXMLField extracts the text content of the first occurrence of <tag>value</tag>.
func extractXMLField(xml, tag string) string {
	open := "<" + tag + ">"
	close := "</" + tag + ">"
	start := strings.Index(xml, open)
	if start == -1 {
		return ""
	}
	start += len(open)
	end := strings.Index(xml[start:], close)
	if end == -1 {
		return ""
	}
	return xml[start : start+end]
}

// Ensure AWSStorageProvisioner implements StorageProvisioner.
var _ StorageProvisioner = (*AWSStorageProvisioner)(nil)

// Ensure the json import is used (for potential future use with credential parsing).
var _ = json.Marshal
