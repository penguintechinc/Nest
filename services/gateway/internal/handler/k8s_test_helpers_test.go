package handler

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// makeFakeK8sClient returns a controller-runtime fake client with the Nest v1 scheme registered.
func makeFakeK8sClient(objs ...client.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = nestv1.AddToScheme(scheme)
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(objs...).
		Build()
}

// makeDataResource builds a minimal DataResource for use in tests.
func makeDataResource(name, tenant, resType string) *nestv1.DataResource {
	return &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: tenant,
			Labels: map[string]string{
				"nest.penguintech.io/tenant": tenant,
			},
		},
		Spec: nestv1.DataResourceSpec{
			Type:        resType,
			Tenant:      tenant,
			Origination: nestv1.OriginationManaged,
		},
	}
}
