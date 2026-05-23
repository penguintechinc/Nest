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

// reconcileTrino reconciles a warehouse/trino DataResource by creating/updating
// a Trino coordinator Deployment + worker Deployments + ConfigMaps + Service.
func (r *DataResourceReconciler) reconcileTrino(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	// Ensure tenant namespace exists
	namespace := trinoNamespace(dr)
	if err := r.ensurePostgresNamespace(ctx, namespace); err != nil {
		return err
	}

	coordinatorName := trinoCoordinatorName(dr)
	workerName := trinoWorkerName(dr)
	coordinatorConfigMapName := trinoCoordinatorConfigMapName(dr)
	workerConfigMapName := trinoWorkerConfigMapName(dr)
	serviceName := trinoServiceName(dr)

	// Determine worker replicas (default 2)
	replicas := int32(2)
	if dr.Spec.Replicas != nil && dr.Spec.Replicas.Write != nil && dr.Spec.Replicas.Write.Default > 0 {
		replicas = dr.Spec.Replicas.Write.Default
	}

	// Get iceberg endpoint from annotations or use default
	icebergEndpoint := dr.Spec.Annotations["nest.penguintech.io/iceberg-endpoint"]
	if icebergEndpoint == "" {
		icebergEndpoint = fmt.Sprintf("iceberg-rest.%s.svc.cluster.local:8181", namespace)
	}

	// Create coordinator ConfigMap
	coordinatorCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      coordinatorConfigMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
				"app":                              "trino",
				"component":                        "coordinator",
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
			"config.properties": `coordinator=true
node-scheduler.include-coordinator=false
http-server.http.port=8080
discovery.uri=http://localhost:8080`,
			"jvm.config": `-server
-Xmx2G
-XX:+UseG1GC
-XX:G1HeapRegionSize=32M
-XX:+UseGCOverheadLimit
-XX:+ExplicitGCInvokesConcurrent
-Xbootclasspath/a:${HADOOP_HOME}/share/hadoop/common/lib/hadoop-auth.jar`,
			"log.properties": `io.trino=INFO`,
			"catalog/iceberg.properties": fmt.Sprintf(`connector.name=iceberg
iceberg.catalog.type=rest
iceberg.rest-catalog.uri=http://%s`, icebergEndpoint),
		},
	}

	existingCoordinatorCM := &corev1.ConfigMap{}
	err := r.Get(ctx, client.ObjectKey{Name: coordinatorCM.Name, Namespace: namespace}, existingCoordinatorCM)
	if errors.IsNotFound(err) {
		logger.Info("creating Trino coordinator ConfigMap", "name", coordinatorCM.Name, "namespace", namespace)
		if err := r.Create(ctx, coordinatorCM); err != nil {
			return fmt.Errorf("creating Trino coordinator ConfigMap %s: %w", coordinatorCM.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("getting Trino coordinator ConfigMap %s: %w", coordinatorCM.Name, err)
	} else {
		existingCoordinatorCM.Data = coordinatorCM.Data
		if err := r.Update(ctx, existingCoordinatorCM); err != nil {
			return fmt.Errorf("updating Trino coordinator ConfigMap %s: %w", coordinatorCM.Name, err)
		}
	}

	// Create worker ConfigMap
	workerCM := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workerConfigMapName,
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
				"app":                              "trino",
				"component":                        "worker",
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
			"config.properties": fmt.Sprintf(`coordinator=false
http-server.http.port=8080
discovery.uri=http://%s.%s.svc.cluster.local:8080`, coordinatorName, namespace),
			"jvm.config": `-server
-Xmx2G
-XX:+UseG1GC
-XX:G1HeapRegionSize=32M
-XX:+UseGCOverheadLimit
-XX:+ExplicitGCInvokesConcurrent
-Xbootclasspath/a:${HADOOP_HOME}/share/hadoop/common/lib/hadoop-auth.jar`,
			"log.properties": `io.trino=INFO`,
			"catalog/iceberg.properties": fmt.Sprintf(`connector.name=iceberg
iceberg.catalog.type=rest
iceberg.rest-catalog.uri=http://%s`, icebergEndpoint),
		},
	}

	existingWorkerCM := &corev1.ConfigMap{}
	err = r.Get(ctx, client.ObjectKey{Name: workerCM.Name, Namespace: namespace}, existingWorkerCM)
	if errors.IsNotFound(err) {
		logger.Info("creating Trino worker ConfigMap", "name", workerCM.Name, "namespace", namespace)
		if err := r.Create(ctx, workerCM); err != nil {
			return fmt.Errorf("creating Trino worker ConfigMap %s: %w", workerCM.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("getting Trino worker ConfigMap %s: %w", workerCM.Name, err)
	} else {
		existingWorkerCM.Data = workerCM.Data
		if err := r.Update(ctx, existingWorkerCM); err != nil {
			return fmt.Errorf("updating Trino worker ConfigMap %s: %w", workerCM.Name, err)
		}
	}

	// Create coordinator Deployment
	coordinatorDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      coordinatorName,
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
				"app":                              "trino",
				"component":                        "coordinator",
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
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"nest.penguintech.io/dataresource": dr.Name,
					"app":                              "trino",
					"component":                        "coordinator",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"nest.penguintech.io/tenant":       dr.Spec.Tenant,
						"nest.penguintech.io/dataresource": dr.Name,
						"app":                              "trino",
						"component":                        "coordinator",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "trino",
							Image: "trinodb/trino:435",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8080,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/trino",
									ReadOnly:  true,
								},
								{
									Name:      "tmp",
									MountPath: "/tmp",
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/v1/info",
										Port:   intstr.FromInt32(8080),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/v1/info",
										Port:   intstr.FromInt32(8080),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 60,
								PeriodSeconds:       30,
								TimeoutSeconds:      5,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2000m"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot:             boolPtr(true),
								RunAsUser:                int64Ptr(1000),
								AllowPrivilegeEscalation: boolPtr(false),
								ReadOnlyRootFilesystem:  boolPtr(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
						},
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: boolPtr(true),
						RunAsUser:    int64Ptr(1000),
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: coordinatorConfigMapName,
									},
									DefaultMode: int32Ptr(0644),
								},
							},
						},
						{
							Name: "tmp",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	existingCoordinatorDeploy := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: coordinatorDeploy.Name, Namespace: namespace}, existingCoordinatorDeploy)
	if errors.IsNotFound(err) {
		logger.Info("creating Trino coordinator Deployment", "name", coordinatorDeploy.Name, "namespace", namespace)
		if err := r.Create(ctx, coordinatorDeploy); err != nil {
			return fmt.Errorf("creating Trino coordinator Deployment %s: %w", coordinatorDeploy.Name, err)
		}
		r.setPhase(dr, nestv1.PhaseProvisioning, "Trino coordinator Deployment created")
		return r.Status().Update(ctx, dr)
	}
	if err != nil {
		return fmt.Errorf("getting Trino coordinator Deployment %s: %w", coordinatorDeploy.Name, err)
	}

	// Create worker Deployment
	workerDeploy := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      workerName,
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
				"app":                              "trino",
				"component":                        "worker",
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
					"app":                              "trino",
					"component":                        "worker",
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"nest.penguintech.io/tenant":       dr.Spec.Tenant,
						"nest.penguintech.io/dataresource": dr.Name,
						"app":                              "trino",
						"component":                        "worker",
					},
				},
				Spec: corev1.PodSpec{
					Containers: []corev1.Container{
						{
							Name:  "trino",
							Image: "trinodb/trino:435",
							Ports: []corev1.ContainerPort{
								{
									Name:          "http",
									ContainerPort: 8080,
									Protocol:      corev1.ProtocolTCP,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/trino",
									ReadOnly:  true,
								},
								{
									Name:      "tmp",
									MountPath: "/tmp",
								},
							},
							ReadinessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/v1/info",
										Port:   intstr.FromInt32(8080),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
								TimeoutSeconds:      5,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							LivenessProbe: &corev1.Probe{
								ProbeHandler: corev1.ProbeHandler{
									HTTPGet: &corev1.HTTPGetAction{
										Path:   "/v1/info",
										Port:   intstr.FromInt32(8080),
										Scheme: corev1.URISchemeHTTP,
									},
								},
								InitialDelaySeconds: 60,
								PeriodSeconds:       30,
								TimeoutSeconds:      5,
								SuccessThreshold:    1,
								FailureThreshold:    3,
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("2Gi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2000m"),
									corev1.ResourceMemory: resource.MustParse("4Gi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot:             boolPtr(true),
								RunAsUser:                int64Ptr(1000),
								AllowPrivilegeEscalation: boolPtr(false),
								ReadOnlyRootFilesystem:  boolPtr(false),
								Capabilities: &corev1.Capabilities{
									Drop: []corev1.Capability{"ALL"},
								},
							},
						},
					},
					SecurityContext: &corev1.PodSecurityContext{
						RunAsNonRoot: boolPtr(true),
						RunAsUser:    int64Ptr(1000),
					},
					Volumes: []corev1.Volume{
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: workerConfigMapName,
									},
									DefaultMode: int32Ptr(0644),
								},
							},
						},
						{
							Name: "tmp",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
					},
				},
			},
		},
	}

	existingWorkerDeploy := &appsv1.Deployment{}
	err = r.Get(ctx, client.ObjectKey{Name: workerDeploy.Name, Namespace: namespace}, existingWorkerDeploy)
	if errors.IsNotFound(err) {
		logger.Info("creating Trino worker Deployment", "name", workerDeploy.Name, "namespace", namespace)
		if err := r.Create(ctx, workerDeploy); err != nil {
			return fmt.Errorf("creating Trino worker Deployment %s: %w", workerDeploy.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("getting Trino worker Deployment %s: %w", workerDeploy.Name, err)
	}

	// Create Service
	svc := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      serviceName,
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
				"app":                              "trino",
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
				"app":                              "trino",
				"component":                        "coordinator",
			},
			Ports: []corev1.ServicePort{
				{
					Name:       "http",
					Port:       8080,
					TargetPort: intstr.FromInt32(8080),
					Protocol:   corev1.ProtocolTCP,
				},
			},
		},
	}

	existingSvc := &corev1.Service{}
	err = r.Get(ctx, client.ObjectKey{Name: svc.Name, Namespace: namespace}, existingSvc)
	if errors.IsNotFound(err) {
		logger.Info("creating Trino Service", "name", svc.Name, "namespace", namespace)
		if err := r.Create(ctx, svc); err != nil {
			return fmt.Errorf("creating Trino Service %s: %w", svc.Name, err)
		}
	} else if err != nil {
		return fmt.Errorf("getting Trino Service %s: %w", svc.Name, err)
	}

	// Check if coordinator is ready
	if existingCoordinatorDeploy.Status.AvailableReplicas > 0 {
		// Check if worker replicas are ready
		if existingWorkerDeploy.Status.AvailableReplicas >= replicas {
			endpoint := fmt.Sprintf("%s.%s.svc.cluster.local:8080", serviceName, namespace)
			dr.Status.Endpoints = &nestv1.ResourceEndpoints{
				REST: endpoint,
			}
			r.setPhase(dr, nestv1.PhaseReady, fmt.Sprintf("Trino ready (coordinator: 1/1, workers: %d/%d)", existingWorkerDeploy.Status.AvailableReplicas, replicas))
		} else {
			r.setPhase(dr, nestv1.PhaseProvisioning, fmt.Sprintf("Waiting for Trino workers (%d/%d ready)", existingWorkerDeploy.Status.AvailableReplicas, replicas))
		}
	} else {
		r.setPhase(dr, nestv1.PhaseProvisioning, "Waiting for Trino coordinator")
	}

	return r.Status().Update(ctx, dr)
}

// reconcileTrinoDelete removes the Trino coordinator/worker Deployments, Service, and ConfigMaps on DataResource deletion.
func (r *DataResourceReconciler) reconcileTrinoDelete(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)
	namespace := trinoNamespace(dr)

	// Delete coordinator Deployment
	coordinatorDeploy := &appsv1.Deployment{}
	coordinatorDeploy.Name = trinoCoordinatorName(dr)
	coordinatorDeploy.Namespace = namespace
	if err := r.Delete(ctx, coordinatorDeploy); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete Trino coordinator Deployment", "name", coordinatorDeploy.Name)
		return err
	}

	// Delete worker Deployment
	workerDeploy := &appsv1.Deployment{}
	workerDeploy.Name = trinoWorkerName(dr)
	workerDeploy.Namespace = namespace
	if err := r.Delete(ctx, workerDeploy); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete Trino worker Deployment", "name", workerDeploy.Name)
		return err
	}

	// Delete Service
	svc := &corev1.Service{}
	svc.Name = trinoServiceName(dr)
	svc.Namespace = namespace
	if err := r.Delete(ctx, svc); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete Trino Service", "name", svc.Name)
		return err
	}

	// Delete coordinator ConfigMap
	coordinatorCM := &corev1.ConfigMap{}
	coordinatorCM.Name = trinoCoordinatorConfigMapName(dr)
	coordinatorCM.Namespace = namespace
	if err := r.Delete(ctx, coordinatorCM); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete Trino coordinator ConfigMap", "name", coordinatorCM.Name)
		return err
	}

	// Delete worker ConfigMap
	workerCM := &corev1.ConfigMap{}
	workerCM.Name = trinoWorkerConfigMapName(dr)
	workerCM.Namespace = namespace
	if err := r.Delete(ctx, workerCM); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to delete Trino worker ConfigMap", "name", workerCM.Name)
		return err
	}

	return nil
}

// Helper functions for Trino reconciliation

func trinoNamespace(dr *nestv1.DataResource) string {
	return dr.Spec.Tenant
}

func trinoCoordinatorName(dr *nestv1.DataResource) string {
	name := fmt.Sprintf("%s-%s-trino-coordinator", dr.Spec.Tenant, dr.Name)
	if len(name) > 63 {
		return name[:63]
	}
	return name
}

func trinoWorkerName(dr *nestv1.DataResource) string {
	name := fmt.Sprintf("%s-%s-trino-worker", dr.Spec.Tenant, dr.Name)
	if len(name) > 63 {
		return name[:63]
	}
	return name
}

func trinoServiceName(dr *nestv1.DataResource) string {
	name := fmt.Sprintf("%s-%s-trino", dr.Spec.Tenant, dr.Name)
	if len(name) > 63 {
		return name[:63]
	}
	return name
}

func trinoCoordinatorConfigMapName(dr *nestv1.DataResource) string {
	name := fmt.Sprintf("%s-%s-trino-config", dr.Spec.Tenant, dr.Name)
	if len(name) > 63 {
		return name[:63]
	}
	return name
}

func trinoWorkerConfigMapName(dr *nestv1.DataResource) string {
	name := fmt.Sprintf("%s-%s-trino-worker-config", dr.Spec.Tenant, dr.Name)
	if len(name) > 63 {
		return name[:63]
	}
	return name
}
