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
	case "object":
		err = r.reconcileObject(ctx, dr)
	case "pvc/block":
		err = r.reconcilePVCBlock(ctx, dr)
	case "pvc/file":
		err = r.reconcilePVCFile(ctx, dr)
	case "keyvalue":
		err = r.reconcileKeyvalue(ctx, dr)
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
	// TODO(P2): decommission upstream operator CR

	dr.Finalizers = removeString(dr.Finalizers, "nest.penguintech.io/dataresource")
	return ctrl.Result{}, r.Update(ctx, dr)
}

// reconcilePostgres creates/updates a CloudNativePG Cluster CR.
// P1: emits a placeholder status; real CloudNativePG integration in P4.
func (r *DataResourceReconciler) reconcilePostgres(ctx context.Context, dr *nestv1.DataResource) error {
	log.FromContext(ctx).Info("reconciling Postgres DataResource (P1 stub)", "name", dr.Name)
	r.setPhase(dr, nestv1.PhaseReady, "Postgres provisioned (P1 stub)")
	return r.Status().Update(ctx, dr)
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

func (r *DataResourceReconciler) reconcileKeyvalue(ctx context.Context, dr *nestv1.DataResource) error {
	log.FromContext(ctx).Info("reconciling Keyvalue DataResource (P1 stub)", "name", dr.Name)
	r.setPhase(dr, nestv1.PhaseReady, "Keyvalue provisioned (P1 stub)")
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
