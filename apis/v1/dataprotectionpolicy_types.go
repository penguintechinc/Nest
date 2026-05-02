package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:storageversion

type DataProtectionPolicy struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              DataProtectionPolicySpec `json:"spec,omitempty"`
}

type DataProtectionPolicySpec struct {
	// Snapshots configures local snapshot schedule
	Snapshots *SnapshotConfig `json:"snapshots,omitempty"`
	// Backups configures remote backup schedule
	Backups *BackupConfig `json:"backups,omitempty"`
	// PITR enables point-in-time recovery
	PITR *PITRConfig `json:"pitr,omitempty"`
	// Verify configures restore testing
	Verify *VerifyConfig `json:"verify,omitempty"`
}

type SnapshotConfig struct {
	// Schedule is a cron expression (@hourly, @daily, @weekly, @monthly, or @every Xh/Xm)
	Schedule string `json:"schedule"`
	// PVCName is the PersistentVolumeClaim to snapshot
	PVCName string `json:"pvcName"`
	// Retention policy
	Retention *RetentionPolicy `json:"retention,omitempty"`
}

type BackupConfig struct {
	// Schedule is a cron expression
	Schedule string `json:"schedule"`
	// Destination is the target object DataResource
	Destination *BackupDestination `json:"destination,omitempty"`
	// CrossRegionCopy enables cross-region backup replication
	CrossRegionCopy *CrossRegionCopyConfig `json:"crossRegionCopy,omitempty"`
	// Retention policy
	Retention *RetentionPolicy `json:"retention,omitempty"`
	// Encryption references a KMS key
	Encryption string `json:"encryption,omitempty"`
}

type BackupDestination struct {
	Kind     string `json:"kind"`
	Resource string `json:"resource"`
	Region   string `json:"region,omitempty"`
}

type CrossRegionCopyConfig struct {
	Enabled              bool   `json:"enabled"`
	Destination          string `json:"destination"`
	Mode                 string `json:"mode,omitempty"`
	LagBudgetSeconds     int64  `json:"lagBudgetSeconds,omitempty"`
}

type RetentionPolicy struct {
	Hourly  int32 `json:"hourly,omitempty"`
	Daily   int32 `json:"daily,omitempty"`
	Weekly  int32 `json:"weekly,omitempty"`
	Monthly int32 `json:"monthly,omitempty"`
	Yearly  int32 `json:"yearly,omitempty"`
}

type PITRConfig struct {
	Enabled     bool  `json:"enabled"`
	WindowDays  int32 `json:"windowDays,omitempty"`
}

type VerifyConfig struct {
	RestoreTest *RestoreTestConfig `json:"restoreTest,omitempty"`
}

type RestoreTestConfig struct {
	Schedule string `json:"schedule,omitempty"`
	Target   string `json:"target,omitempty"`
}

// +kubebuilder:object:root=true
type DataProtectionPolicyList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DataProtectionPolicy `json:"items"`
}
