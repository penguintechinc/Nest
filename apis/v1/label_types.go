package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// ResourceLabel allows operators to manually apply or override classification labels
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type ResourceLabel struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              ResourceLabelSpec   `json:"spec,omitempty"`
	Status            ResourceLabelStatus `json:"status,omitempty"`
}

type ResourceLabelSpec struct {
	Resource  string   `json:"resource"`             // DataResource name
	Table     string   `json:"table,omitempty"`
	Column    string   `json:"column,omitempty"`
	Labels    []string `json:"labels"`               // PII, PCI, PHI, CREDENTIALS, SENSITIVE, INTERNAL
	Override  bool     `json:"override,omitempty"`  // if true, replaces auto-classification
	AppliedBy string   `json:"appliedBy,omitempty"` // operator who applied this
}

type ResourceLabelStatus struct {
	Applied            bool   `json:"applied,omitempty"`
	AppliedAt          string `json:"appliedAt,omitempty"`
	SyncedToIndexer    bool   `json:"syncedToIndexer,omitempty"`
}

// +kubebuilder:object:root=true
type ResourceLabelList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []ResourceLabel `json:"items"`
}

func init() {
	SchemeBuilder.Register(&ResourceLabel{}, &ResourceLabelList{})
}
