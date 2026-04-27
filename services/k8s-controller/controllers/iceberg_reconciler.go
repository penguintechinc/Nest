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

// cloudNativePGGVR is the GVR for CloudNativePG Cluster CRs (used by Iceberg catalog backend).
var icebergCloudNativePGGVR = schema.GroupVersionResource{
	Group:    "postgresql.cnpg.io",
	Version:  "v1",
	Resource: "clusters",
}

// reconcileIceberg reconciles a lakehouse/iceberg DataResource by creating/updating:
// - A CloudNativePG Cluster for the Iceberg catalog metadata store
// - An Iceberg REST Catalog Deployment
// - A Service for the Iceberg REST API
func (r *DataResourceReconciler) reconcileIceberg(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	// Ensure namespace exists
	if err := r.ensurePostgresNamespace(ctx, icebergNamespace(dr)); err != nil {
		return err
	}

	namespace := icebergNamespace(dr)

	// Create CloudNativePG cluster for Iceberg catalog metadata
	if err := r.reconcileIcebergPostgres(ctx, dr); err != nil {
		return err
	}

	// Create Iceberg REST Catalog Deployment
	deployment := r.icebergRestDeployment(dr)
	existingDeployment := &appsv1.Deployment{}
	err := r.Get(ctx, client.ObjectKey{Name: deployment.Name, Namespace: namespace}, existingDeployment)
	if errors.IsNotFound(err) {
		logger.Info("creating Iceberg REST Deployment", "name", deployment.Name, "namespace", namespace)
		if err := r.Create(ctx, deployment); err != nil {
			return fmt.Errorf("creating Iceberg REST Deployment %s: %w", deployment.Name, err)
		}
		r.setPhase(dr, nestv1.PhaseProvisioning, "Iceberg REST Deployment created")
		return r.Status().Update(ctx, dr)
	}
	if err != nil {
		return fmt.Errorf("getting Iceberg REST Deployment %s: %w", deployment.Name, err)
	}

	// Create Iceberg REST Service
	svc := r.icebergRestService(dr)
	existingSvc := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: namespace}, existingSvc)
	if errors.IsNotFound(err) {
		logger.Info("creating Iceberg REST Service", "name", svc.Name, "namespace", namespace)
		if err := r.Create(ctx, svc); err != nil {
			return fmt.Errorf("creating Iceberg REST Service %s: %w", svc.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("getting Iceberg REST Service %s: %w", svc.Name, err)
	}

	// Check if Deployment is ready
	if existingDeployment.Status.AvailableReplicas > 0 {
		endpoint := fmt.Sprintf("%s.%s.svc.cluster.local:8181", icebergRestName(dr), namespace)
		dr.Status.Endpoints = &nestv1.ResourceEndpoints{
			REST: endpoint,
		}
		r.setPhase(dr, nestv1.PhaseReady, fmt.Sprintf("Iceberg REST ready (%d/%d replicas)", existingDeployment.Status.AvailableReplicas, 1))
	} else {
		r.setPhase(dr, nestv1.PhaseProvisioning, fmt.Sprintf("Waiting for Iceberg REST replicas (%d/%d ready)", existingDeployment.Status.AvailableReplicas, 1))
	}

	return r.Status().Update(ctx, dr)
}

// reconcileIcebergPostgres creates/updates the CloudNativePG cluster for Iceberg catalog metadata.
func (r *DataResourceReconciler) reconcileIcebergPostgres(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	clusterName := icebergPGClusterName(dr)
	namespace := icebergNamespace(dr)

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
					"nest.penguintech.io/component":    "iceberg-catalog",
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
					"size": "10Gi",
				},
				"postgresql": map[string]interface{}{
					"pg_hba": []interface{}{
						"hostssl all all 0.0.0.0/0 scram-sha-256",
					},
				},
				"bootstrap": map[string]interface{}{
					"initdb": map[string]interface{}{
						"database": "iceberg",
						"owner":    "iceberg",
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
		logger.Info("creating Iceberg Postgres Cluster", "cluster", clusterName, "namespace", namespace)
		if createErr := r.Create(ctx, cluster); createErr != nil {
			return fmt.Errorf("creating CloudNativePG Cluster for Iceberg %s: %w", clusterName, createErr)
		}
		return nil
	}
	if err != nil {
		return fmt.Errorf("getting CloudNativePG Cluster for Iceberg %s: %w", clusterName, err)
	}

	// Cluster exists; no update needed for this reconciliation
	return nil
}

// reconcileIcebergDelete removes the Iceberg REST Deployment, Service, and Postgres cluster on DataResource deletion.
func (r *DataResourceReconciler) reconcileIcebergDelete(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)
	namespace := icebergNamespace(dr)

	// Delete Iceberg REST Deployment
	deployment := &appsv1.Deployment{}
	deployment.Name = icebergRestName(dr)
	deployment.Namespace = namespace
	if err := r.Delete(ctx, deployment); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete Iceberg REST Deployment", "name", deployment.Name)
		return err
	}

	// Delete Iceberg REST Service
	svc := &corev1.Service{}
	svc.Name = icebergRestName(dr)
	svc.Namespace = namespace
	if err := r.Delete(ctx, svc); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete Iceberg REST Service", "name", svc.Name)
		return err
	}

	// Delete CloudNativePG cluster
	pgCluster := &unstructured.Unstructured{}
	pgCluster.SetGroupVersionKind(schema.GroupVersionKind{
		Group: "postgresql.cnpg.io", Version: "v1", Kind: "Cluster",
	})
	pgCluster.SetName(icebergPGClusterName(dr))
	pgCluster.SetNamespace(namespace)
	if err := r.Delete(ctx, pgCluster); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete Iceberg Postgres Cluster", "name", icebergPGClusterName(dr))
		return err
	}

	return nil
}

// icebergRestDeployment constructs the Iceberg REST Catalog Deployment.
func (r *DataResourceReconciler) icebergRestDeployment(dr *nestv1.DataResource) *appsv1.Deployment {
	namespace := icebergNamespace(dr)
	name := icebergRestName(dr)
	replicas := int32(1)

	return &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
				"app":                              "iceberg-rest",
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
					"app":                              "iceberg-rest",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"nest.penguintech.io/tenant":       dr.Spec.Tenant,
						"nest.penguintech.io/dataresource": dr.Name,
						"app":                              "iceberg-rest",
					},
				},
				Spec: corev1.PodSpec{
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: boolPtr(true),
						RunAsUser:    int64Ptr(1000),
					},
					Containers: []corev1.Container{
						{
							Name:  "iceberg-rest",
							Image: "tabulario/iceberg-rest:0.10.0",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8181,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Env: []corev1.EnvVar{
								{
									Name:  "CATALOG_CATALOG__IMPL",
									Value: "org.apache.iceberg.jdbc.JdbcCatalog",
								},
								{
									Name:  "CATALOG_URI",
									Value: fmt.Sprintf("jdbc:postgresql://%s-rw.%s.svc.cluster.local:5432/iceberg", icebergPGClusterName(dr), namespace),
								},
								{
									Name:  "CATALOG_JDBC_USER",
									Value: "iceberg",
								},
								{
									Name:  "CATALOG_JDBC_PASSWORD",
									Value: "iceberg",
								},
								{
									Name:  "CATALOG_WAREHOUSE",
									Value: fmt.Sprintf("s3://%s-warehouse/", dr.Name),
								},
								{
									Name:  "CATALOG_S3_ENDPOINT",
									Value: "http://rook-ceph-rgw-nest.rook-ceph.svc.cluster.local:80",
								},
								{
									Name:  "CATALOG_S3_ACCESS_KEY_ID",
									Value: "nest-iceberg",
								},
								{
									Name:  "CATALOG_S3_SECRET_ACCESS_KEY",
									Value: "nest-iceberg-secret",
								},
								{
									Name:  "AWS_REGION",
									Value: "us-east-1",
								},
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot:             boolPtr(true),
								RunAsUser:                int64Ptr(1000),
								AllowPrivilegeEscalation: boolPtr(false),
								ReadOnlyRootFilesystem:   boolPtr(true),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/v1/config",
										Port:   intstr.FromInt32(8181),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 20,
								PeriodSeconds:       10,
								TimeoutSeconds:      3,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/v1/config",
										Port:   intstr.FromInt32(8181),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 40,
								PeriodSeconds:       30,
								TimeoutSeconds:      3,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("200m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
						},
					},
				},
			},
		},
	}
}

// icebergRestService constructs the Iceberg REST Catalog Service.
func (r *DataResourceReconciler) icebergRestService(dr *nestv1.DataResource) *corev1.Service {
	namespace := icebergNamespace(dr)
	name := icebergRestName(dr)

	return &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
				"app":                              "iceberg-rest",
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
				"app":                              "iceberg-rest",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8181,
					TargetPort: intstr.FromInt32(8181),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}
}

// Helper functions for Iceberg reconciliation

func icebergNamespace(dr *nestv1.DataResource) string {
	return dr.Spec.Tenant
}

func icebergRestName(dr *nestv1.DataResource) string {
	name := fmt.Sprintf("%s-%s-iceberg-rest", dr.Spec.Tenant, dr.Name)
	// Truncate to K8s max name length (63 chars)
	if len(name) > 63 {
		name = name[:63]
	}
	return name
}

func icebergPGClusterName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-%s-iceberg-pg", dr.Spec.Tenant, dr.Name)
}
