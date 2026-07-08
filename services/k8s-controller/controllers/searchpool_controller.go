package controllers

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// +kubebuilder:rbac:groups=nest.penguintech.io,resources=searchpools,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nest.penguintech.io,resources=searchpools/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nest.penguintech.io,resources=searchpools/finalizers,verbs=update
// +kubebuilder:rbac:groups=opensearch.opster.io,resources=opensearchclusters,verbs=get;list;watch;create;update;patch;delete

type SearchPoolReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	DynClient dynamic.Interface
}

func (r *SearchPoolReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var pool nestv1.SearchPool
	if err := r.Get(ctx, req.NamespacedName, &pool); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("reconciling SearchPool", "name", pool.Name)

	// Ensure the target namespace exists
	namespace := pool.Spec.Namespace
	if namespace == "" {
		namespace = "nest-search"
	}
	ns := &corev1.Namespace{}
	ns.Name = namespace
	if err := r.Create(ctx, ns); err != nil && !errors.IsAlreadyExists(err) {
		return ctrl.Result{}, fmt.Errorf("creating namespace %s: %w", namespace, err)
	}

	clusterName := fmt.Sprintf("nest-search-%s", pool.Name)
	version := pool.Spec.Version
	if version == "" {
		version = "2.11.0"
	}
	diskSize := pool.Spec.DiskSizeGi
	if diskSize == 0 {
		diskSize = 100
	}
	replicas := pool.Spec.Replicas
	if replicas == 0 {
		replicas = 3
	}
	storageClass := pool.Spec.StorageClass
	if storageClass == "" {
		storageClass = "nest-block"
	}
	adminSecretName := fmt.Sprintf("%s-admin", clusterName)

	cluster := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "opensearch.opster.io/v1",
			"kind":       "OpenSearchCluster",
			"metadata": map[string]interface{}{
				"name":      clusterName,
				"namespace": namespace,
				"labels": map[string]interface{}{
					"nest.penguintech.io/searchpool": pool.Name,
					"nest.penguintech.io/managed-by": "nest-controller",
				},
			},
			"spec": map[string]interface{}{
				"general": map[string]interface{}{
					"serviceName": clusterName,
					"version":     version,
					"httpPort":    float64(9200),
				},
				"nodePools": []interface{}{
					map[string]interface{}{
						"component": "nodes",
						"replicas":  float64(replicas),
						"diskSize":  fmt.Sprintf("%dGi", diskSize),
						"resources": map[string]interface{}{
							"requests": map[string]interface{}{
								"memory": "2Gi",
								"cpu":    "500m",
							},
							"limits": map[string]interface{}{
								"memory": "4Gi",
								"cpu":    "2000m",
							},
						},
						"roles": []interface{}{"master", "data"},
					},
				},
				"security": map[string]interface{}{
					"config": map[string]interface{}{
						"adminCredentialsSecret": map[string]interface{}{
							"name": adminSecretName,
						},
					},
					"tls": map[string]interface{}{
						"http":      map[string]interface{}{"generate": true},
						"transport": map[string]interface{}{"generate": true},
					},
				},
			},
		},
	}

	existing := &unstructured.Unstructured{}
	existing.SetGroupVersionKind(openSearchGVR.GroupVersion().WithKind("OpenSearchCluster"))
	err := r.Get(ctx, client.ObjectKey{Name: clusterName, Namespace: namespace}, existing)
	if errors.IsNotFound(err) {
		if _, err := r.DynClient.Resource(openSearchGVR).Namespace(namespace).Create(ctx, cluster, metav1.CreateOptions{FieldManager: "nest-controller"}); err != nil {
			logger.Error(err, "failed to create shared OpenSearchCluster", "name", clusterName)
			pool.Status.Phase = "Failed"
			_ = r.Status().Update(ctx, &pool)
			return ctrl.Result{}, err
		}
		logger.Info("shared OpenSearchCluster created", "name", clusterName, "namespace", namespace)
	} else if err != nil {
		return ctrl.Result{}, err
	}

	// Check cluster phase
	liveCluster, err := r.DynClient.Resource(openSearchGVR).Namespace(namespace).Get(ctx, clusterName, metav1.GetOptions{})
	if err == nil {
		phase, _, _ := unstructured.NestedString(liveCluster.Object, "status", "phase")
		endpoint := fmt.Sprintf("%s.%s.svc.cluster.local:9200", clusterName, namespace)

		if phase == "RUNNING" {
			pool.Status.Phase = "Running"
			pool.Status.Endpoint = endpoint
			pool.Status.AdminCredentialsSecret = adminSecretName
		} else if phase != "" {
			pool.Status.Phase = "Provisioning"
		}
	}

	if err := r.Status().Update(ctx, &pool); err != nil && !errors.IsConflict(err) {
		logger.Error(err, "failed to update SearchPool status")
	}

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (r *SearchPoolReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nestv1.SearchPool{}).
		Complete(r)
}
