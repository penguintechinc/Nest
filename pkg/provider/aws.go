package provider

import (
	"context"
	"fmt"
	"strings"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	"github.com/google/uuid"
)

// AWSStorageProvisioner implements StorageProvisioner for AWS (EBS + S3).
// It uses the AWS SDK v2 for EC2 and S3 operations with automatic SigV4 signing.
// Credentials come from the config (with keys "access_key_id", "secret_access_key")
// with fallback to AWS default credential chain (env vars, instance profile, etc.).
type AWSStorageProvisioner struct {
	ec2Client *ec2.Client
	s3Client  *s3.Client
}

func NewAWSStorageProvisioner() *AWSStorageProvisioner {
	return &AWSStorageProvisioner{}
}

// initClients initializes EC2 and S3 clients using credentials from cfg or default chain.
func (p *AWSStorageProvisioner) initClients(ctx context.Context, cfg ExternalProviderConfig) error {
	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	var awsCfg aws.Config
	var err error

	// Check if credentials are provided in cfg.Extra
	if accessKeyID, ok := cfg.Extra["access_key_id"]; ok {
		if secretAccessKey, ok := cfg.Extra["secret_access_key"]; ok {
			awsCfg, err = config.LoadDefaultConfig(ctx,
				config.WithRegion(region),
				config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider(
					accessKeyID, secretAccessKey, "")),
			)
		} else {
			return fmt.Errorf("secret_access_key required when access_key_id is provided")
		}
	} else {
		// Use default credential chain (env vars, instance profile, etc.)
		awsCfg, err = config.LoadDefaultConfig(ctx, config.WithRegion(region))
	}

	if err != nil {
		return fmt.Errorf("load AWS config: %w", err)
	}

	p.ec2Client = ec2.NewFromConfig(awsCfg)
	p.s3Client = s3.NewFromConfig(awsCfg)
	return nil
}

func (p *AWSStorageProvisioner) ProvisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, spec BlockVolumeSpec) (*BlockVolumeInfo, error) {
	if err := p.initClients(ctx, cfg); err != nil {
		return nil, err
	}

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

	volumeID, err := p.createEBSVolume(ctx, region, volumeType, sizeGB, spec)
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

func (p *AWSStorageProvisioner) createEBSVolume(ctx context.Context, region, volumeType string, sizeGB int64, spec BlockVolumeSpec) (string, error) {
	input := &ec2.CreateVolumeInput{
		AvailabilityZone: aws.String(spec.AvailabilityZone),
		Size:             aws.Int32(int32(sizeGB)),
		VolumeType:       ec2types.VolumeType(volumeType),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeVolume,
				Tags: []ec2types.Tag{
					{
						Key:   aws.String("IdempotencyToken"),
						Value: aws.String(uuid.New().String()),
					},
				},
			},
		},
	}

	if spec.IOPS > 0 && (volumeType == "gp3" || volumeType == "io1" || volumeType == "io2") {
		input.Iops = aws.Int32(int32(spec.IOPS))
	}
	if spec.Throughput > 0 && volumeType == "gp3" {
		input.Throughput = aws.Int32(int32(spec.Throughput))
	}
	if spec.EncryptionKeyID != "" {
		input.Encrypted = aws.Bool(true)
		input.KmsKeyId = aws.String(spec.EncryptionKeyID)
	}

	result, err := p.ec2Client.CreateVolume(ctx, input)
	if err != nil {
		return "", fmt.Errorf("EC2 CreateVolume: %w", err)
	}

	if result.VolumeId == nil {
		return "", fmt.Errorf("EC2 CreateVolume returned no volume ID")
	}
	return *result.VolumeId, nil
}

func (p *AWSStorageProvisioner) DeprovisionBlockVolume(ctx context.Context, cfg ExternalProviderConfig, volumeID string) error {
	if err := p.initClients(ctx, cfg); err != nil {
		return err
	}

	_, err := p.ec2Client.DeleteVolume(ctx, &ec2.DeleteVolumeInput{VolumeId: aws.String(volumeID)})
	if err != nil {
		// Treat volume not found as success (idempotent)
		if strings.Contains(err.Error(), "InvalidVolume.NotFound") {
			return nil
		}
		return fmt.Errorf("EC2 DeleteVolume: %w", err)
	}
	return nil
}

func (p *AWSStorageProvisioner) GetBlockVolumeStatus(ctx context.Context, cfg ExternalProviderConfig, volumeID string) (*BlockVolumeInfo, error) {
	if err := p.initClients(ctx, cfg); err != nil {
		return nil, err
	}

	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}

	result, err := p.ec2Client.DescribeVolumes(ctx, &ec2.DescribeVolumesInput{
		VolumeIds: []string{volumeID},
	})
	if err != nil {
		return nil, fmt.Errorf("EC2 DescribeVolumes: %w", err)
	}

	if len(result.Volumes) == 0 {
		return nil, fmt.Errorf("volume %s not found", volumeID)
	}

	vol := result.Volumes[0]
	state := string(vol.State)

	return &BlockVolumeInfo{
		VolumeID:         volumeID,
		State:            state,
		SizeGB:           int64(*vol.Size),
		AvailabilityZone: *vol.AvailabilityZone,
		Endpoint:         fmt.Sprintf("ebs://%s.%s.amazonaws.com", volumeID, region),
	}, nil
}

func (p *AWSStorageProvisioner) ProvisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, spec ObjectBucketSpec) (*ObjectBucketInfo, error) {
	if err := p.initClients(ctx, cfg); err != nil {
		return nil, err
	}

	region := cfg.Region
	if region == "" {
		region = "us-east-1"
	}
	bucketName := spec.BucketName
	if bucketName == "" {
		return nil, fmt.Errorf("bucket name is required")
	}

	// Create bucket
	if err := p.createS3Bucket(ctx, region, bucketName); err != nil {
		return nil, fmt.Errorf("create S3 bucket: %w", err)
	}

	// Apply versioning if requested
	if spec.Versioning {
		if err := p.putBucketVersioning(ctx, region, bucketName); err != nil {
			return nil, fmt.Errorf("enable versioning: %w", err)
		}
	}

	// Apply encryption (SSE-S3 by default)
	encType := spec.EncryptionType
	if encType == "" || encType == "AES256" {
		if err := p.putBucketEncryption(ctx, region, bucketName, "AES256", ""); err != nil {
			return nil, fmt.Errorf("enable encryption: %w", err)
		}
	} else if encType == "aws:kms" {
		kmsKeyID, ok := cfg.Extra["kms_key_id"]
		if !ok || kmsKeyID == "" {
			return nil, fmt.Errorf("KMS encryption requested but kms_key_id not provided in config")
		}
		if err := p.putBucketEncryption(ctx, region, bucketName, "aws:kms", kmsKeyID); err != nil {
			return nil, fmt.Errorf("enable KMS encryption: %w", err)
		}
	} else {
		return nil, fmt.Errorf("unsupported encryption type: %s", encType)
	}

	// Block public access
	if spec.PublicAccessBlock {
		if err := p.putPublicAccessBlock(ctx, region, bucketName); err != nil {
			return nil, fmt.Errorf("apply public access block: %w", err)
		}
	} else {
		return nil, fmt.Errorf("PublicAccessBlock must be true for security; public buckets are not supported")
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

func (p *AWSStorageProvisioner) createS3Bucket(ctx context.Context, region, bucketName string) error {
	input := &s3.CreateBucketInput{
		Bucket: aws.String(bucketName),
	}

	// LocationConstraint is only needed for non-us-east-1 regions
	if region != "us-east-1" {
		input.CreateBucketConfiguration = &s3types.CreateBucketConfiguration{
			LocationConstraint: s3types.BucketLocationConstraint(region),
		}
	}

	_, err := p.s3Client.CreateBucket(ctx, input)
	if err != nil {
		return fmt.Errorf("S3 CreateBucket: %w", err)
	}
	return nil
}

func (p *AWSStorageProvisioner) putBucketVersioning(ctx context.Context, region, bucketName string) error {
	_, err := p.s3Client.PutBucketVersioning(ctx, &s3.PutBucketVersioningInput{
		Bucket: aws.String(bucketName),
		VersioningConfiguration: &s3types.VersioningConfiguration{
			Status: s3types.BucketVersioningStatusEnabled,
		},
	})
	if err != nil {
		return fmt.Errorf("S3 PutBucketVersioning: %w", err)
	}
	return nil
}

func (p *AWSStorageProvisioner) putBucketEncryption(ctx context.Context, region, bucketName, encType, kmsKeyID string) error {
	rule := s3types.ServerSideEncryptionRule{
		ApplyServerSideEncryptionByDefault: &s3types.ServerSideEncryptionByDefault{},
	}

	if encType == "aws:kms" {
		rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm = s3types.ServerSideEncryptionAwsKms
		if kmsKeyID != "" {
			rule.ApplyServerSideEncryptionByDefault.KMSMasterKeyID = aws.String(kmsKeyID)
		}
	} else {
		rule.ApplyServerSideEncryptionByDefault.SSEAlgorithm = s3types.ServerSideEncryptionAes256
	}

	_, err := p.s3Client.PutBucketEncryption(ctx, &s3.PutBucketEncryptionInput{
		Bucket: aws.String(bucketName),
		ServerSideEncryptionConfiguration: &s3types.ServerSideEncryptionConfiguration{
			Rules: []s3types.ServerSideEncryptionRule{rule},
		},
	})
	if err != nil {
		return fmt.Errorf("S3 PutBucketEncryption: %w", err)
	}
	return nil
}

func (p *AWSStorageProvisioner) putPublicAccessBlock(ctx context.Context, region, bucketName string) error {
	_, err := p.s3Client.PutPublicAccessBlock(ctx, &s3.PutPublicAccessBlockInput{
		Bucket: aws.String(bucketName),
		PublicAccessBlockConfiguration: &s3types.PublicAccessBlockConfiguration{
			BlockPublicAcls:       aws.Bool(true),
			IgnorePublicAcls:      aws.Bool(true),
			BlockPublicPolicy:     aws.Bool(true),
			RestrictPublicBuckets: aws.Bool(true),
		},
	})
	if err != nil {
		return fmt.Errorf("S3 PutPublicAccessBlock: %w", err)
	}
	return nil
}

func (p *AWSStorageProvisioner) DeprovisionObjectBucket(ctx context.Context, cfg ExternalProviderConfig, bucketName string) error {
	if err := p.initClients(ctx, cfg); err != nil {
		return err
	}

	_, err := p.s3Client.DeleteBucket(ctx, &s3.DeleteBucketInput{
		Bucket: aws.String(bucketName),
	})
	if err != nil {
		// Treat bucket not found as success (idempotent)
		if strings.Contains(err.Error(), "NoSuchBucket") {
			return nil
		}
		return fmt.Errorf("S3 DeleteBucket: %w", err)
	}
	return nil
}

// Ensure AWSStorageProvisioner implements StorageProvisioner.
var _ StorageProvisioner = (*AWSStorageProvisioner)(nil)
