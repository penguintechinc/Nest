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
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// mysqlGVR is the GVR for MySQL InnoDB Operator CRs.
var mysqlGVR = schema.GroupVersionResource{
	Group:    "mysql.oracle.com",
	Version:  "v2",
	Resource: "innodBclusters",
}

// reconcileMySQL reconciles a mysql DataResource by creating/updating
// an InnoDBCluster CR. Uses the dynamic client because the MySQL operator
// CRDs are not vendored into this binary.
func (r *DataResourceReconciler) reconcileMySQL(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	// Ensure tenant namespace exists
	if err := r.ensureMySQLNamespace(ctx, dr.Spec.Tenant); err != nil {
		return err
	}

	clusterName := mysqlClusterName(dr)
	namespace := dr.Spec.Tenant

	// Default to 1 primary instance
	instances := int64(1)
	if dr.Spec.Replicas != nil && dr.Spec.Replicas.Write != nil && dr.Spec.Replicas.Write.Min > 0 {
		instances = int64(dr.Spec.Replicas.Write.Min)
	}

	// Default to 1 read replica
	readReplicas := int64(1)
	if dr.Spec.Replicas != nil && dr.Spec.Replicas.Read != nil && dr.Spec.Replicas.Read.Default > 0 {
		readReplicas = int64(dr.Spec.Replicas.Read.Default)
	}

	totalInstances := instances + readReplicas
	storageSize := mysqlStorageSize(dr)
	rootPassword := fmt.Sprintf("nest-%s-%s-root", dr.Spec.Tenant, dr.Name)

	// Create root password Secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("mysql-%s-%s-root", dr.Spec.Tenant, dr.Name),
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
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"rootPassword": []byte(rootPassword),
		},
	}

	// Create or update Secret
	existingSecret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: secret.Name, Namespace: namespace}, existingSecret)
	if errors.IsNotFound(err) {
		logger.Info("creating MySQL root password Secret", "secret", secret.Name)
		if createErr := r.Create(ctx, secret); createErr != nil {
			return fmt.Errorf("creating MySQL root password Secret %s: %w", secret.Name, createErr)
		}
	} else if err != nil {
		return fmt.Errorf("getting MySQL root password Secret %s: %w", secret.Name, err)
	}

	// Create InnoDBCluster CR
	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "mysql.oracle.com/v2",
			"kind":       "InnoDBCluster",
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
				"secretName":       secret.Name,
				"tlsUseSelfSigned": true,
				"instances":        totalInstances,
				"router": map[string]interface{}{
					"instances": int64(1),
				},
				"datadirVolumeClaimTemplate": map[string]interface{}{
					"accessModes": []interface{}{"ReadWriteOnce"},
					"resources": map[string]interface{}{
						"requests": map[string]interface{}{
							"storage": storageSize,
						},
					},
				},
			},
		},
	}

	// Attempt to get existing cluster
	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(cluster.GroupVersionKind())
	err = r.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: namespace}, existing)
	if errors.IsNotFound(err) {
		logger.Info("creating MySQL InnoDB cluster", "cluster", clusterName, "namespace", namespace)
		if createErr := r.Create(ctx, cluster); createErr != nil {
			return fmt.Errorf("creating MySQL InnoDB cluster %s: %w", clusterName, createErr)
		}
		r.setPhase(dr, nestv1.PhaseProvisioning, "MySQL InnoDB cluster created")
		return r.Status().Update(ctx, dr)
	}
	if err != nil {
		return fmt.Errorf("getting MySQL InnoDB cluster %s: %w", clusterName, err)
	}

	// Check if the cluster is ready (status.cluster.status == "ONLINE")
	clusterStatus, _, _ := unstructured.NestedString(existing.Object, "status", "cluster", "status")
	if clusterStatus == "ONLINE" {
		// Router ports: 6446 = RW, 6447 = RO
		endpoint := fmt.Sprintf("mysql://%s.%s.svc.cluster.local:6446/%s", clusterName, namespace, dr.Spec.Tenant)

		dr.Status.Endpoints = &nestv1.ResourceEndpoints{
			Native: endpoint,
		}
		r.setPhase(dr, nestv1.PhaseReady, "MySQL InnoDB cluster ready")
	} else {
		r.setPhase(dr, nestv1.PhaseProvisioning, fmt.Sprintf("Waiting for MySQL InnoDB cluster (status: %s)", clusterStatus))
	}

	return r.Status().Update(ctx, dr)
}

// reconcileMySQLDelete removes the InnoDBCluster and its Secret on DataResource deletion.
func (r *DataResourceReconciler) reconcileMySQLDelete(ctx context.Context, dr *nestv1.DataResource) error {
	clusterName := mysqlClusterName(dr)
	namespace := dr.Spec.Tenant

	// Delete InnoDBCluster CR
	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "mysql.oracle.com", Version: "v2", Kind: "InnoDBCluster",
	})
	cluster.SetName(clusterName)
	cluster.SetNamespace(namespace)

	if err := r.Delete(ctx, cluster); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting MySQL InnoDB cluster %s: %w", clusterName, err)
	}

	// Delete Secret
	secret := &corev1.Secret{}
	secret.Name = fmt.Sprintf("mysql-%s-%s-root", dr.Spec.Tenant, dr.Name)
	secret.Namespace = namespace

	if err := r.Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting MySQL root password Secret %s: %w", secret.Name, err)
	}

	return nil
}

// reconcileDblbConfigMySQL creates/updates the DBLB ConfigMap for a MySQL DataResource.
// Uses router RW (6446) and RO (6447) ports with read_write_split enabled.
func (r *DataResourceReconciler) reconcileDblbConfigMySQL(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	clusterName := mysqlClusterName(dr)
	namespace := dr.Spec.Tenant

	primaryDSN := fmt.Sprintf("host=%s.%s.svc.cluster.local port=6446 dbname=%s user=%s",
		clusterName, namespace, dr.Spec.Tenant, dr.Spec.Tenant)
	replicaDSN := fmt.Sprintf("host=%s.%s.svc.cluster.local port=6447 dbname=%s user=%s",
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
			Name:      fmt.Sprintf("dblb-mysql-%s-%s", dr.Spec.Tenant, dr.Name),
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
		logger.Info("creating MySQL DBLB ConfigMap", "name", cm.Name)
		return r.Create(ctx, cm)
	}
	if err != nil {
		return err
	}

	existing.Data = cm.Data
	return r.Update(ctx, existing)
}

func mysqlClusterName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-%s-mysql", dr.Spec.Tenant, dr.Name)
}

func mysqlStorageSize(dr *nestv1.DataResource) string {
	if dr.Spec.Size != nil && dr.Spec.Size.Storage != "" {
		q, err := resource.ParseQuantity(dr.Spec.Size.Storage)
		if err == nil {
			return q.String()
		}
	}
	return "20Gi"
}

// ensureMySQLNamespace creates the tenant namespace if it doesn't exist.
func (r *DataResourceReconciler) ensureMySQLNamespace(ctx context.Context, ns string) error {
	namespace := &corev1.Namespace{}
	namespace.Name = ns
	if err := r.Create(ctx, namespace); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("creating namespace %s: %w", ns, err)
	}
	return nil
}
