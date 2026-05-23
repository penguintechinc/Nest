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

// mariadbGVR is the GVR for MariaDB Galera operator CRs.
var mariadbGVR = schema.GroupVersionResource{
	Group:    "mariadb.mmontes.io",
	Version:  "v1alpha1",
	Resource: "mariadbs",
}

// reconcileMariaDB reconciles a mariadb DataResource by creating/updating
// a MariaDB Galera CR. Uses the dynamic client because the MariaDB operator
// CRDs are not vendored into this binary.
func (r *DataResourceReconciler) reconcileMariaDB(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	// Ensure tenant namespace exists
	if err := r.ensureMariaDBNamespace(ctx, dr.Spec.Tenant); err != nil {
		return err
	}

	clusterName := mariadbClusterName(dr)
	namespace := dr.Spec.Tenant

	// Default to 3 replicas for Galera quorum
	replicas := int64(3)
	if dr.Spec.Replicas != nil && dr.Spec.Replicas.Write != nil && dr.Spec.Replicas.Write.Min > 0 {
		writeMin := int64(dr.Spec.Replicas.Write.Min)
		if writeMin >= 3 {
			replicas = writeMin
		}
	}

	storageSize := mariadbStorageSize(dr)
	rootPassword := fmt.Sprintf("nest-%s-%s-root", dr.Spec.Tenant, dr.Name)

	// Create root password Secret
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("mariadb-%s-%s-root", dr.Spec.Tenant, dr.Name),
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
			"password": []byte(rootPassword),
		},
	}

	// Create or update Secret
	existingSecret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: secret.Name, Namespace: namespace}, existingSecret)
	if errors.IsNotFound(err) {
		logger.Info("creating MariaDB root password Secret", "secret", secret.Name)
		if createErr := r.Create(ctx, secret); createErr != nil {
			return fmt.Errorf("creating MariaDB root password Secret %s: %w", secret.Name, createErr)
		}
	} else if err != nil {
		return fmt.Errorf("getting MariaDB root password Secret %s: %w", secret.Name, err)
	}

	// Create MariaDB Galera CR
	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "mariadb.mmontes.io/v1alpha1",
			"kind":       "MariaDB",
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
				"rootPasswordSecretKeyRef": map[string]interface{}{
					"name": secret.Name,
					"key":  "password",
				},
				"image":    "docker.io/mariadb:11.4",
				"port":     int64(3306),
				"replicas": replicas,
				"galera": map[string]interface{}{
					"enabled": true,
					"primary": map[string]interface{}{
						"podIndex":          int64(0),
						"automaticFailover": true,
					},
					"recovery": map[string]interface{}{
						"enabled": true,
					},
				},
				"storage": map[string]interface{}{
					"size": storageSize,
				},
				"resources": map[string]interface{}{
					"requests": map[string]interface{}{
						"cpu":    "250m",
						"memory": "512Mi",
					},
					"limits": map[string]interface{}{
						"cpu":    "1000m",
						"memory": "2Gi",
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
		logger.Info("creating MariaDB Galera cluster", "cluster", clusterName, "namespace", namespace)
		if createErr := r.Create(ctx, cluster); createErr != nil {
			return fmt.Errorf("creating MariaDB Galera cluster %s: %w", clusterName, createErr)
		}
		r.setPhase(dr, nestv1.PhaseProvisioning, "MariaDB Galera cluster created")
		return r.Status().Update(ctx, dr)
	}
	if err != nil {
		return fmt.Errorf("getting MariaDB Galera cluster %s: %w", clusterName, err)
	}

	// Check if the cluster is ready
	ready, _, _ := unstructured.NestedBool(existing.Object, "status", "ready")
	if ready {
		endpoint := fmt.Sprintf("mysql://%s.%s.svc.cluster.local:3306/%s", clusterName, namespace, dr.Spec.Tenant)

		dr.Status.Endpoints = &nestv1.ResourceEndpoints{
			Native: endpoint,
		}
		r.setPhase(dr, nestv1.PhaseReady, "MariaDB Galera cluster ready")
	} else {
		r.setPhase(dr, nestv1.PhaseProvisioning, "Waiting for MariaDB Galera cluster to be ready")
	}

	return r.Status().Update(ctx, dr)
}

// reconcileMariaDBDelete removes the MariaDB cluster and its Secret on DataResource deletion.
func (r *DataResourceReconciler) reconcileMariaDBDelete(ctx context.Context, dr *nestv1.DataResource) error {
	clusterName := mariadbClusterName(dr)
	namespace := dr.Spec.Tenant

	// Delete MariaDB CR
	cluster := &unstructured.Unstructured{}
	cluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "mariadb.mmontes.io", Version: "v1alpha1", Kind: "MariaDB",
	})
	cluster.SetName(clusterName)
	cluster.SetNamespace(namespace)

	if err := r.Delete(ctx, cluster); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting MariaDB cluster %s: %w", clusterName, err)
	}

	// Delete Secret
	secret := &corev1.Secret{}
	secret.Name = fmt.Sprintf("mariadb-%s-%s-root", dr.Spec.Tenant, dr.Name)
	secret.Namespace = namespace

	if err := r.Delete(ctx, secret); err != nil && !errors.IsNotFound(err) {
		return fmt.Errorf("deleting MariaDB root password Secret %s: %w", secret.Name, err)
	}

	return nil
}

// reconcileDblbConfigMariaDB creates/updates the DBLB ConfigMap for a MariaDB DataResource.
// For Galera, all nodes are writable, so read_write_split is disabled.
func (r *DataResourceReconciler) reconcileDblbConfigMariaDB(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	clusterName := mariadbClusterName(dr)
	namespace := dr.Spec.Tenant

	primaryDSN := fmt.Sprintf("host=%s.%s.svc.cluster.local port=3306 dbname=%s user=%s",
		clusterName, namespace, dr.Spec.Tenant, dr.Spec.Tenant)

	configData := fmt.Sprintf(`[dblb]
tenant = %s
resource = %s
pool_mode = transaction
pool_size = 20
max_client_conn = 10000

[upstream_primary]
dsn = %s

[read_write_split]
enabled = false

[rate_limits]
ops_per_sec = 10000
connections_max = 100
`,
		dr.Spec.Tenant, dr.Name,
		primaryDSN,
	)

	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      fmt.Sprintf("dblb-mariadb-%s-%s", dr.Spec.Tenant, dr.Name),
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
		logger.Info("creating MariaDB DBLB ConfigMap", "name", cm.Name)
		return r.Create(ctx, cm)
	}
	if err != nil {
		return err
	}

	existing.Data = cm.Data
	return r.Update(ctx, existing)
}

func mariadbClusterName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-%s-galera", dr.Spec.Tenant, dr.Name)
}

func mariadbStorageSize(dr *nestv1.DataResource) string {
	if dr.Spec.Size != nil && dr.Spec.Size.Storage != "" {
		q, err := resource.ParseQuantity(dr.Spec.Size.Storage)
		if err == nil {
			return q.String()
		}
	}
	return "20Gi"
}

// ensureMariaDBNamespace creates the tenant namespace if it doesn't exist.
func (r *DataResourceReconciler) ensureMariaDBNamespace(ctx context.Context, ns string) error {
	namespace := &corev1.Namespace{}
	namespace.Name = ns
	if err := r.Create(ctx, namespace); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("creating namespace %s: %w", ns, err)
	}
	return nil
}
