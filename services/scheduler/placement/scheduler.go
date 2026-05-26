// Package placement implements the Nest hardware-aware placement engine (§3.2).
// It runs as a controller that watches DataResource CRDs and assigns each to
// a HardwarePool based on the filter+score pipeline.
package placement

import (
	"context"
	"fmt"

	"go.uber.org/zap"
	"k8s.io/apimachinery/pkg/api/errors"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// Scheduler watches DataResource + HardwarePool CRDs and assigns placement
type Scheduler struct {
	client.Client
	Logger *zap.Logger
}

func (s *Scheduler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var dr nestv1.DataResource
	if err := s.Get(ctx, req.NamespacedName, &dr); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Only schedule if pending
	if dr.Status.Phase != nestv1.PhasePending {
		return ctrl.Result{}, nil
	}

	pool, err := s.selectPool(ctx, &dr)
	if err != nil {
		s.Logger.Warn("placement failed",
			zap.String("resource", dr.Name),
			zap.Error(err),
		)
		return ctrl.Result{}, err
	}

	s.Logger.Info("placed DataResource",
		zap.String("resource", dr.Name),
		zap.String("pool", pool),
	)

	// Annotate the resource with the selected pool
	if dr.Annotations == nil {
		dr.Annotations = make(map[string]string)
	}
	dr.Annotations["nest.penguintech.io/hardware-pool"] = pool
	return ctrl.Result{}, s.Update(ctx, &dr)
}

// selectPool runs the filter+score pipeline to select a HardwarePool
func (s *Scheduler) selectPool(ctx context.Context, dr *nestv1.DataResource) (string, error) {
	// Fetch the DataResourceClass
	var class nestv1.DataResourceClass
	if err := s.Get(ctx, client.ObjectKey{
		Namespace: dr.Namespace,
		Name:      dr.Spec.Class,
	}, &class); err != nil {
		// Class not found — use default pool
		return s.defaultPool(ctx)
	}

	// List all HardwarePools
	var pools nestv1.HardwarePoolList
	if err := s.List(ctx, &pools); err != nil {
		return "", err
	}

	if len(pools.Items) == 0 {
		return "default", nil
	}

	// Filter phase: keep only pools that satisfy the class placement policy
	var candidates []nestv1.HardwarePool
	for _, pool := range pools.Items {
		if s.filterPool(pool, class.Spec.Placement) {
			candidates = append(candidates, pool)
		}
	}

	if len(candidates) == 0 {
		return "", fmt.Errorf("no HardwarePool satisfies placement policy for class %s", dr.Spec.Class)
	}

	// Score phase: prefer pools with dark drives, then pick by most free bytes
	best := candidates[0]
	for _, pool := range candidates[1:] {
		bestHasDark := s.poolHasDarkDrives(ctx, &best)
		poolHasDark := s.poolHasDarkDrives(ctx, &pool)
		// Pool with dark drives beats pool without
		if poolHasDark && !bestHasDark {
			best = pool
			continue
		}
		// Both same dark-drive status: pick by free bytes
		if poolHasDark == bestHasDark && pool.Status.FreeBytes > best.Status.FreeBytes {
			best = pool
		}
	}

	return best.Name, nil
}

// poolHasDarkDrives checks if a HardwarePool has any dark (unallocated) drives by
// querying HardwareInventory CRs for nodes in the pool.
func (s *Scheduler) poolHasDarkDrives(ctx context.Context, pool *nestv1.HardwarePool) bool {
	inventories := &nestv1.HardwareInventoryList{}
	if err := s.List(ctx, inventories); err != nil {
		return false
	}

	for _, inv := range inventories.Items {
		for _, node := range pool.Spec.Nodes {
			if inv.Spec.Node == node && inv.Status.DarkDriveCount > 0 {
				return true
			}
		}
	}
	return false
}

// filterPool returns true if the pool satisfies the placement policy
func (s *Scheduler) filterPool(pool nestv1.HardwarePool, placement *nestv1.PlacementSpec) bool {
	if placement == nil {
		return true
	}
	// Check forbid list
	for _, forbidden := range placement.Forbid {
		if pool.Spec.Class == forbidden {
			return false
		}
	}
	// Check prefer/allow list (if non-empty, pool must be in allow)
	if len(placement.Allow) > 0 || len(placement.Prefer) > 0 {
		allowed := append(placement.Allow, placement.Prefer...)
		for _, a := range allowed {
			if pool.Spec.Class == a {
				return true
			}
		}
		return false
	}
	return true
}

// defaultPool selects the default HardwarePool when no class preference is given.
// Pools containing dark (unallocated) drives are preferred over pools with only active drives,
// reducing wear on system drives and ensuring dedicated storage is used first.
func (s *Scheduler) defaultPool(ctx context.Context) (string, error) {
	var pools nestv1.HardwarePoolList
	if err := s.List(ctx, &pools); err != nil || len(pools.Items) == 0 {
		return "default", nil
	}

	// Prefer pools with dark drives (DarkDriveCount > 0 in status)
	for _, pool := range pools.Items {
		if s.poolHasDarkDrives(ctx, &pool) {
			return pool.Name, nil
		}
	}

	// Fall back to largest free capacity
	best := pools.Items[0]
	for _, pool := range pools.Items[1:] {
		if pool.Status.FreeBytes > best.Status.FreeBytes {
			best = pool
		}
	}
	return best.Name, nil
}

func (s *Scheduler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nestv1.DataResource{}).
		Complete(s)
}
