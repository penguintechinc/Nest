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

// openSearchGVR is the GVR for OpenSearch Kubernetes Operator CRs.
var openSearchGVR = schema.GroupVersionResource{
	Group:    "opensearch.opster.io",
	Version:  "v1",
	Resource: "opensearchclusters",
}

// reconcileOpenSearch dispatches to dedicated or shared search provisioning.
func (r *DataResourceReconciler) reconcileOpenSearch(ctx context.Context, dr *nestv1.DataResource) error {
	if dr.Spec.Search != nil && dr.Spec.Search.Mode == "shared" {
		return r.reconcileSharedOpenSearch(ctx, dr)
	}
	return r.reconcileDedicatedOpenSearch(ctx, dr)
}

// reconcileDedicatedOpenSearch reconciles a search DataResource by creating/updating
// a dedicated OpenSearchCluster CR via the OpenSearch Kubernetes Operator.
// Uses unstructured because the OpenSearch operator CRDs are not vendored.
func (r *DataResourceReconciler) reconcileDedicatedOpenSearch(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	clusterName := opensearchClusterName(dr)
	namespace := opensearchNamespace(dr)

	// Ensure tenant namespace exists
	if err := r.ensurePostgresNamespace(ctx, namespace); err != nil {
		return err
	}

	storageSize := opensearchStorageSize(dr)

	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "opensearch.opster.io/v1",
			"kind":       "OpenSearchCluster",
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
				"general": map[string]interface{}{
					"serviceName": fmt.Sprintf("%s-opensearch", dr.Name),
					"version":     "2.11.0",
					"httpPort":    int64(9200),
				},
				"nodePools": []interface{}{
					map[string]interface{}{
						"component": "nodes",
						"replicas":  int64(3),
						"diskSize":  storageSize,
						"jvm":       "-Xmx1024m -Xms1024m",
						"resources": map[string]interface{}{
							"requests": map[string]interface{}{
								"memory": "2Gi",
								"cpu":    "500m",
							},
							"limits": map[string]interface{}{
								"memory": "2Gi",
								"cpu":    "1000m",
							},
						},
						"roles": []interface{}{
							"master",
							"data",
						},
					},
				},
				"security": map[string]interface{}{
					"config": map[string]interface{}{
						"adminCredentialsSecret": map[string]interface{}{
							"name": fmt.Sprintf("%s-opensearch-admin", dr.Name),
						},
					},
					"tls": map[string]interface{}{
						"http": map[string]interface{}{
							"generate": true,
						},
						"transport": map[string]interface{}{
							"generate": true,
						},
					},
				},
			},
		},
	}

	// Attempt to get existing OpenSearchCluster CR
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(cluster.GroupVersionKind())
	err := r.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: namespace}, existing)
	if errors.IsNotFound(err) {
		logger.Info("creating OpenSearchCluster CR", "cluster", clusterName, "namespace", namespace)
		if createErr := r.Create(ctx, cluster); createErr != nil {
			return fmt.Errorf("creating OpenSearchCluster %s: %w", clusterName, createErr)
		}
		r.setPhase(dr, nestv1.PhaseProvisioning, "OpenSearchCluster created")
		return r.Status().Update(ctx, dr)
	}
	if err != nil {
		return fmt.Errorf("getting OpenSearchCluster %s: %w", clusterName, err)
	}

	// Check if the cluster is ready by checking status.phase == "RUNNING"
	phase, _, _ := unstructured.NestedString(existing.Object, "status", "phase")
	if phase == "RUNNING" {
		// Extract endpoint
		endpoint := fmt.Sprintf("%s.%s.svc.cluster.local:9200", clusterName, namespace)
		dr.Status.Endpoints = &nestv1.ResourceEndpoints{
			REST: endpoint,
		}
		r.setPhase(dr, nestv1.PhaseReady, "OpenSearchCluster ready")
	} else {
		r.setPhase(dr, nestv1.PhaseProvisioning, fmt.Sprintf("Waiting for OpenSearchCluster (phase: %s)", phase))
	}

	return r.Status().Update(ctx, dr)
}

// reconcileSharedOpenSearch reconciles a DataResource using a shared SearchPool.
func (r *DataResourceReconciler) reconcileSharedOpenSearch(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	poolRef := "nest-search-pool"
	if dr.Spec.Search != nil && dr.Spec.Search.PoolRef != "" {
		poolRef = dr.Spec.Search.PoolRef
	}

	// Look up the SearchPool
	var pool nestv1.SearchPool
	if err := r.Get(ctx, client.ObjectKey{Name: poolRef, Namespace: dr.Namespace}, &pool); err != nil {
		if errors.IsNotFound(err) {
			logger.Info("SearchPool not found, requeueing", "pool", poolRef)
			r.setPhase(dr, nestv1.PhaseProvisioning, fmt.Sprintf("SearchPool %q not found", poolRef))
			_ = r.Status().Update(ctx, dr)
			return fmt.Errorf("SearchPool %q not found", poolRef)
		}
		return err
	}

	if pool.Status.Phase != "Running" {
		logger.Info("SearchPool not yet running, requeueing", "pool", poolRef, "phase", pool.Status.Phase)
		r.setPhase(dr, nestv1.PhaseProvisioning, fmt.Sprintf("SearchPool %q phase=%s", poolRef, pool.Status.Phase))
		_ = r.Status().Update(ctx, dr)
		return fmt.Errorf("SearchPool %q phase=%s", poolRef, pool.Status.Phase)
	}

	// Tenant index prefix: {tenant}_{dataresource-name}
	indexPrefix := fmt.Sprintf("%s_%s", dr.Spec.Tenant, dr.Name)
	endpoint := pool.Status.Endpoint

	logger.Info("shared OpenSearch provisioned", "pool", poolRef, "indexPrefix", indexPrefix, "endpoint", endpoint)

	dr.Status.Endpoints = &nestv1.ResourceEndpoints{
		REST: endpoint,
	}
	r.setPhase(dr, nestv1.PhaseReady, fmt.Sprintf("Using shared pool %q", poolRef))
	if dr.Annotations == nil {
		dr.Annotations = make(map[string]string)
	}
	dr.Annotations["nest.penguintech.io/search-index-prefix"] = indexPrefix
	dr.Annotations["nest.penguintech.io/search-pool"] = poolRef
	return r.Status().Update(ctx, dr)
}

// reconcileOpenSearchDelete removes the OpenSearchCluster CR on DataResource deletion.
func (r *DataResourceReconciler) reconcileOpenSearchDelete(ctx context.Context, dr *nestv1.DataResource) error {
	clusterName := opensearchClusterName(dr)
	namespace := opensearchNamespace(dr)

	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "opensearch.opster.io", Version: "v1", Kind: "OpenSearchCluster",
	})
	cluster.SetName(clusterName)
	cluster.SetNamespace(namespace)

	if err := r.Delete(ctx, cluster); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting OpenSearchCluster %s: %w", clusterName, err)
	}
	return nil
}

// Helper functions for OpenSearch reconciliation

func opensearchClusterName(dr *nestv1.DataResource) string {
	name := fmt.Sprintf("%s-%s-opensearch", dr.Spec.Tenant, dr.Name)
	// Kubernetes names are limited to 63 chars
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func opensearchNamespace(dr *nestv1.DataResource) string {
	return dr.Spec.Tenant
}

func opensearchStorageSize(dr *nestv1.DataResource) string {
	if dr.Spec.Size != nil && dr.Spec.Size.Storage != "" {
		q, err := resource.ParseQuantity(dr.Spec.Size.Storage)
		if err == nil {
			return q.String()
		}
	}
	return "30Gi"
}
