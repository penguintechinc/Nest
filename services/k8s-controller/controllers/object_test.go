package controllers

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

func TestObjectReconcileCreate(t *testing.T) {
	ctx := context.Background()
	tenant := "test-tenant"
	drName := "test-bucket"

	// Create a simple scheme with the necessary types
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := nestv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add nestv1 to scheme: %v", err)
	}

	// Create tenant namespace
	tenantNS := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: tenant,
		},
	}

	// Create a DataResource with type "object"
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      drName,
			Namespace: "default",
			UID:       "12345",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "object",
			Tenant: tenant,
		},
		Status: nestv1.DataResourceStatus{
			Phase: nestv1.PhasePending,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dr, tenantNS).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()

	// Create the reconciler
	reconciler := &DataResourceReconciler{
		Client: client,
		Scheme: scheme,
	}

	// First call: creates CephObjectStoreUser, returns Provisioning
	err := reconciler.reconcileObject(ctx, dr)
	if err != nil {
		t.Fatalf("reconcileObject failed: %v", err)
	}

	// Verify CephObjectStoreUser was created
	userCR := &unstructured.Unstructured{}
	userCR.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "ceph.rook.io",
		Version: "v1",
		Kind:    "CephObjectStoreUser",
	})
	if err := client.Get(ctx, types.NamespacedName{Name: cephObjectStoreUserName(dr), Namespace: "rook-ceph"}, userCR); err != nil {
		t.Errorf("CephObjectStoreUser not created: %v", err)
	}

	// Verify labels were set correctly
	labels := userCR.GetLabels()
	if labels["nest.penguintech.io/tenant"] != tenant {
		t.Errorf("expected tenant label, got %v", labels)
	}
}

func TestObjectReconcileDelete(t *testing.T) {
	ctx := context.Background()
	tenant := "test-tenant"
	drName := "test-bucket"

	// Create scheme
	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := nestv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add nestv1 to scheme: %v", err)
	}

	// Create DataResource
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      drName,
			Namespace: "default",
			UID:       "12345",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "object",
			Tenant: tenant,
		},
	}

	// Pre-create CephObjectStoreUser
	userCR := &unstructured.Unstructured{}
	userCR.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "ceph.rook.io",
		Version: "v1",
		Kind:    "CephObjectStoreUser",
	})
	userCR.SetName(cephObjectStoreUserName(dr))
	userCR.SetNamespace("rook-ceph")

	// Pre-create credentials secret in tenant namespace
	credSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      credentialsSecretName(dr),
			Namespace: tenant,
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"AccessKey": []byte("test-key"),
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(userCR, credSecret).
		Build()

	reconciler := &DataResourceReconciler{
		Client: client,
		Scheme: scheme,
	}

	// Run deletion reconciliation
	err := reconciler.reconcileObjectDelete(ctx, dr)
	if err != nil {
		t.Fatalf("reconcileObjectDelete failed: %v", err)
	}

	// Verify CephObjectStoreUser was deleted
	deletedUser := &unstructured.Unstructured{}
	deletedUser.SetGroupVersionKind(userCR.GroupVersionKind())
	err = client.Get(ctx, types.NamespacedName{Name: userCR.GetName(), Namespace: "rook-ceph"}, deletedUser)
	if err == nil {
		t.Errorf("CephObjectStoreUser should have been deleted")
	}
	if !errors.IsNotFound(err) {
		t.Errorf("unexpected error when checking deleted CephObjectStoreUser: %v", err)
	}

	// Verify credentials secret was deleted
	deletedSecret := &corev1.Secret{}
	err = client.Get(ctx, types.NamespacedName{Name: credSecret.Name, Namespace: tenant}, deletedSecret)
	if err == nil {
		t.Errorf("credentials secret should have been deleted")
	}
	if !errors.IsNotFound(err) {
		t.Errorf("unexpected error when checking deleted credentials secret: %v", err)
	}
}

func TestCephObjectStoreUserName(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-bucket",
		},
		Spec: nestv1.DataResourceSpec{
			Tenant: "tenant-a",
		},
	}

	expected := "nest-tenant-a-my-bucket"
	result := cephObjectStoreUserName(dr)
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}

func TestCredentialsSecretName(t *testing.T) {
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name: "my-bucket",
		},
	}

	expected := "my-bucket-s3-credentials"
	result := credentialsSecretName(dr)
	if result != expected {
		t.Errorf("expected %s, got %s", expected, result)
	}
}
