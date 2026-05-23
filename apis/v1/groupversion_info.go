// Package v1 contains Nest API types for version v1.
package v1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "nest.penguintech.io", Version: "v1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)

func init() {
	SchemeBuilder.Register(
		&DataResource{}, &DataResourceList{},
		&DataResourceClass{}, &DataResourceClassList{},
		&Tenant{}, &TenantList{},
		&HardwarePool{}, &HardwarePoolList{},
		&HardwareInventory{}, &HardwareInventoryList{},
	)
}
