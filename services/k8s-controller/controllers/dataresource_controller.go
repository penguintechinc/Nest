package controllers

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// DataResourceReconciler reconciles DataResource CRDs.
// It translates DataResource specs into upstream operator CRs
// (e.g., CloudNativePG Cluster, Strimzi Kafka, etc.).
type DataResourceReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=nest.penguintech.io,resources=dataresources,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nest.penguintech.io,resources=dataresources/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nest.penguintech.io,resources=dataresources/finalizers,verbs=update

func (r *DataResourceReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var dr nestv1.DataResource
	if err := r.Get(ctx, req.NamespacedName, &dr); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("reconciling DataResource",
		"name", dr.Name,
		"tenant", dr.Spec.Tenant,
		"type", dr.Spec.Type,
		"phase", dr.Status.Phase,
	)

	// Add finalizer
	if !containsString(dr.Finalizers, "nest.penguintech.io/dataresource") {
		dr.Finalizers = append(dr.Finalizers, "nest.penguintech.io/dataresource")
		if err := r.Update(ctx, &dr); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Handle deletion
	if !dr.DeletionTimestamp.IsZero() {
		return r.reconcileDelete(ctx, &dr)
	}

	return r.reconcileCreate(ctx, &dr)
}

func (r *DataResourceReconciler) reconcileCreate(ctx context.Context, dr *nestv1.DataResource) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	if dr.Status.Phase == "" || dr.Status.Phase == nestv1.PhasePending {
		r.setPhase(dr, nestv1.PhaseProvisioning, "Provisioning started")
		if err := r.Status().Update(ctx, dr); err != nil {
			return ctrl.Result{}, err
		}
	}

	// Dispatch to engine-specific provisioner
	var err error
	switch dr.Spec.Type {
	case "postgres":
		err = r.reconcilePostgres(ctx, dr)
	case "mariadb":
		err = r.reconcileMariaDB(ctx, dr)
	case "mysql":
		err = r.reconcileMySQL(ctx, dr)
	case "object":
		err = r.reconcileObject(ctx, dr)
	case "pvc/block":
		err = r.reconcilePVCBlock(ctx, dr)
	case "pvc/file":
		err = r.reconcilePVCFile(ctx, dr)
	case "keyvalue":
		err = r.reconcileKeyvalue(ctx, dr)
	case "kafka":
		err = r.reconcileKafka(ctx, dr)
	case "search":
		err = r.reconcileOpenSearch(ctx, dr)
	case "rockfs":
		err = r.reconcileFerretDB(ctx, dr)
	case "vector":
		err = r.reconcileVector(ctx, dr)
	case "clickhouse":
		err = r.reconcileClickhouse(ctx, dr)
	case "timeseries":
		err = r.reconcileTimeseries(ctx, dr)
	case "warehouse/trino":
		err = r.reconcileTrino(ctx, dr)
	case "lakehouse/iceberg":
		err = r.reconcileIceberg(ctx, dr)
	case "nfs":
		err = r.reconcileNFS(ctx, dr)
	case "iscsi":
		err = r.reconcileISCSI(ctx, dr)
	case "filesystem":
		err = r.reconcileFilesystem(ctx, dr)
	default:
		err = fmt.Errorf("unsupported DataResource type: %s", dr.Spec.Type)
	}

	if err != nil {
		logger.Error(err, "provisioning failed")
		r.setPhase(dr, nestv1.PhaseFailed, err.Error())
		_ = r.Status().Update(ctx, dr)
		return ctrl.Result{}, err
	}

	return ctrl.Result{}, nil
}

func (r *DataResourceReconciler) reconcileDelete(ctx context.Context, dr *nestv1.DataResource) (ctrl.Result, error) {
	log.FromContext(ctx).Info("deleting DataResource", "name", dr.Name)

	// Dispatch to engine-specific delete handler
	switch dr.Spec.Type {
	case "postgres":
		_ = r.reconcilePostgresDelete(ctx, dr)
	case "mariadb":
		_ = r.reconcileMariaDBDelete(ctx, dr)
	case "mysql":
		_ = r.reconcileMySQLDelete(ctx, dr)
	case "keyvalue":
		_ = r.reconcileKeyvalueDelete(ctx, dr)
	case "kafka":
		_ = r.reconcileKafkaDelete(ctx, dr)
	case "search":
		_ = r.reconcileOpenSearchDelete(ctx, dr)
	case "rockfs":
		_ = r.reconcileFerretDBDelete(ctx, dr)
	case "vector":
		_ = r.reconcileVectorDelete(ctx, dr)
	case "clickhouse":
		_ = r.reconcileClickhouseDelete(ctx, dr)
	case "timeseries":
		_ = r.reconcileTimeseriesDelete(ctx, dr)
	case "warehouse/trino":
		_ = r.reconcileTrinoDelete(ctx, dr)
	case "lakehouse/iceberg":
		_ = r.reconcileIcebergDelete(ctx, dr)
	case "nfs":
		_ = r.reconcileNFSDelete(ctx, dr)
	case "iscsi":
		_ = r.reconcileISCSIDelete(ctx, dr)
	case "filesystem":
		_ = r.reconcileFilesystemDelete(ctx, dr)
	}

	dr.Finalizers = removeString(dr.Finalizers, "nest.penguintech.io/dataresource")
	return ctrl.Result{}, r.Update(ctx, dr)
}


func (r *DataResourceReconciler) reconcileObject(ctx context.Context, dr *nestv1.DataResource) error {
	log.FromContext(ctx).Info("reconciling Object DataResource (P1 stub)", "name", dr.Name)
	r.setPhase(dr, nestv1.PhaseReady, "Object store provisioned (P1 stub)")
	return r.Status().Update(ctx, dr)
}

func (r *DataResourceReconciler) reconcilePVCBlock(ctx context.Context, dr *nestv1.DataResource) error {
	log.FromContext(ctx).Info("reconciling PVC/Block DataResource (P1 stub)", "name", dr.Name)
	r.setPhase(dr, nestv1.PhaseReady, "PVC/Block provisioned (P1 stub)")
	return r.Status().Update(ctx, dr)
}

func (r *DataResourceReconciler) reconcilePVCFile(ctx context.Context, dr *nestv1.DataResource) error {
	log.FromContext(ctx).Info("reconciling PVC/File DataResource (P1 stub)", "name", dr.Name)
	r.setPhase(dr, nestv1.PhaseReady, "PVC/File provisioned (P1 stub)")
	return r.Status().Update(ctx, dr)
}


func (r *DataResourceReconciler) setPhase(dr *nestv1.DataResource, phase nestv1.DataResourcePhase, msg string) {
	dr.Status.Phase = phase
	meta.SetStatusCondition(&dr.Status.Conditions, metav1.Condition{
		Type:               string(phase),
		Status:             metav1.ConditionTrue,
		ObservedGeneration: dr.Generation,
		Reason:             string(phase),
		Message:            msg,
	})
}

func (r *DataResourceReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nestv1.DataResource{}).
		Complete(r)
}

func containsString(slice []string, s string) bool {
	for _, item := range slice {
		if item == s {
			return true
		}
	}
	return false
}

func removeString(slice []string, s string) []string {
	result := make([]string, 0, len(slice))
	for _, item := range slice {
		if item != s {
			result = append(result, item)
		}
	}
	return result
}
