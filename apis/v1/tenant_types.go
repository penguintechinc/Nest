package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

type Tenant struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              TenantSpec   `json:"spec,omitempty"`
	Status            TenantStatus `json:"status,omitempty"`
}

type TenantSpec struct {
	// DisplayName is the human-readable tenant name
	DisplayName string `json:"displayName"`
	// Quota defines resource limits for this tenant
	Quota *QuotaSpec `json:"quota,omitempty"`
	// DefaultSecretsBackend for new resources
	// +kubebuilder:default=nest-envelope
	DefaultSecretsBackend string `json:"defaultSecretsBackend,omitempty"`
	// EncryptionKEKRef references the tenant's key encryption key
	EncryptionKEKRef string `json:"encryptionKekRef,omitempty"`
	// ConfigSource: api or gitops
	// +kubebuilder:default=api
	ConfigSource string `json:"configSource,omitempty"`
	// LicenseTier: free, pro, enterprise
	// +kubebuilder:default=free
	LicenseTier string `json:"licenseTier,omitempty"`
}

type QuotaSpec struct {
	// MaxDataResources is the max number of DataResources (free tier: 5)
	MaxDataResources int32 `json:"maxDataResources,omitempty"`
	// MaxOperatorAccounts (free tier: 3)
	MaxOperatorAccounts int32 `json:"maxOperatorAccounts,omitempty"`
	// MaxResourceAccounts total data-resource credentials (free tier: 3)
	MaxResourceAccounts int32 `json:"maxResourceAccounts,omitempty"`
	// MaxStorageBytes capacity quota
	MaxStorageBytes int64 `json:"maxStorageBytes,omitempty"`
	// MaxMonthlyTokens NestToken budget
	MaxMonthlyTokens int64 `json:"maxMonthlyTokens,omitempty"`
}

type TenantStatus struct {
	// DataResourceCount current provisioned count
	DataResourceCount int32 `json:"dataResourceCount,omitempty"`
	// OperatorAccountCount current operator accounts
	OperatorAccountCount int32 `json:"operatorAccountCount,omitempty"`
	// ResourceAccountCount current data-resource credentials
	ResourceAccountCount int32              `json:"resourceAccountCount,omitempty"`
	Conditions           []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type TenantList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Tenant `json:"items"`
}
