package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

type ComplianceBundle struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ComplianceBundleSpec   `json:"spec,omitempty"`
	Status            ComplianceBundleStatus `json:"status,omitempty"`
}

// +kubebuilder:validation:Enum=soc2;hipaa;pci-dss;gdpr;iso27001
type ComplianceFramework string

type ComplianceBundleSpec struct {
	Framework ComplianceFramework `json:"framework"`
	// Tenant scopes the bundle to a specific tenant; empty = cluster-wide
	Tenant string `json:"tenant,omitempty"`
	// AuditRetentionDays defines how long audit logs are retained
	// +kubebuilder:default=90
	AuditRetentionDays int32 `json:"auditRetentionDays,omitempty"`
	// EncryptionRequired mandates encryption at rest + in transit
	// +kubebuilder:default=true
	EncryptionRequired bool `json:"encryptionRequired"`
	// DataResidencyRegion limits resource placement to this region
	DataResidencyRegion string `json:"dataResidencyRegion,omitempty"`
}

type ComplianceBundleStatus struct {
	Phase      string             `json:"phase,omitempty"`
	Violations []PolicyViolation  `json:"violations,omitempty"`
	LastAudit  string             `json:"lastAudit,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

type PolicyViolation struct {
	Resource   string `json:"resource"`
	Policy     string `json:"policy"`
	Severity   string `json:"severity"` // critical, high, medium, low
	Message    string `json:"message"`
	DetectedAt string `json:"detectedAt"`
}

// +kubebuilder:object:root=true
type ComplianceBundleList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ComplianceBundle `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ComplianceBundle{}, &ComplianceBundleList{})
}
