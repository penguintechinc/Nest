//go:build integration

package provider

import (
	"context"
	"net/http"
	"os"
	"testing"
)

// Integration test for S3-compatible object storage against a real S3 server.
//
// Unit tests assert that a SigV4 signature is produced and well-formed, but
// only a real S3 implementation can confirm the signature is actually
// ACCEPTED — which is the whole question, since the previous implementation
// sent unsigned requests that every real provider rejects with 403.
//
// MinIO enforces SigV4 exactly as S3 does, so it validates the signer without
// needing a cloud account or incurring cost.
//
//	docker run -d --name nest-sigv4-minio -p 19000:9000 \
//	  -e MINIO_ROOT_USER=nesttestkey -e MINIO_ROOT_PASSWORD=nesttestsecret123 \
//	  minio/minio:RELEASE.2025-04-22T22-12-26Z server /data
//
//	go test -tags=integration ./pkg/provider/ -run TestIntegrationS3Compat
//
// Override the defaults with S3_TEST_ENDPOINT / S3_TEST_ACCESS_KEY /
// S3_TEST_SECRET_KEY to point at DigitalOcean Spaces, Vultr, Linode, or any
// other S3-compatible endpoint.
func s3TestConfig(t *testing.T) ExternalProviderConfig {
	t.Helper()

	endpoint := os.Getenv("S3_TEST_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://127.0.0.1:19000"
	}
	accessKey := os.Getenv("S3_TEST_ACCESS_KEY")
	if accessKey == "" {
		accessKey = "nesttestkey"
	}
	secretKey := os.Getenv("S3_TEST_SECRET_KEY")
	if secretKey == "" {
		secretKey = "nesttestsecret123"
	}

	return ExternalProviderConfig{
		Provider: "s3-compat",
		Endpoint: endpoint,
		Extra: map[string]string{
			"access_key": accessKey,
			"secret_key": secretKey,
		},
	}
}

// TestIntegrationS3CompatSignatureAccepted is the end-to-end proof that the
// SigV4 signature this package produces is accepted by a real S3 server.
func TestIntegrationS3CompatSignatureAccepted(t *testing.T) {
	ctx := context.Background()
	cfg := s3TestConfig(t)
	p := NewS3CompatProvisioner()

	const bucket = "nest-sigv4-integration"

	// Clean up first in case a previous run left the bucket behind.
	_ = p.DeprovisionObjectBucket(ctx, cfg, bucket)

	info, err := p.ProvisionObjectBucket(ctx, cfg, ObjectBucketSpec{BucketName: bucket})
	if err != nil {
		t.Fatalf("ProvisionObjectBucket against a real S3 server failed — the signature "+
			"was rejected: %v", err)
	}
	if info.BucketName != bucket {
		t.Errorf("BucketName = %q, want %q", info.BucketName, bucket)
	}

	t.Cleanup(func() {
		if err := p.DeprovisionObjectBucket(ctx, cfg, bucket); err != nil {
			t.Errorf("cleanup: failed to delete bucket %s: %v", bucket, err)
		}
	})

	// Independently confirm the bucket really exists, rather than trusting a
	// 200 that might have come from a misrouted request.
	req, err := http.NewRequestWithContext(ctx, http.MethodHead, cfg.Endpoint+"/"+bucket, nil)
	if err != nil {
		t.Fatalf("build verification request: %v", err)
	}
	if err := signS3Request(req, cfg, "S3-compatible", "us-east-1", nil); err != nil {
		t.Fatalf("sign verification request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("verification request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("HEAD bucket after create returned %d, want 200", resp.StatusCode)
	}
}

// An unsigned request must be rejected. This pins the behavior the fix exists
// for: before signing was implemented, every request looked like this one.
func TestIntegrationUnsignedRequestIsRejected(t *testing.T) {
	cfg := s3TestConfig(t)

	req, err := http.NewRequest(http.MethodPut, cfg.Endpoint+"/nest-unsigned-probe", nil)
	if err != nil {
		t.Fatalf("build request: %v", err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusCreated {
		t.Fatalf("server accepted an UNSIGNED bucket create (%d) — it is not enforcing "+
			"SigV4, so this test proves nothing about the signer", resp.StatusCode)
	}
	if resp.StatusCode != http.StatusForbidden {
		t.Logf("unsigned request rejected with %d (expected 403)", resp.StatusCode)
	}
}

// Wrong credentials must be rejected, proving the server actually validates
// the signature rather than merely checking that a header is present.
func TestIntegrationBadCredentialsRejected(t *testing.T) {
	ctx := context.Background()
	cfg := s3TestConfig(t)
	cfg.Extra = map[string]string{
		"access_key": "nesttestkey",
		"secret_key": "deliberately-wrong-secret",
	}

	p := NewS3CompatProvisioner()
	_, err := p.ProvisionObjectBucket(ctx, cfg, ObjectBucketSpec{BucketName: "nest-badcreds-probe"})
	if err == nil {
		t.Fatal("server accepted a signature computed with the wrong secret key — " +
			"signature validation is not being enforced")
	}
}
