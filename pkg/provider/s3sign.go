package provider

import (
	"fmt"
	"net/http"

	"github.com/penguintechinc/nest/pkg/sigv4"
)

// S3-compatible object storage (DigitalOcean Spaces, Vultr Object Storage,
// Linode Object Storage, MinIO, Ceph RGW) authenticates with AWS Signature
// Version 4 using an access key / secret key pair.
//
// Those keys are issued SEPARATELY from the provider's management API token.
// A DigitalOcean account has a `do_token` for the REST API (droplets, block
// volumes) and a distinct Spaces access key/secret generated under the Spaces
// section; Vultr and Linode work the same way. Signing object-storage requests
// with the management token, or omitting credentials entirely, yields 403.

// s3Credentials returns the S3-compatible access key and secret from cfg.
//
// These are read from the credential Secret keys "access_key" and
// "secret_key" — deliberately not the provider's management API token, which
// is not valid for the object-storage endpoint.
func s3Credentials(cfg ExternalProviderConfig, provider string) (accessKey, secretKey string, err error) {
	accessKey, _ = cfg.Credential("access_key")
	secretKey, _ = cfg.Credential("secret_key")

	if accessKey == "" || secretKey == "" {
		return "", "", fmt.Errorf(
			"%s requires S3 credentials: set \"access_key\" and \"secret_key\" in the "+
				"referenced credential Secret. These are issued separately from the "+
				"management API token and are not interchangeable with it", provider)
	}
	return accessKey, secretKey, nil
}

// s3SigningRegion resolves the region used in the SigV4 credential scope.
//
// Region is optional in these providers' configuration — the endpoint helpers
// fall back to a default host, so signing falls back to the matching default
// region to stay consistent with the URL actually being called. An explicit
// override is available via extra["signing_region"] for deployments whose
// signing region differs from the endpoint label.
func s3SigningRegion(cfg ExternalProviderConfig, defaultRegion string) string {
	if v, ok := cfg.Extra["signing_region"]; ok && v != "" {
		return v
	}
	if cfg.Region != "" {
		return cfg.Region
	}
	return defaultRegion
}

// signS3Request signs req in place for an S3-compatible endpoint.
//
// body must be the exact bytes being sent (nil for an empty body); SigV4
// includes a hash of the payload, so a mismatch produces a signature the
// provider rejects.
func signS3Request(req *http.Request, cfg ExternalProviderConfig, provider, defaultRegion string, body []byte) error {
	accessKey, secretKey, err := s3Credentials(cfg, provider)
	if err != nil {
		return err
	}
	region := s3SigningRegion(cfg, defaultRegion)
	if err := sigv4.Sign(req, "s3", region, accessKey, secretKey, "", body); err != nil {
		return fmt.Errorf("sign %s object storage request: %w", provider, err)
	}
	return nil
}
