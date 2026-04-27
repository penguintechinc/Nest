package controllers

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

func TestPVCBlockReconcileCreate(t *testing.T) {
	ctx := context.Background()

	// Create DataResource with pvc/block type
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-block",
			Namespace: "default",
			UID:       "test-uid-123",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "pvc/block",
			Tenant: "test-tenant",
			Size: &nestv1.ResourceSize{
				Storage: "20Gi",
			},
		},
	}

	// Create reconciler with status subresource
	scheme := newTestScheme(t)
	fclient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&nestv1.DataResource{}).
		WithObjects(dr).
		Build()

	reconciler := &DataResourceReconciler{
		Client: fclient,
		Scheme: scheme,
	}

	// Reconcile create
	err := reconciler.reconcilePVCBlock(ctx, dr)
	if err != nil {
		t.Fatalf("reconcilePVCBlock failed: %v", err)
	}

	// Verify PVC was created with correct storage class
	pvc := &corev1.PersistentVolumeClaim{}
	err = fclient.Get(ctx, types.NamespacedName{
		Name:      "test-tenant-test-block-rbd",
		Namespace: "test-tenant",
	}, pvc)
	if err != nil {
		t.Fatalf("PVC not found: %v", err)
	}

	// Verify PVC labels
	if pvc.Labels["nest.penguintech.io/tenant"] != "test-tenant" {
		t.Errorf("tenant label incorrect: got %v", pvc.Labels["nest.penguintech.io/tenant"])
	}
	if pvc.Labels["nest.penguintech.io/dataresource"] != "test-block" {
		t.Errorf("dataresource label incorrect: got %v", pvc.Labels["nest.penguintech.io/dataresource"])
	}

	// Verify storage class
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "rook-ceph-block" {
		t.Errorf("storage class incorrect: got %v", pvc.Spec.StorageClassName)
	}

	// Verify storage size
	qty := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if qty.String() != "20Gi" {
		t.Errorf("storage size incorrect: got %v", qty.String())
	}

	// Verify access mode is RWO
	if len(pvc.Spec.AccessModes) != 1 || pvc.Spec.AccessModes[0] != corev1.ReadWriteOnce {
		t.Errorf("access mode incorrect: got %v", pvc.Spec.AccessModes)
	}

	// Verify owner reference
	if len(pvc.OwnerReferences) != 1 {
		t.Errorf("owner references incorrect: got %v", len(pvc.OwnerReferences))
	}
	if pvc.OwnerReferences[0].Kind != "DataResource" {
		t.Errorf("owner kind incorrect: got %v", pvc.OwnerReferences[0].Kind)
	}

	// Verify phase transition
	if dr.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("phase incorrect: got %v", dr.Status.Phase)
	}
}

func TestPVCBlockReconcileDelete(t *testing.T) {
	ctx := context.Background()

	// Create DataResource and PVC
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-block",
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "pvc/block",
			Tenant: "test-tenant",
		},
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tenant-test-block-rbd",
			Namespace: "test-tenant",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		},
	}

	scheme := newTestScheme(t)
	fclient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&nestv1.DataResource{}).
		WithObjects(dr, pvc).
		Build()

	reconciler := &DataResourceReconciler{
		Client: fclient,
		Scheme: scheme,
	}

	// Reconcile delete
	err := reconciler.reconcilePVCBlockDelete(ctx, dr)
	if err != nil {
		t.Fatalf("reconcilePVCBlockDelete failed: %v", err)
	}

	// Verify PVC was deleted
	foundPVC := &corev1.PersistentVolumeClaim{}
	err = fclient.Get(ctx, types.NamespacedName{
		Name:      "test-tenant-test-block-rbd",
		Namespace: "test-tenant",
	}, foundPVC)
	if err == nil {
		t.Errorf("PVC should have been deleted but still exists")
	}
}

func TestPVCFileReconcileCreate(t *testing.T) {
	ctx := context.Background()

	// Create DataResource with pvc/file type
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-file",
			Namespace: "default",
			UID:       "test-uid-456",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "pvc/file",
			Tenant: "test-tenant",
			Size: &nestv1.ResourceSize{
				Storage: "50Gi",
			},
		},
	}

	// Create reconciler with status subresource
	scheme := newTestScheme(t)
	fclient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&nestv1.DataResource{}).
		WithObjects(dr).
		Build()

	reconciler := &DataResourceReconciler{
		Client: fclient,
		Scheme: scheme,
	}

	// Reconcile create
	err := reconciler.reconcilePVCFile(ctx, dr)
	if err != nil {
		t.Fatalf("reconcilePVCFile failed: %v", err)
	}

	// Verify PVC was created with correct storage class
	pvc := &corev1.PersistentVolumeClaim{}
	err = fclient.Get(ctx, types.NamespacedName{
		Name:      "test-tenant-test-file-cephfs-rwo",
		Namespace: "test-tenant",
	}, pvc)
	if err != nil {
		t.Fatalf("PVC not found: %v", err)
	}

	// Verify storage class
	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != "rook-cephfs-rwo" {
		t.Errorf("storage class incorrect: got %v", pvc.Spec.StorageClassName)
	}

	// Verify storage size
	qty := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if qty.String() != "50Gi" {
		t.Errorf("storage size incorrect: got %v", qty.String())
	}

	// Verify access mode is RWO
	if len(pvc.Spec.AccessModes) != 1 || pvc.Spec.AccessModes[0] != corev1.ReadWriteOnce {
		t.Errorf("access mode incorrect: got %v", pvc.Spec.AccessModes)
	}

	// Verify labels
	if pvc.Labels["nest.penguintech.io/tenant"] != "test-tenant" {
		t.Errorf("tenant label incorrect: got %v", pvc.Labels["nest.penguintech.io/tenant"])
	}
	if pvc.Labels["nest.penguintech.io/dataresource"] != "test-file" {
		t.Errorf("dataresource label incorrect: got %v", pvc.Labels["nest.penguintech.io/dataresource"])
	}
}

func TestPVCFileReconcileDelete(t *testing.T) {
	ctx := context.Background()

	// Create DataResource and PVC
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-file",
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "pvc/file",
			Tenant: "test-tenant",
		},
	}

	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tenant-test-file-cephfs-rwo",
			Namespace: "test-tenant",
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
		},
	}

	scheme := newTestScheme(t)
	fclient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&nestv1.DataResource{}).
		WithObjects(dr, pvc).
		Build()

	reconciler := &DataResourceReconciler{
		Client: fclient,
		Scheme: scheme,
	}

	// Reconcile delete
	err := reconciler.reconcilePVCFileDelete(ctx, dr)
	if err != nil {
		t.Fatalf("reconcilePVCFileDelete failed: %v", err)
	}

	// Verify PVC was deleted
	foundPVC := &corev1.PersistentVolumeClaim{}
	err = fclient.Get(ctx, types.NamespacedName{
		Name:      "test-tenant-test-file-cephfs-rwo",
		Namespace: "test-tenant",
	}, foundPVC)
	if err == nil {
		t.Errorf("PVC should have been deleted but still exists")
	}
}

func TestPVCBlockDefaultSize(t *testing.T) {
	ctx := context.Background()

	// Create DataResource without explicit size
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-block-default",
			Namespace: "default",
			UID:       "test-uid-789",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "pvc/block",
			Tenant: "test-tenant",
		},
	}

	// Create reconciler with status subresource
	scheme := newTestScheme(t)
	fclient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&nestv1.DataResource{}).
		WithObjects(dr).
		Build()

	reconciler := &DataResourceReconciler{
		Client: fclient,
		Scheme: scheme,
	}

	// Reconcile create
	err := reconciler.reconcilePVCBlock(ctx, dr)
	if err != nil {
		t.Fatalf("reconcilePVCBlock failed: %v", err)
	}

	// Verify PVC has default size
	pvc := &corev1.PersistentVolumeClaim{}
	err = fclient.Get(ctx, types.NamespacedName{
		Name:      "test-tenant-test-block-default-rbd",
		Namespace: "test-tenant",
	}, pvc)
	if err != nil {
		t.Fatalf("PVC not found: %v", err)
	}

	qty := pvc.Spec.Resources.Requests[corev1.ResourceStorage]
	if qty.String() != "10Gi" {
		t.Errorf("default storage size incorrect: got %v", qty.String())
	}
}

func TestPVCBlockCustomStorageClass(t *testing.T) {
	ctx := context.Background()

	// Create DataResource with custom storage class annotation
	customSC := "custom-rbd-sc"
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-block-custom",
			Namespace: "default",
			UID:       "test-uid-custom",
			Annotations: map[string]string{
				"nest.penguintech.io/storage-class": customSC,
			},
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "pvc/block",
			Tenant: "test-tenant",
		},
	}

	// Create reconciler with status subresource
	scheme := newTestScheme(t)
	fclient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithStatusSubresource(&nestv1.DataResource{}).
		WithObjects(dr).
		Build()

	reconciler := &DataResourceReconciler{
		Client: fclient,
		Scheme: scheme,
	}

	// Reconcile create
	err := reconciler.reconcilePVCBlock(ctx, dr)
	if err != nil {
		t.Fatalf("reconcilePVCBlock failed: %v", err)
	}

	// Verify PVC uses custom storage class
	pvc := &corev1.PersistentVolumeClaim{}
	err = fclient.Get(ctx, types.NamespacedName{
		Name:      "test-tenant-test-block-custom-rbd",
		Namespace: "test-tenant",
	}, pvc)
	if err != nil {
		t.Fatalf("PVC not found: %v", err)
	}

	if pvc.Spec.StorageClassName == nil || *pvc.Spec.StorageClassName != customSC {
		t.Errorf("custom storage class not applied: got %v", pvc.Spec.StorageClassName)
	}
}
