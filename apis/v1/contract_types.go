package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// DataContract enforces schema version compatibility for a DataResource
// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion
type DataContract struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DataContractSpec   `json:"spec,omitempty"`
	Status            DataContractStatus `json:"status,omitempty"`
}

type DataContractSpec struct {
	Resource      string              `json:"resource"`
	Compatibility string              `json:"compatibility"`
	Version       int64               `json:"version,omitempty"`
	Fields        []DataContractField `json:"fields,omitempty"`
	Enforcement   string              `json:"enforcement,omitempty"`
}

type DataContractField struct {
	Name     string `json:"name"`
	Type     string `json:"type"`
	Required bool   `json:"required,omitempty"`
}

type DataContractStatus struct {
	ObservedVersion int64  `json:"observedVersion,omitempty"`
	Violations      int    `json:"violations,omitempty"`
	LastChecked     string `json:"lastChecked,omitempty"`
	Phase           string `json:"phase,omitempty"`
}

// +kubebuilder:object:root=true
type DataContractList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataContract `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DataContract{}, &DataContractList{})
}
