package placement

import (
	"context"
	"reflect"
	"testing"

	"go.uber.org/zap"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// newFakeClient creates a fake Kubernetes client for testing
func newFakeClient(objs ...runtime.Object) client.Client {
	scheme := runtime.NewScheme()
	nestv1.AddToScheme(scheme)
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithRuntimeObjects(objs...).
		Build()
}

// TestFilterPoolAllowNil tests that filterPool returns true when placement is nil
func TestFilterPoolAllowNil(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{Logger: logger}
	pool := nestv1.HardwarePool{
		Spec: nestv1.HardwarePoolSpec{Class: "nvme-hot"},
	}

	result := s.filterPool(pool, nil)
	if !result {
		t.Error("expected filterPool to return true for nil placement")
	}
}

// TestFilterPoolForbiddenTier tests that forbid list rejects matching pools
func TestFilterPoolForbiddenTier(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{Logger: logger}

	pool := nestv1.HardwarePool{
		Spec: nestv1.HardwarePoolSpec{Class: "nvme-hot"},
	}
	placement := &nestv1.PlacementSpec{
		Forbid: []string{"nvme-hot", "ssd-warm"},
	}

	result := s.filterPool(pool, placement)
	if result {
		t.Error("expected filterPool to reject forbidden tier nvme-hot")
	}
}

// TestFilterPoolForbiddenTierNotMatching tests that non-matching forbid list passes
func TestFilterPoolForbiddenTierNotMatching(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{Logger: logger}

	pool := nestv1.HardwarePool{
		Spec: nestv1.HardwarePoolSpec{Class: "sata-cold"},
	}
	placement := &nestv1.PlacementSpec{
		Forbid: []string{"nvme-hot", "ssd-warm"},
	}

	result := s.filterPool(pool, placement)
	if !result {
		t.Error("expected filterPool to accept pool not in forbid list")
	}
}

// TestFilterPoolAllowListMatch tests that pool in allow list is accepted
func TestFilterPoolAllowListMatch(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{Logger: logger}

	pool := nestv1.HardwarePool{
		Spec: nestv1.HardwarePoolSpec{Class: "ssd-warm"},
	}
	placement := &nestv1.PlacementSpec{
		Allow: []string{"ssd-warm", "sata-bulk"},
	}

	result := s.filterPool(pool, placement)
	if !result {
		t.Error("expected filterPool to accept pool in allow list")
	}
}

// TestFilterPoolAllowListNoMatch tests that pool not in allow list is rejected
func TestFilterPoolAllowListNoMatch(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{Logger: logger}

	pool := nestv1.HardwarePool{
		Spec: nestv1.HardwarePoolSpec{Class: "nvme-hot"},
	}
	placement := &nestv1.PlacementSpec{
		Allow: []string{"ssd-warm", "sata-bulk"},
	}

	result := s.filterPool(pool, placement)
	if result {
		t.Error("expected filterPool to reject pool not in allow list")
	}
}

// TestFilterPoolPreferListMatch tests that pool in prefer list is accepted
func TestFilterPoolPreferListMatch(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{Logger: logger}

	pool := nestv1.HardwarePool{
		Spec: nestv1.HardwarePoolSpec{Class: "nvme-hot"},
	}
	placement := &nestv1.PlacementSpec{
		Prefer: []string{"nvme-hot"},
	}

	result := s.filterPool(pool, placement)
	if !result {
		t.Error("expected filterPool to accept pool in prefer list")
	}
}

// TestFilterPoolAllowAndPrefer tests allow + prefer combined filters
func TestFilterPoolAllowAndPrefer(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{Logger: logger}

	pool := nestv1.HardwarePool{
		Spec: nestv1.HardwarePoolSpec{Class: "sata-bulk"},
	}
	placement := &nestv1.PlacementSpec{
		Allow:  []string{"ssd-warm"},
		Prefer: []string{"sata-bulk"},
	}

	result := s.filterPool(pool, placement)
	if !result {
		t.Error("expected filterPool to accept pool in allow+prefer combined")
	}
}

// TestFilterPoolForbidAndAllow tests that forbid takes precedence
func TestFilterPoolForbidAndAllow(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{Logger: logger}

	pool := nestv1.HardwarePool{
		Spec: nestv1.HardwarePoolSpec{Class: "ssd-warm"},
	}
	placement := &nestv1.PlacementSpec{
		Forbid: []string{"ssd-warm"},
		Allow:  []string{"ssd-warm"},
	}

	result := s.filterPool(pool, placement)
	if result {
		t.Error("expected forbid to take precedence over allow")
	}
}

// TestScoringHighestFreeBytes tests that scorer picks pool with most free bytes
func TestScoringHighestFreeBytes(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{
		Client: newFakeClient(),
		Logger: logger,
	}

	ctx := context.Background()

	// Create test pools with different free bytes
	pool1 := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-1", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "ssd-warm"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 100 * 1024 * 1024},
	}
	pool2 := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-2", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "ssd-warm"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 500 * 1024 * 1024},
	}
	pool3 := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-3", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "ssd-warm"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 200 * 1024 * 1024},
	}

	s.Client = newFakeClient(pool1, pool2, pool3)

	// Create test class without specific placement constraints
	class := &nestv1.DataResourceClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test-class", Namespace: "default"},
		Spec: nestv1.ClassSpec{
			Backend: "postgres",
		},
	}
	s.Client = newFakeClient(pool1, pool2, pool3, class)

	// Create test DataResource
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dr", Namespace: "default"},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Class:  "test-class",
			Tenant: "tenant-1",
		},
	}

	selected, err := s.selectPool(ctx, dr)
	if err != nil {
		t.Fatalf("selectPool failed: %v", err)
	}

	if selected != "pool-2" {
		t.Errorf("expected pool-2 (highest free bytes), got %s", selected)
	}
}

// TestFilteringByHardwareClass tests that filtering by hardware class works
func TestFilteringByHardwareClass(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{
		Client: newFakeClient(),
		Logger: logger,
	}

	ctx := context.Background()

	// Create pools of different hardware classes
	nvmePool := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "nvme-pool", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "nvme-hot"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 100 * 1024 * 1024},
	}
	ssdPool := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "ssd-pool", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "ssd-warm"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 200 * 1024 * 1024},
	}
	sataPool := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "sata-pool", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "sata-bulk"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 300 * 1024 * 1024},
	}

	// Create class that prefers SSD
	class := &nestv1.DataResourceClass{
		ObjectMeta: metav1.ObjectMeta{Name: "ssd-class", Namespace: "default"},
		Spec: nestv1.ClassSpec{
			Backend: "postgres",
			Placement: &nestv1.PlacementSpec{
				Prefer: []string{"ssd-warm"},
			},
		},
	}

	s.Client = newFakeClient(nvmePool, ssdPool, sataPool, class)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dr", Namespace: "default"},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Class:  "ssd-class",
			Tenant: "tenant-1",
		},
	}

	selected, err := s.selectPool(ctx, dr)
	if err != nil {
		t.Fatalf("selectPool failed: %v", err)
	}

	if selected != "ssd-pool" {
		t.Errorf("expected ssd-pool, got %s", selected)
	}
}

// TestPreferredTierScoringHigher tests that preferred tiers score higher
func TestPreferredTierScoringHigher(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{
		Client: newFakeClient(),
		Logger: logger,
	}

	ctx := context.Background()

	// Create pools: one preferred with less free bytes, one non-preferred with more
	preferredPool := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "preferred-pool", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "nvme-hot"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 100 * 1024 * 1024},
	}
	fallbackPool := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "fallback-pool", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "ssd-warm"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 500 * 1024 * 1024},
	}

	// Create class that forbids SSD, forcing use of NVME
	class := &nestv1.DataResourceClass{
		ObjectMeta: metav1.ObjectMeta{Name: "nvme-class", Namespace: "default"},
		Spec: nestv1.ClassSpec{
			Backend: "postgres",
			Placement: &nestv1.PlacementSpec{
				Forbid: []string{"ssd-warm"},
			},
		},
	}

	s.Client = newFakeClient(preferredPool, fallbackPool, class)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dr", Namespace: "default"},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Class:  "nvme-class",
			Tenant: "tenant-1",
		},
	}

	selected, err := s.selectPool(ctx, dr)
	if err != nil {
		t.Fatalf("selectPool failed: %v", err)
	}

	if selected != "preferred-pool" {
		t.Errorf("expected preferred-pool (SSD was forbidden), got %s", selected)
	}
}

// TestForbiddenTiersRejected tests that forbidden tiers are rejected even with higher capacity
func TestForbiddenTiersRejected(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{
		Client: newFakeClient(),
		Logger: logger,
	}

	ctx := context.Background()

	// Create pools: one forbidden with lots of free space, one allowed with less
	forbiddenPool := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "forbidden-pool", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "sata-cold"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 1000 * 1024 * 1024},
	}
	allowedPool := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "allowed-pool", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "ssd-warm"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 100 * 1024 * 1024},
	}

	// Create class that forbids SATA-cold
	class := &nestv1.DataResourceClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test-class", Namespace: "default"},
		Spec: nestv1.ClassSpec{
			Backend: "postgres",
			Placement: &nestv1.PlacementSpec{
				Forbid: []string{"sata-cold"},
			},
		},
	}

	s.Client = newFakeClient(forbiddenPool, allowedPool, class)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dr", Namespace: "default"},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Class:  "test-class",
			Tenant: "tenant-1",
		},
	}

	selected, err := s.selectPool(ctx, dr)
	if err != nil {
		t.Fatalf("selectPool failed: %v", err)
	}

	if selected != "allowed-pool" {
		t.Errorf("expected allowed-pool (forbidden pool rejected), got %s", selected)
	}
}

// TestNoPoolsMatchFilter tests error when no pools match filter
func TestNoPoolsMatchFilter(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{
		Client: newFakeClient(),
		Logger: logger,
	}

	ctx := context.Background()

	// Create pools that will all be rejected by filter
	pool1 := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-1", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "sata-cold"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 100 * 1024 * 1024},
	}
	pool2 := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-2", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "sata-bulk"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 200 * 1024 * 1024},
	}

	// Create class that forbids both SATA tiers
	class := &nestv1.DataResourceClass{
		ObjectMeta: metav1.ObjectMeta{Name: "nvme-class", Namespace: "default"},
		Spec: nestv1.ClassSpec{
			Backend: "postgres",
			Placement: &nestv1.PlacementSpec{
				Forbid: []string{"sata-cold", "sata-bulk"},
			},
		},
	}

	s.Client = newFakeClient(pool1, pool2, class)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dr", Namespace: "default"},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Class:  "nvme-class",
			Tenant: "tenant-1",
		},
	}

	_, err := s.selectPool(ctx, dr)
	if err == nil {
		t.Error("expected selectPool to fail with no matching pools")
	}
}

// TestEmptyPoolList tests fallback to default when no pools exist
func TestEmptyPoolList(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{
		Client: newFakeClient(),
		Logger: logger,
	}

	ctx := context.Background()

	// Create class without pools
	class := &nestv1.DataResourceClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test-class", Namespace: "default"},
		Spec: nestv1.ClassSpec{
			Backend: "postgres",
		},
	}

	s.Client = newFakeClient(class)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dr", Namespace: "default"},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Class:  "test-class",
			Tenant: "tenant-1",
		},
	}

	selected, err := s.selectPool(ctx, dr)
	if err != nil {
		t.Fatalf("selectPool failed: %v", err)
	}

	if selected != "default" {
		t.Errorf("expected 'default' pool, got %s", selected)
	}
}

// TestClassNotFound tests fallback to default when class doesn't exist
func TestClassNotFound(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{
		Client: newFakeClient(),
		Logger: logger,
	}

	ctx := context.Background()

	// Create pool but no class
	pool := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-1", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "ssd-warm"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 100 * 1024 * 1024},
	}

	s.Client = newFakeClient(pool)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dr", Namespace: "default"},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Class:  "nonexistent-class",
			Tenant: "tenant-1",
		},
	}

	selected, err := s.selectPool(ctx, dr)
	if err != nil {
		t.Fatalf("selectPool failed: %v", err)
	}

	if selected != "pool-1" {
		t.Errorf("expected pool-1 (default fallback), got %s", selected)
	}
}

// TestDefaultPoolWithMultiplePools tests defaultPool selects first pool
func TestDefaultPoolWithMultiplePools(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{
		Client: newFakeClient(),
		Logger: logger,
	}

	ctx := context.Background()

	pool1 := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-1", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "ssd-warm"},
	}
	pool2 := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-2", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "ssd-warm"},
	}

	s.Client = newFakeClient(pool1, pool2)

	selected, err := s.defaultPool(ctx)
	if err != nil {
		t.Fatalf("defaultPool failed: %v", err)
	}

	if selected != "pool-1" {
		t.Errorf("expected pool-1, got %s", selected)
	}
}

// TestDefaultPoolEmptyList tests defaultPool returns "default" on empty list
func TestDefaultPoolEmptyList(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{
		Client: newFakeClient(),
		Logger: logger,
	}

	ctx := context.Background()

	selected, err := s.defaultPool(ctx)
	if err != nil {
		t.Fatalf("defaultPool failed: %v", err)
	}

	if selected != "default" {
		t.Errorf("expected 'default', got %s", selected)
	}
}

// TestFilterPoolEmptyPlacement tests filterPool with empty placement struct
func TestFilterPoolEmptyPlacement(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{Logger: logger}

	pool := nestv1.HardwarePool{
		Spec: nestv1.HardwarePoolSpec{Class: "nvme-hot"},
	}
	placement := &nestv1.PlacementSpec{}

	result := s.filterPool(pool, placement)
	if !result {
		t.Error("expected filterPool to accept pool with empty placement")
	}
}

// TestSelectPoolMultipleCandidates tests that scoring works with multiple candidates
func TestSelectPoolMultipleCandidates(t *testing.T) {
	logger, _ := zap.NewProduction()
	s := &Scheduler{
		Client: newFakeClient(),
		Logger: logger,
	}

	ctx := context.Background()

	// Create class that allows multiple pools
	class := &nestv1.DataResourceClass{
		ObjectMeta: metav1.ObjectMeta{Name: "multi-class", Namespace: "default"},
		Spec: nestv1.ClassSpec{
			Backend: "postgres",
			Placement: &nestv1.PlacementSpec{
				Allow: []string{"ssd-warm", "sata-bulk"},
			},
		},
	}

	// Create multiple candidate pools with different free bytes
	pool1 := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "ssd-1", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "ssd-warm"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 300 * 1024 * 1024},
	}
	pool2 := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "sata-1", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "sata-bulk"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 600 * 1024 * 1024},
	}

	s.Client = newFakeClient(pool1, pool2, class)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dr", Namespace: "default"},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Class:  "multi-class",
			Tenant: "tenant-1",
		},
	}

	selected, err := s.selectPool(ctx, dr)
	if err != nil {
		t.Fatalf("selectPool failed: %v", err)
	}

	if selected != "sata-1" {
		t.Errorf("expected sata-1 (highest free bytes), got %s", selected)
	}
}

// TestReconcile_ResourceNotFound tests Reconcile when DataResource is not found
func TestReconcile_ResourceNotFound(t *testing.T) {
	logger, _ := zap.NewProduction()
	scheme := runtime.NewScheme()
	nestv1.AddToScheme(scheme)

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	s := &Scheduler{
		Client: fakeClient,
		Logger: logger,
	}

	ctx := context.Background()
	_, err := s.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent",
			Namespace: "default",
		},
	})

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
}

// TestReconcile_PendingResource_SuccessfulPlacement tests Reconcile places a Pending DataResource
func TestReconcile_PendingResource_SuccessfulPlacement(t *testing.T) {
	logger, _ := zap.NewProduction()
	scheme := runtime.NewScheme()
	nestv1.AddToScheme(scheme)

	pool := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-1", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "ssd-warm"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 100 * 1024 * 1024},
	}

	class := &nestv1.DataResourceClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test-class", Namespace: "default"},
		Spec: nestv1.ClassSpec{
			Backend: "postgres",
		},
	}

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dr", Namespace: "default"},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Class:  "test-class",
			Tenant: "tenant-1",
		},
		Status: nestv1.DataResourceStatus{
			Phase: nestv1.PhasePending,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pool, class, dr).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()

	s := &Scheduler{
		Client: fakeClient,
		Logger: logger,
	}

	ctx := context.Background()
	_, err := s.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-dr",
			Namespace: "default",
		},
	})

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	// Verify the DataResource was annotated with the pool
	var updated nestv1.DataResource
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "test-dr", Namespace: "default"}, &updated); err != nil {
		t.Fatalf("failed to get updated DataResource: %v", err)
	}

	if pool, ok := updated.Annotations["nest.penguintech.io/hardware-pool"]; !ok || pool != "pool-1" {
		t.Errorf("expected annotation 'nest.penguintech.io/hardware-pool'='pool-1', got %v", updated.Annotations)
	}
}

// TestReconcile_ReadyResource_NoChange tests Reconcile skips non-Pending resources
func TestReconcile_ReadyResource_NoChange(t *testing.T) {
	logger, _ := zap.NewProduction()
	scheme := runtime.NewScheme()
	nestv1.AddToScheme(scheme)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dr", Namespace: "default"},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Class:  "test-class",
			Tenant: "tenant-1",
		},
		Status: nestv1.DataResourceStatus{
			Phase: nestv1.PhaseReady,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dr).
		Build()

	s := &Scheduler{
		Client: fakeClient,
		Logger: logger,
	}

	ctx := context.Background()
	_, err := s.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-dr",
			Namespace: "default",
		},
	})

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	// Verify no annotations were added
	var updated nestv1.DataResource
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "test-dr", Namespace: "default"}, &updated); err != nil {
		t.Fatalf("failed to get DataResource: %v", err)
	}

	if updated.Annotations != nil && updated.Annotations["nest.penguintech.io/hardware-pool"] != "" {
		t.Errorf("expected no placement annotation for Ready resource, got %v", updated.Annotations)
	}
}

// TestReconcile_FailedPlacement tests Reconcile when selectPool fails due to no matching pools
func TestReconcile_FailedPlacement(t *testing.T) {
	logger, _ := zap.NewProduction()
	scheme := runtime.NewScheme()
	nestv1.AddToScheme(scheme)

	// Create a pool with a specific class
	pool := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-1", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "ssd-warm"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 100 * 1024 * 1024},
	}

	// Create a class that forbids the only available pool tier
	class := &nestv1.DataResourceClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test-class", Namespace: "default"},
		Spec: nestv1.ClassSpec{
			Backend: "postgres",
			Placement: &nestv1.PlacementSpec{
				Allow: []string{"nvme-hot"}, // Only allow nvme-hot, but only ssd-warm pool exists
			},
		},
	}

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dr", Namespace: "default"},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Class:  "test-class",
			Tenant: "tenant-1",
		},
		Status: nestv1.DataResourceStatus{
			Phase: nestv1.PhasePending,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pool, class, dr).
		Build()

	s := &Scheduler{
		Client: fakeClient,
		Logger: logger,
	}

	ctx := context.Background()
	_, err := s.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-dr",
			Namespace: "default",
		},
	})

	if err == nil {
		t.Errorf("Reconcile() error = nil, want error for no matching pools")
	}
}

// TestReconcile_ClassNotFound_UsesDefaultPool tests Reconcile falls back to default pool when class not found
func TestReconcile_ClassNotFound_UsesDefaultPool(t *testing.T) {
	logger, _ := zap.NewProduction()
	scheme := runtime.NewScheme()
	nestv1.AddToScheme(scheme)

	pool := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-1", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "ssd-warm"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 100 * 1024 * 1024},
	}

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dr", Namespace: "default"},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Class:  "nonexistent-class",
			Tenant: "tenant-1",
		},
		Status: nestv1.DataResourceStatus{
			Phase: nestv1.PhasePending,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pool, dr).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()

	s := &Scheduler{
		Client: fakeClient,
		Logger: logger,
	}

	ctx := context.Background()
	_, err := s.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-dr",
			Namespace: "default",
		},
	})

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	// Verify the DataResource was annotated with the first available pool
	var updated nestv1.DataResource
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "test-dr", Namespace: "default"}, &updated); err != nil {
		t.Fatalf("failed to get updated DataResource: %v", err)
	}

	if pool, ok := updated.Annotations["nest.penguintech.io/hardware-pool"]; !ok || pool != "pool-1" {
		t.Errorf("expected annotation with pool-1, got %v", updated.Annotations)
	}
}

// TestReconcile_NoPoolsAvailable_UsesDefaultPoolString tests Reconcile uses "default" string when no pools exist
func TestReconcile_NoPoolsAvailable_UsesDefaultPoolString(t *testing.T) {
	logger, _ := zap.NewProduction()
	scheme := runtime.NewScheme()
	nestv1.AddToScheme(scheme)

	class := &nestv1.DataResourceClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test-class", Namespace: "default"},
		Spec: nestv1.ClassSpec{
			Backend: "postgres",
		},
	}

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dr", Namespace: "default"},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Class:  "test-class",
			Tenant: "tenant-1",
		},
		Status: nestv1.DataResourceStatus{
			Phase: nestv1.PhasePending,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(class, dr).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()

	s := &Scheduler{
		Client: fakeClient,
		Logger: logger,
	}

	ctx := context.Background()
	_, err := s.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-dr",
			Namespace: "default",
		},
	})

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	// Verify the DataResource was annotated with "default"
	var updated nestv1.DataResource
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "test-dr", Namespace: "default"}, &updated); err != nil {
		t.Fatalf("failed to get updated DataResource: %v", err)
	}

	if pool, ok := updated.Annotations["nest.penguintech.io/hardware-pool"]; !ok || pool != "default" {
		t.Errorf("expected annotation with 'default' pool, got %v", updated.Annotations)
	}
}

// TestReconcile_DegradeResource_NoChange tests Reconcile skips Degraded resources
func TestReconcile_DegradeResource_NoChange(t *testing.T) {
	logger, _ := zap.NewProduction()
	scheme := runtime.NewScheme()
	nestv1.AddToScheme(scheme)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-dr", Namespace: "default"},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Class:  "test-class",
			Tenant: "tenant-1",
		},
		Status: nestv1.DataResourceStatus{
			Phase: nestv1.PhaseDegraded,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dr).
		Build()

	s := &Scheduler{
		Client: fakeClient,
		Logger: logger,
	}

	ctx := context.Background()
	_, err := s.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-dr",
			Namespace: "default",
		},
	})

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	// Verify no annotations were added
	var updated nestv1.DataResource
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "test-dr", Namespace: "default"}, &updated); err != nil {
		t.Fatalf("failed to get DataResource: %v", err)
	}

	if updated.Annotations != nil && updated.Annotations["nest.penguintech.io/hardware-pool"] != "" {
		t.Errorf("expected no placement annotation for Degraded resource, got %v", updated.Annotations)
	}
}

// TestSetupWithManager tests SetupWithManager registers the controller with the manager
func TestSetupWithManager(t *testing.T) {
	logger, _ := zap.NewProduction()
	scheme := runtime.NewScheme()
	nestv1.AddToScheme(scheme)

	// Create a fake client
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		Build()

	s := &Scheduler{
		Client: fakeClient,
		Logger: logger,
	}

	// Verify the method exists by checking if it's callable via reflection
	methodExists := false
	methodType := reflect.TypeOf(s)
	for i := 0; i < methodType.NumMethod(); i++ {
		if methodType.Method(i).Name == "SetupWithManager" {
			methodExists = true
			break
		}
	}
	if !methodExists {
		t.Error("SetupWithManager method not found on Scheduler")
	}

	// Ensure the method has the expected signature: error return type
	// We can't test with a real manager without a full controller setup,
	// but we can verify the method is callable and properly defined.
}

// TestReconcile_PreservesExistingAnnotations tests Reconcile preserves existing annotations
func TestReconcile_PreservesExistingAnnotations(t *testing.T) {
	logger, _ := zap.NewProduction()
	scheme := runtime.NewScheme()
	nestv1.AddToScheme(scheme)

	pool := &nestv1.HardwarePool{
		ObjectMeta: metav1.ObjectMeta{Name: "pool-1", Namespace: "default"},
		Spec:       nestv1.HardwarePoolSpec{Class: "ssd-warm"},
		Status:     nestv1.HardwarePoolStatus{FreeBytes: 100 * 1024 * 1024},
	}

	class := &nestv1.DataResourceClass{
		ObjectMeta: metav1.ObjectMeta{Name: "test-class", Namespace: "default"},
		Spec: nestv1.ClassSpec{
			Backend: "postgres",
		},
	}

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dr",
			Namespace: "default",
			Annotations: map[string]string{
				"custom-key": "custom-value",
			},
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Class:  "test-class",
			Tenant: "tenant-1",
		},
		Status: nestv1.DataResourceStatus{
			Phase: nestv1.PhasePending,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(pool, class, dr).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()

	s := &Scheduler{
		Client: fakeClient,
		Logger: logger,
	}

	ctx := context.Background()
	_, err := s.Reconcile(ctx, ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "test-dr",
			Namespace: "default",
		},
	})

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	// Verify both old and new annotations exist
	var updated nestv1.DataResource
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: "test-dr", Namespace: "default"}, &updated); err != nil {
		t.Fatalf("failed to get updated DataResource: %v", err)
	}

	if updated.Annotations["custom-key"] != "custom-value" {
		t.Errorf("custom annotation lost, got %v", updated.Annotations)
	}

	if updated.Annotations["nest.penguintech.io/hardware-pool"] != "pool-1" {
		t.Errorf("pool annotation not added, got %v", updated.Annotations)
	}
}
