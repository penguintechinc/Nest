// Package v1alpha1 is the legacy API version for Nest resources.
// This version exists to support potential conversion paths for future API versions.
// All types should be defined in v1 (the current storage version).
// See crd-versioning.md for the versioning strategy.
package v1alpha1

import (
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/scheme"
)

var (
	GroupVersion  = schema.GroupVersion{Group: "nest.penguintech.io", Version: "v1alpha1"}
	SchemeBuilder = &scheme.Builder{GroupVersion: GroupVersion}
	AddToScheme   = SchemeBuilder.AddToScheme
)

func init() {
	// v1alpha1 is reserved for future use; all types currently defined in v1.
	// When a breaking change requires v2, this namespace can be used as an intermediary
	// conversion path if needed (e.g., for gradual rollout of a conversion webhook).
}
