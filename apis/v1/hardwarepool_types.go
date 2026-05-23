package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

type HardwarePool struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              HardwarePoolSpec   `json:"spec,omitempty"`
	Status            HardwarePoolStatus `json:"status,omitempty"`
}

type HardwarePoolSpec struct {
	// Class is the hardware tier: nvme-hot, ssd-warm, sata-bulk, sata-cold
	// +kubebuilder:validation:Enum=nvme-hot;ssd-warm;sata-bulk;sata-cold;tape
	Class string `json:"class"`
	// Nodes lists the K8s node names in this pool
	Nodes []string `json:"nodes,omitempty"`
	// Labels to select nodes for this pool
	NodeSelector map[string]string `json:"nodeSelector,omitempty"`
}

type HardwarePoolStatus struct {
	// CapacityBytes total pool capacity
	CapacityBytes int64 `json:"capacityBytes,omitempty"`
	// UsedBytes allocated capacity
	UsedBytes int64 `json:"usedBytes,omitempty"`
	// FreeBytes available capacity
	FreeBytes int64 `json:"freeBytes,omitempty"`
	// DriveCount number of drives in pool
	DriveCount int32 `json:"driveCount,omitempty"`
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type HardwarePoolList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HardwarePool `json:"items"`
}
