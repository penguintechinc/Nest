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

// altinityClickHouseGVR is the GVR for Altinity ClickHouseInstallation CRs.
var altinityClickHouseGVR = schema.GroupVersionResource{
	Group:    "clickhouse.altinity.com",
	Version:  "v1",
	Resource: "clickhouseinstallations",
}

// reconcileClickhouse reconciles a clickhouse DataResource by creating/updating
// an Altinity ClickHouseInstallation CR. Uses the dynamic client because the Altinity
// CRDs are not vendored into this binary.
func (r *DataResourceReconciler) reconcileClickhouse(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	chiName := clickhouseClusterName(dr)
	namespace := clickhouseNamespace(dr)

	storageSize := clickhouseStorageSize(dr)

	chi := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "clickhouse.altinity.com/v1",
			"kind":       "ClickHouseInstallation",
			"metadata": map[string]interface{}{
				"name":      chiName,
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
				"configuration": map[string]interface{}{
					"clusters": []interface{}{
						map[string]interface{}{
							"name": "default",
							"layout": map[string]interface{}{
								"shardsCount":   int64(1),
								"replicasCount": int64(1),
							},
						},
					},
					"settings": map[string]interface{}{
						"max_concurrent_queries": "100",
					},
				},
				"defaults": map[string]interface{}{
					"templates": map[string]interface{}{
						"dataVolumeClaimTemplate": "data-volume-template",
					},
				},
				"templates": map[string]interface{}{
					"volumeClaimTemplates": []interface{}{
						map[string]interface{}{
							"name": "data-volume-template",
							"spec": map[string]interface{}{
								"accessModes": []interface{}{
									"ReadWriteOnce",
								},
								"resources": map[string]interface{}{
									"requests": map[string]interface{}{
										"storage": storageSize,
									},
								},
							},
						},
					},
				},
			},
		},
	}

	// Attempt to get existing ClickHouseInstallation CR
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(chi.GroupVersionKind())
	err := r.Get(ctx, client.ObjectKey{Name: chiName, Namespace: namespace}, existing)
	if errors.IsNotFound(err) {
		logger.Info("creating Altinity ClickHouseInstallation CR", "chi", chiName, "namespace", namespace)
		if createErr := r.Create(ctx, chi); createErr != nil {
			return fmt.Errorf("creating Altinity ClickHouseInstallation %s: %w", chiName, createErr)
		}
		r.setPhase(dr, nestv1.PhaseProvisioning, "Altinity ClickHouseInstallation created")
		return r.Status().Update(ctx, dr)
	}
	if err != nil {
		return fmt.Errorf("getting Altinity ClickHouseInstallation %s: %w", chiName, err)
	}

	// Check if the ClickHouseInstallation is ready
	// Look for status.status == "Completed"
	statusStr, _, _ := unstructured.NestedString(existing.Object, "status", "status")
	ready := statusStr == "Completed"

	if ready {
		// Extract ClickHouse endpoint (chi-{name}-default-0-0 is the pod name)
		endpoint := fmt.Sprintf("chi-%s-default-0-0.%s.svc.cluster.local:9000", chiName, namespace)
		dr.Status.Endpoints = &nestv1.ResourceEndpoints{
			Native: endpoint,
		}
		r.setPhase(dr, nestv1.PhaseReady, "Altinity ClickHouseInstallation ready")
	} else {
		r.setPhase(dr, nestv1.PhaseProvisioning, fmt.Sprintf("Waiting for Altinity ClickHouseInstallation to be ready (status: %s)", statusStr))
	}

	return r.Status().Update(ctx, dr)
}

// reconcileClickhouseDelete removes the Altinity ClickHouseInstallation CR on DataResource deletion.
func (r *DataResourceReconciler) reconcileClickhouseDelete(ctx context.Context, dr *nestv1.DataResource) error {
	chiName := clickhouseClusterName(dr)
	namespace := clickhouseNamespace(dr)

	chi := &unstructured.Unstructured{}
	chi.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "clickhouse.altinity.com", Version: "v1", Kind: "ClickHouseInstallation",
	})
	chi.SetName(chiName)
	chi.SetNamespace(namespace)

	if err := r.Delete(ctx, chi); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting Altinity ClickHouseInstallation %s: %w", chiName, err)
	}
	return nil
}

func clickhouseClusterName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-%s", dr.Spec.Tenant, dr.Name)
}

func clickhouseNamespace(dr *nestv1.DataResource) string {
	return dr.Spec.Tenant
}

func clickhouseStorageSize(dr *nestv1.DataResource) string {
	if dr.Spec.Size != nil && dr.Spec.Size.Storage != "" {
		q, err := resource.ParseQuantity(dr.Spec.Size.Storage)
		if err == nil {
			return q.String()
		}
	}
	return "10Gi"
}
