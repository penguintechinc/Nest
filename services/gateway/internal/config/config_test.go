package config

import (
	"os"
	"testing"
)

func TestFromEnv(t *testing.T) {
	tests := []struct {
		name     string
		env      map[string]string
		expected Config
	}{
		{
			name: "default values",
			env:  map[string]string{},
			expected: Config{
				OIDCIssuer:   "http://nest-api.nest.svc.cluster.local:8080",
				OIDCAudience: "nest",
				OIDCJwksURL:  "http://nest-api.nest.svc.cluster.local:8080/.well-known/jwks.json",
				Namespace:    "nest",
				APIEndpoint:  "http://nest-api.nest.svc.cluster.local:8080",
			},
		},
		{
			name: "custom values",
			env: map[string]string{
				"OIDC_ISSUER":       "http://custom-issuer:8080",
				"OIDC_AUDIENCE":     "custom-audience",
				"OIDC_JWKS_URL":     "http://custom-issuer:8080/.well-known/jwks.json",
				"NAMESPACE":         "custom-ns",
				"NEST_API_ENDPOINT": "http://custom-api:8080",
			},
			expected: Config{
				OIDCIssuer:   "http://custom-issuer:8080",
				OIDCAudience: "custom-audience",
				OIDCJwksURL:  "http://custom-issuer:8080/.well-known/jwks.json",
				Namespace:    "custom-ns",
				APIEndpoint:  "http://custom-api:8080",
			},
		},
		{
			name: "partial override",
			env: map[string]string{
				"OIDC_ISSUER": "http://custom-issuer:8080",
			},
			expected: Config{
				OIDCIssuer:   "http://custom-issuer:8080",
				OIDCAudience: "nest",
				OIDCJwksURL:  "http://nest-api.nest.svc.cluster.local:8080/.well-known/jwks.json",
				Namespace:    "nest",
				APIEndpoint:  "http://nest-api.nest.svc.cluster.local:8080",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Save and clear all config env vars
			allKeys := []string{"OIDC_ISSUER", "OIDC_AUDIENCE", "OIDC_JWKS_URL", "NAMESPACE", "NEST_API_ENDPOINT"}
			saved := map[string]string{}
			for _, key := range allKeys {
				saved[key] = os.Getenv(key)
				os.Unsetenv(key)
			}
			defer func() {
				for key, val := range saved {
					if val != "" {
						os.Setenv(key, val)
					} else {
						os.Unsetenv(key)
					}
				}
			}()

			// Set test env
			for key, val := range tt.env {
				os.Setenv(key, val)
			}

			got := FromEnv()
			if got != tt.expected {
				t.Errorf("FromEnv() = %+v, want %+v", got, tt.expected)
			}
		})
	}
}

func TestGetEnv(t *testing.T) {
	tests := []struct {
		name      string
		key       string
		value     string
		fallback  string
		expected  string
	}{
		{
			name:     "returns env value",
			key:      "TEST_KEY_EXISTS",
			value:    "test-value",
			fallback: "fallback",
			expected: "test-value",
		},
		{
			name:     "returns fallback when not set",
			key:      "TEST_KEY_NOT_EXISTS",
			value:    "",
			fallback: "fallback",
			expected: "fallback",
		},
		{
			name:     "returns env value over fallback",
			key:      "TEST_KEY_OVERRIDE",
			value:    "override",
			fallback: "fallback",
			expected: "override",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if tt.value != "" {
				os.Setenv(tt.key, tt.value)
				defer os.Unsetenv(tt.key)
			} else {
				os.Unsetenv(tt.key)
			}

			got := getEnv(tt.key, tt.fallback)
			if got != tt.expected {
				t.Errorf("getEnv(%q, %q) = %q, want %q", tt.key, tt.fallback, got, tt.expected)
			}
		})
	}
}
