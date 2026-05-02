package v1

import metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

type NestFederation struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              NestFederationSpec   `json:"spec,omitempty"`
	Status            NestFederationStatus `json:"status,omitempty"`
}

// +kubebuilder:validation:Enum=active-standby;active-active
type FederationTopology string

const (
	FederationActiveStandby FederationTopology = "active-standby"
	FederationActiveActive  FederationTopology = "active-active"
)

type NestFederationSpec struct {
	Topology FederationTopology `json:"topology"`
	Primary  ClusterRef         `json:"primary"`
	Standbys []ClusterRef       `json:"standbys,omitempty"`
	// ResourceTypes lists which DataResource types to replicate (empty = all)
	ResourceTypes []string `json:"resourceTypes,omitempty"`
	// LagThresholdSeconds: alert if replication lag exceeds this
	// +kubebuilder:default=300
	LagThresholdSeconds int32 `json:"lagThresholdSeconds,omitempty"`
}

type ClusterRef struct {
	// Name is a friendly name for the cluster
	Name string `json:"name"`
	// Endpoint is the API server URL
	Endpoint string `json:"endpoint"`
	// CASecretRef references a Secret containing the CA certificate
	CASecretRef string `json:"caSecretRef,omitempty"`
	// TokenSecretRef references a Secret containing the auth token
	TokenSecretRef string `json:"tokenSecretRef,omitempty"`
	// Region is the geographic region (optional, for geo-DR)
	Region string `json:"region,omitempty"`
}

type NestFederationStatus struct {
	Phase              string             `json:"phase,omitempty"`
	Conditions         []metav1.Condition `json:"conditions,omitempty"`
	// ClusterStatuses holds per-cluster replication status
	ClusterStatuses []ClusterStatus `json:"clusterStatuses,omitempty"`
}

type ClusterStatus struct {
	Name              string `json:"name"`
	State             string `json:"state"` // connected, disconnected, syncing
	ReplicationLagSec int64  `json:"replicationLagSec,omitempty"`
	LastSyncTime      string `json:"lastSyncTime,omitempty"`
}

// +kubebuilder:object:root=true
type NestFederationList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []NestFederation `json:"items"`
}

func init() {
	SchemeBuilder.Register(&NestFederation{}, &NestFederationList{})
}
