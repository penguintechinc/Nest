package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
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
					"nest.penguintech.io/tenant":        dr.Spec.Tenant,
					"nest.penguintech.io/dataresource":  dr.Name,
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

	// Check if the cluster is ready
	readyInstances, _, _ := unstructured.NestedInt64(existing.Object, "status", "readyInstances")
	if readyInstances >= instances {
		// Extract primary service name for endpoints
		primarySvc := fmt.Sprintf("postgres://%s-rw.%s.svc.cluster.local:5432/%s", clusterName, namespace, dr.Spec.Tenant)

		dr.Status.Endpoints = &nestv1.ResourceEndpoints{
			Native: primarySvc,
		}
		r.setPhase(dr, nestv1.PhaseReady, fmt.Sprintf("CloudNativePG Cluster ready (%d/%d instances)", readyInstances, totalInstances))
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
	namespace := &corev1.Namespace{}
	namespace.Name = ns
	if err := r.Create(ctx, namespace); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("creating namespace %s: %w", ns, err)
	}
	return nil
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

// dblbConfigMapName returns the name of the DBLB ConfigMap for a Postgres DataResource.
func dblbConfigMapName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("dblb-%s-%s", dr.Spec.Tenant, dr.Name)
}

// reconcileDblbConfig creates/updates the MarchProxy DBLB ConfigMap for a Postgres DataResource.
// DBLB is the connection pool + read-write split proxy per spec §39.3.
func (r *DataResourceReconciler) reconcileDblbConfig(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	clusterName := postgresClusterName(dr)
	namespace := postgresNamespace(dr)

	primaryDSN := fmt.Sprintf("host=%s-rw.%s.svc.cluster.local port=5432 dbname=%s user=%s sslmode=require",
		clusterName, namespace, dr.Spec.Tenant, dr.Spec.Tenant)
	replicaDSN := fmt.Sprintf("host=%s-ro.%s.svc.cluster.local port=5432 dbname=%s user=%s sslmode=require",
		clusterName, namespace, dr.Spec.Tenant, dr.Spec.Tenant)

	configData := fmt.Sprintf(`[dblb]
tenant = %s
resource = %s
pool_mode = transaction
pool_size = 20
max_client_conn = 10000

[upstream_primary]
dsn = %s

[upstream_replica]
dsn = %s

[read_write_split]
enabled = true
primary_hint = nest_primary

[rate_limits]
ops_per_sec = 10000
connections_max = 100
`,
		dr.Spec.Tenant, dr.Name,
		primaryDSN,
		replicaDSN,
	)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      dblbConfigMapName(dr),
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
				"nest.penguintech.io/component":    "dblb",
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
		Data: map[string]string{
			"dblb.conf": configData,
		},
	}

	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: cm.Name, Namespace: namespace}, existing)
	if errors.IsNotFound(err) {
		logger.Info("creating DBLB ConfigMap", "name", cm.Name)
		return r.Create(ctx, cm)
	}
	if err != nil {
		return err
	}

	existing.Data = cm.Data
	return r.Update(ctx, existing)
}

func boolPtr(b bool) *bool { return &b }
