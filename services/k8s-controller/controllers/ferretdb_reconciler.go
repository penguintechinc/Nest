package controllers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// cloudNativePGGVK is used for FerretDB backend Postgres clusters.
var ferretdbPostgresGVK = schema.GroupVersionKind{
	Group:   "postgresql.cnpg.io",
	Version: "v1",
	Kind:    "Cluster",
}

// reconcileFerretDB reconciles a rockfs DataResource by:
// 1. Creating/updating a CloudNativePG Cluster for the backend Postgres
// 2. Creating/updating a FerretDB Deployment
// 3. Creating/updating a FerretDB Service on port 27017
func (r *DataResourceReconciler) reconcileFerretDB(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	pgClusterName := ferretdbPGClusterName(dr)
	ferretdbName := ferretdbDeploymentName(dr)
	namespace := ferretdbNamespace(dr)

	// Ensure tenant namespace exists
	if err := r.ensurePostgresNamespace(ctx, namespace); err != nil {
		return err
	}

	storageSize := ferretdbStorageSize(dr)

	// Step 1: Create/update backend CloudNativePG Cluster
	logger.Info("reconciling FerretDB backend Postgres cluster", "cluster", pgClusterName)
	pgCluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "postgresql.cnpg.io/v1",
			"kind":       "Cluster",
			"metadata": map[string]interface{}{
				"name":      pgClusterName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"nest.penguintech.io/tenant":       dr.Spec.Tenant,
					"nest.penguintech.io/dataresource": dr.Name,
					"nest.penguintech.io/component":    "ferretdb-backend",
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
				"instances": int64(1),
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
						"database": "ferretdb",
						"owner":    "ferretdb",
					},
				},
			},
		},
	}

	pgCluster.SetGroupVersionKind(ferretdbPostgresGVK)

	// Get or create Postgres cluster
	existingPG := &unstructured.Unstructured{}
	existingPG.SetGroupVersionKind(ferretdbPostgresGVK)
	err := r.Get(ctx, client.ObjectKey{Name: pgClusterName, Namespace: namespace}, existingPG)
	if errors.IsNotFound(err) {
		logger.Info("creating FerretDB backend Postgres cluster", "cluster", pgClusterName)
		if createErr := r.Create(ctx, pgCluster); createErr != nil {
			return fmt.Errorf("creating FerretDB Postgres cluster %s: %w", pgClusterName, createErr)
		}
		r.setPhase(dr, nestv1.PhaseProvisioning, "FerretDB backend Postgres cluster created")
		return r.Status().Update(ctx, dr)
	}
	if err != nil {
		return fmt.Errorf("getting FerretDB Postgres cluster %s: %w", pgClusterName, err)
	}

	// Check if Postgres cluster is ready
	pgReadyInstances, _, _ := unstructured.NestedInt64(existingPG.Object, "status", "readyInstances")
	if pgReadyInstances < 1 {
		r.setPhase(dr, nestv1.PhaseProvisioning, "Waiting for FerretDB backend Postgres cluster to be ready")
		return r.Status().Update(ctx, dr)
	}

	// Step 2: Create/update FerretDB Service
	pgConnectionURL := fmt.Sprintf("postgres://ferretdb:ferretdb@%s-rw.%s.svc.cluster.local:5432/ferretdb?sslmode=disable",
		pgClusterName, namespace)

	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ferretdbName,
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
				"app":                              "ferretdb",
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
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeClusterIP,
			Selector: map[string]string{
				"nest.penguintech.io/dataresource": dr.Name,
				"app":                              "ferretdb",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "ferretdb",
					Port:       27017,
					TargetPort: intstr.FromInt32(27017),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	existingSvc := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: namespace}, existingSvc)
	if errors.IsNotFound(err) {
		logger.Info("creating FerretDB Service", "name", svc.Name, "namespace", namespace)
		if err := r.Create(ctx, svc); err != nil {
			return fmt.Errorf("creating FerretDB Service %s: %w", svc.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("getting FerretDB Service %s: %w", svc.Name, err)
	}

	// Step 3: Create/update FerretDB Deployment
	replicas := int32(1)
	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ferretdbName,
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
				"app":                              "ferretdb",
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
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"nest.penguintech.io/dataresource": dr.Name,
					"app":                              "ferretdb",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"nest.penguintech.io/tenant":       dr.Spec.Tenant,
						"nest.penguintech.io/dataresource": dr.Name,
						"app":                              "ferretdb",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "ferretdb",
							Image: "ghcr.io/ferretdb/ferretdb:1.21.0",
							Ports: []corev1.ContainerPort{
								{
									Name:          "ferretdb",
									ContainerPort: 27017,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "FERRETDB_POSTGRESQL_URL",
									Value: pgConnectionURL,
								},
								{
									Name:  "FERRETDB_LISTEN_ADDR",
									Value: "0.0.0.0:27017",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt32(27017),
									},
								},
								InitialDelaySeconds: 5,
								PeriodSeconds:       5,
								TimeoutSeconds:      1,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									TCPSocket: &corev1.TCPSocketAction{
										Port: intstr.FromInt32(27017),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								TimeoutSeconds:      1,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot:             boolPtr(true),
								RunAsUser:                int64Ptr(1000),
								AllowPrivilegeEscalation: boolPtr(false),
								ReadOnlyRootFilesystem:   boolPtr(true),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{corev1.Capability("ALL")},
								},
							},
						},
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: boolPtr(true),
						RunAsUser:    int64Ptr(1000),
					},
				},
			},
		},
	}

	existingDeploy := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: deployment.Name, Namespace: namespace}, existingDeploy)
	if errors.IsNotFound(err) {
		logger.Info("creating FerretDB Deployment", "name", deployment.Name, "namespace", namespace)
		if err := r.Create(ctx, deployment); err != nil {
			return fmt.Errorf("creating FerretDB Deployment %s: %w", deployment.Name, err)
		}
		r.setPhase(dr, nestv1.PhaseProvisioning, "FerretDB Deployment created")
		return r.Status().Update(ctx, dr)
	}
	if err != nil {
		return fmt.Errorf("getting FerretDB Deployment %s: %w", deployment.Name, err)
	}

	// Check if FerretDB Deployment is ready
	if existingDeploy.Status.AvailableReplicas >= replicas {
		endpoint := fmt.Sprintf("%s.%s.svc.cluster.local:27017", ferretdbName, namespace)
		dr.Status.Endpoints = &nestv1.ResourceEndpoints{
			Native: endpoint,
		}
		r.setPhase(dr, nestv1.PhaseReady, "FerretDB ready")
	} else {
		r.setPhase(dr, nestv1.PhaseProvisioning, fmt.Sprintf("Waiting for FerretDB replicas (%d/%d available)", existingDeploy.Status.AvailableReplicas, replicas))
	}

	return r.Status().Update(ctx, dr)
}

// reconcileFerretDBDelete removes the FerretDB Deployment, Service, and backend Postgres Cluster on DataResource deletion.
func (r *DataResourceReconciler) reconcileFerretDBDelete(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	pgClusterName := ferretdbPGClusterName(dr)
	ferretdbName := ferretdbDeploymentName(dr)
	namespace := ferretdbNamespace(dr)

	// Delete FerretDB Deployment
	deploy := &appsv1.Deployment{}
	deploy.Name = ferretdbName
	deploy.Namespace = namespace
	if err := r.Delete(ctx, deploy); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete FerretDB Deployment", "name", deploy.Name)
		return err
	}

	// Delete FerretDB Service
	svc := &corev1.Service{}
	svc.Name = ferretdbName
	svc.Namespace = namespace
	if err := r.Delete(ctx, svc); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete FerretDB Service", "name", svc.Name)
		return err
	}

	// Delete backend Postgres Cluster
	pgCluster := &unstructured.Unstructured{}
	pgCluster.SetGroupVersionKind(ferretdbPostgresGVK)
	pgCluster.SetName(pgClusterName)
	pgCluster.SetNamespace(namespace)
	if err := r.Delete(ctx, pgCluster); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete FerretDB Postgres cluster", "name", pgClusterName)
		return err
	}

	return nil
}

// Helper functions for FerretDB reconciliation

func ferretdbPGClusterName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-%s-ferretdb-pg", dr.Spec.Tenant, dr.Name)
}

func ferretdbDeploymentName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-%s-ferretdb", dr.Spec.Tenant, dr.Name)
}

func ferretdbNamespace(dr *nestv1.DataResource) string {
	return dr.Spec.Tenant
}

func ferretdbStorageSize(dr *nestv1.DataResource) string {
	if dr.Spec.Size != nil && dr.Spec.Size.Storage != "" {
		q, err := resource.ParseQuantity(dr.Spec.Size.Storage)
		if err == nil {
			return q.String()
		}
	}
	return "10Gi"
}
