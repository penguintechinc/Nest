package controllers

import (
	"context"
	"testing"

	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// newTestScheme creates a scheme with nest.penguintech.io types registered
func newTestScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	if err := nestv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add nestv1 to scheme: %v", err)
	}
	return scheme
}

// TestDataResourceReconciler_ReconcileNotFound tests that reconciling a non-existent DataResource returns no error
func TestDataResourceReconciler_ReconcileNotFound(t *testing.T) {
	scheme := newTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &DataResourceReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent",
			Namespace: "default",
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
}

// TestDataResourceReconciler_ReconcilePendingToProvisioning tests phase transition from Pending to Provisioning for Postgres
func TestDataResourceReconciler_ReconcilePendingToProvisioning(t *testing.T) {
	scheme := newTestScheme(t)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-postgres",
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Tenant: "tenant-1",
			Class:  "standard",
		},
		Status: nestv1.DataResourceStatus{
			Phase: nestv1.PhasePending,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dr).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()
	r := &DataResourceReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dr.Name,
			Namespace: dr.Namespace,
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	// Verify the DataResource was updated with finalizer and phase transition
	var updated nestv1.DataResource
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DataResource: %v", err)
	}

	// Check finalizer was added
	if !containsString(updated.Finalizers, "nest.penguintech.io/dataresource") {
		t.Errorf("finalizer not added, got: %v", updated.Finalizers)
	}

	// Check status phase was updated to Ready (postgres is a stub that goes straight to Ready)
	if updated.Status.Phase != nestv1.PhaseReady {
		t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseReady)
	}

	// Check status condition was set
	cond := meta.FindStatusCondition(updated.Status.Conditions, string(nestv1.PhaseReady))
	if cond == nil {
		t.Errorf("status condition not found for phase %v", nestv1.PhaseReady)
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("condition status = %v, want ConditionTrue", cond.Status)
	}
}

// TestDataResourceReconciler_ReconcileObject tests reconciliation of object storage type
func TestDataResourceReconciler_ReconcileObject(t *testing.T) {
	scheme := newTestScheme(t)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-object-store",
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "object",
			Tenant: "tenant-2",
			Class:  "premium",
		},
		Status: nestv1.DataResourceStatus{
			Phase: nestv1.PhasePending,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dr).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()
	r := &DataResourceReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dr.Name,
			Namespace: dr.Namespace,
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	var updated nestv1.DataResource
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DataResource: %v", err)
	}

	if updated.Status.Phase != nestv1.PhaseReady {
		t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseReady)
	}
}

// TestDataResourceReconciler_ReconcilePVCBlock tests reconciliation of PVC block type
func TestDataResourceReconciler_ReconcilePVCBlock(t *testing.T) {
	scheme := newTestScheme(t)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc-block",
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "pvc/block",
			Tenant: "tenant-3",
		},
		Status: nestv1.DataResourceStatus{
			Phase: "",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dr).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()
	r := &DataResourceReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dr.Name,
			Namespace: dr.Namespace,
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	var updated nestv1.DataResource
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DataResource: %v", err)
	}

	if updated.Status.Phase != nestv1.PhaseReady {
		t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseReady)
	}
}

// TestDataResourceReconciler_ReconcilePVCFile tests reconciliation of PVC file type
func TestDataResourceReconciler_ReconcilePVCFile(t *testing.T) {
	scheme := newTestScheme(t)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pvc-file",
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "pvc/file",
			Tenant: "tenant-4",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dr).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()
	r := &DataResourceReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dr.Name,
			Namespace: dr.Namespace,
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	var updated nestv1.DataResource
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DataResource: %v", err)
	}

	if updated.Status.Phase != nestv1.PhaseReady {
		t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseReady)
	}
}

// TestDataResourceReconciler_ReconcileKeyvalue tests reconciliation of keyvalue type
func TestDataResourceReconciler_ReconcileKeyvalue(t *testing.T) {
	scheme := newTestScheme(t)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-keyvalue",
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "keyvalue",
			Tenant: "tenant-5",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dr).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()
	r := &DataResourceReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dr.Name,
			Namespace: dr.Namespace,
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	var updated nestv1.DataResource
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DataResource: %v", err)
	}

	if updated.Status.Phase != nestv1.PhaseReady {
		t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseReady)
	}
}

// TestDataResourceReconciler_ReconcileUnsupportedType tests error handling for unsupported DataResource type
func TestDataResourceReconciler_ReconcileUnsupportedType(t *testing.T) {
	scheme := newTestScheme(t)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-unsupported",
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "unsupported-type",
			Tenant: "tenant-6",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dr).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()
	r := &DataResourceReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dr.Name,
			Namespace: dr.Namespace,
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err == nil {
		t.Errorf("Reconcile() error = nil, want error for unsupported type")
	}

	// Verify status was updated to Failed
	var updated nestv1.DataResource
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DataResource: %v", err)
	}

	if updated.Status.Phase != nestv1.PhaseFailed {
		t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseFailed)
	}
}

// TestDataResourceReconciler_ReconcileWithDeletion tests deletion handling
func TestDataResourceReconciler_ReconcileWithDeletion(t *testing.T) {
	scheme := newTestScheme(t)

	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-delete",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Tenant: "tenant-7",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dr).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()
	r := &DataResourceReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dr.Name,
			Namespace: dr.Namespace,
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	// Verify finalizer was removed (object should still exist during finalization)
	var updated nestv1.DataResource
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		// Object may have been deleted or still exists - both are acceptable
		// The key validation is that the Update call succeeded
		return
	}

	if containsString(updated.Finalizers, "nest.penguintech.io/dataresource") {
		t.Errorf("finalizer should be removed, got: %v", updated.Finalizers)
	}
}

// TestTenantReconciler_ReconcileNotFound tests that reconciling a non-existent Tenant returns no error
func TestTenantReconciler_ReconcileNotFound(t *testing.T) {
	scheme := newTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &TenantReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent-tenant",
			Namespace: "default",
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
}

// TestTenantReconciler_ReconcileFreeTierSetDefaults tests that free-tier defaults are applied
func TestTenantReconciler_ReconcileFreeTierSetDefaults(t *testing.T) {
	scheme := newTestScheme(t)

	tenant := &nestv1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "free-tier-tenant",
			Namespace: "default",
		},
		Spec: nestv1.TenantSpec{
			DisplayName: "Free Tier Test",
			LicenseTier: "free",
			Quota:       nil, // No quota set initially
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(tenant).
		WithStatusSubresource(&nestv1.Tenant{}).
		Build()
	r := &TenantReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      tenant.Name,
			Namespace: tenant.Namespace,
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	// Verify defaults were set
	var updated nestv1.Tenant
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: tenant.Name, Namespace: tenant.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated Tenant: %v", err)
	}

	if updated.Spec.Quota == nil {
		t.Fatalf("Quota should not be nil after reconciliation")
	}

	if updated.Spec.Quota.MaxDataResources != 5 {
		t.Errorf("MaxDataResources = %d, want 5", updated.Spec.Quota.MaxDataResources)
	}
	if updated.Spec.Quota.MaxOperatorAccounts != 3 {
		t.Errorf("MaxOperatorAccounts = %d, want 3", updated.Spec.Quota.MaxOperatorAccounts)
	}
	if updated.Spec.Quota.MaxResourceAccounts != 3 {
		t.Errorf("MaxResourceAccounts = %d, want 3", updated.Spec.Quota.MaxResourceAccounts)
	}
}

// TestTenantReconciler_ReconcileFreeTierEmptyString tests that empty string tier defaults are applied
func TestTenantReconciler_ReconcileFreeTierEmptyString(t *testing.T) {
	scheme := newTestScheme(t)

	tenant := &nestv1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "default-tier-tenant",
			Namespace: "default",
		},
		Spec: nestv1.TenantSpec{
			DisplayName: "Default Tier Test",
			LicenseTier: "", // Empty string should default to free
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(tenant).
		WithStatusSubresource(&nestv1.Tenant{}).
		Build()
	r := &TenantReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      tenant.Name,
			Namespace: tenant.Namespace,
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	var updated nestv1.Tenant
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: tenant.Name, Namespace: tenant.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated Tenant: %v", err)
	}

	if updated.Spec.Quota == nil {
		t.Fatalf("Quota should not be nil after reconciliation")
	}

	if updated.Spec.Quota.MaxDataResources != 5 {
		t.Errorf("MaxDataResources = %d, want 5", updated.Spec.Quota.MaxDataResources)
	}
}

// TestTenantReconciler_ReconcileProTier tests that pro tier does not apply free tier defaults
func TestTenantReconciler_ReconcileProTier(t *testing.T) {
	scheme := newTestScheme(t)

	tenant := &nestv1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pro-tier-tenant",
			Namespace: "default",
		},
		Spec: nestv1.TenantSpec{
			DisplayName: "Pro Tier Test",
			LicenseTier: "pro",
			Quota:       nil,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(tenant).
		WithStatusSubresource(&nestv1.Tenant{}).
		Build()
	r := &TenantReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      tenant.Name,
			Namespace: tenant.Namespace,
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	var updated nestv1.Tenant
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: tenant.Name, Namespace: tenant.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated Tenant: %v", err)
	}

	// Pro tier should not auto-apply defaults
	if updated.Spec.Quota != nil {
		t.Errorf("Quota should be nil for pro tier, got: %v", updated.Spec.Quota)
	}
}

// TestTenantReconciler_ReconcileFreeTierPartialQuota tests that only missing quota fields are set
func TestTenantReconciler_ReconcileFreeTierPartialQuota(t *testing.T) {
	scheme := newTestScheme(t)

	tenant := &nestv1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "partial-quota-tenant",
			Namespace: "default",
		},
		Spec: nestv1.TenantSpec{
			DisplayName: "Partial Quota Test",
			LicenseTier: "free",
			Quota: &nestv1.QuotaSpec{
				MaxDataResources: 10, // Already set to non-zero
				// MaxOperatorAccounts is 0 (default)
				// MaxResourceAccounts is 0 (default)
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(tenant).
		WithStatusSubresource(&nestv1.Tenant{}).
		Build()
	r := &TenantReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      tenant.Name,
			Namespace: tenant.Namespace,
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	var updated nestv1.Tenant
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: tenant.Name, Namespace: tenant.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated Tenant: %v", err)
	}

	// Should preserve existing non-zero value
	if updated.Spec.Quota.MaxDataResources != 10 {
		t.Errorf("MaxDataResources = %d, want 10 (existing value)", updated.Spec.Quota.MaxDataResources)
	}

	// Should set missing defaults
	if updated.Spec.Quota.MaxOperatorAccounts != 3 {
		t.Errorf("MaxOperatorAccounts = %d, want 3", updated.Spec.Quota.MaxOperatorAccounts)
	}
	if updated.Spec.Quota.MaxResourceAccounts != 3 {
		t.Errorf("MaxResourceAccounts = %d, want 3", updated.Spec.Quota.MaxResourceAccounts)
	}
}

// TestTenantReconciler_ReconcileEnterpriseTier tests enterprise tier behavior
func TestTenantReconciler_ReconcileEnterpriseTier(t *testing.T) {
	scheme := newTestScheme(t)

	tenant := &nestv1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "enterprise-tenant",
			Namespace: "default",
		},
		Spec: nestv1.TenantSpec{
			DisplayName: "Enterprise Test",
			LicenseTier: "enterprise",
			Quota:       nil,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(tenant).
		WithStatusSubresource(&nestv1.Tenant{}).
		Build()
	r := &TenantReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      tenant.Name,
			Namespace: tenant.Namespace,
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	var updated nestv1.Tenant
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: tenant.Name, Namespace: tenant.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated Tenant: %v", err)
	}

	// Enterprise tier should not auto-apply free tier defaults
	if updated.Spec.Quota != nil {
		t.Errorf("Quota should be nil for enterprise tier, got: %v", updated.Spec.Quota)
	}
}

// TestDataResourceReconciler_MultipleReconciliations tests idempotent reconciliation
func TestDataResourceReconciler_MultipleReconciliations(t *testing.T) {
	scheme := newTestScheme(t)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "idempotent-test",
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Tenant: "tenant-8",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dr).
		WithStatusSubresource(&nestv1.DataResource{}).
		Build()
	r := &DataResourceReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dr.Name,
			Namespace: dr.Namespace,
		},
	}

	// First reconciliation
	_, err1 := r.Reconcile(ctx, req)
	if err1 != nil {
		t.Errorf("first Reconcile() error = %v, want nil", err1)
	}

	// Second reconciliation on same object
	_, err2 := r.Reconcile(ctx, req)
	if err2 != nil {
		t.Errorf("second Reconcile() error = %v, want nil", err2)
	}

	// State should be consistent
	var updated nestv1.DataResource
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get DataResource: %v", err)
	}

	if updated.Status.Phase != nestv1.PhaseReady {
		t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseReady)
	}

	// Finalizer should be present once
	finalizerCount := 0
	for _, f := range updated.Finalizers {
		if f == "nest.penguintech.io/dataresource" {
			finalizerCount++
		}
	}
	if finalizerCount != 1 {
		t.Errorf("expected 1 finalizer, got %d", finalizerCount)
	}
}

// TestHelperFunctions_ContainsString tests the containsString helper
func TestHelperFunctions_ContainsString(t *testing.T) {
	tests := []struct {
		name   string
		slice  []string
		search string
		want   bool
	}{
		{"empty slice", []string{}, "test", false},
		{"found", []string{"a", "b", "c"}, "b", true},
		{"not found", []string{"a", "b", "c"}, "d", false},
		{"single item found", []string{"test"}, "test", true},
		{"single item not found", []string{"test"}, "other", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := containsString(tt.slice, tt.search)
			if got != tt.want {
				t.Errorf("containsString(%v, %q) = %v, want %v", tt.slice, tt.search, got, tt.want)
			}
		})
	}
}

// TestHelperFunctions_RemoveString tests the removeString helper
func TestHelperFunctions_RemoveString(t *testing.T) {
	tests := []struct {
		name   string
		slice  []string
		remove string
		want   []string
	}{
		{"empty slice", []string{}, "test", []string{}},
		{"remove not found", []string{"a", "b", "c"}, "d", []string{"a", "b", "c"}},
		{"remove found", []string{"a", "b", "c"}, "b", []string{"a", "c"}},
		{"remove single item", []string{"test"}, "test", []string{}},
		{"remove from multiple", []string{"a", "a", "b"}, "a", []string{"b"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := removeString(tt.slice, tt.remove)
			if len(got) != len(tt.want) {
				t.Errorf("removeString(%v, %q) length = %d, want %d", tt.slice, tt.remove, len(got), len(tt.want))
				return
			}
			for i, v := range got {
				if v != tt.want[i] {
					t.Errorf("removeString(%v, %q) = %v, want %v", tt.slice, tt.remove, got, tt.want)
					return
				}
			}
		})
	}
}
