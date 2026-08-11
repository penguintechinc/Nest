package controllers

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	intstr "k8s.io/apimachinery/pkg/util/intstr"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// reconcileTimeseries reconciles a timeseries DataResource by creating/updating
// a VictoriaMetrics StatefulSet + headless Service in the tenant namespace.
func (r *DataResourceReconciler) reconcileTimeseries(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	// Ensure tenant namespace exists
	if err := r.reconcileTimeseriesNamespace(ctx, dr.Spec.Tenant); err != nil {
		return err
	}

	serviceName := timeseriesServiceName(dr)
	statefulSetName := timeseriesStatefulSetName(dr)
	namespace := dr.Spec.Tenant

	// Get storage size
	storageSize := timeseriesStorageSize(dr)

	// Create headless Service
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
				"app":                              "victoria-metrics",
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
				"app":                              "victoria-metrics",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8428,
					TargetPort: intstr.FromInt32(8428),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	existingSvc := &corev1.Service{}
	err := r.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: namespace}, existingSvc)
	if errors.IsNotFound(err) {
		logger.Info("creating VictoriaMetrics headless Service", "name", svc.Name, "namespace", namespace)
		if err := r.Create(ctx, svc); err != nil {
			return fmt.Errorf("creating VictoriaMetrics Service %s: %w", svc.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("getting VictoriaMetrics Service %s: %w", svc.Name, err)
	}

	// Create StatefulSet
	replicas := int32(1)
	ss := &appsv1.StatefulSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      statefulSetName,
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
				"app":                              "victoria-metrics",
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
					"app":                              "victoria-metrics",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"nest.penguintech.io/tenant":       dr.Spec.Tenant,
						"nest.penguintech.io/dataresource": dr.Name,
						"app":                              "victoria-metrics",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "victoria-metrics",
							Image: "victoriametrics/victoria-metrics:v1.103.0",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8428,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							Command: []string{
								"/victoria-metrics-prod",
								"-storageDataPath=/var/lib/victoria-metrics",
								"-retentionPeriod=12",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "data",
									MountPath: "/var/lib/victoria-metrics",
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/-/ready",
										Port:   intstr.FromInt32(8428),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       5,
								TimeoutSeconds:      3,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/-/healthy",
										Port:   intstr.FromInt32(8428),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
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
									corev1.ResourceCPU:    resource.MustParse("1000m"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
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
		logger.Info("creating VictoriaMetrics StatefulSet", "name", ss.Name, "namespace", namespace)
		if err := r.Create(ctx, ss); err != nil {
			return fmt.Errorf("creating VictoriaMetrics StatefulSet %s: %w", ss.Name, err)
		}
		r.setPhase(dr, nestv1.PhaseProvisioning, "VictoriaMetrics StatefulSet created")
		return r.Status().Update(ctx, dr)
	}
	if err != nil {
		return fmt.Errorf("getting VictoriaMetrics StatefulSet %s: %w", ss.Name, err)
	}

	// Check if StatefulSet is ready
	if existingSS.Status.ReadyReplicas >= replicas {
		endpoint := fmt.Sprintf("http://%s.%s.svc.cluster.local:8428", serviceName, namespace)
		dr.Status.Endpoints = &nestv1.ResourceEndpoints{
			Native: endpoint,
		}
		r.setPhase(dr, nestv1.PhaseReady, fmt.Sprintf("VictoriaMetrics ready (%d/%d replicas)", existingSS.Status.ReadyReplicas, replicas))
	} else {
		r.setPhase(dr, nestv1.PhaseProvisioning, fmt.Sprintf("Waiting for VictoriaMetrics replicas (%d/%d ready)", existingSS.Status.ReadyReplicas, replicas))
	}

	return r.Status().Update(ctx, dr)
}

// reconcileTimeseriesDelete removes the VictoriaMetrics StatefulSet and Service on DataResource deletion.
func (r *DataResourceReconciler) reconcileTimeseriesDelete(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)
	namespace := dr.Spec.Tenant

	// Delete StatefulSet
	ss := &appsv1.StatefulSet{}
	ss.Name = timeseriesStatefulSetName(dr)
	ss.Namespace = namespace
	if err := r.Delete(ctx, ss); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete VictoriaMetrics StatefulSet", "name", ss.Name)
		return err
	}

	// Delete Service
	svc := &corev1.Service{}
	svc.Name = timeseriesServiceName(dr)
	svc.Namespace = namespace
	if err := r.Delete(ctx, svc); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete VictoriaMetrics Service", "name", svc.Name)
		return err
	}

	return nil
}

// reconcileTimeseriesNamespace creates the tenant namespace if it doesn't exist.
func (r *DataResourceReconciler) reconcileTimeseriesNamespace(ctx context.Context, ns string) error {
	return r.ensureTenantNamespace(ctx, ns)
}

// Helper functions for Timeseries reconciliation

func timeseriesServiceName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-%s-vm", dr.Spec.Tenant, dr.Name)
}

func timeseriesStatefulSetName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-%s-vm", dr.Spec.Tenant, dr.Name)
}

func timeseriesStorageSize(dr *nestv1.DataResource) string {
	if dr.Spec.Size != nil && dr.Spec.Size.Storage != "" {
		q, err := resource.ParseQuantity(dr.Spec.Size.Storage)
		if err == nil {
			return q.String()
		}
	}
	return "50Gi"
}
