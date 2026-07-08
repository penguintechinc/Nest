package controllers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// reconcileVector reconciles a vector DataResource by creating/updating
// a CloudNativePG Cluster CR with the pgvector extension pre-installed.
// Per spec §11.6 and §32.1: "pgvector on shared Postgres".
func (r *DataResourceReconciler) reconcileVector(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	clusterName := vectorClusterName(dr)
	namespace := vectorNamespace(dr)

	// Ensure namespace exists
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

	storageSize := vectorStorageSize(dr)

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
				"imageName": "ghcr.io/cloudnative-pg/postgresql:16",
				"storage": map[string]interface{}{
					"size": storageSize,
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
						"postInitSQL": []interface{}{
							"CREATE EXTENSION IF NOT EXISTS vector;",
						},
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
		logger.Info("creating CloudNativePG Cluster with pgvector", "cluster", clusterName, "namespace", namespace)
		if createErr := r.Create(ctx, cluster); createErr != nil {
			return fmt.Errorf("creating CloudNativePG Cluster %s: %w", clusterName, createErr)
		}
		r.setPhase(dr, nestv1.PhaseProvisioning, "CloudNativePG Cluster with pgvector created")
		return r.Status().Update(ctx, dr)
	}
	if err != nil {
		return fmt.Errorf("getting CloudNativePG Cluster %s: %w", clusterName, err)
	}

	// Check if the cluster is ready
	readyInstances, _, _ := unstructured.NestedInt64(existing.Object, "status", "readyInstances")
	if readyInstances >= instances {
		// Extract primary service name for endpoints
		primarySvc := fmt.Sprintf("postgres://%s-rw.%s.svc.cluster.local:5432/%s", clusterName, namespace, dr.Spec.Tenant)

		dr.Status.Endpoints = &nestv1.ResourceEndpoints{
			Native: primarySvc,
		}
		r.setPhase(dr, nestv1.PhaseReady, fmt.Sprintf("CloudNativePG Cluster with pgvector ready (%d/%d instances)", readyInstances, totalInstances))
	} else {
		r.setPhase(dr, nestv1.PhaseProvisioning, fmt.Sprintf("Waiting for Cluster instances (%d/%d ready)", readyInstances, totalInstances))
	}

	// Reconcile DBLB config for vector resource
	if err := r.reconcileDblbConfig(ctx, dr); err != nil {
		logger.Error(err, "failed to reconcile DBLB config", "cluster", clusterName)
		return err
	}

	return r.Status().Update(ctx, dr)
}

// reconcileVectorDelete removes the CloudNativePG Cluster on DataResource deletion.
func (r *DataResourceReconciler) reconcileVectorDelete(ctx context.Context, dr *nestv1.DataResource) error {
	clusterName := vectorClusterName(dr)
	namespace := vectorNamespace(dr)

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

func vectorClusterName(dr *nestv1.DataResource) string {
	// Format: {tenant}-{resource}-vector, truncate at 63 chars for K8s naming
	name := fmt.Sprintf("%s-%s-vector", dr.Spec.Tenant, dr.Name)
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func vectorNamespace(dr *nestv1.DataResource) string {
	return dr.Spec.Tenant
}

func vectorStorageSize(dr *nestv1.DataResource) string {
	if dr.Spec.Size != nil && dr.Spec.Size.Storage != "" {
		q, err := resource.ParseQuantity(dr.Spec.Size.Storage)
		if err == nil {
			return q.String()
		}
	}
	return "10Gi"
}
