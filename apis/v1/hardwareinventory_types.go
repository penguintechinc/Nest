package v1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:storageversion

// HardwareInventory is one CR per node listing all block devices + states.
// This replaces the per-device DarkDrive CR approach (§11.9) for scalability.
type HardwareInventory struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`
	Spec              HardwareInventorySpec   `json:"spec,omitempty"`
	Status            HardwareInventoryStatus `json:"status,omitempty"`
}

type HardwareInventorySpec struct {
	// Node is the K8s node name this inventory belongs to
	Node string `json:"node"`
	// Devices lists all discovered block devices
	Devices []DeviceSpec `json:"devices,omitempty"`
}

// +kubebuilder:validation:Enum=Active;System;Dark;Failed;Pending;Rejected;Decommissioned
type DeviceState string

const (
	DeviceStateActive         DeviceState = "Active"
	DeviceStateSystem         DeviceState = "System"
	DeviceStateDark           DeviceState = "Dark"
	DeviceStateFailed         DeviceState = "Failed"
	DeviceStatePending        DeviceState = "Pending"
	DeviceStateRejected       DeviceState = "Rejected"
	DeviceStateDecommissioned DeviceState = "Decommissioned"
)

type DeviceSpec struct {
	// Name is the device path (e.g., /dev/nvme0n1)
	Name string `json:"name"`
	// Serial is the drive serial number
	Serial string `json:"serial,omitempty"`
	// Model is the drive model string
	Model string `json:"model,omitempty"`
	// CapacityBytes is the raw device capacity
	CapacityBytes int64 `json:"capacityBytes,omitempty"`
	// Class is the auto-detected hardware tier
	Class string `json:"class,omitempty"`
	// State is the device lifecycle state
	State DeviceState `json:"state"`
	// SMART holds the latest SMART health data
	SMART *SMARTData `json:"smart,omitempty"`
	// Signature describes the partition/filesystem signature
	Signature string `json:"signature,omitempty"`
	// HardwarePool is the pool this device has been adopted into
	HardwarePool string `json:"hardwarePool,omitempty"`
}

type SMARTData struct {
	// Health is the overall SMART assessment
	Health string `json:"health,omitempty"`
	// WearPercent is media wear indicator (0-100)
	WearPercent int32 `json:"wearPercent,omitempty"`
	// HoursOn is total power-on hours
	HoursOn int64 `json:"hoursOn,omitempty"`
	// Temperature in Celsius
	TemperatureCelsius int32 `json:"temperatureCelsius,omitempty"`
	// ReallocatedSectors count
	ReallocatedSectors int64 `json:"reallocatedSectors,omitempty"`
}

type HardwareInventoryStatus struct {
	// LastScanTime is when the node agent last updated this inventory
	LastScanTime *metav1.Time `json:"lastScanTime,omitempty"`
	// DarkDriveCount number of unadopted drives
	DarkDriveCount int32 `json:"darkDriveCount,omitempty"`
	// ActiveDriveCount number of adopted drives
	ActiveDriveCount int32 `json:"activeDriveCount,omitempty"`
	Conditions       []metav1.Condition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true
type HardwareInventoryList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []HardwareInventory `json:"items"`
}
