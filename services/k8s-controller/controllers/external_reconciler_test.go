package controllers

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

func TestExternalReconciler_NilExternal(t *testing.T) {
	ctx := context.Background()
	drName := "test-external"

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := nestv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add nestv1 to scheme: %v", err)
	}

	// Create a DataResource with origination: external but no spec.external
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      drName,
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:        nestv1.TypeEBS,
			Tenant:      "test-tenant",
			Origination: nestv1.OriginationExternal,
			External:    nil, // Missing external spec
		},
		Status: nestv1.DataResourceStatus{
			Phase: nestv1.PhasePending,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dr).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()

	reconciler := &DataResourceReconciler{
		Client: client,
		Scheme: scheme,
	}

	// Should return error for missing external spec
	err := reconciler.reconcileExternal(ctx, dr)
	if err == nil {
		t.Fatal("expected error for nil external spec")
	}
	if err.Error() != "external DataResource default/test-external missing spec.external" {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestExternalReconciler_UnknownProvider(t *testing.T) {
	ctx := context.Background()
	drName := "test-external"

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := nestv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add nestv1 to scheme: %v", err)
	}

	// Create a DataResource with unknown provider
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      drName,
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:        "unknown-type",
			Tenant:      "test-tenant",
			Origination: nestv1.OriginationExternal,
			External: &nestv1.ExternalSpec{
				Provider: "unknown-provider",
				Region:   "us-west-2",
			},
		},
		Status: nestv1.DataResourceStatus{
			Phase: nestv1.PhasePending,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dr).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()

	reconciler := &DataResourceReconciler{
		Client: client,
		Scheme: scheme,
	}

	// Should return error for unknown provider
	err := reconciler.reconcileExternal(ctx, dr)
	if err == nil {
		t.Fatal("expected error for unknown provider")
	}
}

func TestExternalReconciler_Delete_NilExternal(t *testing.T) {
	ctx := context.Background()
	drName := "test-external"

	scheme := runtime.NewScheme()
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := nestv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add nestv1 to scheme: %v", err)
	}

	// Create a DataResource with nil external spec (not a problem for delete)
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      drName,
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:        nestv1.TypeEBS,
			Tenant:      "test-tenant",
			Origination: nestv1.OriginationExternal,
			External:    nil,
		},
		Status: nestv1.DataResourceStatus{
			Phase: nestv1.PhaseReady,
		},
	}

	client := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dr).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()

	reconciler := &DataResourceReconciler{
		Client: client,
		Scheme: scheme,
	}

	// Should gracefully handle nil external spec on delete
	err := reconciler.reconcileExternalDelete(ctx, dr)
	if err != nil {
		t.Fatalf("unexpected error for delete with nil external: %v", err)
	}
}

func TestProviderForType_EBS(t *testing.T) {
	prov := providerForType(nestv1.TypeEBS, "aws")
	if prov == nil {
		t.Fatal("expected AWS provisioner for EBS type")
	}

	prov2 := providerForType(nestv1.TypeEBS, "")
	if prov2 == nil {
		t.Fatal("expected AWS provisioner for EBS type with fallback")
	}
}

func TestProviderForType_S3(t *testing.T) {
	prov := providerForType(nestv1.TypeS3, "aws")
	if prov == nil {
		t.Fatal("expected AWS provisioner for S3 type")
	}
}

func TestProviderForType_AzureDisk(t *testing.T) {
	prov := providerForType(nestv1.TypeAzureDisk, "azure")
	if prov == nil {
		t.Fatal("expected Azure provisioner for Azure Disk type")
	}

	prov2 := providerForType(nestv1.TypeAzureDisk, "")
	if prov2 == nil {
		t.Fatal("expected Azure provisioner for Azure Disk type with fallback")
	}
}

func TestProviderForType_GCPDisk(t *testing.T) {
	prov := providerForType(nestv1.TypeGCPDisk, "gcp")
	if prov == nil {
		t.Fatal("expected GCP provisioner for GCP Disk type")
	}

	prov2 := providerForType(nestv1.TypeGCPDisk, "")
	if prov2 == nil {
		t.Fatal("expected GCP provisioner for GCP Disk type with fallback")
	}
}

func TestProviderForType_Unknown(t *testing.T) {
	prov := providerForType("unknown-type", "unknown-provider")
	if prov != nil {
		t.Fatal("expected nil provisioner for unknown type")
	}
}
