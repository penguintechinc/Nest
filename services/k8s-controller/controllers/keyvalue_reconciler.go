package controllers

import (
	"context"
	"crypto/rand"
	"fmt"
	"math/big"

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
	if dr.Spec.Replicas != nil && dr.Spec.Replicas.Write != nil {
		// Reject explicitly invalid replica count
		if dr.Spec.Replicas.Write.Default <= 0 {
			err := fmt.Errorf("invalid replica count: %d (must be > 0)", dr.Spec.Replicas.Write.Default)
			r.setPhase(dr, nestv1.PhaseFailed, err.Error())
			_ = r.Status().Update(ctx, dr)
			return err
		}
		replicas = dr.Spec.Replicas.Write.Default
	}

	// Get storage size
	storageSize := keyvalueStorageSize(dr)

	// Create or retrieve auth password secret
	secretName := keyvalueAuthSecretName(dr)
	authSecret := &corev1.Secret{}
	secretErr := r.Get(ctx, client.ObjectKey{Name: secretName, Namespace: namespace}, authSecret)
	if secretErr != nil && errors.IsNotFound(secretErr) {
		// Generate new password secret
		pw, genErr := generateRandomPassword(32)
		if genErr != nil {
			return fmt.Errorf("generating Valkey auth password: %w", genErr)
		}
		authSecret = &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      secretName,
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
				"password": []byte(pw),
				// auth.conf is mounted into the container as a file and pulled in
				// via `include`, so the password never lands in the ConfigMap.
				"auth.conf": []byte(fmt.Sprintf("requirepass %s\n", pw)),
			},
		}
		if secretErr := r.Create(ctx, authSecret); secretErr != nil {
			logger.Error(secretErr, "failed to create Valkey auth secret", "name", secretName)
			return fmt.Errorf("creating Valkey auth secret: %w", secretErr)
		}
		logger.Info("created Valkey auth secret", "name", secretName)
	} else if secretErr != nil {
		logger.Error(secretErr, "failed to get Valkey auth secret", "name", secretName)
		return fmt.Errorf("getting Valkey auth secret: %w", secretErr)
	}

	// Extract password from auth secret
	password := string(authSecret.Data["password"])

	// Backfill auth.conf for secrets created before it was tracked, keeping the
	// requirepass directive in the Secret (never the ConfigMap).
	wantAuthConf := fmt.Sprintf("requirepass %s\n", password)
	if string(authSecret.Data["auth.conf"]) != wantAuthConf {
		if authSecret.Data == nil {
			authSecret.Data = map[string][]byte{}
		}
		authSecret.Data["auth.conf"] = []byte(wantAuthConf)
		if authSecret.ResourceVersion != "" {
			if err := r.Update(ctx, authSecret); err != nil {
				return fmt.Errorf("updating Valkey auth secret with auth.conf: %w", err)
			}
		}
	}

	// Create ConfigMap with auth password and persistence enabled
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
					InitContainers: []corev1.Container{
						{
							Name:  "configure-replicaof",
							Image: "busybox:latest",
							Command: []string{
								"sh",
								"-c",
								keyvalueReplicaofScript(serviceName, namespace),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config-work",
									MountPath: "/etc/valkey-work",
								},
								{
									Name:      "config",
									MountPath: "/etc/valkey",
									ReadOnly:  true,
								},
								{
									Name:      "auth",
									MountPath: "/etc/valkey-auth",
									ReadOnly:  true,
								},
							},
						},
					},
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
								"/etc/valkey-work/valkey.conf",
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config-work",
									MountPath: "/etc/valkey-work",
									ReadOnly:  false,
								},
								{
									Name:      "auth",
									MountPath: "/etc/valkey-auth",
									ReadOnly:  true,
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
						{
							Name: "config-work",
							VolumeSource: corev1.VolumeSource{
								EmptyDir: &corev1.EmptyDirVolumeSource{},
							},
						},
						{
							Name: "auth",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName:  secretName,
									DefaultMode: int32Ptr(0400),
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

	// Update existing StatefulSet if spec changed (idempotent reconciliation)
	ssPatch := client.MergeFrom(existingSS.DeepCopy())
	existingSS.Spec.Replicas = &replicas
	if err := r.Patch(ctx, existingSS, ssPatch); err != nil && !errors.IsNotFound(err) {
		logger.Error(err, "failed to patch Valkey StatefulSet")
	}

	// Check if StatefulSet is ready
	if existingSS.Status.ReadyReplicas >= replicas {
		endpoint := keyvalueEndpoints(serviceName, namespace, replicas)
		dr.Status.Endpoints = &nestv1.ResourceEndpoints{
			Native: endpoint,
		}
		r.setPhase(dr, nestv1.PhaseReady, fmt.Sprintf("Valkey ready (%d/%d replicas)", existingSS.Status.ReadyReplicas, replicas))
		dr.Status.ObservedGeneration = dr.Generation
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
	return r.ensureTenantNamespace(ctx, ns)
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
	// Parse storage size to calculate maxmemory
	// Use 80% of allocated storage as the maxmemory limit
	q, err := resource.ParseQuantity(storageSize)
	if err != nil {
		// Fallback if parsing fails
		q = resource.MustParse("1Gi")
	}

	// Calculate maxmemory as 80% of storage allocation in MB
	maxMemoryBytes := (q.Value() * 80) / 100
	maxMemoryMB := maxMemoryBytes / (1024 * 1024)
	if maxMemoryMB < 1 {
		maxMemoryMB = 1
	}
	maxmemory := fmt.Sprintf("%dmb", maxMemoryMB)

	// requirepass is intentionally NOT written here — it lives in the auth Secret,
	// mounted at /etc/valkey-auth/auth.conf and pulled in via `include` so the
	// password never appears in this ConfigMap.
	return fmt.Sprintf(`# Valkey configuration for %s/%s
# Generated configuration with auth and persistence enabled
maxmemory %s
maxmemory-policy allkeys-lru
appendonly yes
appendfsync everysec
include /etc/valkey-auth/auth.conf
`, dr.Spec.Tenant, dr.Name, maxmemory)
}

func keyvalueAuthSecretName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-%s-valkey-auth", dr.Spec.Tenant, dr.Name)
}

// keyvalueReplicaofScript generates a shell script for the init container that configures
// replicaof directive for Valkey replicas. Pod 0 is the primary (no replicaof);
// pods 1+ are configured to replicate from pod 0 with masterauth extracted from the Secret.
func keyvalueReplicaofScript(serviceName, namespace string) string {
	return fmt.Sprintf(`set -e
cp /etc/valkey/valkey.conf /etc/valkey-work/valkey.conf
POD_ORDINAL=$(hostname | rev | cut -d'-' -f1 | rev)
if [ "$POD_ORDINAL" != "0" ]; then
  echo "replicaof %s-0.%s.svc.cluster.local 6379" >> /etc/valkey-work/valkey.conf
  PW=$(grep '^requirepass ' /etc/valkey-auth/auth.conf | awk '{print $2}')
  if [ -n "$PW" ]; then
    echo "masterauth $PW" >> /etc/valkey-work/valkey.conf
  fi
fi
`, serviceName, namespace)
}

// keyvalueEndpoints formats the Valkey endpoint string. For single instance,
// returns the headless service endpoint. For multiple instances, returns
// "primary: pod-0; replicas: pod-1, pod-2, ..." format.
func keyvalueEndpoints(serviceName, namespace string, replicas int32) string {
	if replicas == 1 {
		return fmt.Sprintf("%s.%s.svc.cluster.local:6379", serviceName, namespace)
	}

	// Multiple replicas: expose primary and replica pods individually
	primaryEndpoint := fmt.Sprintf("%s-0.%s.svc.cluster.local:6379", serviceName, namespace)
	replicaEndpoints := ""
	for i := int32(1); i < replicas; i++ {
		if replicaEndpoints != "" {
			replicaEndpoints += ", "
		}
		replicaEndpoints += fmt.Sprintf("%s-%d.%s.svc.cluster.local:6379", serviceName, i, namespace)
	}

	return fmt.Sprintf("primary: %s; replicas: %s", primaryEndpoint, replicaEndpoints)
}

// generateRandomPassword creates a cryptographically-random password for auth.
// It returns an error rather than a weak fallback so callers fail closed if the
// system CSPRNG is unavailable.
func generateRandomPassword(length int) (string, error) {
	// Alphanumeric only — avoids config/shell escaping issues in requirepass.
	const charset = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"
	max := big.NewInt(int64(len(charset)))
	b := make([]byte, length)
	for i := range b {
		n, err := rand.Int(rand.Reader, max)
		if err != nil {
			return "", fmt.Errorf("crypto/rand failure generating password: %w", err)
		}
		b[i] = charset[n.Int64()]
	}
	return string(b), nil
}

// Utility functions for pointer creation

func int32Ptr(i int32) *int32 {
	return &i
}

func int64Ptr(i int64) *int64 {
	return &i
}
