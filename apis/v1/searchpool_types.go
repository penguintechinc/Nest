package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
// +kubebuilder:printcolumn:name="Phase",type=string,JSONPath=`.status.phase`
// +kubebuilder:printcolumn:name="Tenants",type=integer,JSONPath=`.status.tenantCount`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// SearchPool provisions a shared OpenSearch cluster that multiple tenants can use
// via index-prefix isolation. Operators deploy one SearchPool per tier; tenants
// reference it via DataResource spec.search.poolRef.
type SearchPool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              SearchPoolSpec   `json:"spec,omitempty"`
	Status            SearchPoolStatus `json:"status,omitempty"`
}

type SearchPoolSpec struct {
	// Replicas is the number of OpenSearch data nodes (default: 3)
	// +kubebuilder:default=3
	Replicas int32 `json:"replicas,omitempty"`
	// DiskSizeGi is the storage per node in GiB (default: 100)
	// +kubebuilder:default=100
	DiskSizeGi int64 `json:"diskSizeGi,omitempty"`
	// Version is the OpenSearch version (default: "2.11.0")
	// +kubebuilder:default="2.11.0"
	Version string `json:"version,omitempty"`
	// Namespace is where the shared OpenSearchCluster is created (default: "nest-search")
	// +kubebuilder:default="nest-search"
	Namespace string `json:"namespace,omitempty"`
	// StorageClass for OpenSearch data volumes (default: "nest-block")
	// +kubebuilder:default="nest-block"
	StorageClass string `json:"storageClass,omitempty"`
}

type SearchPoolStatus struct {
	// Phase is the lifecycle phase: Provisioning, Running, Degraded, Failed
	Phase string `json:"phase,omitempty"`
	// Endpoint is the HTTP endpoint of the shared cluster
	Endpoint string `json:"endpoint,omitempty"`
	// TenantCount is the number of tenants currently using this pool
	TenantCount int32 `json:"tenantCount,omitempty"`
	// AdminCredentialsSecret holds the admin credentials for the shared cluster
	AdminCredentialsSecret string `json:"adminCredentialsSecret,omitempty"`
	// Conditions holds standard K8s status conditions
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type SearchPoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []SearchPool `json:"items"`
}

// SearchSpec configures a search DataResource.
type SearchSpec struct {
	// Mode is "dedicated" (default) or "shared".
	// dedicated: one OpenSearchCluster per DataResource.
	// shared: tenant joins a SearchPool with index-prefix isolation.
	// +kubebuilder:default="dedicated"
	// +kubebuilder:validation:Enum=dedicated;shared
	Mode string `json:"mode,omitempty"`
	// PoolRef is the name of the SearchPool to join (shared mode only).
	// Defaults to "nest-search-pool" if not set.
	PoolRef string `json:"poolRef,omitempty"`
	// Version overrides the OpenSearch version for dedicated clusters.
	Version string `json:"version,omitempty"`
}
