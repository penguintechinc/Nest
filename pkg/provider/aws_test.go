package provider

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/credentials"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

// EC2 XML response types
type EC2CreateVolumeResponse struct {
	XMLName          xml.Name `xml:"CreateVolumeResponse"`
	VolumeId         string   `xml:"volumeId"`
	Size             int32    `xml:"size"`
	AvailabilityZone string   `xml:"availabilityZone"`
	Status           string   `xml:"status"`
	VolumeType       string   `xml:"volumeType"`
	CreateTime       string   `xml:"createTime"`
	ClientToken      string   `xml:"clientToken"`
	RequestId        string   `xml:"requestId"`
}

type EC2DeleteVolumeResponse struct {
	XMLName   xml.Name `xml:"DeleteVolumeResponse"`
	Return    bool     `xml:"return"`
	RequestId string   `xml:"requestId"`
}

type EC2Volume struct {
	VolumeId         string `xml:"volumeId"`
	Size             int32  `xml:"size"`
	AvailabilityZone string `xml:"availabilityZone"`
	Status           string `xml:"status"`
	VolumeType       string `xml:"volumeType"`
}

type EC2DescribeVolumesResponse struct {
	XMLName   xml.Name    `xml:"DescribeVolumesResponse"`
	Volumes   []EC2Volume `xml:"volumeSet>item"`
	RequestId string      `xml:"requestId"`
}

type EC2Error struct {
	XMLName   xml.Name `xml:"Error"`
	Code      string   `xml:"Code"`
	Message   string   `xml:"Message"`
	RequestId string   `xml:"RequestId"`
}

// newAWSProvisionerWithMockClients creates a provisioner with mocked EC2/S3 clients pointing to a test server.
func newAWSProvisionerWithMockClients(srv *httptest.Server) *AWSStorageProvisioner {
	cfg, _ := config.LoadDefaultConfig(context.Background(),
		config.WithRegion("us-east-1"),
		config.WithCredentialsProvider(credentials.NewStaticCredentialsProvider("test", "test", "")),
	)

	p := &AWSStorageProvisioner{
		ec2Client: ec2.NewFromConfig(cfg, func(o *ec2.Options) { o.BaseEndpoint = aws.String(srv.URL) }),
		s3Client:  s3.NewFromConfig(cfg, func(o *s3.Options) { o.BaseEndpoint = aws.String(srv.URL) }),
	}
	return p
}

// newMockAWSServer creates an httptest.Server that responds to EC2 Query API and S3 REST operations.
func newMockAWSServer(handler func(r *http.Request) (interface{}, error)) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Parse the request
		resp, err := handler(r)
		if err != nil {
			// Return error response
			w.WriteHeader(http.StatusBadRequest)
			w.Header().Set("Content-Type", "application/xml")
			if errResp, ok := resp.(EC2Error); ok {
				b, _ := xml.Marshal(errResp)
				w.Write(b)
			} else {
				fmt.Fprintf(w, "<Error><Message>%s</Message></Error>", err.Error())
			}
			return
		}

		// Return success response
		w.Header().Set("Content-Type", "application/xml")
		if resp != nil {
			b, _ := xml.Marshal(resp)
			w.Write(b)
		}
	}))
}

// Test ProvisionBlockVolume with mock server
func TestAWSStorageProvisioner_ProvisionBlockVolume_Success(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		// EC2 Query API uses POST with form-encoded body
		r.ParseForm()
		action := r.FormValue("Action")

		if action == "CreateVolume" {
			return EC2CreateVolumeResponse{
				VolumeId:         "vol-12345",
				Size:             20,
				AvailabilityZone: "us-west-2a",
				Status:           "creating",
				VolumeType:       "gp3",
				CreateTime:       "2026-08-08T00:00:00Z",
				ClientToken:      r.FormValue("ClientToken"),
				RequestId:        "req-123",
			}, nil
		}
		return nil, fmt.Errorf("unknown action: %s", action)
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-west-2",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	spec := BlockVolumeSpec{
		SizeGB:           20,
		VolumeType:       "gp3",
		AvailabilityZone: "us-west-2a",
	}

	info, err := p.ProvisionBlockVolume(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("ProvisionBlockVolume() error = %v", err)
	}

	if info.VolumeID != "vol-12345" {
		t.Errorf("expected vol-12345, got %s", info.VolumeID)
	}
	if info.State != "creating" {
		t.Errorf("expected state creating, got %s", info.State)
	}
	if info.SizeGB != 20 {
		t.Errorf("expected 20 GB, got %d", info.SizeGB)
	}
}

func TestAWSStorageProvisioner_ProvisionBlockVolume_DefaultsApplied(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		r.ParseForm()
		action := r.FormValue("Action")
		if action == "CreateVolume" {
			sizeStr := r.FormValue("Size")
			if sizeStr == "" {
				t.Error("Size should not be empty in request")
			}
			return EC2CreateVolumeResponse{
				VolumeId:         "vol-default",
				Size:             20,
				AvailabilityZone: "us-east-1a",
				Status:           "creating",
				VolumeType:       "gp3",
				CreateTime:       "2026-08-08T00:00:00.000Z",
				ClientToken:      r.FormValue("ClientToken"),
				RequestId:        "req-123",
			}, nil
		}
		return nil, fmt.Errorf("unknown action")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	// Empty spec — should apply defaults
	spec := BlockVolumeSpec{AvailabilityZone: "us-east-1a"}

	info, err := p.ProvisionBlockVolume(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("ProvisionBlockVolume() error = %v", err)
	}

	if info.SizeGB != 20 {
		t.Errorf("expected default size 20, got %d", info.SizeGB)
	}
}

func TestAWSStorageProvisioner_DeprovisionBlockVolume_Success(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		r.ParseForm()
		action := r.FormValue("Action")
		if action == "DeleteVolume" {
			return EC2DeleteVolumeResponse{Return: true, RequestId: "req-123"}, nil
		}
		return nil, fmt.Errorf("unknown action")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	err := p.DeprovisionBlockVolume(context.Background(), cfg, "vol-12345")
	if err != nil {
		t.Fatalf("DeprovisionBlockVolume() error = %v", err)
	}
}

func TestAWSStorageProvisioner_DeprovisionBlockVolume_NotFoundIsIdempotent(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		r.ParseForm()
		action := r.FormValue("Action")
		if action == "DeleteVolume" {
			// Simulate AWS returning InvalidVolume.NotFound
			return EC2Error{Code: "InvalidVolume.NotFound", Message: "The volume 'vol-nonexist' does not exist"}, nil
		}
		return nil, fmt.Errorf("unknown action")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	// Deleting non-existent volume should succeed (idempotent)
	err := p.DeprovisionBlockVolume(context.Background(), cfg, "vol-nonexist")
	// Should be nil because the code treats InvalidVolume.NotFound as success
	if err != nil {
		t.Errorf("expected nil error for not found (idempotent), got: %v", err)
	}
}

func TestAWSStorageProvisioner_GetBlockVolumeStatus_Success(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		r.ParseForm()
		action := r.FormValue("Action")
		if action == "DescribeVolumes" {
			return EC2DescribeVolumesResponse{
				Volumes: []EC2Volume{
					{
						VolumeId:         "vol-12345",
						Size:             100,
						AvailabilityZone: "us-west-2a",
						Status:           "available",
						VolumeType:       "gp3",
					},
				},
				RequestId: "req-123",
			}, nil
		}
		return nil, fmt.Errorf("unknown action")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-west-2",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	info, err := p.GetBlockVolumeStatus(context.Background(), cfg, "vol-12345")
	if err != nil {
		t.Fatalf("GetBlockVolumeStatus() error = %v", err)
	}

	if info.VolumeID != "vol-12345" {
		t.Errorf("expected vol-12345, got %s", info.VolumeID)
	}
	if info.State != "available" {
		t.Errorf("expected available, got %s", info.State)
	}
	if info.SizeGB != 100 {
		t.Errorf("expected 100 GB, got %d", info.SizeGB)
	}
}

func TestAWSStorageProvisioner_GetBlockVolumeStatus_NotFound(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		r.ParseForm()
		action := r.FormValue("Action")
		if action == "DescribeVolumes" {
			// Return empty volume list
			return EC2DescribeVolumesResponse{Volumes: []EC2Volume{}, RequestId: "req-123"}, nil
		}
		return nil, fmt.Errorf("unknown action")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	_, err := p.GetBlockVolumeStatus(context.Background(), cfg, "vol-nonexist")
	if err == nil {
		t.Error("expected error for non-existent volume")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("expected 'not found' in error, got: %v", err)
	}
}

func TestAWSStorageProvisioner_ProvisionObjectBucket_FullFlow(t *testing.T) {
	callLog := []string{}
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		// S3 REST API
		method := r.Method
		path := r.URL.Path
		callLog = append(callLog, fmt.Sprintf("%s %s", method, path))

		if method == "PUT" && path == "/test-bucket" {
			// CreateBucket
			return nil, nil
		}
		if method == "PUT" && strings.Contains(path, "?versioning") {
			// PutBucketVersioning
			return nil, nil
		}
		if method == "PUT" && strings.Contains(path, "?encryption") {
			// PutBucketEncryption
			return nil, nil
		}
		if method == "PUT" && strings.Contains(path, "?publicAccessBlock") {
			// PutPublicAccessBlock
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected request: %s %s", method, path)
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	spec := ObjectBucketSpec{
		BucketName:        "test-bucket",
		EncryptionType:    "AES256",
		PublicAccessBlock: true,
		Versioning:        true,
	}

	info, err := p.ProvisionObjectBucket(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("ProvisionObjectBucket() error = %v", err)
	}

	if info.BucketName != "test-bucket" {
		t.Errorf("expected test-bucket, got %s", info.BucketName)
	}
	if !strings.HasPrefix(info.ARN, "arn:aws:s3:::test-bucket") {
		t.Errorf("expected ARN, got %s", info.ARN)
	}
}

func TestAWSStorageProvisioner_ProvisionObjectBucket_KMSEncryption(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		method := r.Method
		path := r.URL.Path

		if method == "PUT" && path == "/kms-bucket" {
			return nil, nil
		}
		if method == "PUT" && strings.Contains(path, "?encryption") {
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), "aws:kms") {
				t.Error("expected KMS configuration in request body")
			}
			return nil, nil
		}
		if method == "PUT" && strings.Contains(path, "?publicAccessBlock") {
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected request")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
		Extra: map[string]string{
			"kms_key_id": "arn:aws:kms:us-east-1:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab",
		},
	}

	spec := ObjectBucketSpec{
		BucketName:        "kms-bucket",
		EncryptionType:    "aws:kms",
		PublicAccessBlock: true,
	}

	info, err := p.ProvisionObjectBucket(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("ProvisionObjectBucket() error = %v", err)
	}

	if info.BucketName != "kms-bucket" {
		t.Errorf("expected kms-bucket, got %s", info.BucketName)
	}
}

func TestAWSStorageProvisioner_ProvisionObjectBucket_KMSMissingKeyID(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		method := r.Method
		path := r.URL.Path

		// Bucket creation succeeds
		if method == "PUT" && path == "/kms-bucket" {
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected request")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	spec := ObjectBucketSpec{
		BucketName:        "kms-bucket",
		EncryptionType:    "aws:kms",
		PublicAccessBlock: true,
	}

	_, err := p.ProvisionObjectBucket(context.Background(), cfg, spec)
	if err == nil {
		t.Error("expected error when KMS key ID is missing")
	}
	if !strings.Contains(err.Error(), "kms_key_id not provided") {
		t.Errorf("expected kms_key_id error, got: %v", err)
	}
}

func TestAWSStorageProvisioner_ProvisionObjectBucket_UnsupportedEncryption(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		method := r.Method
		path := r.URL.Path

		// Bucket creation succeeds
		if method == "PUT" && path == "/test-bucket" {
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected request")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	spec := ObjectBucketSpec{
		BucketName:        "test-bucket",
		EncryptionType:    "unsupported-cipher",
		PublicAccessBlock: true,
	}

	_, err := p.ProvisionObjectBucket(context.Background(), cfg, spec)
	if err == nil {
		t.Error("expected error for unsupported encryption")
	}
	if !strings.Contains(err.Error(), "unsupported encryption type") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAWSStorageProvisioner_ProvisionObjectBucket_MissingBucketName(t *testing.T) {
	p := NewAWSStorageProvisioner()

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	spec := ObjectBucketSpec{PublicAccessBlock: true}

	_, err := p.ProvisionObjectBucket(context.Background(), cfg, spec)
	if err == nil {
		t.Error("expected error for missing bucket name")
	}
	if !strings.Contains(err.Error(), "bucket name is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAWSStorageProvisioner_ProvisionObjectBucket_MissingPublicAccessBlock(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		method := r.Method
		path := r.URL.Path

		// Bucket creation succeeds
		if method == "PUT" && path == "/test-bucket" {
			return nil, nil
		}
		// Encryption setup succeeds
		if method == "PUT" && strings.Contains(path, "?encryption") {
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected request")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	spec := ObjectBucketSpec{
		BucketName:        "test-bucket",
		PublicAccessBlock: false,
	}

	_, err := p.ProvisionObjectBucket(context.Background(), cfg, spec)
	if err == nil {
		t.Error("expected error when PublicAccessBlock is false")
	}
	if !strings.Contains(err.Error(), "PublicAccessBlock must be true") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAWSStorageProvisioner_DeprovisionObjectBucket_Success(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		method := r.Method
		path := r.URL.Path

		if method == "DELETE" && path == "/test-bucket" {
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected request: %s %s", method, path)
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	err := p.DeprovisionObjectBucket(context.Background(), cfg, "test-bucket")
	if err != nil {
		t.Fatalf("DeprovisionObjectBucket() error = %v", err)
	}
}

func TestAWSStorageProvisioner_DeprovisionObjectBucket_NotFoundIsIdempotent(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		method := r.Method
		path := r.URL.Path

		if method == "DELETE" && path == "/nonexistent-bucket" {
			// Simulate AWS returning NoSuchBucket error
			return EC2Error{Code: "NoSuchBucket", Message: "The specified bucket does not exist"}, nil
		}
		return nil, fmt.Errorf("unexpected request")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	// Deleting non-existent bucket should succeed (idempotent)
	err := p.DeprovisionObjectBucket(context.Background(), cfg, "nonexistent-bucket")
	// Should be nil because code treats NoSuchBucket as success
	if err != nil {
		t.Errorf("expected nil error for not found (idempotent), got: %v", err)
	}
}

func TestAWSStorageProvisioner_CreateEBSVolume_SizeValidation(t *testing.T) {
	tests := []struct {
		name    string
		sizeGB  int64
		wantErr bool
	}{
		{"negative size", -1, true},
		{"exceeds max int32", 2147483648, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
				return nil, fmt.Errorf("should not reach AWS")
			})
			defer srv.Close()

			p := newAWSProvisionerWithMockClients(srv)

			_, err := p.createEBSVolume(context.Background(), "us-east-1", "gp3", tt.sizeGB, BlockVolumeSpec{AvailabilityZone: "us-east-1a"}, "token")
			if err == nil {
				t.Error("expected error")
			}
			if !strings.Contains(err.Error(), "invalid size") {
				t.Errorf("expected 'invalid size' error, got: %v", err)
			}
		})
	}
}

func TestAWSStorageProvisioner_CreateEBSVolume_IOPSValidation(t *testing.T) {
	tests := []struct {
		name    string
		iops    int64
		wantErr bool
	}{
		{"exceeds max int32", 2147483648, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
				return nil, fmt.Errorf("should not reach AWS")
			})
			defer srv.Close()

			p := newAWSProvisionerWithMockClients(srv)

			spec := BlockVolumeSpec{AvailabilityZone: "us-east-1a", IOPS: tt.iops}
			_, err := p.createEBSVolume(context.Background(), "us-east-1", "gp3", 20, spec, "token")
			if err == nil {
				t.Error("expected error")
			}
			if !strings.Contains(err.Error(), "invalid IOPS") {
				t.Errorf("expected 'invalid IOPS' error, got: %v", err)
			}
		})
	}
}

func TestAWSStorageProvisioner_CreateEBSVolume_ThroughputValidation(t *testing.T) {
	tests := []struct {
		name       string
		throughput int64
		wantErr    bool
	}{
		{"exceeds max int32", 2147483648, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
				return nil, fmt.Errorf("should not reach AWS")
			})
			defer srv.Close()

			p := newAWSProvisionerWithMockClients(srv)

			spec := BlockVolumeSpec{AvailabilityZone: "us-east-1a", Throughput: tt.throughput}
			_, err := p.createEBSVolume(context.Background(), "us-east-1", "gp3", 20, spec, "token")
			if err == nil {
				t.Error("expected error")
			}
			if !strings.Contains(err.Error(), "invalid Throughput") {
				t.Errorf("expected 'invalid Throughput' error, got: %v", err)
			}
		})
	}
}

func TestAWSStorageProvisioner_ProvisionBlockVolume_MissingRegion(t *testing.T) {
	p := NewAWSStorageProvisioner()

	cfg := ExternalProviderConfig{
		Region: "",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	_, err := p.ProvisionBlockVolume(context.Background(), cfg, BlockVolumeSpec{AvailabilityZone: "us-east-1a"})
	if err == nil {
		t.Error("expected error for missing region")
	}
	if !strings.Contains(err.Error(), "region is required") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestAWSStorageProvisioner_InitClients_SkipsReinitWhenClientsExist(t *testing.T) {
	// This test verifies the guard clause in initClients
	p := &AWSStorageProvisioner{
		ec2Client: &ec2.Client{},
		s3Client:  &s3.Client{},
	}

	// initClients should return early without error
	err := p.initClients(context.Background(), ExternalProviderConfig{})
	if err != nil {
		t.Errorf("initClients should skip re-init when clients exist, got error: %v", err)
	}
}

func TestAWSStorageProvisioner_DeprovisionBlockVolume_Error(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		r.ParseForm()
		action := r.FormValue("Action")
		if action == "DeleteVolume" {
			// Return a non-NotFound error
			return nil, fmt.Errorf("VolumeInUse: The specified volume is currently attached to an instance")
		}
		return nil, fmt.Errorf("unknown action")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	err := p.DeprovisionBlockVolume(context.Background(), cfg, "vol-in-use")
	if err == nil {
		t.Error("expected error for in-use volume")
	}
	if !strings.Contains(err.Error(), "DeleteVolume") {
		t.Errorf("expected DeleteVolume error, got: %v", err)
	}
}

func TestAWSStorageProvisioner_DeprovisionObjectBucket_Error(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		method := r.Method
		path := r.URL.Path

		if method == "DELETE" && path == "/not-empty-bucket" {
			// Return a non-NotFound error
			return nil, fmt.Errorf("BucketNotEmpty: The bucket you tried to delete is not empty")
		}
		return nil, fmt.Errorf("unexpected request")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	err := p.DeprovisionObjectBucket(context.Background(), cfg, "not-empty-bucket")
	if err == nil {
		t.Error("expected error for non-empty bucket")
	}
	if !strings.Contains(err.Error(), "DeleteBucket") {
		t.Errorf("expected DeleteBucket error, got: %v", err)
	}
}

func TestAWSStorageProvisioner_CreateEBSVolume_WithIOPS(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		r.ParseForm()
		action := r.FormValue("Action")
		if action == "CreateVolume" {
			iopsStr := r.FormValue("Iops")
			if iopsStr == "" {
				t.Error("IOPS should be present when creating gp3 volume with IOPS specified")
			}
			return EC2CreateVolumeResponse{
				VolumeId:         "vol-with-iops",
				Size:             100,
				AvailabilityZone: "us-east-1a",
				Status:           "creating",
				VolumeType:       "gp3",
				CreateTime:       "2026-08-08T00:00:00.000Z",
				ClientToken:      r.FormValue("ClientToken"),
				RequestId:        "req-123",
			}, nil
		}
		return nil, fmt.Errorf("unknown action")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	spec := BlockVolumeSpec{
		SizeGB:           100,
		IOPS:             3000,
		AvailabilityZone: "us-east-1a",
	}

	info, err := p.ProvisionBlockVolume(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("ProvisionBlockVolume() error = %v", err)
	}

	if info.VolumeID != "vol-with-iops" {
		t.Errorf("expected vol-with-iops, got %s", info.VolumeID)
	}
}

func TestAWSStorageProvisioner_CreateEBSVolume_WithThroughput(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		r.ParseForm()
		action := r.FormValue("Action")
		if action == "CreateVolume" {
			throughputStr := r.FormValue("Throughput")
			if throughputStr == "" {
				t.Error("Throughput should be present when creating gp3 volume with throughput specified")
			}
			return EC2CreateVolumeResponse{
				VolumeId:         "vol-with-throughput",
				Size:             50,
				AvailabilityZone: "us-east-1a",
				Status:           "creating",
				VolumeType:       "gp3",
				CreateTime:       "2026-08-08T00:00:00.000Z",
				ClientToken:      r.FormValue("ClientToken"),
				RequestId:        "req-123",
			}, nil
		}
		return nil, fmt.Errorf("unknown action")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	spec := BlockVolumeSpec{
		SizeGB:           50,
		Throughput:       250,
		AvailabilityZone: "us-east-1a",
	}

	info, err := p.ProvisionBlockVolume(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("ProvisionBlockVolume() error = %v", err)
	}

	if info.VolumeID != "vol-with-throughput" {
		t.Errorf("expected vol-with-throughput, got %s", info.VolumeID)
	}
}

func TestAWSStorageProvisioner_CreateEBSVolume_WithEncryption(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		r.ParseForm()
		action := r.FormValue("Action")
		if action == "CreateVolume" {
			encrypted := r.FormValue("Encrypted")
			if encrypted == "" {
				t.Error("Encrypted flag should be present")
			}
			kmsKey := r.FormValue("KmsKeyId")
			if kmsKey == "" {
				t.Error("KmsKeyId should be present when encryption is enabled")
			}
			return EC2CreateVolumeResponse{
				VolumeId:         "vol-encrypted",
				Size:             50,
				AvailabilityZone: "us-east-1a",
				Status:           "creating",
				VolumeType:       "gp3",
				CreateTime:       "2026-08-08T00:00:00.000Z",
				ClientToken:      r.FormValue("ClientToken"),
				RequestId:        "req-123",
			}, nil
		}
		return nil, fmt.Errorf("unknown action")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	spec := BlockVolumeSpec{
		SizeGB:           50,
		AvailabilityZone: "us-east-1a",
		EncryptionKeyID:  "arn:aws:kms:us-east-1:111122223333:key/1234abcd-12ab-34cd-56ef-1234567890ab",
	}

	info, err := p.ProvisionBlockVolume(context.Background(), cfg, spec)
	if err != nil {
		t.Fatalf("ProvisionBlockVolume() error = %v", err)
	}

	if info.VolumeID != "vol-encrypted" {
		t.Errorf("expected vol-encrypted, got %s", info.VolumeID)
	}
}

func TestAWSStorageProvisioner_CreateS3Bucket_LocationConstraint(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		method := r.Method
		path := r.URL.Path

		if method == "PUT" && path == "/eu-bucket" {
			body, _ := io.ReadAll(r.Body)
			bodyStr := string(body)
			// When creating bucket in non-us-east-1, location constraint should be in body
			if !strings.Contains(bodyStr, "eu-west-1") {
				t.Error("expected LocationConstraint in request for non-us-east-1 region")
			}
			return nil, nil
		}
		return nil, fmt.Errorf("unexpected request")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	// Directly test createS3Bucket with a non-us-east-1 region
	err := p.createS3Bucket(context.Background(), "eu-west-1", "eu-bucket")
	if err != nil {
		t.Fatalf("createS3Bucket() error = %v", err)
	}
}

func TestAWSStorageProvisioner_PutBucketVersioning_Error(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		if strings.Contains(r.URL.Path, "versioning") {
			return nil, fmt.Errorf("S3 PutBucketVersioning failed")
		}
		return nil, fmt.Errorf("unexpected request")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	err := p.putBucketVersioning(context.Background(), "us-east-1", "test-bucket")
	if err == nil {
		t.Error("expected error for putBucketVersioning failure")
	}
	if !strings.Contains(err.Error(), "PutBucketVersioning") {
		t.Errorf("expected PutBucketVersioning error message, got: %v", err)
	}
}

func TestAWSStorageProvisioner_PutBucketEncryption_Error(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		if strings.Contains(r.URL.RawQuery, "encryption") || r.Method == "PUT" && strings.Contains(r.URL.Path, "encryption") {
			return nil, fmt.Errorf("S3 PutBucketEncryption failed")
		}
		return nil, fmt.Errorf("unexpected request")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	err := p.putBucketEncryption(context.Background(), "us-east-1", "test-bucket", "AES256", "")
	if err == nil {
		t.Error("expected error for putBucketEncryption failure")
	}
	if !strings.Contains(err.Error(), "PutBucketEncryption") {
		t.Errorf("expected PutBucketEncryption error message, got: %v", err)
	}
}

func TestAWSStorageProvisioner_PutPublicAccessBlock_Error(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		if strings.Contains(r.URL.RawQuery, "publicAccessBlock") || r.Method == "PUT" && strings.Contains(r.URL.Path, "publicAccessBlock") {
			return nil, fmt.Errorf("S3 PutPublicAccessBlock failed")
		}
		return nil, fmt.Errorf("unexpected request")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	err := p.putPublicAccessBlock(context.Background(), "us-east-1", "test-bucket")
	if err == nil {
		t.Error("expected error for putPublicAccessBlock failure")
	}
	if !strings.Contains(err.Error(), "PutPublicAccessBlock") {
		t.Errorf("expected PutPublicAccessBlock error message, got: %v", err)
	}
}

func TestAWSStorageProvisioner_GetBlockVolumeStatus_DescribeError(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		r.ParseForm()
		action := r.FormValue("Action")
		if action == "DescribeVolumes" {
			return nil, fmt.Errorf("EC2 DescribeVolumes failed")
		}
		return nil, fmt.Errorf("unknown action")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	_, err := p.GetBlockVolumeStatus(context.Background(), cfg, "vol-123")
	if err == nil {
		t.Error("expected error for DescribeVolumes failure")
	}
	if !strings.Contains(err.Error(), "DescribeVolumes") {
		t.Errorf("expected DescribeVolumes error message, got: %v", err)
	}
}

func TestAWSStorageProvisioner_InitClients_MissingSecretAccessKey(t *testing.T) {
	p := &AWSStorageProvisioner{}

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id": []byte("test"),
		},
	}

	err := p.initClients(context.Background(), cfg)
	if err == nil {
		t.Error("expected error for missing secret_access_key")
	}
	if !strings.Contains(err.Error(), "secret_access_key") {
		t.Errorf("expected secret_access_key error message, got: %v", err)
	}
}

func TestAWSStorageProvisioner_CreateS3Bucket_Error(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		if r.Method == "PUT" && r.URL.Path == "/test-bucket" {
			return nil, fmt.Errorf("S3 CreateBucket failed")
		}
		return nil, fmt.Errorf("unexpected request")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	err := p.createS3Bucket(context.Background(), "us-east-1", "test-bucket")
	if err == nil {
		t.Error("expected error for createS3Bucket failure")
	}
	if !strings.Contains(err.Error(), "CreateBucket") {
		t.Errorf("expected CreateBucket error message, got: %v", err)
	}
}

func TestAWSStorageProvisioner_CreateEBSVolume_Error(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		r.ParseForm()
		action := r.FormValue("Action")
		if action == "CreateVolume" {
			return nil, fmt.Errorf("EC2 CreateVolume failed")
		}
		return nil, fmt.Errorf("unknown action")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	spec := BlockVolumeSpec{
		VolumeType:       "gp3",
		SizeGB:           20,
		IOPS:             3000,
		Throughput:       125,
		EncryptionKeyID:  "",
		AvailabilityZone: "us-east-1a",
	}

	_, err := p.createEBSVolume(context.Background(), "us-east-1", "gp3", 20, spec, "idempotency-token-1")
	if err == nil {
		t.Error("expected error for createEBSVolume failure")
	}
	if !strings.Contains(err.Error(), "CreateVolume") {
		t.Errorf("expected CreateVolume error message, got: %v", err)
	}
}

func TestAWSStorageProvisioner_ProvisionObjectBucket_CreateBucketError(t *testing.T) {
	srv := newMockAWSServer(func(r *http.Request) (interface{}, error) {
		if r.Method == "PUT" && r.URL.Path == "/test-bucket" {
			return nil, fmt.Errorf("S3 CreateBucket failed")
		}
		return nil, fmt.Errorf("unexpected request")
	})
	defer srv.Close()

	p := newAWSProvisionerWithMockClients(srv)

	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	spec := ObjectBucketSpec{
		BucketName:        "test-bucket",
		PublicAccessBlock: true,
		Versioning:        false,
		EncryptionType:    "AES256",
	}

	_, err := p.ProvisionObjectBucket(context.Background(), cfg, spec)
	if err == nil {
		t.Error("expected error for create bucket failure")
	}
	if !strings.Contains(err.Error(), "CreateBucket") {
		t.Errorf("expected CreateBucket error message, got: %v", err)
	}
}

func TestAWSStorageProvisioner_ProvisionBlockVolume_RegionRequired(t *testing.T) {
	p := &AWSStorageProvisioner{}
	// Pre-inject clients to avoid calling initClients
	p.ec2Client = &ec2.Client{}
	p.s3Client = &s3.Client{}

	cfg := ExternalProviderConfig{
		Region: "",
		CredentialData: map[string][]byte{
			"access_key_id":     []byte("test"),
			"secret_access_key": []byte("test"),
		},
	}

	spec := BlockVolumeSpec{
		VolumeType:       "gp3",
		SizeGB:           20,
		IOPS:             3000,
		Throughput:       125,
		EncryptionKeyID:  "",
		AvailabilityZone: "",
	}

	_, err := p.ProvisionBlockVolume(context.Background(), cfg, spec)
	if err == nil {
		t.Error("expected error for missing region")
	}
	if !strings.Contains(err.Error(), "region is required") {
		t.Errorf("expected region required error message, got: %v", err)
	}
}

func TestAWSStorageProvisioner_ImplementsInterface(t *testing.T) {
	var _ StorageProvisioner = (*AWSStorageProvisioner)(nil)
}

// The following tests cover each exported method's initClients error-propagation
// branch. Omitting secret_access_key while setting access_key_id makes initClients
// fail its own validation before any network call, so no mock server is needed —
// a fresh (nil-client) provisioner is required so initClients actually runs.

func TestAWSStorageProvisioner_ProvisionBlockVolume_InitClientsError(t *testing.T) {
	p := NewAWSStorageProvisioner()
	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id": []byte("test"),
		},
	}
	_, err := p.ProvisionBlockVolume(context.Background(), cfg, BlockVolumeSpec{})
	if err == nil {
		t.Fatal("expected error from initClients")
	}
	if !strings.Contains(err.Error(), "secret_access_key") {
		t.Errorf("expected secret_access_key error message, got: %v", err)
	}
}

func TestAWSStorageProvisioner_DeprovisionBlockVolume_InitClientsError(t *testing.T) {
	p := NewAWSStorageProvisioner()
	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id": []byte("test"),
		},
	}
	err := p.DeprovisionBlockVolume(context.Background(), cfg, "vol-123")
	if err == nil {
		t.Fatal("expected error from initClients")
	}
	if !strings.Contains(err.Error(), "secret_access_key") {
		t.Errorf("expected secret_access_key error message, got: %v", err)
	}
}

func TestAWSStorageProvisioner_GetBlockVolumeStatus_InitClientsError(t *testing.T) {
	p := NewAWSStorageProvisioner()
	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id": []byte("test"),
		},
	}
	_, err := p.GetBlockVolumeStatus(context.Background(), cfg, "vol-123")
	if err == nil {
		t.Fatal("expected error from initClients")
	}
	if !strings.Contains(err.Error(), "secret_access_key") {
		t.Errorf("expected secret_access_key error message, got: %v", err)
	}
}

func TestAWSStorageProvisioner_ProvisionObjectBucket_InitClientsError(t *testing.T) {
	p := NewAWSStorageProvisioner()
	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id": []byte("test"),
		},
	}
	_, err := p.ProvisionObjectBucket(context.Background(), cfg, ObjectBucketSpec{BucketName: "test-bucket"})
	if err == nil {
		t.Fatal("expected error from initClients")
	}
	if !strings.Contains(err.Error(), "secret_access_key") {
		t.Errorf("expected secret_access_key error message, got: %v", err)
	}
}

func TestAWSStorageProvisioner_DeprovisionObjectBucket_InitClientsError(t *testing.T) {
	p := NewAWSStorageProvisioner()
	cfg := ExternalProviderConfig{
		Region: "us-east-1",
		CredentialData: map[string][]byte{
			"access_key_id": []byte("test"),
		},
	}
	err := p.DeprovisionObjectBucket(context.Background(), cfg, "test-bucket")
	if err == nil {
		t.Fatal("expected error from initClients")
	}
	if !strings.Contains(err.Error(), "secret_access_key") {
		t.Errorf("expected secret_access_key error message, got: %v", err)
	}
}
