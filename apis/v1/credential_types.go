package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

type Credential struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              CredentialSpec   `json:"spec,omitempty"`
	Status            CredentialStatus `json:"status,omitempty"`
}

type CredentialSpec struct {
	// Resource references the parent DataResource
	Resource string `json:"resource"`
	// Tenant is the owning tenant
	Tenant string `json:"tenant"`
	// Type is the credential type: db-user, s3-key, redis-acl, kafka-sasl
	Type string `json:"type"`
	// Username for DB/Redis credentials
	Username string `json:"username,omitempty"`
	// SecretRef is the opaque backend reference (never store plaintext)
	SecretRef string `json:"secretRef,omitempty"`
	// RotationPolicy configures automatic rotation
	RotationPolicy *RotationPolicy `json:"rotationPolicy,omitempty"`
}

type RotationPolicy struct {
	// IntervalDays between rotations
	IntervalDays int32 `json:"intervalDays,omitempty"`
	// NotifyDaysBefore sends notification before rotation
	NotifyDaysBefore int32 `json:"notifyDaysBefore,omitempty"`
}

type CredentialStatus struct {
	// LastRotatedAt is when the credential was last rotated
	LastRotatedAt *metav1.Time `json:"lastRotatedAt,omitempty"`
	// ExpiresAt is when the credential expires (if applicable)
	ExpiresAt *metav1.Time `json:"expiresAt,omitempty"`
	// State: active, rotating, revoked
	State string `json:"state,omitempty"`
}

// +kubebuilder:object:root=true
type CredentialList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Credential `json:"items"`
}
