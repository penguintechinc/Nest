package controllers

import (
	"context"
	"fmt"
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
	Scheme  *runtime.Scheme
	Adopter *DarkDriveAdopter
}

// +kubebuilder:rbac:groups=nest.penguintech.io,resources=darkdrives,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nest.penguintech.io,resources=darkdrives/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nest.penguintech.io,resources=darkdrives/finalizers,verbs=update
// +kubebuilder:rbac:groups=batch,resources=jobs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=ceph.rook.io,resources=cephclusters,verbs=get;list;watch;update;patch

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
		// Check if AutoApprove is enabled and signature allows auto-approval
		if dd.Spec.AutoApprove && (dd.Spec.Signature == "blank" || dd.Spec.Signature == "nest-previous") {
			logger.Info("auto-approving DarkDrive", "name", dd.Name, "signature", dd.Spec.Signature)
			dd.Status.State = nestv1.DarkDriveApproved
			dd.Status.ApprovedBy = "system:auto"
			now := metav1.Now()
			dd.Status.ApprovedAt = &now
			r.setCondition(&dd, "AutoApproved", metav1.ConditionTrue, "AutoApproved", "Drive auto-approved by system")
			if err := r.Status().Update(ctx, &dd); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{Requeue: true}, nil
		}
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
// Phase 1: Format the drive if needed (btrfs/zfs)
// Phase 2: Wait for format job to complete
// Phase 3: Patch CephCluster to enroll the device
// Phase 4: Transition to Adopted state
func (r *DarkDriveReconciler) reconcileApproved(ctx context.Context, dd *nestv1.DarkDrive) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	// Foreign-fs safety gate: require explicit erase confirmation
	if strings.HasPrefix(dd.Spec.Signature, "foreign-fs:") && !dd.Spec.EraseConfirmed {
		r.setCondition(dd, "AdoptionBlocked", metav1.ConditionTrue, "EraseConfirmationRequired",
			"Drive has a foreign filesystem signature; set spec.eraseConfirmed=true to proceed")
		if err := r.Status().Update(ctx, dd); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	// Auto-assign HardwarePool from drive class if not explicitly set
	if dd.Spec.HardwarePool == "" {
		if dd.Spec.Class == "" {
			// No class and no pool — cannot proceed; block adoption until pool is set
			r.setCondition(dd, "AdoptionBlocked", metav1.ConditionTrue, "HardwarePoolRequired",
				"spec.hardwarePool is required; set it or set spec.class to enable auto-assignment")
			if err := r.Status().Update(ctx, dd); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}
		dd.Spec.HardwarePool = hardwarePoolForClass(dd.Spec.Class)
		if err := r.Update(ctx, dd); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil
	}

	// Phase 1: Create format job if needed (btrfs or zfs)
	if dd.Spec.FsType == "btrfs" || dd.Spec.FsType == "zfs" {
		logger.Info("phase 1: formatting device", "fsType", dd.Spec.FsType, "device", dd.Spec.Device)
		if err := r.Adopter.CreateFormatJob(ctx, dd); err != nil {
			r.setCondition(dd, "FormattingFailed", metav1.ConditionTrue, "JobCreationError", fmt.Sprintf("Failed to create format job: %v", err))
			if err := r.Status().Update(ctx, dd); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		// Phase 2: Poll for format job completion
		complete, err := r.Adopter.IsFormatJobComplete(ctx, dd)
		if err != nil {
			r.setCondition(dd, "FormattingFailed", metav1.ConditionTrue, "JobError", fmt.Sprintf("Format job error: %v", err))
			if err := r.Status().Update(ctx, dd); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
		}

		if !complete {
			logger.Info("phase 2: waiting for format job to complete", "device", dd.Spec.Device)
			r.setCondition(dd, "Formatting", metav1.ConditionTrue, "InProgress", "Drive formatting in progress")
			if err := r.Status().Update(ctx, dd); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{RequeueAfter: 15 * time.Second}, nil
		}

		logger.Info("format job completed", "device", dd.Spec.Device, "fsType", dd.Spec.FsType)
		r.setCondition(dd, "Formatting", metav1.ConditionFalse, "Complete", "Drive formatting completed")
	}

	// Phase 3: Patch CephCluster to add device
	logger.Info("phase 3: patching CephCluster", "node", dd.Spec.Node, "device", dd.Spec.Device)
	if err := r.Adopter.PatchCephCluster(ctx, dd); err != nil {
		r.setCondition(dd, "CephClusterPatchFailed", metav1.ConditionTrue, "PatchError", fmt.Sprintf("Failed to patch CephCluster: %v", err))
		if err := r.Status().Update(ctx, dd); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
	}

	r.setCondition(dd, "CephClusterPatched", metav1.ConditionTrue, "Patched", "CephCluster updated with device enrollment")

	// Phase 4: Transition to Adopted state
	logger.Info("phase 4: transitioning to Adopted", "darkdrive", dd.Name)
	dd.Status.State = nestv1.DarkDriveAdopted
	// Clear any previous AdoptionBlocked conditions
	meta.RemoveStatusCondition(&dd.Status.Conditions, "AdoptionBlocked")
	meta.RemoveStatusCondition(&dd.Status.Conditions, "FormattingFailed")
	meta.RemoveStatusCondition(&dd.Status.Conditions, "CephClusterPatchFailed")
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

// hardwarePoolForClass maps the drive class reported by the node-agent to a HardwarePool name.
// The pool names correspond to HardwarePool CRs created during cluster setup.
func hardwarePoolForClass(class string) string {
	switch class {
	case "nvme-hot":
		return "nest-nvme"
	case "ssd-warm":
		return "nest-ssd"
	case "sata-bulk":
		return "nest-sata"
	case "sata-cold":
		return "nest-sata-cold"
	default:
		return "nest-default"
	}
}

func (r *DarkDriveReconciler) SetupWithManager(mgr ctrl.Manager) error {
	r.Adopter = &DarkDriveAdopter{Client: r.Client, Scheme: r.Scheme}
	return ctrl.NewControllerManagedBy(mgr).
		For(&nestv1.DarkDrive{}).
		Complete(r)
}
