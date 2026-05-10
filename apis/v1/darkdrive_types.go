package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// DarkDrive tracks a single unadopted block device discovered by the node agent.
type DarkDrive struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DarkDriveSpec   `json:"spec,omitempty"`
	Status            DarkDriveStatus `json:"status,omitempty"`
}

type DarkDriveSpec struct {
	// Node is the K8s node name where the device was discovered
	Node string `json:"node"`
	// Device is the block device path (e.g. /dev/nvme0n1)
	Device string `json:"device"`
	// Size is the human-readable device capacity (e.g. 1.92TB)
	Size string `json:"size"`
	// Class is the hardware tier: nvme-hot, ssd-warm, sata-bulk, sata-cold
	// +kubebuilder:validation:Enum=nvme-hot;ssd-warm;sata-bulk;sata-cold
	Class string `json:"class"`
	// Serial is the drive serial number
	Serial string `json:"serial,omitempty"`
	// SMART holds the latest SMART health data
	SMART *DarkDriveSMART `json:"smart,omitempty"`
	// Signature describes the partition/filesystem signature: blank | nest-previous | foreign-fs:<type>
	Signature string `json:"signature,omitempty"`
	// HardwarePool is the target HardwarePool name set during approval
	HardwarePool string `json:"hardwarePool,omitempty"`
	// EraseConfirmed must be true for foreign-fs signature drives before adoption
	EraseConfirmed bool `json:"eraseConfirmed,omitempty"`
	// Ignore skips this device from future discovery scans
	// +kubebuilder:default=false
	Ignore bool `json:"ignore,omitempty"`
	// FsType is the filesystem used when formatting this drive for Nest.
	// "raw" means the device is given directly to Rook-Ceph (Bluestore) without pre-formatting.
	// "btrfs" and "zfs" cause the drive to be formatted before handing to Rook.
	// +kubebuilder:default=btrfs
	// +kubebuilder:validation:Enum=btrfs;zfs;raw
	FsType string `json:"fsType,omitempty"`
	// AutoApprove allows blank and nest-previous drives to skip AwaitingApproval and go straight to Approved.
	// +kubebuilder:default=false
	AutoApprove bool `json:"autoApprove,omitempty"`
}

// DarkDriveSMART holds SMART health data for a dark drive.
type DarkDriveSMART struct {
	// Health is the overall SMART assessment
	Health string `json:"health,omitempty"`
	// WearPercent is the media wear indicator (0-100)
	WearPercent int32 `json:"wearPercent,omitempty"`
	// HoursOn is total power-on hours
	HoursOn int64 `json:"hoursOn,omitempty"`
}

// DarkDrivePhase is the lifecycle state of a DarkDrive.
// +kubebuilder:validation:Enum=Discovered;AwaitingApproval;Approved;Adopted;Rejected
type DarkDrivePhase string

const (
	DarkDriveDiscovered       DarkDrivePhase = "Discovered"
	DarkDriveAwaitingApproval DarkDrivePhase = "AwaitingApproval"
	DarkDriveApproved         DarkDrivePhase = "Approved"
	DarkDriveAdopted          DarkDrivePhase = "Adopted"
	DarkDriveRejected         DarkDrivePhase = "Rejected"
)

type DarkDriveStatus struct {
	// State is the current lifecycle phase
	State DarkDrivePhase `json:"state,omitempty"`
	// ApprovedBy is the identity that approved this drive for adoption
	ApprovedBy string `json:"approvedBy,omitempty"`
	// ApprovedAt is the timestamp when approval was granted
	ApprovedAt *metav1.Time `json:"approvedAt,omitempty"`
	// Conditions holds standard Kubernetes condition objects
	Conditions []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type DarkDriveList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DarkDrive `json:"items"`
}
