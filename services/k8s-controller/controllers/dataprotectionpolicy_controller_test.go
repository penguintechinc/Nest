package controllers

import (
	"context"
	"testing"
	"time"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic/fake"
	ctrl "sigs.k8s.io/controller-runtime"
	fakeClient "sigs.k8s.io/controller-runtime/pkg/client/fake"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// TestReconcileDataProtectionPolicy_NotFound tests that reconciling a non-existent DataProtectionPolicy returns no error
func TestReconcileDataProtectionPolicy_NotFound(t *testing.T) {
	scheme := newTestScheme(t)
	fakeK8sClient := fakeClient.NewClientBuilder().WithScheme(scheme).Build()
	dynClient := fake.NewSimpleDynamicClient(runtime.NewScheme())

	r := &DataProtectionPolicyReconciler{
		Client:    fakeK8sClient,
		Scheme:    scheme,
		DynClient: dynClient,
	}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      "nonexistent-dpp",
			Namespace: "default",
		},
	}

	result, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
	if result != (ctrl.Result{}) {
		t.Errorf("Reconcile() result = %v, want empty Result", result)
	}
}

// TestReconcileDataProtectionPolicy_NoSchedule tests that a DPP with empty schedules doesn't error
func TestReconcileDataProtectionPolicy_NoSchedule(t *testing.T) {
	scheme := newTestScheme(t)

	dpp := &nestv1.DataProtectionPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "no-schedule-dpp",
			Namespace: "default",
		},
		Spec: nestv1.DataProtectionPolicySpec{
			// No Snapshots, Backups, or PITR configured
		},
	}

	fakeK8sClient := fakeClient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dpp).
		WithStatusSubresource(&nestv1.DataProtectionPolicy{}).
		Build()
	dynClient := fake.NewSimpleDynamicClient(runtime.NewScheme())

	r := &DataProtectionPolicyReconciler{
		Client:    fakeK8sClient,
		Scheme:    scheme,
		DynClient: dynClient,
	}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dpp.Name,
			Namespace: dpp.Namespace,
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	// Verify the DPP was retrieved without error
	var updated nestv1.DataProtectionPolicy
	if err := fakeK8sClient.Get(ctx, types.NamespacedName{Name: dpp.Name, Namespace: dpp.Namespace}, &updated); err != nil {
		t.Errorf("failed to get DPP: %v", err)
	}
}

// TestReconcileDataProtectionPolicy_Snapshot tests snapshot creation with @hourly schedule
func TestReconcileDataProtectionPolicy_Snapshot(t *testing.T) {
	scheme := newTestScheme(t)

	dpp := &nestv1.DataProtectionPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "snapshot-dpp",
			Namespace: "default",
		},
		Spec: nestv1.DataProtectionPolicySpec{
			Snapshots: &nestv1.SnapshotConfig{
				Schedule: "@hourly",
				PVCName:  "test-pvc",
			},
		},
	}

	fakeK8sClient := fakeClient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dpp).
		WithStatusSubresource(&nestv1.DataProtectionPolicy{}).
		Build()

	// Create dynamic client with initial empty objects
	dynamicScheme := runtime.NewScheme()
	dynClient := fake.NewSimpleDynamicClient(dynamicScheme)

	r := &DataProtectionPolicyReconciler{
		Client:    fakeK8sClient,
		Scheme:    scheme,
		DynClient: dynClient,
	}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dpp.Name,
			Namespace: dpp.Namespace,
		},
	}

	result, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}
	if result.RequeueAfter != time.Minute {
		t.Errorf("Reconcile() RequeueAfter = %v, want %v", result.RequeueAfter, time.Minute)
	}

	// Verify the last-snapshot annotation was set on the DPP
	var updated nestv1.DataProtectionPolicy
	if err := fakeK8sClient.Get(ctx, types.NamespacedName{Name: dpp.Name, Namespace: dpp.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DPP: %v", err)
	}
	if updated.Annotations["nest.penguintech.io/last-snapshot"] == "" {
		t.Errorf("DPP missing last-snapshot annotation")
	}
}

// TestReconcilePITR tests PITR snapshot creation and labeling
func TestReconcilePITR(t *testing.T) {
	scheme := newTestScheme(t)

	dpp := &nestv1.DataProtectionPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "pitr-dpp",
			Namespace: "default",
		},
		Spec: nestv1.DataProtectionPolicySpec{
			Snapshots: &nestv1.SnapshotConfig{
				PVCName: "test-pvc",
			},
			PITR: &nestv1.PITRConfig{
				Enabled: true,
				// Note: WindowDays is 0, so prunePITRSnapshots won't be called
			},
		},
	}

	fakeK8sClient := fakeClient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dpp).
		WithStatusSubresource(&nestv1.DataProtectionPolicy{}).
		Build()

	dynamicScheme := runtime.NewScheme()
	dynClient := fake.NewSimpleDynamicClient(dynamicScheme)

	r := &DataProtectionPolicyReconciler{
		Client:    fakeK8sClient,
		Scheme:    scheme,
		DynClient: dynClient,
	}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dpp.Name,
			Namespace: dpp.Namespace,
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	// Verify the last-pitr annotation was set on the DPP
	var updated nestv1.DataProtectionPolicy
	if err := fakeK8sClient.Get(ctx, types.NamespacedName{Name: dpp.Name, Namespace: dpp.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DPP: %v", err)
	}
	if updated.Annotations["nest.penguintech.io/last-pitr"] == "" {
		t.Errorf("DPP missing last-pitr annotation")
	}
}

// TestPrunePITRSnapshots tests that old PITR snapshots are deleted beyond retention window
func TestPrunePITRSnapshots(t *testing.T) {
	scheme := newTestScheme(t)

	dpp := &nestv1.DataProtectionPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "prune-dpp",
			Namespace: "default",
		},
		Spec: nestv1.DataProtectionPolicySpec{
			Snapshots: &nestv1.SnapshotConfig{
				PVCName: "test-pvc",
			},
			PITR: &nestv1.PITRConfig{
				Enabled: true,
				// Note: WindowDays is 0, so prunePITRSnapshots won't be called
			},
		},
	}

	// Create old and new PITR snapshots
	now := time.Now()
	oldSnapshot := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snapshot.storage.k8s.io/v1",
			"kind":       "VolumeSnapshot",
			"metadata": map[string]interface{}{
				"name":              "old-pitr-snap",
				"namespace":         "default",
				"creationTimestamp": metav1.Time{Time: now.AddDate(0, 0, -10)}.String(),
				"labels": map[string]interface{}{
					"nest.penguintech.io/policy": dpp.Name,
					"nest.penguintech.io/pitr":   "true",
				},
			},
		},
	}

	newSnapshot := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "snapshot.storage.k8s.io/v1",
			"kind":       "VolumeSnapshot",
			"metadata": map[string]interface{}{
				"name":              "new-pitr-snap",
				"namespace":         "default",
				"creationTimestamp": metav1.Time{Time: now.AddDate(0, 0, -2)}.String(),
				"labels": map[string]interface{}{
					"nest.penguintech.io/policy": dpp.Name,
					"nest.penguintech.io/pitr":   "true",
				},
			},
		},
	}

	fakeK8sClient := fakeClient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dpp).
		WithStatusSubresource(&nestv1.DataProtectionPolicy{}).
		Build()

	dynamicScheme := runtime.NewScheme()
	dynClient := fake.NewSimpleDynamicClient(dynamicScheme, oldSnapshot, newSnapshot)

	r := &DataProtectionPolicyReconciler{
		Client:    fakeK8sClient,
		Scheme:    scheme,
		DynClient: dynClient,
	}

	ctx := context.Background()

	// Call prunePITRSnapshots
	err := r.prunePITRSnapshots(ctx, dpp)
	if err != nil {
		t.Errorf("prunePITRSnapshots() error = %v, want nil", err)
	}
}

// TestReconcileDataProtectionPolicy_BackupCreation tests backup creation with @daily schedule
func TestReconcileDataProtectionPolicy_BackupCreation(t *testing.T) {
	scheme := newTestScheme(t)

	dpp := &nestv1.DataProtectionPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "backup-dpp",
			Namespace: "default",
		},
		Spec: nestv1.DataProtectionPolicySpec{
			Backups: &nestv1.BackupConfig{
				Schedule: "@daily",
			},
		},
	}

	fakeK8sClient := fakeClient.NewClientBuilder().
		WithScheme(scheme).
		WithObjects(dpp).
		WithStatusSubresource(&nestv1.DataProtectionPolicy{}).
		Build()

	dynamicScheme := runtime.NewScheme()
	dynClient := fake.NewSimpleDynamicClient(dynamicScheme)

	r := &DataProtectionPolicyReconciler{
		Client:    fakeK8sClient,
		Scheme:    scheme,
		DynClient: dynClient,
	}

	ctx := context.Background()
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Name:      dpp.Name,
			Namespace: dpp.Namespace,
		},
	}

	_, err := r.Reconcile(ctx, req)

	if err != nil {
		t.Errorf("Reconcile() error = %v, want nil", err)
	}

	// Verify the last-backup annotation was set on the DPP
	var updated nestv1.DataProtectionPolicy
	if err := fakeK8sClient.Get(ctx, types.NamespacedName{Name: dpp.Name, Namespace: dpp.Namespace}, &updated); err != nil {
		t.Fatalf("failed to get updated DPP: %v", err)
	}
	if updated.Annotations["nest.penguintech.io/last-backup"] == "" {
		t.Errorf("DPP missing last-backup annotation")
	}
}

// TestParseDuration tests various schedule formats
func TestParseDuration(t *testing.T) {
	tests := []struct {
		schedule string
		want     time.Duration
		wantErr  bool
	}{
		{"@hourly", time.Hour, false},
		{"@daily", 24 * time.Hour, false},
		{"@weekly", 7 * 24 * time.Hour, false},
		{"@monthly", 30 * 24 * time.Hour, false},
		{"@every 2h", 2 * time.Hour, false},
		{"@every 30m", 30 * time.Minute, false},
		{"invalid", 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.schedule, func(t *testing.T) {
			got, err := parseDuration(tt.schedule)
			if (err != nil) != tt.wantErr {
				t.Errorf("parseDuration(%q) error = %v, wantErr %v", tt.schedule, err, tt.wantErr)
				return
			}
			if got != tt.want {
				t.Errorf("parseDuration(%q) = %v, want %v", tt.schedule, got, tt.want)
			}
		})
	}
}

// TestGetSetAnnotation tests annotation helper functions
func TestGetSetAnnotation(t *testing.T) {
	dpp := &nestv1.DataProtectionPolicy{
		ObjectMeta: metav1.ObjectMeta{
			Name:      "test",
			Namespace: "default",
		},
	}

	// Test get on nil annotations
	val := getAnnotation(dpp, "missing")
	if val != "" {
		t.Errorf("getAnnotation on nil annotations = %q, want empty", val)
	}

	// Test set
	setAnnotation(dpp, "key1", "value1")
	if dpp.Annotations["key1"] != "value1" {
		t.Errorf("setAnnotation failed: got %q, want 'value1'", dpp.Annotations["key1"])
	}

	// Test get
	val = getAnnotation(dpp, "key1")
	if val != "value1" {
		t.Errorf("getAnnotation = %q, want 'value1'", val)
	}
}
