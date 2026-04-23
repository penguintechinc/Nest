package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type DataResourceClass struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ClassSpec `json:"spec,omitempty"`
}

type ClassSpec struct {
	// Backend engine: postgres, object, keyvalue, pvc/block, pvc/file, etc.
	Backend string `json:"backend"`
	// Placement controls hardware tier selection
	Placement *PlacementSpec `json:"placement,omitempty"`
	// Replication policy
	Replication *ReplicationSpec `json:"replication,omitempty"`
	// Encryption requirements
	Encryption *EncryptionSpec `json:"encryption,omitempty"`
	// SLO targets for this class
	SLO *SLOSpec `json:"slo,omitempty"`
	// Cache policy
	Cache *CachePolicy `json:"cache,omitempty"`
	// AllowTLSDisabled permits disabling TLS (false for compliance classes)
	AllowTLSDisabled bool `json:"allowTlsDisabled,omitempty"`
	// FIPSRequired mandates FIPS-140 crypto
	// +kubebuilder:default=true
	FIPSRequired bool `json:"fipsRequired,omitempty"`
	// Replicas default configuration
	Replicas *ReplicaConfig `json:"replicas,omitempty"`
}

type PlacementSpec struct {
	// Prefer lists preferred hardware tiers (e.g., nvme-hot, ssd-warm)
	Prefer []string `json:"prefer,omitempty"`
	// Allow lists acceptable tiers
	Allow []string `json:"allow,omitempty"`
	// Forbid lists prohibited tiers
	Forbid []string `json:"forbid,omitempty"`
}

type ReplicationSpec struct {
	// Min is minimum replica count
	Min int32 `json:"min,omitempty"`
	// FailureDomain is the isolation level: node, rack, zone
	FailureDomain string `json:"failureDomain,omitempty"`
}

type EncryptionSpec struct {
	AtRest bool   `json:"atRest,omitempty"`
	// KMS reference: tenant or operator
	KMS string `json:"kms,omitempty"`
}

type SLOSpec struct {
	// AvailabilityPercent (e.g., 99.95)
	AvailabilityPercent float64 `json:"availabilityPercent,omitempty"`
	// WriteLatencyP99Ms max p99 write latency
	WriteLatencyP99Ms int64 `json:"writeLatencyP99Ms,omitempty"`
	// ReadLatencyP99Ms max p99 read latency
	ReadLatencyP99Ms int64 `json:"readLatencyP99Ms,omitempty"`
	// RPOSeconds recovery point objective
	RPOSeconds int64 `json:"rpoSeconds,omitempty"`
	// RTOSeconds same-region recovery time objective
	RTOSeconds int64 `json:"rtoSeconds,omitempty"`
}

type CachePolicy struct {
	// L1Enabled enables DBLB result cache
	L1Enabled bool `json:"l1Enabled,omitempty"`
	// L1MaxBytes per-tenant cache budget
	L1MaxBytes int64 `json:"l1MaxBytes,omitempty"`
	// TTLSeconds default cache TTL
	TTLSeconds int64 `json:"ttlSeconds,omitempty"`
}

// +kubebuilder:object:root=true
type DataResourceClassList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataResourceClass `json:"items"`
}
