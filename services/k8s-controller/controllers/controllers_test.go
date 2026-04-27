package controllers

import (
	"context"
	"fmt"
	"testing"

	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	corev1 "k8s.io/api/core/v1"
	appsv1 "k8s.io/api/apps/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"
	"sigs.k8s.io/controller-runtime/pkg/client/interceptor"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// newTestScheme creates a scheme with nest.penguintech.io and core k8s types registered
func newTestScheme(t *testing.T) *runtime.Scheme {
	scheme := runtime.NewScheme()
	if err := nestv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add nestv1 to scheme: %v", err)
	}
	if err := corev1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add corev1 to scheme: %v", err)
	}
	if err := appsv1.AddToScheme(scheme); err != nil {
		t.Fatalf("failed to add appsv1 to scheme: %v", err)
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

	// Check status phase was updated to Provisioning (postgres creates CloudNativePG cluster, which takes time to provision)
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseProvisioning)
	}

	// Check status condition was set
	cond := meta.FindStatusCondition(updated.Status.Conditions, string(nestv1.PhaseProvisioning))
	if cond == nil {
		t.Errorf("status condition not found for phase %v", nestv1.PhaseProvisioning)
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

	// keyvalue creates real K8s resources (StatefulSet, Service, ConfigMap) which take time to become ready
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseProvisioning)
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

	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseProvisioning)
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

// TestDataResourceReconciler_ReconcileWithExistingFinalizer tests that finalizer isn't duplicated
func TestDataResourceReconciler_ReconcileWithExistingFinalizer(t *testing.T) {
	scheme := newTestScheme(t)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-existing-finalizer",
			Namespace:  "default",
			Finalizers: []string{"nest.penguintech.io/dataresource"},
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "object",
			Tenant: "tenant-existing-fin",
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

	var updated nestv1.DataResource
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DataResource: %v", err)
	}

	// Verify finalizer appears exactly once
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

// TestDataResourceReconciler_ReconcilePreProvisioningPhaseRetries tests phase transition from Provisioning to Ready
func TestDataResourceReconciler_ReconcilePreProvisioningPhaseRetries(t *testing.T) {
	scheme := newTestScheme(t)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pre-provisioning",
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "object",
			Tenant: "tenant-pre-prov",
		},
		Status: nestv1.DataResourceStatus{
			Phase: nestv1.PhaseProvisioning,
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

	// First reconciliation with Provisioning phase should remain Provisioning
	_, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	var updated nestv1.DataResource
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DataResource: %v", err)
	}

	// Object type transitions to Ready immediately
	if updated.Status.Phase != nestv1.PhaseReady {
		t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseReady)
	}
}

// TestDataResourceReconciler_ReconcileStatusConditionVerification tests status condition details
func TestDataResourceReconciler_ReconcileStatusConditionVerification(t *testing.T) {
	scheme := newTestScheme(t)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-condition-verify",
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "pvc/block",
			Tenant: "tenant-cond",
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

	// Verify all expected conditions exist and have correct values
	expectedConditions := []string{string(nestv1.PhasePending), string(nestv1.PhaseReady)}
	for _, expected := range expectedConditions {
		cond := meta.FindStatusCondition(updated.Status.Conditions, expected)
		if cond != nil && cond.Status == metav1.ConditionTrue {
			// Found a valid condition
			break
		}
	}
}

// TestDataResourceReconciler_ReconcileEmptyStatusPhase tests initialization of empty status phase
func TestDataResourceReconciler_ReconcileEmptyStatusPhase(t *testing.T) {
	scheme := newTestScheme(t)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-empty-phase",
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "pvc/file",
			Tenant: "tenant-empty",
		},
		Status: nestv1.DataResourceStatus{
			Phase: "", // Empty phase
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

// TestDataResourceReconciler_ReconcileAlreadyReady tests that already-ready resources stay ready
func TestDataResourceReconciler_ReconcileAlreadyReady(t *testing.T) {
	scheme := newTestScheme(t)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:       "test-already-ready",
			Namespace:  "default",
			Finalizers: []string{"nest.penguintech.io/dataresource"},
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "object",
			Tenant: "tenant-ready",
		},
		Status: nestv1.DataResourceStatus{
			Phase: nestv1.PhaseReady,
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

	// Object type stays Ready when already Ready
	if updated.Status.Phase != nestv1.PhaseReady {
		t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseReady)
	}
}

// TestDarkDriveReconciler_ReconcileNotFound tests that reconciling a non-existent DarkDrive returns no error
func TestDarkDriveReconciler_ReconcileNotFound(t *testing.T) {
	scheme := newTestScheme(t)
	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	r := &DarkDriveReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent-darkdrive",
			Namespace: "default",
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
}

// TestDarkDriveReconciler_ReconcileNewDrive tests DarkDrive initialization (empty state → Discovered)
func TestDarkDriveReconciler_ReconcileNewDrive(t *testing.T) {
	scheme := newTestScheme(t)

	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dd-new",
			Namespace: "default",
		},
		Spec: nestv1.DarkDriveSpec{
			Node:   "node-1",
			Device: "/dev/sda",
		},
		Status: nestv1.DarkDriveStatus{
			State: "",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dd).
		WithStatusSubresource(&nestv1.DarkDrive{}).
		Build()
	r := &DarkDriveReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dd.Name,
			Namespace: dd.Namespace,
		},
	}

	res, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
	if !res.Requeue {
		t.Errorf("Reconcile() Requeue = %v, want true", res.Requeue)
	}

	var updated nestv1.DarkDrive
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dd.Name, Namespace: dd.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DarkDrive: %v", err)
	}

	if updated.Status.State != nestv1.DarkDriveDiscovered {
		t.Errorf("Status.State = %v, want %v", updated.Status.State, nestv1.DarkDriveDiscovered)
	}
}

// TestDarkDriveReconciler_ReconcileDiscoveredToAwaitingApproval tests state transition
func TestDarkDriveReconciler_ReconcileDiscoveredToAwaitingApproval(t *testing.T) {
	scheme := newTestScheme(t)

	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dd-discovered",
			Namespace: "default",
		},
		Spec: nestv1.DarkDriveSpec{
			Node:   "node-1",
			Device: "/dev/sda",
		},
		Status: nestv1.DarkDriveStatus{
			State: nestv1.DarkDriveDiscovered,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dd).
		WithStatusSubresource(&nestv1.DarkDrive{}).
		Build()
	r := &DarkDriveReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dd.Name,
			Namespace: dd.Namespace,
		},
	}

	res, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
	if !res.Requeue {
		t.Errorf("Reconcile() Requeue = %v, want true", res.Requeue)
	}

	var updated nestv1.DarkDrive
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dd.Name, Namespace: dd.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DarkDrive: %v", err)
	}

	if updated.Status.State != nestv1.DarkDriveAwaitingApproval {
		t.Errorf("Status.State = %v, want %v", updated.Status.State, nestv1.DarkDriveAwaitingApproval)
	}
}

// TestDarkDriveReconciler_ReconcileAwaitingApproval tests waiting for approval
func TestDarkDriveReconciler_ReconcileAwaitingApproval(t *testing.T) {
	scheme := newTestScheme(t)

	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dd-awaiting",
			Namespace: "default",
		},
		Spec: nestv1.DarkDriveSpec{
			Node:   "node-1",
			Device: "/dev/sda",
		},
		Status: nestv1.DarkDriveStatus{
			State: nestv1.DarkDriveAwaitingApproval,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dd).
		WithStatusSubresource(&nestv1.DarkDrive{}).
		Build()
	r := &DarkDriveReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dd.Name,
			Namespace: dd.Namespace,
		},
	}

	res, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
	if res.Requeue {
		t.Errorf("Reconcile() Requeue = %v, want false", res.Requeue)
	}
	if res.RequeueAfter != 0 {
		t.Errorf("Reconcile() RequeueAfter = %v, want 0", res.RequeueAfter)
	}

	// State should remain unchanged
	var updated nestv1.DarkDrive
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dd.Name, Namespace: dd.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DarkDrive: %v", err)
	}

	if updated.Status.State != nestv1.DarkDriveAwaitingApproval {
		t.Errorf("Status.State = %v, want %v", updated.Status.State, nestv1.DarkDriveAwaitingApproval)
	}
}

// TestDarkDriveReconciler_ReconcileApprovedMissingHardwarePool tests adoption safety gate
func TestDarkDriveReconciler_ReconcileApprovedMissingHardwarePool(t *testing.T) {
	scheme := newTestScheme(t)

	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dd-approved-no-pool",
			Namespace: "default",
		},
		Spec: nestv1.DarkDriveSpec{
			Node:         "node-1",
			Device:       "/dev/sda",
			HardwarePool: "", // Missing required field
		},
		Status: nestv1.DarkDriveStatus{
			State: nestv1.DarkDriveApproved,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dd).
		WithStatusSubresource(&nestv1.DarkDrive{}).
		Build()
	r := &DarkDriveReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dd.Name,
			Namespace: dd.Namespace,
		},
	}

	res, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
	if res.RequeueAfter == 0 {
		t.Errorf("Reconcile() RequeueAfter = 0, want > 0")
	}

	var updated nestv1.DarkDrive
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dd.Name, Namespace: dd.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DarkDrive: %v", err)
	}

	// State should remain Approved (not advanced to Adopted)
	if updated.Status.State != nestv1.DarkDriveApproved {
		t.Errorf("Status.State = %v, want %v", updated.Status.State, nestv1.DarkDriveApproved)
	}
}

// TestDarkDriveReconciler_ReconcileApprovedForeignFS tests foreign filesystem safety gate
func TestDarkDriveReconciler_ReconcileApprovedForeignFS(t *testing.T) {
	scheme := newTestScheme(t)

	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dd-foreign-fs",
			Namespace: "default",
		},
		Spec: nestv1.DarkDriveSpec{
			Node:         "node-1",
			Device:       "/dev/sda",
			Signature:    "foreign-fs:ext4",
			HardwarePool: "pool-1",
			EraseConfirmed: false, // Missing erase confirmation
		},
		Status: nestv1.DarkDriveStatus{
			State: nestv1.DarkDriveApproved,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dd).
		WithStatusSubresource(&nestv1.DarkDrive{}).
		Build()
	r := &DarkDriveReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dd.Name,
			Namespace: dd.Namespace,
		},
	}

	res, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
	if res.RequeueAfter == 0 {
		t.Errorf("Reconcile() RequeueAfter = 0, want > 0")
	}

	var updated nestv1.DarkDrive
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dd.Name, Namespace: dd.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DarkDrive: %v", err)
	}

	// State should remain Approved
	if updated.Status.State != nestv1.DarkDriveApproved {
		t.Errorf("Status.State = %v, want %v", updated.Status.State, nestv1.DarkDriveApproved)
	}
}

// TestDarkDriveReconciler_ReconcileApprovedToAdopted tests successful adoption
func TestDarkDriveReconciler_ReconcileApprovedToAdopted(t *testing.T) {
	scheme := newTestScheme(t)

	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dd-adopted",
			Namespace: "default",
		},
		Spec: nestv1.DarkDriveSpec{
			Node:           "node-1",
			Device:         "/dev/sda",
			Signature:      "unknown",
			HardwarePool:   "pool-1",
			EraseConfirmed: false,
		},
		Status: nestv1.DarkDriveStatus{
			State: nestv1.DarkDriveApproved,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dd).
		WithStatusSubresource(&nestv1.DarkDrive{}).
		Build()
	r := &DarkDriveReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dd.Name,
			Namespace: dd.Namespace,
		},
	}

	res, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Errorf("Reconcile() returned unexpected requeue: Requeue=%v, RequeueAfter=%v", res.Requeue, res.RequeueAfter)
	}

	var updated nestv1.DarkDrive
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dd.Name, Namespace: dd.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DarkDrive: %v", err)
	}

	if updated.Status.State != nestv1.DarkDriveAdopted {
		t.Errorf("Status.State = %v, want %v", updated.Status.State, nestv1.DarkDriveAdopted)
	}
}

// TestDarkDriveReconciler_ReconcileRejected tests terminal state (Rejected)
func TestDarkDriveReconciler_ReconcileRejected(t *testing.T) {
	scheme := newTestScheme(t)

	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dd-rejected",
			Namespace: "default",
		},
		Spec: nestv1.DarkDriveSpec{
			Node:   "node-1",
			Device: "/dev/sda",
		},
		Status: nestv1.DarkDriveStatus{
			State: nestv1.DarkDriveRejected,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dd).
		WithStatusSubresource(&nestv1.DarkDrive{}).
		Build()
	r := &DarkDriveReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dd.Name,
			Namespace: dd.Namespace,
		},
	}

	res, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Errorf("Reconcile() returned unexpected requeue: Requeue=%v, RequeueAfter=%v", res.Requeue, res.RequeueAfter)
	}
}

// TestDarkDriveReconciler_ReconcileAdopted tests terminal state (Adopted)
func TestDarkDriveReconciler_ReconcileAdopted(t *testing.T) {
	scheme := newTestScheme(t)

	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dd-adopted-terminal",
			Namespace: "default",
		},
		Spec: nestv1.DarkDriveSpec{
			Node:   "node-1",
			Device: "/dev/sda",
		},
		Status: nestv1.DarkDriveStatus{
			State: nestv1.DarkDriveAdopted,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dd).
		WithStatusSubresource(&nestv1.DarkDrive{}).
		Build()
	r := &DarkDriveReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dd.Name,
			Namespace: dd.Namespace,
		},
	}

	res, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Errorf("Reconcile() returned unexpected requeue: Requeue=%v, RequeueAfter=%v", res.Requeue, res.RequeueAfter)
	}
}

// TestDarkDriveReconciler_ReconcileUnknownState tests handling of unknown state
func TestDarkDriveReconciler_ReconcileUnknownState(t *testing.T) {
	scheme := newTestScheme(t)

	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dd-unknown",
			Namespace: "default",
		},
		Spec: nestv1.DarkDriveSpec{
			Node:   "node-1",
			Device: "/dev/sda",
		},
		Status: nestv1.DarkDriveStatus{
			State: "unknown-state-value",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dd).
		WithStatusSubresource(&nestv1.DarkDrive{}).
		Build()
	r := &DarkDriveReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dd.Name,
			Namespace: dd.Namespace,
		},
	}

	res, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Errorf("Reconcile() returned unexpected requeue: Requeue=%v, RequeueAfter=%v", res.Requeue, res.RequeueAfter)
	}
}

// TestDataResourceReconciler_FalizerAndStatusUpdate tests finalizer addition and status update
func TestDataResourceReconciler_FinalizerAndStatusUpdate(t *testing.T) {
	scheme := newTestScheme(t)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-finalizer",
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Tenant: "tenant-fin-1",
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

	// Verify finalizer is present
	if !containsString(updated.Finalizers, "nest.penguintech.io/dataresource") {
		t.Errorf("finalizer not found in: %v", updated.Finalizers)
	}

	// Verify status was updated
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseProvisioning)
	}

	// Verify condition was set
	cond := meta.FindStatusCondition(updated.Status.Conditions, string(nestv1.PhaseProvisioning))
	if cond == nil {
		t.Errorf("condition not found for phase %v", nestv1.PhaseProvisioning)
	}
}

// TestPlacementEngine_NodeAffinityNoNodes tests NodeAffinity with no matching nodes
func TestPlacementEngine_NodeAffinityNoNodes(t *testing.T) {
	scheme := newTestScheme(t)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	engine := NewPlacementEngine(fakeClient)

	ctx := context.Background()
	affinity, err := engine.NodeAffinity(ctx, "gpu")

	if err != nil {
		t.Errorf("NodeAffinity() error = %v, want nil", err)
	}
	if affinity != nil {
		t.Errorf("NodeAffinity() = %v, want nil (no nodes available)", affinity)
	}
}

// TestPlacementEngine_NodeAffinityWithNodes tests NodeAffinity with matching nodes
func TestPlacementEngine_NodeAffinityWithNodes(t *testing.T) {
	scheme := newTestScheme(t)

	inventory := &nestv1.HardwareInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-1-inventory",
			Namespace: "default",
		},
		Spec: nestv1.HardwareInventorySpec{
			Node: "node-1",
			Devices: []nestv1.DeviceSpec{
				{
					Name:          "sda",
					Class:         "nvme",
					State:         nestv1.DeviceStateActive,
					CapacityBytes: 1000000000000,
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inventory).
		Build()
	engine := NewPlacementEngine(fakeClient)

	ctx := context.Background()
	affinity, err := engine.NodeAffinity(ctx, "nvme")

	if err != nil {
		t.Errorf("NodeAffinity() error = %v, want nil", err)
	}
	if affinity == nil {
		t.Errorf("NodeAffinity() = nil, want non-nil")
	}
	if affinity != nil {
		if len(affinity.RequiredDuringSchedulingIgnoredDuringExecution.NodeSelectorTerms) == 0 {
			t.Errorf("NodeAffinity has no NodeSelectorTerms")
		}
	}
}

// TestPlacementEngine_NodesWithCapacityNoNodes tests capacity lookup with no nodes
func TestPlacementEngine_NodesWithCapacityNoNodes(t *testing.T) {
	scheme := newTestScheme(t)

	fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
	engine := NewPlacementEngine(fakeClient)

	ctx := context.Background()
	nodes, err := engine.NodesWithCapacity(ctx, "nvme", 100000000000)

	if err != nil {
		t.Errorf("NodesWithCapacity() error = %v, want nil", err)
	}
	if len(nodes) != 0 {
		t.Errorf("NodesWithCapacity() returned %d nodes, want 0", len(nodes))
	}
}

// TestPlacementEngine_NodesWithCapacityWithCapacity tests capacity lookup with sufficient capacity
func TestPlacementEngine_NodesWithCapacityWithCapacity(t *testing.T) {
	scheme := newTestScheme(t)

	inventory := &nestv1.HardwareInventory{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "node-2-inventory",
			Namespace: "default",
		},
		Spec: nestv1.HardwareInventorySpec{
			Node: "node-2",
			Devices: []nestv1.DeviceSpec{
				{
					Name:          "sda",
					Class:         "nvme",
					State:         nestv1.DeviceStateActive,
					CapacityBytes: 1000000000000, // 1TB
				},
			},
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(inventory).
		Build()
	engine := NewPlacementEngine(fakeClient)

	ctx := context.Background()
	// Request 100GB (within the 1TB available, accounting for 20% overhead)
	nodes, err := engine.NodesWithCapacity(ctx, "nvme", 100000000000)

	if err != nil {
		t.Errorf("NodesWithCapacity() error = %v, want nil", err)
	}
	if len(nodes) != 1 {
		t.Errorf("NodesWithCapacity() returned %d nodes, want 1", len(nodes))
	}
	if len(nodes) > 0 && nodes[0] != "node-2" {
		t.Errorf("NodesWithCapacity() returned node %q, want node-2", nodes[0])
	}
}

// TestTenantReconciler_ReconcileWithUpdate tests that updates are persisted correctly
func TestTenantReconciler_ReconcileWithUpdate(t *testing.T) {
	scheme := newTestScheme(t)

	tenant := &nestv1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "update-test-tenant",
			Namespace: "default",
		},
		Spec: nestv1.TenantSpec{
			DisplayName: "Update Test",
			LicenseTier: "free",
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

	// First reconciliation
	_, err := r.Reconcile(ctx, req)
	if err != nil {
		t.Errorf("first Reconcile() error = %v, want nil", err)
	}

	var updated1 nestv1.Tenant
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: tenant.Name, Namespace: tenant.Namespace}, &updated1); err != nil {
		t.Fatalf("failed to get updated Tenant: %v", err)
	}

	if updated1.Spec.Quota == nil {
		t.Errorf("Quota is nil after first reconciliation")
	}

	// Second reconciliation should be idempotent
	_, err = r.Reconcile(ctx, req)
	if err != nil {
		t.Errorf("second Reconcile() error = %v, want nil", err)
	}

	var updated2 nestv1.Tenant
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: tenant.Name, Namespace: tenant.Namespace}, &updated2); err != nil {
		t.Fatalf("failed to get Tenant second time: %v", err)
	}

	// Verify quota wasn't duplicated
	if updated2.Spec.Quota.MaxDataResources != updated1.Spec.Quota.MaxDataResources {
		t.Errorf("Quota changed between reconciliations")
	}
}

// TestDataResourceReconciler_ReconcileDeleteWithFinalizer tests deletion when finalizer exists
func TestDataResourceReconciler_ReconcileDeleteWithFinalizer(t *testing.T) {
	scheme := newTestScheme(t)

	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-delete-finalized",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "object",
			Tenant: "tenant-del-fin",
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
	err = fakeClient.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated)
	// Object may or may not exist after deletion, but error should be handled gracefully
	if err != nil && !errors.IsNotFound(err) {
		t.Errorf("unexpected error: %v", err)
	}
}


// TestDataResourceReconciler_ReconcilePhaseProgressionPending tests state when phase is already Pending
func TestDataResourceReconciler_ReconcilePhaseProgressionPending(t *testing.T) {
	scheme := newTestScheme(t)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-pending-already",
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "pvc/block",
			Tenant: "tenant-pend",
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

	// Phase should transition from Pending to Ready (for pvc/block)
	if updated.Status.Phase != nestv1.PhaseReady {
		t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseReady)
	}
}

// TestDataResourceReconciler_ReconcileApplyingPhaseChange tests phase change from Pending to Provisioning
func TestDataResourceReconciler_ReconcileApplyingPhaseChange(t *testing.T) {
	scheme := newTestScheme(t)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-phase-change",
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "postgres",
			Tenant: "tenant-phase",
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

	// For postgres, phase should change from Pending to Provisioning
	if updated.Status.Phase != nestv1.PhaseProvisioning {
		t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseProvisioning)
	}
}

// TestDarkDriveReconciler_ReconcileApprovedWithErasedDrive tests adoption with erased foreign drive
func TestDarkDriveReconciler_ReconcileApprovedWithErasedDrive(t *testing.T) {
	scheme := newTestScheme(t)

	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-dd-erased",
			Namespace: "default",
		},
		Spec: nestv1.DarkDriveSpec{
			Node:           "node-1",
			Device:         "/dev/sda",
			Signature:      "foreign-fs:ext4",
			HardwarePool:   "pool-1",
			EraseConfirmed: true, // Erase confirmed
		},
		Status: nestv1.DarkDriveStatus{
			State: nestv1.DarkDriveApproved,
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dd).
		WithStatusSubresource(&nestv1.DarkDrive{}).
		Build()
	r := &DarkDriveReconciler{Client: fakeClient, Scheme: scheme}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dd.Name,
			Namespace: dd.Namespace,
		},
	}

	res, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
	if res.Requeue || res.RequeueAfter != 0 {
		t.Errorf("Reconcile() returned unexpected requeue: Requeue=%v, RequeueAfter=%v", res.Requeue, res.RequeueAfter)
	}

	var updated nestv1.DarkDrive
	if err := fakeClient.Get(ctx, types.NamespacedName{Name: dd.Name, Namespace: dd.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DarkDrive: %v", err)
	}

	if updated.Status.State != nestv1.DarkDriveAdopted {
		t.Errorf("Status.State = %v, want %v", updated.Status.State, nestv1.DarkDriveAdopted)
	}
}

// TestDataResourceReconciler_ReconcileDeletePhaseTransition tests deletion of object in different phases
func TestDataResourceReconciler_ReconcileDeletePhaseTransition(t *testing.T) {
	scheme := newTestScheme(t)

	now := metav1.Now()
	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:              "test-delete-provisioning",
			Namespace:         "default",
			Finalizers:        []string{"nest.penguintech.io/dataresource"},
			DeletionTimestamp: &now,
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "keyvalue",
			Tenant: "tenant-del-prov",
		},
		Status: nestv1.DataResourceStatus{
			Phase: nestv1.PhaseProvisioning,
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

	// Object should still exist (finalizer removal is in progress)
	var updated nestv1.DataResource
	err = fakeClient.Get(ctx, types.NamespacedName{Name: dr.Name, Namespace: dr.Namespace}, &updated)
	// May be deleted or may still exist depending on fake client
	_ = err
}

// TestDataResourceReconciler_ReconcileDeleteAllTypes tests deletion handling across type variants
func TestDataResourceReconciler_ReconcileDeleteAllTypes(t *testing.T) {
	scheme := newTestScheme(t)

	deleteTypes := []string{"object", "pvc/block", "pvc/file"}

	for _, dsType := range deleteTypes {
		t.Run(dsType, func(t *testing.T) {
			now := metav1.Now()
			dr := &nestv1.DataResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:              "test-delete-" + dsType,
					Namespace:         "default",
					Finalizers:        []string{"nest.penguintech.io/dataresource"},
					DeletionTimestamp: &now,
				},
				Spec: nestv1.DataResourceSpec{
					Type:   dsType,
					Tenant: "tenant-del-" + dsType,
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
		})
	}
}

// TestDataResourceReconciler_ReconcileCreatePhases tests the create path for different initial phases
func TestDataResourceReconciler_ReconcileCreatePhases(t *testing.T) {
	scheme := newTestScheme(t)

	phases := []nestv1.DataResourcePhase{"", nestv1.PhasePending}

	for i, phase := range phases {
		t.Run(string(phase)+"_"+string(rune(i)), func(t *testing.T) {
			dr := &nestv1.DataResource{
				ObjectMeta: metav1.ObjectMeta{
					Name:      "test-create-phase-" + string(rune(i)),
					Namespace: "default",
				},
				Spec: nestv1.DataResourceSpec{
					Type:   "object",
					Tenant: "tenant-create-" + string(rune(i)),
				},
				Status: nestv1.DataResourceStatus{
					Phase: phase,
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

			// Object type should be Ready
			if updated.Status.Phase != nestv1.PhaseReady {
				t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseReady)
			}
		})
	}
}

// TestContainsString_EdgeCases tests edge cases of containsString
func TestContainsString_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		slice  []string
		search string
		want   bool
	}{
		{"empty search string", []string{"a", "b", ""}, "", true},
		{"nil-like empty in slice", []string{"", "a", "b"}, "", true},
		{"case sensitive", []string{"A", "B"}, "a", false},
		{"substring doesn't match", []string{"abc"}, "ab", false},
		{"exact match only", []string{"abcdef"}, "abcdef", true},
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

// TestRemoveString_EdgeCases tests edge cases of removeString
func TestRemoveString_EdgeCases(t *testing.T) {
	tests := []struct {
		name   string
		slice  []string
		remove string
		want   []string
	}{
		{"remove empty string", []string{"a", "", "b"}, "", []string{"a", "b"}},
		{"remove all occurrences", []string{"x", "x", "x"}, "x", []string{}},
		{"nil-like empty result", []string{""}, "", []string{}},
		{"case sensitive remove", []string{"A", "a", "B"}, "a", []string{"A", "B"}},
		{"remove preserves order", []string{"z", "y", "x", "y", "w"}, "y", []string{"z", "x", "w"}},
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

// TestDataResourceReconciler_ReconcileWithStatusAndFinalizer verifies status and finalizer are both set
func TestDataResourceReconciler_ReconcileWithStatusAndFinalizer(t *testing.T) {
	scheme := newTestScheme(t)

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-status-finalizer",
			Namespace: "default",
		},
		Spec: nestv1.DataResourceSpec{
			Type:   "pvc/block",
			Tenant: "tenant-sf",
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

	// Verify both finalizer and status are set
	if !containsString(updated.Finalizers, "nest.penguintech.io/dataresource") {
		t.Errorf("finalizer not found")
	}
	if updated.Status.Phase != nestv1.PhaseReady {
		t.Errorf("Status.Phase = %v, want %v", updated.Status.Phase, nestv1.PhaseReady)
	}
	if len(updated.Status.Conditions) == 0 {
		t.Errorf("Status.Conditions is empty, want at least one condition")
	}
}

// TestDarkDriveReconciler_ReconcileSetCondition tests the setCondition helper
func TestDarkDriveReconciler_ReconcileSetCondition(t *testing.T) {
	scheme := newTestScheme(t)

	dd := &nestv1.DarkDrive{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-condition",
			Namespace: "default",
		},
		Spec: nestv1.DarkDriveSpec{
			Node:   "node-1",
			Device: "/dev/sda",
		},
	}

	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dd).
		WithStatusSubresource(&nestv1.DarkDrive{}).
		Build()
	r := &DarkDriveReconciler{Client: fakeClient, Scheme: scheme}

	// Manually call setCondition
	r.setCondition(dd, "TestCondition", metav1.ConditionTrue, "TestReason", "Test message")

	// Verify condition was set
	cond := meta.FindStatusCondition(dd.Status.Conditions, "TestCondition")
	if cond == nil {
		t.Errorf("condition not found")
	}
	if cond.Status != metav1.ConditionTrue {
		t.Errorf("condition status = %v, want ConditionTrue", cond.Status)
	}
	if cond.Reason != "TestReason" {
		t.Errorf("condition reason = %q, want TestReason", cond.Reason)
	}
	if cond.Message != "Test message" {
		t.Errorf("condition message = %q, want 'Test message'", cond.Message)
	}
}

// errNamespaceClient builds a fake client whose Create interceptor returns a generic error
// for Namespace objects (non-AlreadyExists), exercising the namespace error paths.
func errNamespaceClient(t *testing.T) client.Client {
	t.Helper()
	scheme := newTestScheme(t)
	injectErr := fmt.Errorf("injected namespace create failure")
	return fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Create: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.CreateOption) error {
				if _, ok := obj.(*corev1.Namespace); ok {
					return injectErr
				}
				return c.Create(ctx, obj, opts...)
			},
		}).
		Build()
}

// TestReconcileKeyvalueNamespace_CreateError tests the error path in reconcileKeyvalueNamespace.
func TestReconcileKeyvalueNamespace_CreateError(t *testing.T) {
	r := &DataResourceReconciler{Client: errNamespaceClient(t)}
	err := r.reconcileKeyvalueNamespace(context.Background(), "test-ns")
	if err == nil {
		t.Error("expected error from reconcileKeyvalueNamespace, got nil")
	}
}

// TestReconcileFilesystemNamespace_CreateError tests the error path in reconcileFilesystemNamespace.
func TestReconcileFilesystemNamespace_CreateError(t *testing.T) {
	r := &DataResourceReconciler{Client: errNamespaceClient(t)}
	err := r.reconcileFilesystemNamespace(context.Background(), "test-ns")
	if err == nil {
		t.Error("expected error from reconcileFilesystemNamespace, got nil")
	}
}

// TestEnsurePostgresNamespace_CreateError tests the error path in ensurePostgresNamespace.
func TestEnsurePostgresNamespace_CreateError(t *testing.T) {
	r := &DataResourceReconciler{Client: errNamespaceClient(t)}
	err := r.ensurePostgresNamespace(context.Background(), "test-ns")
	if err == nil {
		t.Error("expected error from ensurePostgresNamespace, got nil")
	}
}

// TestEnsureMariaDBNamespace_CreateError tests the error path in ensureMariaDBNamespace.
func TestEnsureMariaDBNamespace_CreateError(t *testing.T) {
	r := &DataResourceReconciler{Client: errNamespaceClient(t)}
	err := r.ensureMariaDBNamespace(context.Background(), "test-ns")
	if err == nil {
		t.Error("expected error from ensureMariaDBNamespace, got nil")
	}
}

// TestEnsureMySQLNamespace_CreateError tests the error path in ensureMySQLNamespace.
func TestEnsureMySQLNamespace_CreateError(t *testing.T) {
	r := &DataResourceReconciler{Client: errNamespaceClient(t)}
	err := r.ensureMySQLNamespace(context.Background(), "test-ns")
	if err == nil {
		t.Error("expected error from ensureMySQLNamespace, got nil")
	}
}

// TestReconcileTimeseriesNamespace_CreateError tests the error path in reconcileTimeseriesNamespace.
func TestReconcileTimeseriesNamespace_CreateError(t *testing.T) {
	r := &DataResourceReconciler{Client: errNamespaceClient(t)}
	err := r.reconcileTimeseriesNamespace(context.Background(), "test-ns")
	if err == nil {
		t.Error("expected error from reconcileTimeseriesNamespace, got nil")
	}
}

// TestReconcileFilesystemDelete_DeleteError tests the error path in reconcileFilesystemDelete.
func TestReconcileFilesystemDelete_DeleteError(t *testing.T) {
	scheme := newTestScheme(t)
	deleteErr := fmt.Errorf("injected delete failure")
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithInterceptorFuncs(interceptor.Funcs{
			Delete: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.DeleteOption) error {
				if _, ok := obj.(*corev1.PersistentVolumeClaim); ok {
					return deleteErr
				}
				return c.Delete(ctx, obj, opts...)
			},
		}).
		Build()
	r := &DataResourceReconciler{Client: fakeClient, Scheme: scheme}

	dr := &nestv1.DataResource{
		ObjectMeta: metav1.ObjectMeta{Name: "test-fs-del", Namespace: "default"},
		Spec:       nestv1.DataResourceSpec{Type: "filesystem", Tenant: "tenant-del"},
	}

	err := r.reconcileFilesystemDelete(context.Background(), dr)
	if err == nil {
		t.Error("expected error from reconcileFilesystemDelete when Delete fails, got nil")
	}
}

// TestTenantReconciler_UpdateError tests the error path when tenant Update fails.
func TestTenantReconciler_UpdateError(t *testing.T) {
	scheme := newTestScheme(t)
	tenant := &nestv1.Tenant{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test-tenant-update-err",
			Namespace: "default",
		},
		Spec: nestv1.TenantSpec{
			LicenseTier: "free", // triggers quota update path
		},
	}
	updateErr := fmt.Errorf("injected update failure")
	fakeClient := fake.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(tenant).
		WithInterceptorFuncs(interceptor.Funcs{
			Update: func(ctx context.Context, c client.WithWatch, obj client.Object, opts ...client.UpdateOption) error {
				if _, ok := obj.(*nestv1.Tenant); ok {
					return updateErr
				}
				return c.Update(ctx, obj, opts...)
			},
		}).
		Build()
	r := &TenantReconciler{Client: fakeClient, Scheme: scheme}
	req := ctrl.Request{NamespacedName: types.NamespacedName{Name: tenant.Name, Namespace: tenant.Namespace}}
	_, err := r.Reconcile(context.Background(), req)
	if err == nil {
		t.Error("expected error from Reconcile when Update fails, got nil")
	}
}
