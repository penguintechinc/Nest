package articdbm

import (
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// SchemeGroupVersion is the canonical GroupVersion for all ArticDBM CRDs.
// Group: articdbm.io  Version: v1alpha1
var SchemeGroupVersion = schema.GroupVersion{Group: "articdbm.io", Version: "v1alpha1"}

// Resource returns a GroupResource for the given resource name within the
// articdbm.io API group. Useful for constructing owner references and RBAC rules.
func Resource(resource string) schema.GroupResource {
	return SchemeGroupVersion.WithResource(resource).GroupResource()
}

// AddToScheme registers the ArticDBM and ArticDBMList types with the provided
// runtime.Scheme so that the controller-runtime client can encode/decode them.
func AddToScheme(s *runtime.Scheme) error {
	s.AddKnownTypes(SchemeGroupVersion,
		&ArticDBM{},
		&ArticDBMList{},
	)
	return nil
}
