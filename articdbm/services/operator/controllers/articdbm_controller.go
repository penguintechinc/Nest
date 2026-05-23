package controllers

import (
	"context"
	"fmt"
	"time"

	"github.com/go-logr/logr"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	articdbmv1alpha1 "github.com/penguintechinc/articdbm-operator/api/v1alpha1"
)

// ArticDBMReconciler reconciles ArticDBM objects
type ArticDBMReconciler struct {
	client.Client
	Log                     logr.Logger
	Scheme                  *runtime.Scheme
	XDPEnabled              bool
	MaxConcurrentReconciles int
}

// +kubebuilder:rbac:groups=articdbm.io,resources=articdbms,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=articdbm.io,resources=articdbms/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=articdbm.io,resources=articdbms/finalizers,verbs=update
// +kubebuilder:rbac:groups=apps,resources=deployments;daemonsets;statefulsets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=core,resources=pods;services;endpoints;persistentvolumeclaims;configmaps;secrets,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=monitoring.coreos.com,resources=servicemonitors,verbs=get;list;watch;create;update;patch;delete

func (r *ArticDBMReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := r.Log.WithValues("articdbm", req.NamespacedName)

	// Fetch the ArticDBM instance
	articdbm := &articdbmv1alpha1.ArticDBM{}
	err := r.Get(ctx, req.NamespacedName, articdbm)
	if err != nil {
		if errors.IsNotFound(err) {
			log.Info("ArticDBM resource not found. Ignoring since object must be deleted")
			return ctrl.Result{}, nil
		}
		log.Error(err, "Failed to get ArticDBM")
		return ctrl.Result{}, err
	}

	// Check if the instance is marked for deletion
	if articdbm.GetDeletionTimestamp() != nil {
		if controllerutil.ContainsFinalizer(articdbm, "articdbm.io/finalizer") {
			// Cleanup XDP resources
			if err := r.cleanupXDPResources(ctx, articdbm); err != nil {
				log.Error(err, "Failed to cleanup XDP resources")
				return ctrl.Result{}, err
			}

			// Remove finalizer
			controllerutil.RemoveFinalizer(articdbm, "articdbm.io/finalizer")
			if err := r.Update(ctx, articdbm); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// Add finalizer if not present
	if !controllerutil.ContainsFinalizer(articdbm, "articdbm.io/finalizer") {
		controllerutil.AddFinalizer(articdbm, "articdbm.io/finalizer")
		if err = r.Update(ctx, articdbm); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Reconcile ConfigMap
	if err := r.reconcileConfigMap(ctx, articdbm); err != nil {
		log.Error(err, "Failed to reconcile ConfigMap")
		return ctrl.Result{}, err
	}

	// Reconcile Secrets
	if err := r.reconcileSecrets(ctx, articdbm); err != nil {
		log.Error(err, "Failed to reconcile Secrets")
		return ctrl.Result{}, err
	}

	// Reconcile XDP DaemonSet if enabled
	if r.XDPEnabled && articdbm.Spec.XDP.Enabled {
		if err := r.reconcileXDPDaemonSet(ctx, articdbm); err != nil {
			log.Error(err, "Failed to reconcile XDP DaemonSet")
			return ctrl.Result{}, err
		}
	}

	// Reconcile main Deployment
	if err := r.reconcileDeployment(ctx, articdbm); err != nil {
		log.Error(err, "Failed to reconcile Deployment")
		return ctrl.Result{}, err
	}

	// Reconcile Service
	if err := r.reconcileService(ctx, articdbm); err != nil {
		log.Error(err, "Failed to reconcile Service")
		return ctrl.Result{}, err
	}

	// Reconcile monitoring resources
	if articdbm.Spec.Monitoring.Prometheus.Enabled {
		if err := r.reconcileServiceMonitor(ctx, articdbm); err != nil {
			log.Error(err, "Failed to reconcile ServiceMonitor")
			// Don't fail if monitoring setup fails
		}
	}

	// Update status
	if err := r.updateStatus(ctx, articdbm); err != nil {
		log.Error(err, "Failed to update status")
		return ctrl.Result{}, err
	}

	// Requeue for periodic checks
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

func (r *ArticDBMReconciler) reconcileConfigMap(ctx context.Context, articdbm *articdbmv1alpha1.ArticDBM) error {
	configMap := &corev1.ConfigMap{
		ObjectMeta: metav1.ObjectMeta{
			Name:      articdbm.Name + "-config",
			Namespace: articdbm.Namespace,
		},
		Data: map[string]string{
			"config.yaml": r.generateConfig(articdbm),
			"xdp.conf":    r.generateXDPConfig(articdbm),
		},
	}

	if err := controllerutil.SetControllerReference(articdbm, configMap, r.Scheme); err != nil {
		return err
	}

	found := &corev1.ConfigMap{}
	err := r.Get(ctx, types.NamespacedName{Name: configMap.Name, Namespace: configMap.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, configMap)
	} else if err != nil {
		return err
	}

	// Update if configuration changed
	found.Data = configMap.Data
	return r.Update(ctx, found)
}

func (r *ArticDBMReconciler) reconcileXDPDaemonSet(ctx context.Context, articdbm *articdbmv1alpha1.ArticDBM) error {
	daemonSet := &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      articdbm.Name + "-xdp",
			Namespace: articdbm.Namespace,
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":       "articdbm-xdp",
					"instance":  articdbm.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       "articdbm-xdp",
						"instance":  articdbm.Name,
						"component": "xdp",
					},
				},
				Spec: corev1.PodSpec{
					HostNetwork: true,
					HostPID:     true,
					NodeSelector: map[string]string{
						"node-role.kubernetes.io/xdp": "true",
					},
					Tolerations: []corev1.Toleration{
						{
							Key:      "node-role.kubernetes.io/xdp",
							Operator: corev1.TolerationOpExists,
							Effect:   corev1.TaintEffectNoSchedule,
						},
					},
					InitContainers: []corev1.Container{
						{
							Name:  "xdp-loader",
							Image: articdbm.Spec.Proxy.Image + ":xdp-" + articdbm.Spec.Proxy.Version,
							Command: []string{"/bin/sh", "-c"},
							Args: []string{`
								# Mount BPF filesystem
								mount -t bpf bpf /sys/fs/bpf || true

								# Load XDP programs
								ip link set dev ` + articdbm.Spec.XDP.Interface + ` xdpgeneric obj /xdp/ip_blocklist.o sec prog
								ip link set dev ` + articdbm.Spec.XDP.Interface + ` xdpgeneric obj /xdp/rate_limiter.o sec prog
								ip link set dev ` + articdbm.Spec.XDP.Interface + ` xdpgeneric obj /xdp/query_cache.o sec prog

								# Configure huge pages for AF_XDP
								echo 2048 > /proc/sys/vm/nr_hugepages
							`},
							SecurityContext: &corev1.SecurityContext{
								Privileged: &[]bool{true}[0],
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{
										"SYS_ADMIN",
										"NET_ADMIN",
										"BPF",
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "bpf-maps",
									MountPath: "/sys/fs/bpf",
								},
								{
									Name:      "proc",
									MountPath: "/host/proc",
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "xdp-controller",
							Image: articdbm.Spec.Proxy.Image + ":" + articdbm.Spec.Proxy.Version,
							Command: []string{"/articdbm-xdp-controller"},
							Env: []corev1.EnvVar{
								{
									Name:  "XDP_INTERFACE",
									Value: articdbm.Spec.XDP.Interface,
								},
								{
									Name:  "XDP_RATE_LIMIT_PPS",
									Value: fmt.Sprintf("%d", articdbm.Spec.XDP.RateLimitPPS),
								},
								{
									Name:  "XDP_BURST_LIMIT",
									Value: fmt.Sprintf("%d", articdbm.Spec.XDP.BurstLimit),
								},
								{
									Name:  "XDP_CACHE_SIZE",
									Value: fmt.Sprintf("%d", articdbm.Spec.XDP.CacheSize),
								},
								{
									Name:  "XDP_CACHE_TTL",
									Value: fmt.Sprintf("%d", articdbm.Spec.XDP.CacheTTL),
								},
								{
									Name:  "REDIS_ADDR",
									Value: r.getRedisAddress(articdbm),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Privileged: &[]bool{true}[0],
								Capabilities: &corev1.Capabilities{
									Add: []corev1.Capability{
										"SYS_ADMIN",
										"NET_ADMIN",
										"BPF",
									},
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "bpf-maps",
									MountPath: "/sys/fs/bpf",
								},
								{
									Name:      "config",
									MountPath: "/etc/articdbm",
								},
							},
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("500m"),
									corev1.ResourceMemory: resource.MustParse("256Mi"),
								},
								Limits: corev1.ResourceList{
									corev1.ResourceCPU:    resource.MustParse("2"),
									corev1.ResourceMemory: resource.MustParse("1Gi"),
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "bpf-maps",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/sys/fs/bpf",
									Type: &[]corev1.HostPathType{corev1.HostPathDirectoryOrCreate}[0],
								},
							},
						},
						{
							Name: "proc",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/proc",
									Type: &[]corev1.HostPathType{corev1.HostPathDirectory}[0],
								},
							},
						},
						{
							Name: "config",
							VolumeSource: corev1.VolumeSource{
								ConfigMap: &corev1.ConfigMapVolumeSource{
									LocalObjectReference: corev1.LocalObjectReference{
										Name: articdbm.Name + "-config",
									},
								},
							},
						},
					},
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(articdbm, daemonSet, r.Scheme); err != nil {
		return err
	}

	found := &appsv1.DaemonSet{}
	err := r.Get(ctx, types.NamespacedName{Name: daemonSet.Name, Namespace: daemonSet.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, daemonSet)
	} else if err != nil {
		return err
	}

	// Update if spec changed
	found.Spec = daemonSet.Spec
	return r.Update(ctx, found)
}

func (r *ArticDBMReconciler) reconcileDeployment(ctx context.Context, articdbm *articdbmv1alpha1.ArticDBM) error {
	replicas := int32(3)
	if articdbm.Spec.Proxy.Replicas != nil {
		replicas = *articdbm.Spec.Proxy.Replicas
	}

	deployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      articdbm.Name,
			Namespace: articdbm.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: &replicas,
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"app":      "articdbm",
					"instance": articdbm.Name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{
						"app":       "articdbm",
						"instance":  articdbm.Name,
						"component": "proxy",
					},
				},
				Spec: corev1.PodSpec{
					ServiceAccountName: articdbm.Name,
					NodeSelector:       articdbm.Spec.Proxy.NodeSelector,
					Affinity: &corev1.Affinity{
						PodAntiAffinity: &corev1.PodAntiAffinity{
							PreferredDuringSchedulingIgnoredDuringExecution: []corev1.WeightedPodAffinityTerm{
								{
									Weight: 100,
									PodAffinityTerm: corev1.PodAffinityTerm{
										LabelSelector: &metav1.LabelSelector{
											MatchLabels: map[string]string{
												"app":      "articdbm",
												"instance": articdbm.Name,
											},
										},
										TopologyKey: "kubernetes.io/hostname",
									},
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "proxy",
							Image: articdbm.Spec.Proxy.Image + ":" + articdbm.Spec.Proxy.Version,
							Ports: []corev1.ContainerPort{
								{Name: "mysql", ContainerPort: 3306},
								{Name: "postgresql", ContainerPort: 5432},
								{Name: "mssql", ContainerPort: 1433},
								{Name: "mongodb", ContainerPort: 27017},
								{Name: "redis", ContainerPort: 6379},
								{Name: "metrics", ContainerPort: 9090},
							},
							Env: r.generateEnvVars(articdbm),
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "config",
									MountPath: "/etc/articdbm",
								},
								{
									Name:      "tls",
									MountPath: "/etc/articdbm/tls",
									ReadOnly:  true,
								},
							},
							Resources: r.generateResourceRequirements(articdbm),
							LivenessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/health",
										Port: intstr.FromInt(9090),
									},
								},
								InitialDelaySeconds: 30,
								PeriodSeconds:       10,
							},
							ReadinessProbe: &corev1.Probe{
								Handler: corev1.Handler{
									HTTPGet: &corev1.HTTPGetAction{
										Path: "/ready",
										Port: intstr.FromInt(9090),
									},
								},
								InitialDelaySeconds: 10,
								PeriodSeconds:       5,
							},
						},
					},
					Volumes: r.generateVolumes(articdbm),
				},
			},
		},
	}

	if err := controllerutil.SetControllerReference(articdbm, deployment, r.Scheme); err != nil {
		return err
	}

	found := &appsv1.Deployment{}
	err := r.Get(ctx, types.NamespacedName{Name: deployment.Name, Namespace: deployment.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, deployment)
	} else if err != nil {
		return err
	}

	// Update if spec changed
	found.Spec = deployment.Spec
	return r.Update(ctx, found)
}

func (r *ArticDBMReconciler) reconcileService(ctx context.Context, articdbm *articdbmv1alpha1.ArticDBM) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      articdbm.Name,
			Namespace: articdbm.Namespace,
		},
		Spec: corev1.ServiceSpec{
			Type: corev1.ServiceTypeLoadBalancer,
			Selector: map[string]string{
				"app":      "articdbm",
				"instance": articdbm.Name,
			},
			Ports: []corev1.ServicePort{
				{Name: "mysql", Port: 3306, TargetPort: intstr.FromInt(3306)},
				{Name: "postgresql", Port: 5432, TargetPort: intstr.FromInt(5432)},
				{Name: "mssql", Port: 1433, TargetPort: intstr.FromInt(1433)},
				{Name: "mongodb", Port: 27017, TargetPort: intstr.FromInt(27017)},
				{Name: "redis", Port: 6379, TargetPort: intstr.FromInt(6379)},
				{Name: "metrics", Port: 9090, TargetPort: intstr.FromInt(9090)},
			},
		},
	}

	if err := controllerutil.SetControllerReference(articdbm, service, r.Scheme); err != nil {
		return err
	}

	found := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: service.Name, Namespace: service.Namespace}, found)
	if err != nil && errors.IsNotFound(err) {
		return r.Create(ctx, service)
	} else if err != nil {
		return err
	}

	// Update if spec changed
	found.Spec = service.Spec
	return r.Update(ctx, found)
}

// Helper functions

func (r *ArticDBMReconciler) generateConfig(articdbm *articdbmv1alpha1.ArticDBM) string {
	// Generate YAML configuration for ArticDBM
	config := fmt.Sprintf(`
# ArticDBM Configuration
version: %s
cluster_mode: true

mysql:
  enabled: %v
  port: 3306

postgresql:
  enabled: %v
  port: 5432

mssql:
  enabled: %v
  port: 1433

mongodb:
  enabled: %v
  port: 27017

redis_proxy:
  enabled: %v
  port: 6379

xdp:
  enabled: %v
  interface: %s
  rate_limit_pps: %d
  burst_limit: %d

cache:
  enabled: %v
  auth_validation: %v
  hit_counter_based: %v
  min_hits_to_cache: %d
  eviction_policy: %s

security:
  sql_injection_detection: %v
  tls_enabled: %v

monitoring:
  prometheus:
    enabled: %v
    metrics_path: %s
`,
		articdbm.Spec.Proxy.Version,
		len(articdbm.Spec.Backends.MySQL) > 0,
		len(articdbm.Spec.Backends.PostgreSQL) > 0,
		len(articdbm.Spec.Backends.MSSQL) > 0,
		len(articdbm.Spec.Backends.MongoDB) > 0,
		len(articdbm.Spec.Backends.Redis) > 0,
		articdbm.Spec.XDP.Enabled,
		articdbm.Spec.XDP.Interface,
		articdbm.Spec.XDP.RateLimitPPS,
		articdbm.Spec.XDP.BurstLimit,
		articdbm.Spec.Cache.Enabled,
		articdbm.Spec.Cache.AuthValidation,
		articdbm.Spec.Cache.HitCounterBased,
		articdbm.Spec.Cache.MinHitsToCache,
		articdbm.Spec.Cache.EvictionPolicy,
		articdbm.Spec.Security.SQLInjectionDetection,
		articdbm.Spec.Security.TLS.Enabled,
		articdbm.Spec.Monitoring.Prometheus.Enabled,
		articdbm.Spec.Monitoring.Prometheus.MetricsPath,
	)

	return config
}

func (r *ArticDBMReconciler) generateXDPConfig(articdbm *articdbmv1alpha1.ArticDBM) string {
	// Generate XDP configuration
	return fmt.Sprintf(`
# XDP Configuration
interface: %s
rate_limit_pps: %d
burst_limit: %d
cache_size: %d
cache_ttl: %d
numa_optimized: %v
ip_blocklist: %s
`,
		articdbm.Spec.XDP.Interface,
		articdbm.Spec.XDP.RateLimitPPS,
		articdbm.Spec.XDP.BurstLimit,
		articdbm.Spec.XDP.CacheSize,
		articdbm.Spec.XDP.CacheTTL,
		articdbm.Spec.XDP.NumaOptimized,
		articdbm.Spec.XDP.IPBlocklistFile,
	)
}

func (r *ArticDBMReconciler) generateEnvVars(articdbm *articdbmv1alpha1.ArticDBM) []corev1.EnvVar {
	return []corev1.EnvVar{
		{Name: "XDP_ENABLED", Value: fmt.Sprintf("%v", articdbm.Spec.XDP.Enabled)},
		{Name: "CACHE_ENABLED", Value: fmt.Sprintf("%v", articdbm.Spec.Cache.Enabled)},
		{Name: "CACHE_AUTH_VALIDATION", Value: fmt.Sprintf("%v", articdbm.Spec.Cache.AuthValidation)},
		{Name: "SQL_INJECTION_DETECTION", Value: fmt.Sprintf("%v", articdbm.Spec.Security.SQLInjectionDetection)},
		{Name: "TLS_ENABLED", Value: fmt.Sprintf("%v", articdbm.Spec.Security.TLS.Enabled)},
		{Name: "REDIS_ADDR", Value: r.getRedisAddress(articdbm)},
	}
}

func (r *ArticDBMReconciler) generateResourceRequirements(articdbm *articdbmv1alpha1.ArticDBM) corev1.ResourceRequirements {
	requirements := corev1.ResourceRequirements{
		Requests: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("1"),
			corev1.ResourceMemory: resource.MustParse("2Gi"),
		},
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resource.MustParse("4"),
			corev1.ResourceMemory: resource.MustParse("8Gi"),
		},
	}

	if articdbm.Spec.Resources.Requests.CPU != "" {
		requirements.Requests[corev1.ResourceCPU] = resource.MustParse(articdbm.Spec.Resources.Requests.CPU)
	}
	if articdbm.Spec.Resources.Requests.Memory != "" {
		requirements.Requests[corev1.ResourceMemory] = resource.MustParse(articdbm.Spec.Resources.Requests.Memory)
	}
	if articdbm.Spec.Resources.Limits.CPU != "" {
		requirements.Limits[corev1.ResourceCPU] = resource.MustParse(articdbm.Spec.Resources.Limits.CPU)
	}
	if articdbm.Spec.Resources.Limits.Memory != "" {
		requirements.Limits[corev1.ResourceMemory] = resource.MustParse(articdbm.Spec.Resources.Limits.Memory)
	}

	// Add huge pages if XDP is enabled
	if articdbm.Spec.XDP.Enabled && articdbm.Spec.Resources.Limits.HugePages != "" {
		requirements.Limits["hugepages-2Mi"] = resource.MustParse(articdbm.Spec.Resources.Limits.HugePages)
		requirements.Requests["hugepages-2Mi"] = resource.MustParse(articdbm.Spec.Resources.Requests.HugePages)
	}

	return requirements
}

func (r *ArticDBMReconciler) generateVolumes(articdbm *articdbmv1alpha1.ArticDBM) []corev1.Volume {
	volumes := []corev1.Volume{
		{
			Name: "config",
			VolumeSource: corev1.VolumeSource{
				ConfigMap: &corev1.ConfigMapVolumeSource{
					LocalObjectReference: corev1.LocalObjectReference{
						Name: articdbm.Name + "-config",
					},
				},
			},
		},
	}

	if articdbm.Spec.Security.TLS.Enabled {
		volumes = append(volumes, corev1.Volume{
			Name: "tls",
			VolumeSource: corev1.VolumeSource{
				Secret: &corev1.SecretVolumeSource{
					SecretName: articdbm.Name + "-tls",
				},
			},
		})
	}

	return volumes
}

func (r *ArticDBMReconciler) getRedisAddress(articdbm *articdbmv1alpha1.ArticDBM) string {
	if len(articdbm.Spec.Backends.Redis) > 0 {
		return articdbm.Spec.Backends.Redis[0].Endpoints[0]
	}
	return "redis-service:6379"
}

func (r *ArticDBMReconciler) cleanupXDPResources(ctx context.Context, articdbm *articdbmv1alpha1.ArticDBM) error {
	// Cleanup XDP programs and maps
	// This would be implemented to clean up kernel resources
	return nil
}

func (r *ArticDBMReconciler) reconcileSecrets(ctx context.Context, articdbm *articdbmv1alpha1.ArticDBM) error {
	// Create secrets for database passwords and TLS certificates
	// Implementation would handle secret creation
	return nil
}

func (r *ArticDBMReconciler) reconcileServiceMonitor(ctx context.Context, articdbm *articdbmv1alpha1.ArticDBM) error {
	// Create ServiceMonitor for Prometheus
	// Implementation would create Prometheus ServiceMonitor CRD
	return nil
}

func (r *ArticDBMReconciler) updateStatus(ctx context.Context, articdbm *articdbmv1alpha1.ArticDBM) error {
	// Update ArticDBM status with current state
	// Implementation would update status subresource
	return nil
}

// SetupWithManager sets up the controller with the Manager
func (r *ArticDBMReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&articdbmv1alpha1.ArticDBM{}).
		Owns(&appsv1.Deployment{}).
		Owns(&appsv1.DaemonSet{}).
		Owns(&corev1.Service{}).
		Owns(&corev1.ConfigMap{}).
		WithOptions(controller.Options{
			MaxConcurrentReconciles: r.MaxConcurrentReconciles,
		}).
		Complete(r)
}