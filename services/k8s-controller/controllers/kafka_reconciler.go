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

// strimziKafkaGVR is the GVR for Strimzi Kafka CRs.
var strimziKafkaGVR = schema.GroupVersionResource{
	Group:    "kafka.strimzi.io",
	Version:  "v1beta2",
	Resource: "kafkas",
}

// reconcileKafka reconciles a kafka DataResource by creating/updating
// a Strimzi Kafka CR. Uses the dynamic client because the Strimzi
// CRDs are not vendored into this binary.
func (r *DataResourceReconciler) reconcileKafka(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	kafkaName := kafkaClusterName(dr)
	namespace := kafkaNamespace(dr)

	// Determine replica count (default 3)
	replicas := int64(3)
	if dr.Spec.Replicas != nil && dr.Spec.Replicas.Write != nil && dr.Spec.Replicas.Write.Default > 0 {
		replicas = int64(dr.Spec.Replicas.Write.Default)
	}

	storageSize := kafkaStorageSize(dr)

	kafka := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "kafka.strimzi.io/v1beta2",
			"kind":       "Kafka",
			"metadata": map[string]interface{}{
				"name":      kafkaName,
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
				"kafka": map[string]interface{}{
					"version":  "3.8.0",
					"replicas": replicas,
					"listeners": []interface{}{
						map[string]interface{}{
							"name": "plain",
							"port": int64(9092),
							"type": "internal",
							"tls":  false,
						},
						map[string]interface{}{
							"name": "tls",
							"port": int64(9093),
							"type": "internal",
							"tls":  true,
						},
					},
					"storage": map[string]interface{}{
						"type": "jbod",
						"volumes": []interface{}{
							map[string]interface{}{
								"id":          int64(0),
								"type":        "persistent-claim",
								"size":        storageSize,
								"deleteClaim": false,
							},
						},
					},
					"config": map[string]interface{}{
						"offsets.topic.replication.factor":         "3",
						"transaction.state.log.replication.factor": "3",
						"transaction.state.log.min.isr":            "2",
						"default.replication.factor":               "3",
						"min.insync.replicas":                      "2",
					},
				},
				"zookeeper": map[string]interface{}{
					"replicas": int64(3),
					"storage": map[string]interface{}{
						"type":        "persistent-claim",
						"size":        "5Gi",
						"deleteClaim": false,
					},
				},
			},
		},
	}

	// Attempt to get existing Kafka CR
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(kafka.GroupVersionKind())
	err := r.Get(ctx, client.ObjectKey{Name: kafkaName, Namespace: namespace}, existing)
	if errors.IsNotFound(err) {
		logger.Info("creating Strimzi Kafka CR", "kafka", kafkaName, "namespace", namespace)
		if createErr := r.Create(ctx, kafka); createErr != nil {
			return fmt.Errorf("creating Strimzi Kafka %s: %w", kafkaName, createErr)
		}
		r.setPhase(dr, nestv1.PhaseProvisioning, "Strimzi Kafka created")
		return r.Status().Update(ctx, dr)
	}
	if err != nil {
		return fmt.Errorf("getting Strimzi Kafka %s: %w", kafkaName, err)
	}

	// Check if the Kafka cluster is ready
	// Look for Ready condition in status.conditions
	conditions, _, _ := unstructured.NestedSlice(existing.Object, "status", "conditions")
	ready := false
	for _, condObj := range conditions {
		if cond, ok := condObj.(map[string]interface{}); ok {
			if condType, ok := cond["type"].(string); ok && condType == "Ready" {
				if status, ok := cond["status"].(string); ok && status == "True" {
					ready = true
					break
				}
			}
		}
	}

	if ready {
		// Extract bootstrap endpoint
		endpoint := fmt.Sprintf("%s-kafka-bootstrap.%s.svc.cluster.local:9092", kafkaName, namespace)
		dr.Status.Endpoints = &nestv1.ResourceEndpoints{
			Native: endpoint,
		}
		r.setPhase(dr, nestv1.PhaseReady, "Strimzi Kafka ready")
	} else {
		r.setPhase(dr, nestv1.PhaseProvisioning, "Waiting for Strimzi Kafka to be ready")
	}

	return r.Status().Update(ctx, dr)
}

// reconcileKafkaDelete removes the Strimzi Kafka CR on DataResource deletion.
func (r *DataResourceReconciler) reconcileKafkaDelete(ctx context.Context, dr *nestv1.DataResource) error {
	kafkaName := kafkaClusterName(dr)
	namespace := kafkaNamespace(dr)

	kafka := &unstructured.Unstructured{}
	kafka.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "kafka.strimzi.io", Version: "v1beta2", Kind: "Kafka",
	})
	kafka.SetName(kafkaName)
	kafka.SetNamespace(namespace)

	if err := r.Delete(ctx, kafka); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting Strimzi Kafka %s: %w", kafkaName, err)
	}
	return nil
}

func kafkaClusterName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-%s", dr.Spec.Tenant, dr.Name)
}

func kafkaNamespace(dr *nestv1.DataResource) string {
	return dr.Spec.Tenant
}

func kafkaStorageSize(dr *nestv1.DataResource) string {
	if dr.Spec.Size != nil && dr.Spec.Size.Storage != "" {
		q, err := resource.ParseQuantity(dr.Spec.Size.Storage)
		if err == nil {
			return q.String()
		}
	}
	return "10Gi"
}
