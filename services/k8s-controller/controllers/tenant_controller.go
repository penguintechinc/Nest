package controllers

import (
	"context"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// TenantReconciler reconciles Tenant CRDs.
type TenantReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=nest.penguintech.io,resources=tenants,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nest.penguintech.io,resources=tenants/status,verbs=get;update;patch

func (r *TenantReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var tenant nestv1.Tenant
	if err := r.Get(ctx, req.NamespacedName, &tenant); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	logger.Info("reconciling Tenant", "name", tenant.Name, "tier", tenant.Spec.LicenseTier)

	// Ensure default quotas for free tier
	if tenant.Spec.LicenseTier == "" || tenant.Spec.LicenseTier == "free" {
		changed := false
		if tenant.Spec.Quota == nil {
			tenant.Spec.Quota = &nestv1.QuotaSpec{}
		}
		if tenant.Spec.Quota.MaxDataResources == 0 {
			tenant.Spec.Quota.MaxDataResources = 5
			changed = true
		}
		if tenant.Spec.Quota.MaxOperatorAccounts == 0 {
			tenant.Spec.Quota.MaxOperatorAccounts = 3
			changed = true
		}
		if tenant.Spec.Quota.MaxResourceAccounts == 0 {
			tenant.Spec.Quota.MaxResourceAccounts = 3
			changed = true
		}
		if changed {
			if err := r.Update(ctx, &tenant); err != nil {
				return ctrl.Result{}, err
			}
		}
	}

	return ctrl.Result{}, nil
}

func (r *TenantReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nestv1.Tenant{}).
		Complete(r)
}
