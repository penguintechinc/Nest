package config

import (
	"os"

	"sigs.k8s.io/controller-runtime/pkg/client"
)

type Config struct {
	// OIDC
	OIDCIssuer   string
	OIDCAudience string
	OIDCJwksURL  string

	// K8s namespace for endpoint discovery
	Namespace string

	// Internal service endpoints (override for testing)
	APIEndpoint string

	// K8sClient is the controller-runtime client for CRD operations.
	// Nil if K8s is unavailable (local dev without kubeconfig).
	K8sClient client.Client
}

func FromEnv() Config {
	return Config{
		OIDCIssuer:   getEnv("OIDC_ISSUER", "http://nest-api.nest.svc.cluster.local:8080"),
		OIDCAudience: getEnv("OIDC_AUDIENCE", "nest"),
		OIDCJwksURL:  getEnv("OIDC_JWKS_URL", "http://nest-api.nest.svc.cluster.local:8080/.well-known/jwks.json"),
		Namespace:    getEnv("NAMESPACE", "nest"),
		APIEndpoint:  getEnv("NEST_API_ENDPOINT", "http://nest-api.nest.svc.cluster.local:8080"),
	}
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
