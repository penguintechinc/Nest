package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// captureAuth spins up a server that records the Authorization header and the
// host the client actually addressed, then returns success.
func captureAuth(t *testing.T, status int) (*httptest.Server, *string, *string) {
	t.Helper()
	var auth, host string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		host = r.Host
		w.WriteHeader(status)
	}))
	t.Cleanup(srv.Close)
	return srv, &auth, &host
}

func s3Creds() map[string]string {
	return map[string]string{
		"access_key": "AKIATESTKEY",
		"secret_key": "test-secret-key",
	}
}

// Every S3-compatible provider must sign its bucket requests. Before this was
// implemented the requests went out bare and every real provider answered 403.
func TestObjectBucketRequestsAreSigned(t *testing.T) {
	cases := []struct {
		name string
		call func(t *testing.T, endpoint string, extra map[string]string) error
	}{
		{"digitalocean-provision", func(t *testing.T, ep string, ex map[string]string) error {
			p := NewDOStorageProvisioner()
			_, err := p.ProvisionObjectBucket(context.Background(),
				ExternalProviderConfig{Provider: "digitalocean", Endpoint: ep, Extra: ex},
				ObjectBucketSpec{BucketName: "b"})
			return err
		}},
		{"digitalocean-deprovision", func(t *testing.T, ep string, ex map[string]string) error {
			p := NewDOStorageProvisioner()
			return p.DeprovisionObjectBucket(context.Background(),
				ExternalProviderConfig{Provider: "digitalocean", Endpoint: ep, Extra: ex}, "b")
		}},
		{"vultr-provision", func(t *testing.T, ep string, ex map[string]string) error {
			p := NewVultrStorageProvisioner()
			_, err := p.ProvisionObjectBucket(context.Background(),
				ExternalProviderConfig{Provider: "vultr", Endpoint: ep, Extra: ex},
				ObjectBucketSpec{BucketName: "b"})
			return err
		}},
		{"vultr-deprovision", func(t *testing.T, ep string, ex map[string]string) error {
			p := NewVultrStorageProvisioner()
			return p.DeprovisionObjectBucket(context.Background(),
				ExternalProviderConfig{Provider: "vultr", Endpoint: ep, Extra: ex}, "b")
		}},
		{"linode-provision", func(t *testing.T, ep string, ex map[string]string) error {
			p := NewLinodeStorageProvisioner()
			_, err := p.ProvisionObjectBucket(context.Background(),
				ExternalProviderConfig{Provider: "linode", Endpoint: ep, Extra: ex},
				ObjectBucketSpec{BucketName: "b"})
			return err
		}},
		{"linode-deprovision", func(t *testing.T, ep string, ex map[string]string) error {
			p := NewLinodeStorageProvisioner()
			return p.DeprovisionObjectBucket(context.Background(),
				ExternalProviderConfig{Provider: "linode", Endpoint: ep, Extra: ex}, "b")
		}},
		{"s3compat-provision", func(t *testing.T, ep string, ex map[string]string) error {
			p := NewS3CompatProvisioner()
			_, err := p.ProvisionObjectBucket(context.Background(),
				ExternalProviderConfig{Provider: "s3-compat", Endpoint: ep, Extra: ex},
				ObjectBucketSpec{BucketName: "b"})
			return err
		}},
		{"s3compat-deprovision", func(t *testing.T, ep string, ex map[string]string) error {
			p := NewS3CompatProvisioner()
			return p.DeprovisionObjectBucket(context.Background(),
				ExternalProviderConfig{Provider: "s3-compat", Endpoint: ep, Extra: ex}, "b")
		}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv, auth, host := captureAuth(t, http.StatusOK)
			if err := tc.call(t, srv.URL, s3Creds()); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if *auth == "" {
				t.Fatal("request was sent with no Authorization header — provider would reject it with 403")
			}
			if !strings.HasPrefix(*auth, "AWS4-HMAC-SHA256 ") {
				t.Errorf("not a SigV4 signature: %q", *auth)
			}
			if !strings.Contains(*auth, "Credential=AKIATESTKEY/") {
				t.Errorf("signature does not use the S3 access key: %q", *auth)
			}
			if !strings.Contains(*auth, "/s3/aws4_request") {
				t.Errorf("credential scope is not for the s3 service: %q", *auth)
			}
			// host must be signed, and must match the host actually addressed.
			if !strings.Contains(*auth, "SignedHeaders=host;") {
				t.Errorf("host not signed: %q", *auth)
			}
			if *host == "" {
				t.Error("server saw no Host")
			}
		})
	}
}

// Missing S3 credentials must fail loudly and name the right keys, rather than
// sending an unsigned request that fails opaquely at the provider.
func TestObjectBucketRequiresS3Credentials(t *testing.T) {
	srv, _, _ := captureAuth(t, http.StatusOK)

	calls := map[string]func() error{
		"digitalocean": func() error {
			_, err := NewDOStorageProvisioner().ProvisionObjectBucket(context.Background(),
				ExternalProviderConfig{Endpoint: srv.URL}, ObjectBucketSpec{BucketName: "b"})
			return err
		},
		"vultr": func() error {
			_, err := NewVultrStorageProvisioner().ProvisionObjectBucket(context.Background(),
				ExternalProviderConfig{Endpoint: srv.URL}, ObjectBucketSpec{BucketName: "b"})
			return err
		},
		"linode": func() error {
			_, err := NewLinodeStorageProvisioner().ProvisionObjectBucket(context.Background(),
				ExternalProviderConfig{Endpoint: srv.URL}, ObjectBucketSpec{BucketName: "b"})
			return err
		},
		"s3-compat": func() error {
			_, err := NewS3CompatProvisioner().ProvisionObjectBucket(context.Background(),
				ExternalProviderConfig{Endpoint: srv.URL}, ObjectBucketSpec{BucketName: "b"})
			return err
		},
	}
	for name, call := range calls {
		t.Run(name, func(t *testing.T) {
			err := call()
			if err == nil {
				t.Fatal("expected an error when S3 credentials are absent")
			}
			if !strings.Contains(err.Error(), "access_key") || !strings.Contains(err.Error(), "secret_key") {
				t.Errorf("error should name the required keys, got: %v", err)
			}
			// The management API token must not be presented as a substitute.
			if strings.Contains(err.Error(), "do_token") {
				t.Errorf("error should not suggest the management token: %v", err)
			}
		})
	}
}

// The management API token is NOT valid for object storage — supplying only it
// must still fail. This is the distinction that made the original bug subtle.
func TestManagementTokenIsNotAcceptedForObjectStorage(t *testing.T) {
	srv, _, _ := captureAuth(t, http.StatusOK)

	_, err := NewDOStorageProvisioner().ProvisionObjectBucket(context.Background(),
		ExternalProviderConfig{
			Endpoint: srv.URL,
			Extra:    map[string]string{"do_token": "a-valid-management-token"},
		},
		ObjectBucketSpec{BucketName: "b"})
	if err == nil {
		t.Fatal("do_token alone must not authorize an object-storage request")
	}
	if !strings.Contains(err.Error(), "access_key") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestS3SigningRegion(t *testing.T) {
	cases := []struct {
		name string
		cfg  ExternalProviderConfig
		def  string
		want string
	}{
		{"explicit region", ExternalProviderConfig{Region: "ams3"}, "nyc3", "ams3"},
		{"falls back to default when region unset", ExternalProviderConfig{}, "nyc3", "nyc3"},
		{"signing_region overrides region", ExternalProviderConfig{
			Region: "ams3",
			Extra:  map[string]string{"signing_region": "us-east-1"},
		}, "nyc3", "us-east-1"},
		{"empty signing_region is ignored", ExternalProviderConfig{
			Region: "ams3",
			Extra:  map[string]string{"signing_region": ""},
		}, "nyc3", "ams3"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := s3SigningRegion(tc.cfg, tc.def); got != tc.want {
				t.Errorf("s3SigningRegion() = %q, want %q", got, tc.want)
			}
		})
	}
}

// Region is optional for these providers, so a bucket call with no region must
// still produce a valid signature using the provider's default region.
func TestSigningWorksWithNoRegionConfigured(t *testing.T) {
	srv, auth, _ := captureAuth(t, http.StatusOK)

	p := NewDOStorageProvisioner()
	_, err := p.ProvisionObjectBucket(context.Background(),
		ExternalProviderConfig{Endpoint: srv.URL, Extra: s3Creds()},
		ObjectBucketSpec{BucketName: "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(*auth, "/nyc3/s3/aws4_request") {
		t.Errorf("expected the nyc3 default signing region, got: %q", *auth)
	}
}
