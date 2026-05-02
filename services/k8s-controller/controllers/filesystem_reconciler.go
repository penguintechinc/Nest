package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// reconcileFilesystem reconciles a filesystem DataResource by creating a CephFS PVC.
func (r *DataResourceReconciler) reconcileFilesystem(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	// Ensure tenant namespace exists
	if err := r.reconcileFilesystemNamespace(ctx, dr.Spec.Tenant); err != nil {
		return err
	}

	pvcName := filesystemPVCName(dr)
	namespace := dr.Spec.Tenant
	storageSize := filesystemStorageSize(dr)

	// Get storage class from annotation or use default
	storageClass := "rook-cephfs"
	if dr.Annotations != nil {
		if sc, ok := dr.Annotations["nest.penguintech.io/storage-class"]; ok {
			storageClass = sc
		}
	}

	// Create PVC
	pvc := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      pvcName,
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "nest.penguintech.io/v1",
					Kind:               "DataResource",
					Name:               dr.Name,
					UID:                dr.UID,
					BlockOwnerDeletion: boolPtr(true),
				},
			},
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{
				corev1.ReadWriteMany,
			},
			StorageClassName: &storageClass,
			Resources: corev1.VolumeResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse(storageSize),
				},
			},
		},
	}

	// Check if PVC exists
	existingPVC := &corev1.PersistentVolumeClaim{}
	err := r.Get(ctx, client.ObjectKey{Name: pvc.Name, Namespace: namespace}, existingPVC)
	if errors.IsNotFound(err) {
		logger.Info("creating CephFS PVC", "name", pvc.Name, "namespace", namespace)
		if err := r.Create(ctx, pvc); err != nil {
			return fmt.Errorf("creating CephFS PVC %s: %w", pvc.Name, err)
		}
		r.setPhase(dr, nestv1.PhaseProvisioning, "CephFS PVC created")
		return r.Status().Update(ctx, dr)
	} else if err != nil {
		return fmt.Errorf("getting CephFS PVC %s: %w", pvc.Name, err)
	}

	// Check PVC status
	if existingPVC.Status.Phase == corev1.ClaimBound {
		endpoint := fmt.Sprintf("cephfs://%s.%s.svc.cluster.local", pvcName, namespace)
		dr.Status.Endpoints = &nestv1.ResourceEndpoints{
			Native: endpoint,
		}
		r.setPhase(dr, nestv1.PhaseReady, "CephFS PVC bound")
	} else {
		r.setPhase(dr, nestv1.PhaseProvisioning, fmt.Sprintf("Waiting for PVC to bind (current: %s)", existingPVC.Status.Phase))
	}

	return r.Status().Update(ctx, dr)
}

// reconcileFilesystemDelete removes the CephFS PVC on DataResource deletion.
func (r *DataResourceReconciler) reconcileFilesystemDelete(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)
	namespace := dr.Spec.Tenant

	// Delete PVC
	pvc := &corev1.PersistentVolumeClaim{}
	pvc.Name = filesystemPVCName(dr)
	pvc.Namespace = namespace
	if err := r.Delete(ctx, pvc); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete CephFS PVC", "name", pvc.Name)
		return err
	}

	return nil
}

// reconcileFilesystemNamespace creates the tenant namespace if it doesn't exist.
func (r *DataResourceReconciler) reconcileFilesystemNamespace(ctx context.Context, ns string) error {
	namespace := &corev1.Namespace{}
	namespace.Name = ns
	if err := r.Create(ctx, namespace); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("creating namespace %s: %w", ns, err)
	}
	return nil
}

// Helper functions for Filesystem reconciliation

func filesystemPVCName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-%s-cephfs", dr.Spec.Tenant, dr.Name)
}

func filesystemStorageSize(dr *nestv1.DataResource) string {
	if dr.Spec.Size != nil && dr.Spec.Size.Storage != "" {
		q, err := resource.ParseQuantity(dr.Spec.Size.Storage)
		if err == nil {
			return q.String()
		}
	}
	return "10Gi"
}
