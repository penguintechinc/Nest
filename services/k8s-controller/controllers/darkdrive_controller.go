package controllers

import (
	"context"
	"strings"
	"time"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// DarkDriveReconciler reconciles DarkDrive CRDs.
// It implements the approval state machine: Discovered → AwaitingApproval → Approved → Adopted (or Rejected).
type DarkDriveReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=nest.penguintech.io,resources=darkdrives,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nest.penguintech.io,resources=darkdrives/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nest.penguintech.io,resources=darkdrives/finalizers,verbs=update

func (r *DarkDriveReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var dd nestv1.DarkDrive
	if err := r.Get(ctx, req.NamespacedName, &dd); err != nil {
		if apierrors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("reconciling DarkDrive",
		"name", dd.Name,
		"node", dd.Spec.Node,
		"device", dd.Spec.Device,
		"state", dd.Status.State,
	)

	switch dd.Status.State {
	case "":
		// New resource — initialize to Discovered
		dd.Status.State = nestv1.DarkDriveDiscovered
		r.setCondition(&dd, "Discovered", metav1.ConditionTrue, "Discovered", "Drive discovered by node agent")
		if err := r.Status().Update(ctx, &dd); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil

	case nestv1.DarkDriveDiscovered:
		// Auto-advance to AwaitingApproval
		dd.Status.State = nestv1.DarkDriveAwaitingApproval
		r.setCondition(&dd, "AwaitingApproval", metav1.ConditionTrue, "AwaitingApproval", "Drive is awaiting operator approval")
		if err := r.Status().Update(ctx, &dd); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil

	case nestv1.DarkDriveAwaitingApproval:
		// Wait for operator to set state=Approved via UI/kubectl — nothing to do
		return ctrl.Result{}, nil

	case nestv1.DarkDriveApproved:
		return r.reconcileApproved(ctx, &dd)

	case nestv1.DarkDriveRejected, nestv1.DarkDriveAdopted:
		// Terminal states — nothing to do
		return ctrl.Result{}, nil

	default:
		logger.Info("unknown DarkDrive state, ignoring", "state", dd.Status.State)
		return ctrl.Result{}, nil
	}
}

// reconcileApproved handles the Approved → Adopted transition with safety checks.
func (r *DarkDriveReconciler) reconcileApproved(ctx context.Context, dd *nestv1.DarkDrive) (ctrl.Result, error) {
	// Foreign-fs safety gate: require explicit erase confirmation
	if strings.HasPrefix(dd.Spec.Signature, "foreign-fs:") && !dd.Spec.EraseConfirmed {
		r.setCondition(dd, "AdoptionBlocked", metav1.ConditionTrue, "EraseConfirmationRequired",
			"Drive has a foreign filesystem signature; set spec.eraseConfirmed=true to proceed")
		if err := r.Status().Update(ctx, dd); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// HardwarePool must be set before adoption
	if dd.Spec.HardwarePool == "" {
		r.setCondition(dd, "AdoptionBlocked", metav1.ConditionTrue, "HardwarePoolRequired",
			"spec.hardwarePool must be set before adoption can proceed")
		if err := r.Status().Update(ctx, dd); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// All checks passed — adopt the drive
	dd.Status.State = nestv1.DarkDriveAdopted
	// Clear any previous AdoptionBlocked condition
	meta.RemoveStatusCondition(&dd.Status.Conditions, "AdoptionBlocked")
	r.setCondition(dd, "Adopted", metav1.ConditionTrue, "Adopted",
		"Drive adopted into hardware pool "+dd.Spec.HardwarePool)
	if err := r.Status().Update(ctx, dd); err != nil {
		return ctrl.Result{}, err
	}
	return ctrl.Result{}, nil
}

// setCondition is a helper that calls meta.SetStatusCondition with the drive's generation.
func (r *DarkDriveReconciler) setCondition(dd *nestv1.DarkDrive, condType string, status metav1.ConditionStatus, reason, msg string) {
	meta.SetStatusCondition(&dd.Status.Conditions, metav1.Condition{
		Type:               condType,
		Status:             status,
		ObservedGeneration: dd.Generation,
		Reason:             reason,
		Message:            msg,
	})
}

func (r *DarkDriveReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nestv1.DarkDrive{}).
		Complete(r)
}
