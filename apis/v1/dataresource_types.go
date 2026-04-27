package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:printcolumn:name="Type",type=string,JSONPath=`.spec.type`
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

type DataResource struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DataResourceSpec   `json:"spec,omitempty"`
	Status            DataResourceStatus `json:"status,omitempty"`
}

type DataResourceSpec struct {
	// Type is the resource type: postgres, object, pvc/block, pvc/file, keyvalue, etc.
	Type string `json:"type"`
	// Class references a DataResourceClass name
	Class string `json:"class"`
	// Tenant is the tenant ID (mandatory)
	Tenant string `json:"tenant"`
	// Protocols lists enabled access protocols: native, grpc, rest
	Protocols []Protocol `json:"protocols,omitempty"`
	// Size describes capacity requirements
	Size *ResourceSize `json:"size,omitempty"`
	// TLS configures transport security
	TLS *TLSConfig `json:"tls,omitempty"`
	// SecretsBackend selects secrets storage
	SecretsBackend *SecretsBackendRef `json:"secretsBackend,omitempty"`
	// DataProtectionPolicy references a DataProtectionPolicy
	DataProtectionPolicy string `json:"dataProtectionPolicy,omitempty"`
	// Annotations are opaque key/value pairs (documented namespace only)
	Annotations map[string]string `json:"annotations,omitempty"`
	// Origination mode: managed, imported, external
	// +kubebuilder:default=managed
	Origination Origination `json:"origination,omitempty"`
	// HA enables high availability
	HA bool `json:"ha,omitempty"`
	// Replicas configuration
	Replicas *ReplicaConfig `json:"replicas,omitempty"`
	// Import holds connection details when origination=imported
	Import *ImportSpec `json:"import,omitempty"`
	// External holds cloud provider details when origination=external
	External *ExternalSpec `json:"external,omitempty"`
}

// +kubebuilder:validation:Enum=native;grpc;rest
type Protocol string

const (
	ProtocolNative Protocol = "native"
	ProtocolGRPC   Protocol = "grpc"
	ProtocolREST   Protocol = "rest"
)

// +kubebuilder:validation:Enum=managed;imported;external
type Origination string

const (
	OriginationManaged  Origination = "managed"
	OriginationImported Origination = "imported"
	OriginationExternal Origination = "external"
)

type ResourceSize struct {
	// Storage capacity (e.g. "200Gi")
	Storage string `json:"storage,omitempty"`
	// IOPS target
	IOPS int64 `json:"iops,omitempty"`
}

type TLSConfig struct {
	// +kubebuilder:validation:Enum=required;preferred;disabled
	// +kubebuilder:default=required
	Mode string `json:"mode,omitempty"`
	// +kubebuilder:default="1.3"
	MinVersion string `json:"minVersion,omitempty"`
	// +kubebuilder:validation:Enum=none;optional;required
	ClientAuth string `json:"clientAuth,omitempty"`
	// +kubebuilder:validation:Enum=nest-ca;cert-manager;byo
	CertSource string `json:"certSource,omitempty"`
}

type SecretsBackendRef struct {
	// +kubebuilder:validation:Enum=nest-envelope;vault;infisical;aws-sm;gcp-sm;azure-kv;bitwarden
	Kind string `json:"kind"`
	Ref  string `json:"ref,omitempty"`
}

type ReplicaConfig struct {
	Write *ReplicaCountSpec `json:"write,omitempty"`
	Read  *ReplicaCountSpec `json:"read,omitempty"`
}

type ReplicaCountSpec struct {
	Min     int32 `json:"min,omitempty"`
	Max     int32 `json:"max,omitempty"`
	Default int32 `json:"default,omitempty"`
	Count   int32 `json:"count,omitempty"`
}

// ImportSpec holds connection details for imported (pre-existing) resources.
type ImportSpec struct {
	// ConnectionString is the engine connection string (password extracted and stored via SAL)
	ConnectionString string `json:"connectionString,omitempty"`
	// TLSMode: verify-full, verify-ca, require, disable
	// +kubebuilder:default=verify-full
	TLSMode string `json:"tlsMode,omitempty"`
	// CredentialSecret is the K8s Secret holding credentials (absorbed into SAL on import)
	CredentialSecret string `json:"credentialSecret,omitempty"`
	// ManagedCredentials: allow Nest to rotate credentials on the engine
	ManagedCredentials bool `json:"managedCredentials,omitempty"`
	// ManagedFailover: allow Nest to perform failover on this imported engine
	ManagedFailover bool `json:"managedFailover,omitempty"`
}

// ExternalSpec holds details for cloud-managed external resources.
type ExternalSpec struct {
	// Provider: aws, gcp, azure, vultr, cloudflare, or any Tier 2 standard-protocol provider
	Provider string `json:"provider"`
	// Region is the cloud provider region (Tier 1)
	Region string `json:"region,omitempty"`
	// ResourceID is the cloud resource ARN/ID/self-link
	ResourceID string `json:"resourceId,omitempty"`
	// CredentialSecret holds cloud provider credentials
	CredentialSecret string `json:"credentialSecret,omitempty"`
	// CostTagKey is the billing tag key used by the provider for cost attribution
	CostTagKey string `json:"costTagKey,omitempty"`
	// Endpoint overrides resource endpoint (Tier 2 standard-protocol)
	Endpoint string `json:"endpoint,omitempty"`
	// EngineType for Tier 2: postgres, mysql, redis, kafka, s3
	EngineType string `json:"engineType,omitempty"`
}

// +kubebuilder:validation:Enum=Unknown;Pending;Provisioning;Ready;Degraded;Failed;Deleting
type DataResourcePhase string

const (
	PhaseUnknown      DataResourcePhase = "Unknown"
	PhasePending      DataResourcePhase = "Pending"
	PhaseProvisioning DataResourcePhase = "Provisioning"
	PhaseReady        DataResourcePhase = "Ready"
	PhaseDegraded     DataResourcePhase = "Degraded"
	PhaseFailed       DataResourcePhase = "Failed"
	PhaseDeleting     DataResourcePhase = "Deleting"
)

type DataResourceStatus struct {
	Phase            DataResourcePhase `json:"phase,omitempty"`
	Conditions       []metav1.Condition `json:"conditions,omitempty"`
	Endpoints        *ResourceEndpoints `json:"endpoints,omitempty"`
	Health           *HealthSignal      `json:"health,omitempty"`
	CurrentOperation string             `json:"currentOperation,omitempty"`
	ProvisionedAt    *metav1.Time       `json:"provisionedAt,omitempty"`
}

type ResourceEndpoints struct {
	// Native wire-protocol endpoint (e.g., postgres://host:5432/db)
	Native string `json:"native,omitempty"`
	// GRPC endpoint
	GRPC string `json:"grpc,omitempty"`
	// REST endpoint
	REST string `json:"rest,omitempty"`
}

// +kubebuilder:validation:Enum=healthy;degraded;down
type HealthState string

const (
	HealthHealthy  HealthState = "healthy"
	HealthDegraded HealthState = "degraded"
	HealthDown     HealthState = "down"
)

type HealthSignal struct {
	State   HealthState `json:"state"`
	Message string      `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
type DataResourceList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataResource `json:"items"`
}
