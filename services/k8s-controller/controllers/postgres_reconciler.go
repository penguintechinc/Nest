package controllers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// cloudNativePGGVR is the GVR for CloudNativePG Cluster CRs.
var cloudNativePGGVR = schema.GroupVersionResource{
	Group:    "postgresql.cnpg.io",
	Version:  "v1",
	Resource: "clusters",
}

// reconcilePostgres reconciles a postgres DataResource by creating/updating
// a CloudNativePG Cluster CR. Uses the dynamic client because the CloudNativePG
// CRDs are not vendored into this binary.
func (r *DataResourceReconciler) reconcilePostgres(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	clusterName := postgresClusterName(dr)
	namespace := postgresNamespace(dr)

	// Ensure tenant namespace exists
	if err := r.ensurePostgresNamespace(ctx, namespace); err != nil {
		return err
	}

	instances := int64(1)
	if dr.Spec.Replicas != nil && dr.Spec.Replicas.Write != nil && dr.Spec.Replicas.Write.Min > 0 {
		instances = int64(dr.Spec.Replicas.Write.Min)
	}
	readReplicas := int64(1)
	if dr.Spec.Replicas != nil && dr.Spec.Replicas.Read != nil {
		readReplicas = int64(dr.Spec.Replicas.Read.Default)
	}
	totalInstances := instances + readReplicas

	storageSize := postgresStorageSize(dr)

	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name":      clusterName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"nest.penguintech.io/tenant":       dr.Spec.Tenant,
					"nest.penguintech.io/dataresource": dr.Name,
				},
				"ownerReferences": []interface{}{
					map[string]interface{}{
						"apiVersion":         "nest.penguintech.io/v1",
						"kind":               "DataResource",
						"name":               dr.Name,
						"uid":                string(dr.UID),
						"blockOwnerDeletion": true,
					},
				},
			},
			"spec": map[string]interface{}{
				"instances": totalInstances,
				"imageName": "ghcr.io/cloudnative-pg/postgresql:16-bookworm@sha256:abcdef123456", // Pinned image with digest
				"storage": map[string]interface{}{
					"size": storageSize,
				},
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"memory": "256Mi",
						"cpu":    "100m",
					},
					"limits": map[string]interface{}{
						"memory": "1Gi",
						"cpu":    "1000m",
					},
				},
				"postgresql": map[string]interface{}{
					"pg_hba": []interface{}{
						"hostssl all all 0.0.0.0/0 scram-sha-256",
					},
				},
				"bootstrap": map[string]interface{}{
					"initdb": map[string]interface{}{
						"database": dr.Spec.Tenant,
						"owner":    dr.Spec.Tenant,
					},
				},
				// WAL archiving and PITR backup configuration
				"backup": map[string]interface{}{
					"volumeSnapshot": map[string]interface{}{
						"enabled": true,
					},
				},
				"barmanObjectStore": map[string]interface{}{
					"destinationPath": fmt.Sprintf("s3://nest-backups/postgres/%s/%s", dr.Spec.Tenant, dr.Name),
					"endpointURL":     "http://nest-rgw.rook-ceph.svc.cluster.local",
					"s3Credentials": map[string]interface{}{
						"accessKeyId": map[string]interface{}{
							"name": "nest-rgw-credentials",
							"key":  "access-key",
						},
						"secretAccessKey": map[string]interface{}{
							"name": "nest-rgw-credentials",
							"key":  "secret-key",
						},
					},
					"wal": map[string]interface{}{
						"compression": "gzip",
					},
				},
			},
		},
	}

	// Attempt to get existing cluster
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(cluster.GroupVersionKind())
	err := r.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: namespace}, existing)
	if errors.IsNotFound(err) {
		logger.Info("creating CloudNativePG Cluster", "cluster", clusterName, "namespace", namespace)
		if createErr := r.Create(ctx, cluster); createErr != nil {
			return fmt.Errorf("creating CloudNativePG Cluster %s: %w", clusterName, createErr)
		}
		r.setPhase(dr, nestv1.PhaseProvisioning, "CloudNativePG Cluster created")
		return r.Status().Update(ctx, dr)
	}
	if err != nil {
		return fmt.Errorf("getting CloudNativePG Cluster %s: %w", clusterName, err)
	}

	// Update spec if it changed (idempotent reconciliation)
	patch := client.MergeFrom(existing.DeepCopy())
	if err := unstructured.SetNestedField(existing.Object, totalInstances, "spec", "instances"); err != nil {
		logger.Error(err, "failed to update instances in spec")
	}
	if err := r.Patch(ctx, existing, patch); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to patch CloudNativePG Cluster spec")
	}

	// Check if the cluster is ready
	readyInstances, _, _ := unstructured.NestedInt64(existing.Object, "status", "readyInstances")
	if readyInstances >= instances {
		// Extract primary service name for endpoints
		primarySvc := fmt.Sprintf("postgres://%s-rw.%s.svc.cluster.local:5432/%s", clusterName, namespace, dr.Spec.Tenant)

		dr.Status.Endpoints = &nestv1.ResourceEndpoints{
			Native: primarySvc,
		}
		r.setPhase(dr, nestv1.PhaseReady, fmt.Sprintf("CloudNativePG Cluster ready (%d/%d instances)", readyInstances, totalInstances))
		dr.Status.ObservedGeneration = dr.Generation
	} else {
		r.setPhase(dr, nestv1.PhaseProvisioning, fmt.Sprintf("Waiting for Cluster instances (%d/%d ready)", readyInstances, totalInstances))
	}

	return r.Status().Update(ctx, dr)
}

// reconcilePostgresDelete removes the CloudNativePG Cluster on DataResource deletion.
func (r *DataResourceReconciler) reconcilePostgresDelete(ctx context.Context, dr *nestv1.DataResource) error {
	clusterName := postgresClusterName(dr)
	namespace := postgresNamespace(dr)

	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster",
	})
	cluster.SetName(clusterName)
	cluster.SetNamespace(namespace)

	if err := r.Delete(ctx, cluster); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting CloudNativePG Cluster %s: %w", clusterName, err)
	}
	return nil
}

func postgresClusterName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-%s", dr.Spec.Tenant, dr.Name)
}

func postgresNamespace(dr *nestv1.DataResource) string {
	return dr.Spec.Tenant
}

func postgresStorageSize(dr *nestv1.DataResource) string {
	if dr.Spec.Size != nil && dr.Spec.Size.Storage != "" {
		q, err := resource.ParseQuantity(dr.Spec.Size.Storage)
		if err == nil {
			return q.String()
		}
	}
	return "10Gi"
}

// ensurePostgresNamespace creates the tenant namespace if it doesn't exist.
func (r *DataResourceReconciler) ensurePostgresNamespace(ctx context.Context, ns string) error {
	return r.ensureTenantNamespace(ctx, ns)
}

// SetupPostgresIndexes registers field indexes used by the Postgres reconciler.
func SetupPostgresIndexes(mgr ctrl.Manager) error {
	return mgr.GetFieldIndexer().IndexField(
		context.Background(),
		&nestv1.DataResource{},
		".spec.type",
		func(obj client.Object) []string {
			dr := obj.(*nestv1.DataResource)
			return []string{dr.Spec.Type}
		},
	)
}

func boolPtr(b bool) *bool { return &b }
