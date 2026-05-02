package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// Operation represents a long-running operation (LRO) per §17.1 GCP LRO pattern.
type Operation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              OperationSpec   `json:"spec,omitempty"`
	Status            OperationStatus `json:"status,omitempty"`
}

type OperationSpec struct {
	// Tenant is the owning tenant
	Tenant string `json:"tenant"`
	// OperationType describes the operation kind
	OperationType string `json:"operationType"`
	// ResourceRef references the target DataResource
	ResourceRef string `json:"resourceRef,omitempty"`
	// Parameters is an opaque JSON blob for operation-specific params
	Parameters string `json:"parameters,omitempty"`
}

// +kubebuilder:validation:Enum=Pending;Running;Succeeded;Failed;Cancelled
type OperationPhase string

const (
	OperationPhasePending   OperationPhase = "Pending"
	OperationPhaseRunning   OperationPhase = "Running"
	OperationPhaseSucceeded OperationPhase = "Succeeded"
	OperationPhaseFailed    OperationPhase = "Failed"
	OperationPhaseCancelled OperationPhase = "Cancelled"
)

type OperationStatus struct {
	Phase      OperationPhase     `json:"phase,omitempty"`
	StartedAt  *metav1.Time       `json:"startedAt,omitempty"`
	FinishedAt *metav1.Time       `json:"finishedAt,omitempty"`
	Message    string             `json:"message,omitempty"`
	Result     string             `json:"result,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type OperationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []Operation `json:"items"`
}
