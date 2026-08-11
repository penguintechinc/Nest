package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// reconcileObject provisions an S3-compatible object bucket using Rook-Ceph RGW.
func (r *DataResourceReconciler) reconcileObject(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	// Ensure tenant namespace exists
	if err := r.ensureNamespace(ctx, dr.Spec.Tenant); err != nil {
		return err
	}

	// Create CephObjectStoreUser CR
	if err := r.createCephObjectStoreUser(ctx, dr); err != nil {
		return err
	}

	// Check if CephObjectStoreUser is ready
	isReady, err := r.checkCephObjectStoreUserReady(ctx, dr)
	if err != nil {
		return err
	}

	if !isReady {
		logger.Info("waiting for CephObjectStoreUser to be ready", "name", dr.Name, "tenant", dr.Spec.Tenant)
		r.setPhase(dr, nestv1.PhaseProvisioning, "Waiting for CephObjectStoreUser to be ready")
		return r.Status().Update(ctx, dr)
	}

	// Copy credentials to tenant namespace
	if err := r.copyCephCredentialsToTenant(ctx, dr); err != nil {
		return err
	}

	// Set endpoint and mark as ready
	bucketName := dr.Name
	dr.Status.Endpoints = &nestv1.ResourceEndpoints{
		Native: fmt.Sprintf("s3://nest-rgw.rook-ceph.svc.cluster.local"),
	}
	if dr.Annotations == nil {
		dr.Annotations = make(map[string]string)
	}
	dr.Annotations["nest.penguintech.io/bucket-name"] = bucketName

	r.setPhase(dr, nestv1.PhaseReady, "Object bucket provisioned")
	logger.Info("object bucket ready", "name", dr.Name, "tenant", dr.Spec.Tenant, "bucket", bucketName)

	// Update both metadata (annotations) and status
	if err := r.Update(ctx, dr); err != nil {
		return fmt.Errorf("updating DataResource metadata: %w", err)
	}
	return r.Status().Update(ctx, dr)
}

// reconcileObjectDelete removes the CephObjectStoreUser and credentials secret on DataResource deletion.
func (r *DataResourceReconciler) reconcileObjectDelete(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	// Delete CephObjectStoreUser CR
	userCR := &unstructured.Unstructured{}
	userCR.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "ceph.rook.io",
		Version: "v1",
		Kind:    "CephObjectStoreUser",
	})
	userCR.SetName(cephObjectStoreUserName(dr))
	userCR.SetNamespace("rook-ceph")

	if err := r.Delete(ctx, userCR); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete CephObjectStoreUser", "name", userCR.GetName())
		return err
	}

	// Delete credentials secret in tenant namespace
	credSecret := &corev1.Secret{}
	credSecret.Name = credentialsSecretName(dr)
	credSecret.Namespace = dr.Spec.Tenant

	if err := r.Delete(ctx, credSecret); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete credentials secret", "name", credSecret.Name, "namespace", credSecret.Namespace)
		return err
	}

	logger.Info("object bucket deleted", "name", dr.Name, "tenant", dr.Spec.Tenant)
	return nil
}

// createCephObjectStoreUser creates the CephObjectStoreUser CR if it doesn't exist.
func (r *DataResourceReconciler) createCephObjectStoreUser(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	userCR := &unstructured.Unstructured{}
	userCR.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "ceph.rook.io",
		Version: "v1",
		Kind:    "CephObjectStoreUser",
	})

	userName := cephObjectStoreUserName(dr)
	userCR.SetName(userName)
	userCR.SetNamespace("rook-ceph")
	userCR.SetLabels(map[string]string{
		"nest.penguintech.io/tenant":       dr.Spec.Tenant,
		"nest.penguintech.io/dataresource": dr.Name,
	})

	// Set spec fields using unstructured
	if err := unstructured.SetNestedField(userCR.Object, "nest-rgw", "spec", "store"); err != nil {
		return fmt.Errorf("setting store field: %w", err)
	}
	if err := unstructured.SetNestedField(userCR.Object, fmt.Sprintf("Nest tenant %s bucket %s", dr.Spec.Tenant, dr.Name), "spec", "displayName"); err != nil {
		return fmt.Errorf("setting displayName field: %w", err)
	}

	// Check if user CR already exists
	existingUser := &unstructured.Unstructured{}
	existingUser.SetGroupVersionKind(userCR.GroupVersionKind())
	err := r.Get(ctx, types.NamespacedName{Name: userName, Namespace: "rook-ceph"}, existingUser)
	if errors.IsNotFound(err) {
		logger.Info("creating CephObjectStoreUser", "name", userName, "namespace", "rook-ceph")
		if err := r.Create(ctx, userCR); err != nil {
			return fmt.Errorf("creating CephObjectStoreUser %s: %w", userName, err)
		}
	} else if err != nil {
		return fmt.Errorf("checking CephObjectStoreUser %s: %w", userName, err)
	}

	return nil
}

// checkCephObjectStoreUserReady polls the CephObjectStoreUser status for readiness.
func (r *DataResourceReconciler) checkCephObjectStoreUserReady(ctx context.Context, dr *nestv1.DataResource) (bool, error) {
	userCR := &unstructured.Unstructured{}
	userCR.SetGroupVersionKind(schema.GroupVersionKind{
		Group:   "ceph.rook.io",
		Version: "v1",
		Kind:    "CephObjectStoreUser",
	})

	userName := cephObjectStoreUserName(dr)
	err := r.Get(ctx, types.NamespacedName{Name: userName, Namespace: "rook-ceph"}, userCR)
	if err != nil {
		if errors.IsNotFound(err) {
			return false, nil
		}
		return false, err
	}

	// Check status.phase field
	phase, found, err := unstructured.NestedString(userCR.Object, "status", "phase")
	if err != nil {
		return false, fmt.Errorf("getting phase from CephObjectStoreUser %s: %w", userName, err)
	}

	if found && phase == "Ready" {
		return true, nil
	}

	return false, nil
}

// copyCephCredentialsToTenant copies the Rook-generated credentials secret from rook-ceph to the tenant namespace.
func (r *DataResourceReconciler) copyCephCredentialsToTenant(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	// The credentials secret is created by Rook with this naming pattern
	sourceSecretName := fmt.Sprintf("rook-ceph-object-user-nest-rgw-%s", cephObjectStoreUserName(dr))
	sourceCR := &corev1.Secret{}

	err := r.Get(ctx, client.ObjectKey{Name: sourceSecretName, Namespace: "rook-ceph"}, sourceCR)
	if err != nil {
		if errors.IsNotFound(err) {
			logger.Info("credentials secret not yet available from Rook", "name", sourceSecretName)
			return fmt.Errorf("waiting for credentials secret from Rook: %w", err)
		}
		return fmt.Errorf("getting credentials secret %s: %w", sourceSecretName, err)
	}

	// Create a copy in the tenant namespace
	destSecret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      credentialsSecretName(dr),
			Namespace: dr.Spec.Tenant,
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
		Type: corev1.SecretTypeOpaque,
		Data: sourceCR.Data,
	}

	// Check if destination secret already exists
	existingSecret := &corev1.Secret{}
	err = r.Get(ctx, client.ObjectKey{Name: destSecret.Name, Namespace: destSecret.Namespace}, existingSecret)
	if errors.IsNotFound(err) {
		logger.Info("creating credentials secret in tenant namespace", "name", destSecret.Name, "namespace", destSecret.Namespace)
		if err := r.Create(ctx, destSecret); err != nil {
			return fmt.Errorf("creating credentials secret %s in %s: %w", destSecret.Name, destSecret.Namespace, err)
		}
	} else if err != nil {
		return fmt.Errorf("checking credentials secret %s: %w", destSecret.Name, err)
	}

	return nil
}

// ensureNamespace creates the tenant namespace if it doesn't exist.
func (r *DataResourceReconciler) ensureNamespace(ctx context.Context, ns string) error {
	return r.ensureTenantNamespace(ctx, ns)
}

// Helper functions for object reconciliation

func cephObjectStoreUserName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("nest-%s-%s", dr.Spec.Tenant, dr.Name)
}

func credentialsSecretName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-s3-credentials", dr.Name)
}
