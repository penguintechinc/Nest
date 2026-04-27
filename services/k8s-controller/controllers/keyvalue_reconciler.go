package controllers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// reconcileKeyvalue reconciles a keyvalue DataResource by creating/updating
// a Valkey StatefulSet + headless Service + ConfigMap in the tenant namespace.
func (r *DataResourceReconciler) reconcileKeyvalue(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	// Ensure tenant namespace exists
	if err := r.reconcileKeyvalueNamespace(ctx, dr.Spec.Tenant); err != nil {
		return err
	}

	serviceName := keyvalueServiceName(dr)
	statefulSetName := keyvalueStatefulSetName(dr)
	configMapName := keyvalueConfigMapName(dr)
	namespace := dr.Spec.Tenant

	// Determine replica count (default 1)
	replicas := int32(1)
	if dr.Spec.Replicas != nil && dr.Spec.Replicas.Write != nil && dr.Spec.Replicas.Write.Default > 0 {
		replicas = dr.Spec.Replicas.Write.Default
	}

	// Get storage size
	storageSize := keyvalueStorageSize(dr)

	// Create ConfigMap
	cm := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      configMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
				"app":                              "valkey",
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
			"valkey.conf": keyvalueConfigContent(dr, storageSize),
		},
	}

	existing := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: cm.Name, Namespace: namespace}, existing)
	if errors.IsNotFound(err) {
		logger.Info("creating Valkey ConfigMap", "name", cm.Name, "namespace", namespace)
		if err := r.Create(ctx, cm); err != nil {
			return fmt.Errorf("creating Valkey ConfigMap %s: %w", cm.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("getting Valkey ConfigMap %s: %w", cm.Name, err)
	} else {
		// Update existing ConfigMap
		existing.Data = cm.Data
		if err := r.Update(ctx, existing); err != nil {
			return fmt.Errorf("updating Valkey ConfigMap %s: %w", cm.Name, err)
		}
	}

	// Create headless Service
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
				"app":                              "valkey",
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
			Type:      corev1.ServiceTypeClusterIP,
			ClusterIP: corev1.ClusterIPNone, // Headless service
			Selector: map[string]string{
				"nest.penguintech.io/dataresource": dr.Name,
				"app":                              "valkey",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "valkey",
					Port:       6379,
					TargetPort: intstr.FromInt32(6379),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	existingSvc := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: namespace}, existingSvc)
	if errors.IsNotFound(err) {
		logger.Info("creating Valkey headless Service", "name", svc.Name, "namespace", namespace)
		if err := r.Create(ctx, svc); err != nil {
			return fmt.Errorf("creating Valkey Service %s: %w", svc.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("getting Valkey Service %s: %w", svc.Name, err)
	}

	// Create StatefulSet
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetName,
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
				"app":                              "valkey",
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
		Spec: appsv1.StatefulSetSpec{
			ServiceName: serviceName,
			Replicas:    &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"nest.penguintech.io/dataresource": dr.Name,
					"app":                              "valkey",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"nest.penguintech.io/tenant":       dr.Spec.Tenant,
						"nest.penguintech.io/dataresource": dr.Name,
						"app":                              "valkey",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "valkey",
							Image: "docker.io/valkey/valkey:8-bookworm",
							Ports: []corev1.ContainerPort{
								{
									Name:          "valkey",
									ContainerPort: 6379,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Command: []string{
								"valkey-server",
								"/etc/valkey/valkey.conf",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/valkey",
									ReadOnly:  false,
								},
								{
									Name:      "data",
									MountPath: "/data",
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									Exec: &corev1.ExecAction{
										Command: []string{
											"valkey-cli",
											"ping",
										},
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
									Exec: &corev1.ExecAction{
										Command: []string{
											"valkey-cli",
											"ping",
										},
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       10,
								TimeoutSeconds:      1,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("100m"),
									corev1.ResourceMemory: resource.MustParse("128Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("512Mi"),
								},
							},
						},
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: boolPtr(true),
						RunAsUser:    int64Ptr(999),
						FSGroup:      int64Ptr(999),
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: configMapName,
									},
									DefaultMode: int32Ptr(0644),
								},
							},
						},
					},
				},
			},
			VolumeClaimTemplates: []corev1.PersistentVolumeClaim{
				{
					ObjectMeta: metav1.ObjectMeta{
						Name: "data",
					},
					Spec: corev1.PersistentVolumeClaimSpec{
						AccessModes: []corev1.PersistentVolumeAccessMode{
							corev1.ReadWriteOnce,
						},
						Resources: corev1.VolumeResourceRequirements{
							Requests: corev1.ResourceList{
								corev1.ResourceStorage: resource.MustParse(storageSize),
							},
						},
					},
				},
			},
		},
	}

	existingSS := &appsv1.StatefulSet{}
	err = r.Get(ctx, client.ObjectKey{Name: ss.Name, Namespace: namespace}, existingSS)
	if errors.IsNotFound(err) {
		logger.Info("creating Valkey StatefulSet", "name", ss.Name, "namespace", namespace)
		if err := r.Create(ctx, ss); err != nil {
			return fmt.Errorf("creating Valkey StatefulSet %s: %w", ss.Name, err)
		}
		r.setPhase(dr, nestv1.PhaseProvisioning, "Valkey StatefulSet created")
		return r.Status().Update(ctx, dr)
	}
	if err != nil {
		return fmt.Errorf("getting Valkey StatefulSet %s: %w", ss.Name, err)
	}

	// Check if StatefulSet is ready
	if existingSS.Status.ReadyReplicas >= replicas {
		endpoint := fmt.Sprintf("%s.%s.svc.cluster.local:6379", serviceName, namespace)
		dr.Status.Endpoints = &nestv1.ResourceEndpoints{
			Native: endpoint,
		}
		r.setPhase(dr, nestv1.PhaseReady, fmt.Sprintf("Valkey ready (%d/%d replicas)", existingSS.Status.ReadyReplicas, replicas))
	} else {
		r.setPhase(dr, nestv1.PhaseProvisioning, fmt.Sprintf("Waiting for Valkey replicas (%d/%d ready)", existingSS.Status.ReadyReplicas, replicas))
	}

	return r.Status().Update(ctx, dr)
}

// reconcileKeyvalueDelete removes the Valkey StatefulSet, Service, and ConfigMap on DataResource deletion.
func (r *DataResourceReconciler) reconcileKeyvalueDelete(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)
	namespace := dr.Spec.Tenant

	// Delete StatefulSet
	ss := &appsv1.StatefulSet{}
	ss.Name = keyvalueStatefulSetName(dr)
	ss.Namespace = namespace
	if err := r.Delete(ctx, ss); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete Valkey StatefulSet", "name", ss.Name)
		return err
	}

	// Delete Service
	svc := &corev1.Service{}
	svc.Name = keyvalueServiceName(dr)
	svc.Namespace = namespace
	if err := r.Delete(ctx, svc); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete Valkey Service", "name", svc.Name)
		return err
	}

	// Delete ConfigMap
	cm := &corev1.ConfigMap{}
	cm.Name = keyvalueConfigMapName(dr)
	cm.Namespace = namespace
	if err := r.Delete(ctx, cm); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete Valkey ConfigMap", "name", cm.Name)
		return err
	}

	return nil
}

// reconcileKeyvalueNamespace creates the tenant namespace if it doesn't exist.
func (r *DataResourceReconciler) reconcileKeyvalueNamespace(ctx context.Context, ns string) error {
	namespace := &corev1.Namespace{}
	namespace.Name = ns
	if err := r.Create(ctx, namespace); err != nil && !errors.IsAlreadyExists(err) {
		return fmt.Errorf("creating namespace %s: %w", ns, err)
	}
	return nil
}

// Helper functions for Keyvalue reconciliation

func keyvalueServiceName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-%s-valkey-headless", dr.Spec.Tenant, dr.Name)
}

func keyvalueStatefulSetName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-%s-valkey", dr.Spec.Tenant, dr.Name)
}

func keyvalueConfigMapName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-%s-valkey-config", dr.Spec.Tenant, dr.Name)
}

func keyvalueStorageSize(dr *nestv1.DataResource) string {
	if dr.Spec.Size != nil && dr.Spec.Size.Storage != "" {
		q, err := resource.ParseQuantity(dr.Spec.Size.Storage)
		if err == nil {
			return q.String()
		}
	}
	return "1Gi"
}

func keyvalueConfigContent(dr *nestv1.DataResource, storageSize string) string {
	// Parse storage size to calculate maxmemory (80% of requested memory)
	// For now, use a reasonable default of 80% of 512Mi limit = 410Mi
	maxmemory := "410mb"

	return fmt.Sprintf(`# Valkey configuration for %s/%s
maxmemory %s
maxmemory-policy allkeys-lru
save ""
appendonly no
`, dr.Spec.Tenant, dr.Name, maxmemory)
}

// Utility functions for pointer creation

func int32Ptr(i int32) *int32 {
	return &i
}

func int64Ptr(i int64) *int64 {
	return &i
}
