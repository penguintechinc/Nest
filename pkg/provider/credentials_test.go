package provider

import (
	"context"
	"testing"
)

// Credential must prefer Secret-sourced data over the plaintext Extra map, so a
// credential placed in the referenced Kubernetes Secret always wins and operators
// are never forced to put secrets in the cleartext DataResource spec.
func TestExternalProviderConfig_Credential(t *testing.T) {
	tests := []struct {
		name    string
		cfg     ExternalProviderConfig
		key     string
		wantVal string
		wantOK  bool
	}{
		{
			name:    "from secret data",
			cfg:     ExternalProviderConfig{CredentialData: map[string][]byte{"do_token": []byte("secret-tok")}},
			key:     "do_token",
			wantVal: "secret-tok",
			wantOK:  true,
		},
		{
			name:    "falls back to extra",
			cfg:     ExternalProviderConfig{Extra: map[string]string{"do_token": "extra-tok"}},
			key:     "do_token",
			wantVal: "extra-tok",
			wantOK:  true,
		},
		{
			name: "secret wins over extra",
			cfg: ExternalProviderConfig{
				CredentialData: map[string][]byte{"do_token": []byte("secret-tok")},
				Extra:          map[string]string{"do_token": "extra-tok"},
			},
			key:     "do_token",
			wantVal: "secret-tok",
			wantOK:  true,
		},
		{
			name:    "missing everywhere",
			cfg:     ExternalProviderConfig{},
			key:     "do_token",
			wantVal: "",
			wantOK:  false,
		},
		{
			name: "empty secret value falls through as present-but-empty",
			cfg: ExternalProviderConfig{
				CredentialData: map[string][]byte{"do_token": []byte("")},
				Extra:          map[string]string{"do_token": "extra-tok"},
			},
			key:     "do_token",
			wantVal: "",
			wantOK:  true, // key exists in secret data, even if empty — provider decides how to treat empty
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := tt.cfg.Credential(tt.key)
			if got != tt.wantVal || ok != tt.wantOK {
				t.Errorf("Credential(%q) = (%q, %v), want (%q, %v)", tt.key, got, ok, tt.wantVal, tt.wantOK)
			}
		})
	}
}

// Each token-based provider must read its API token from Secret data, not only
// from Extra. These guard the wiring per provider.
func TestProviderTokens_ReadFromSecretData(t *testing.T) {
	cases := []struct {
		provider string
		key      string
		token    func(ExternalProviderConfig) (string, error)
	}{
		{"digitalocean", "do_token", NewDOStorageProvisioner().token},
		{"linode", "linode_token", NewLinodeStorageProvisioner().token},
		{"vultr", "vultr_api_key", NewVultrStorageProvisioner().token},
	}

	for _, c := range cases {
		t.Run(c.provider+"/from-secret", func(t *testing.T) {
			cfg := ExternalProviderConfig{CredentialData: map[string][]byte{c.key: []byte("tok-from-secret")}}
			got, err := c.token(cfg)
			if err != nil {
				t.Fatalf("token() error = %v", err)
			}
			if got != "tok-from-secret" {
				t.Errorf("token() = %q, want tok-from-secret", got)
			}
		})

		t.Run(c.provider+"/missing", func(t *testing.T) {
			if _, err := c.token(ExternalProviderConfig{}); err == nil {
				t.Error("expected error when token credential is absent")
			}
		})
	}
}

// AWS static credentials must be accepted from Secret data, and an access key with
// no matching secret key must be rejected rather than silently falling through to
// the default (possibly ambient) credential chain.
func TestAWSInitClients_StaticCredentials(t *testing.T) {
	ctx := context.Background()

	t.Run("secret data provides both keys", func(t *testing.T) {
		p := NewAWSStorageProvisioner()
		cfg := ExternalProviderConfig{
			Region: "us-west-2",
			CredentialData: map[string][]byte{
				"access_key_id":     []byte("AKIAEXAMPLE"),
				"secret_access_key": []byte("shhh"),
			},
		}
		if err := p.initClients(ctx, cfg); err != nil {
			t.Fatalf("initClients() error = %v", err)
		}
		if p.ec2Client == nil || p.s3Client == nil {
			t.Error("initClients did not construct EC2/S3 clients")
		}
	})

	t.Run("access key without secret key is rejected", func(t *testing.T) {
		p := NewAWSStorageProvisioner()
		cfg := ExternalProviderConfig{
			Region:         "us-west-2",
			CredentialData: map[string][]byte{"access_key_id": []byte("AKIAEXAMPLE")},
		}
		if err := p.initClients(ctx, cfg); err == nil {
			t.Error("expected error when secret_access_key is missing")
		}
	})
}
