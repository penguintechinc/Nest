package controllers

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// DataProtectionPolicyReconciler reconciles DataProtectionPolicy CRDs.
// It creates VolumeSnapshot and Velero Backup CRs on the configured schedules
// and enforces retention policies.
type DataProtectionPolicyReconciler struct {
	client.Client
	Scheme    *runtime.Scheme
	DynClient dynamic.Interface
}

var (
	volumeSnapshotGVR = schema.GroupVersionResource{
		Group:    "snapshot.storage.k8s.io",
		Version:  "v1",
		Resource: "volumesnapshots",
	}
	veleroBackupGVR = schema.GroupVersionResource{
		Group:    "velero.io",
		Version:  "v1",
		Resource: "backups",
	}
	veleroRestoreGVR = schema.GroupVersionResource{
		Group:    "velero.io",
		Version:  "v1",
		Resource: "restores",
	}
)

// +kubebuilder:rbac:groups=nest.penguintech.io,resources=dataprotectionpolicies,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=nest.penguintech.io,resources=dataprotectionpolicies/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=nest.penguintech.io,resources=dataprotectionpolicies/finalizers,verbs=update
// +kubebuilder:rbac:groups=snapshot.storage.k8s.io,resources=volumesnapshots,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=velero.io,resources=backups,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=velero.io,resources=restores,verbs=get;list;watch;create;update;patch;delete

func (r *DataProtectionPolicyReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	logger := log.FromContext(ctx)

	var policy nestv1.DataProtectionPolicy
	if err := r.Get(ctx, req.NamespacedName, &policy); err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		logger.Error(err, "unable to fetch DataProtectionPolicy")
		return ctrl.Result{}, err
	}

	logger.Info("reconciling DataProtectionPolicy", "name", policy.Name, "namespace", policy.Namespace)

	// Apply policy labels to covered resources so Velero/snapshot selectors can find them
	if policy.Spec.Scope != nil {
		if err := r.applyPolicyLabels(ctx, &policy); err != nil {
			logger.Error(err, "failed to apply policy labels to resources")
			// Continue with backup/snapshot creation even if labeling fails
		}
	}

	// Process snapshots if configured
	if policy.Spec.Snapshots != nil {
		if err := r.reconcileSnapshots(ctx, &policy); err != nil {
			logger.Error(err, "failed to reconcile snapshots")
		}
	}

	// Process backups if configured
	if policy.Spec.Backups != nil {
		if err := r.reconcileBackups(ctx, &policy); err != nil {
			logger.Error(err, "failed to reconcile backups")
		}
	}

	// Process PITR if configured
	if policy.Spec.PITR != nil && policy.Spec.PITR.Enabled {
		if err := r.reconcilePITR(ctx, &policy); err != nil {
			logger.Error(err, "failed to reconcile PITR")
		}
	}

	// Process cross-region copy if configured
	if policy.Spec.Backups != nil && policy.Spec.Backups.CrossRegionCopy != nil && policy.Spec.Backups.CrossRegionCopy.Enabled {
		if err := r.reconcileCrossRegionCopy(ctx, &policy); err != nil {
			logger.Error(err, "failed to reconcile cross-region copy")
		}
	}

	// Process restore verification if configured
	if policy.Spec.Verify != nil && policy.Spec.Verify.RestoreTest != nil {
		if err := r.reconcileVerify(ctx, &policy); err != nil {
			logger.Error(err, "failed to reconcile restore verification")
		}
	}

	// Requeue after 1 minute minimum
	return ctrl.Result{RequeueAfter: time.Minute}, nil
}

// reconcileSnapshots checks if a new snapshot should be created based on schedule.
func (r *DataProtectionPolicyReconciler) reconcileSnapshots(ctx context.Context, policy *nestv1.DataProtectionPolicy) error {
	logger := log.FromContext(ctx)

	if policy.Spec.Snapshots.PVCName == "" {
		logger.Info("snapshot configured but pvcName is empty", "policy", policy.Name)
		return nil
	}

	lastSnapshotTime := getAnnotation(policy, "nest.penguintech.io/last-snapshot")
	lastTime, _ := time.Parse(time.RFC3339, lastSnapshotTime)

	// Parse schedule and check if next snapshot is due
	interval, err := parseDuration(policy.Spec.Snapshots.Schedule)
	if err != nil {
		logger.Error(err, "invalid snapshot schedule", "schedule", policy.Spec.Snapshots.Schedule)
		return err
	}

	now := time.Now()
	if lastTime.IsZero() || now.Sub(lastTime) >= interval {
		snapName := fmt.Sprintf("%s-snap-%d", policy.Name, now.Unix())
		snap := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "snapshot.storage.k8s.io/v1",
				"kind":       "VolumeSnapshot",
				"metadata": map[string]interface{}{
					"name":      snapName,
					"namespace": policy.Namespace,
					"labels": map[string]interface{}{
						"nest.penguintech.io/policy": policy.Name,
					},
				},
				"spec": map[string]interface{}{
					"volumeSnapshotClassName": "nest-rbd-snapshot",
					"source": map[string]interface{}{
						"persistentVolumeClaimName": policy.Spec.Snapshots.PVCName,
					},
				},
			},
		}

		_, err := r.DynClient.Resource(volumeSnapshotGVR).Namespace(policy.Namespace).Create(ctx, snap, metav1.CreateOptions{FieldManager: "nest-controller"})
		if err != nil {
			logger.Error(err, "failed to create VolumeSnapshot", "name", snapName)
			return err
		}
		logger.Info("VolumeSnapshot created", "name", snapName)

		// Update last snapshot time annotation
		setAnnotation(policy, "nest.penguintech.io/last-snapshot", now.Format(time.RFC3339))
		if err := r.Update(ctx, policy); err != nil {
			logger.Error(err, "failed to update policy annotation")
			return err
		}
	}

	// Enforce retention if configured
	if policy.Spec.Snapshots.Retention != nil {
		if err := r.enforceSnapshotRetention(ctx, policy); err != nil {
			logger.Error(err, "failed to enforce snapshot retention")
			return err
		}
	}

	return nil
}

// reconcileBackups checks if a new backup should be created based on schedule.
func (r *DataProtectionPolicyReconciler) reconcileBackups(ctx context.Context, policy *nestv1.DataProtectionPolicy) error {
	logger := log.FromContext(ctx)

	lastBackupTime := getAnnotation(policy, "nest.penguintech.io/last-backup")
	lastTime, _ := time.Parse(time.RFC3339, lastBackupTime)

	// Parse schedule and check if next backup is due
	interval, err := parseDuration(policy.Spec.Backups.Schedule)
	if err != nil {
		logger.Error(err, "invalid backup schedule", "schedule", policy.Spec.Backups.Schedule)
		return err
	}

	now := time.Now()
	if lastTime.IsZero() || now.Sub(lastTime) >= interval {
		backupName := fmt.Sprintf("%s-backup-%d", policy.Name, now.Unix())

		// Determine TTL from retention policy
		ttl := "720h0m0s" // 30 days default
		if policy.Spec.Backups.Retention != nil && policy.Spec.Backups.Retention.Daily > 0 {
			ttlSeconds := int64(policy.Spec.Backups.Retention.Daily * 86400)
			ttl = fmt.Sprintf("%dh%dm%ds", ttlSeconds/3600, (ttlSeconds%3600)/60, ttlSeconds%60)
		}

		backup := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "velero.io/v1",
				"kind":       "Backup",
				"metadata": map[string]interface{}{
					"name":      backupName,
					"namespace": "velero",
					"labels": map[string]interface{}{
						"nest.penguintech.io/policy": policy.Name,
					},
				},
				"spec": map[string]interface{}{
					"storageLocation":    "nest-default",
					"includedNamespaces": []interface{}{policy.Namespace},
					"labelSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"nest.penguintech.io/policy": policy.Name,
						},
					},
					"ttl": ttl,
				},
			},
		}

		_, err := r.DynClient.Resource(veleroBackupGVR).Namespace("velero").Create(ctx, backup, metav1.CreateOptions{FieldManager: "nest-controller"})
		if err != nil {
			logger.Error(err, "failed to create Velero Backup", "name", backupName)
			return err
		}
		logger.Info("Velero Backup created", "name", backupName)

		// Update last backup time annotation
		setAnnotation(policy, "nest.penguintech.io/last-backup", now.Format(time.RFC3339))
		if err := r.Update(ctx, policy); err != nil {
			logger.Error(err, "failed to update policy annotation")
			return err
		}
	}

	// Enforce retention if configured
	if policy.Spec.Backups.Retention != nil {
		if err := r.enforceBackupRetention(ctx, policy); err != nil {
			logger.Error(err, "failed to enforce backup retention")
			return err
		}
	}

	return nil
}

// enforceSnapshotRetention deletes old snapshots beyond the retention count.
func (r *DataProtectionPolicyReconciler) enforceSnapshotRetention(ctx context.Context, policy *nestv1.DataProtectionPolicy) error {
	logger := log.FromContext(ctx)

	// Determine max snapshots to keep based on retention policy
	maxSnapshots := policy.Spec.Snapshots.Retention.Daily
	if maxSnapshots == 0 {
		maxSnapshots = policy.Spec.Snapshots.Retention.Hourly
	}
	if maxSnapshots == 0 {
		maxSnapshots = policy.Spec.Snapshots.Retention.Weekly
	}
	if maxSnapshots == 0 {
		maxSnapshots = 7 // default to 7
	}

	// List snapshots for this policy, but exclude PITR snapshots (they have separate retention)
	snapshots, err := r.DynClient.Resource(volumeSnapshotGVR).Namespace(policy.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("nest.penguintech.io/policy=%s,nest.penguintech.io/pitr!=true", policy.Name),
	})
	if err != nil {
		logger.Error(err, "failed to list snapshots")
		return err
	}

	if len(snapshots.Items) <= int(maxSnapshots) {
		return nil
	}

	// Sort by creation time and delete oldest ones
	items := snapshots.Items
	sort.Slice(items, func(i, j int) bool {
		iTime := items[i].GetCreationTimestamp()
		jTime := items[j].GetCreationTimestamp()
		return iTime.Before(&jTime)
	})

	// Delete snapshots beyond retention limit
	for i := 0; i < len(items)-int(maxSnapshots); i++ {
		snapName := items[i].GetName()
		if err := r.DynClient.Resource(volumeSnapshotGVR).Namespace(policy.Namespace).Delete(ctx, snapName, metav1.DeleteOptions{}); err != nil {
			logger.Error(err, "failed to delete old snapshot", "name", snapName)
		} else {
			logger.Info("old snapshot deleted", "name", snapName)
		}
	}

	return nil
}

// enforceBackupRetention deletes old backups beyond the retention count.
func (r *DataProtectionPolicyReconciler) enforceBackupRetention(ctx context.Context, policy *nestv1.DataProtectionPolicy) error {
	logger := log.FromContext(ctx)

	// Determine max backups to keep
	maxBackups := policy.Spec.Backups.Retention.Daily
	if maxBackups == 0 {
		maxBackups = policy.Spec.Backups.Retention.Hourly
	}
	if maxBackups == 0 {
		maxBackups = policy.Spec.Backups.Retention.Weekly
	}
	if maxBackups == 0 {
		maxBackups = 7 // default to 7
	}

	// List backups for this policy
	backups, err := r.DynClient.Resource(veleroBackupGVR).Namespace("velero").List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("nest.penguintech.io/policy=%s", policy.Name),
	})
	if err != nil {
		logger.Error(err, "failed to list backups")
		return err
	}

	if len(backups.Items) <= int(maxBackups) {
		return nil
	}

	// Sort by creation time and delete oldest ones
	items := backups.Items
	sort.Slice(items, func(i, j int) bool {
		iTime := items[i].GetCreationTimestamp()
		jTime := items[j].GetCreationTimestamp()
		return iTime.Before(&jTime)
	})

	// Delete backups beyond retention limit
	for i := 0; i < len(items)-int(maxBackups); i++ {
		backupName := items[i].GetName()
		if err := r.DynClient.Resource(veleroBackupGVR).Namespace("velero").Delete(ctx, backupName, metav1.DeleteOptions{}); err != nil {
			logger.Error(err, "failed to delete old backup", "name", backupName)
		} else {
			logger.Info("old backup deleted", "name", backupName)
		}
	}

	return nil
}

// parseDuration parses schedule strings like @hourly, @daily, @weekly, @monthly, or @every Xh/Xm.
func parseDuration(schedule string) (time.Duration, error) {
	switch schedule {
	case "@hourly":
		return time.Hour, nil
	case "@daily":
		return 24 * time.Hour, nil
	case "@weekly":
		return 7 * 24 * time.Hour, nil
	case "@monthly":
		return 30 * 24 * time.Hour, nil
	}
	if strings.HasPrefix(schedule, "@every ") {
		return time.ParseDuration(strings.TrimPrefix(schedule, "@every "))
	}
	return 0, fmt.Errorf("unsupported schedule format: %q (use @hourly, @daily, @weekly, @monthly, or @every <duration>)", schedule)
}

// getAnnotation retrieves an annotation value from the policy.
func getAnnotation(policy *nestv1.DataProtectionPolicy, key string) string {
	if policy.Annotations == nil {
		return ""
	}
	return policy.Annotations[key]
}

// setAnnotation sets an annotation on the policy.
func setAnnotation(policy *nestv1.DataProtectionPolicy, key, value string) {
	if policy.Annotations == nil {
		policy.Annotations = make(map[string]string)
	}
	policy.Annotations[key] = value
}

// reconcilePITR creates frequent VolumeSnapshots labeled for PITR and enforces the window.
func (r *DataProtectionPolicyReconciler) reconcilePITR(ctx context.Context, policy *nestv1.DataProtectionPolicy) error {
	logger := log.FromContext(ctx)

	if policy.Spec.Snapshots == nil || policy.Spec.Snapshots.PVCName == "" {
		logger.Info("PITR enabled but no snapshot PVC configured", "policy", policy.Name)
		return nil
	}

	lastPITRTime := getAnnotation(policy, "nest.penguintech.io/last-pitr")
	lastTime, _ := time.Parse(time.RFC3339, lastPITRTime)

	now := time.Now()
	// PITR snapshots every 15 minutes
	if lastTime.IsZero() || now.Sub(lastTime) >= 15*time.Minute {
		snapName := fmt.Sprintf("%s-pitr-%d", policy.Name, now.Unix())
		snap := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "snapshot.storage.k8s.io/v1",
				"kind":       "VolumeSnapshot",
				"metadata": map[string]interface{}{
					"name":      snapName,
					"namespace": policy.Namespace,
					"labels": map[string]interface{}{
						"nest.penguintech.io/policy": policy.Name,
						"nest.penguintech.io/pitr":   "true",
					},
				},
				"spec": map[string]interface{}{
					"volumeSnapshotClassName": "nest-rbd-snapshot",
					"source": map[string]interface{}{
						"persistentVolumeClaimName": policy.Spec.Snapshots.PVCName,
					},
				},
			},
		}
		_, err := r.DynClient.Resource(volumeSnapshotGVR).Namespace(policy.Namespace).Create(ctx, snap, metav1.CreateOptions{FieldManager: "nest-controller"})
		if err != nil {
			logger.Error(err, "failed to create PITR snapshot", "name", snapName)
			return err
		}
		logger.Info("PITR snapshot created", "name", snapName)

		setAnnotation(policy, "nest.penguintech.io/last-pitr", now.Format(time.RFC3339))
		if err := r.Update(ctx, policy); err != nil {
			return err
		}
	}

	// Prune PITR snapshots outside the retention window
	if policy.Spec.PITR.WindowDays > 0 {
		if err := r.prunePITRSnapshots(ctx, policy); err != nil {
			logger.Error(err, "failed to prune PITR snapshots")
		}
	}
	return nil
}

// prunePITRSnapshots deletes PITR snapshots older than the configured window.
func (r *DataProtectionPolicyReconciler) prunePITRSnapshots(ctx context.Context, policy *nestv1.DataProtectionPolicy) error {
	logger := log.FromContext(ctx)
	cutoff := time.Now().AddDate(0, 0, -int(policy.Spec.PITR.WindowDays))

	snapshots, err := r.DynClient.Resource(volumeSnapshotGVR).Namespace(policy.Namespace).List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("nest.penguintech.io/policy=%s,nest.penguintech.io/pitr=true", policy.Name),
	})
	if err != nil {
		return err
	}
	for _, snap := range snapshots.Items {
		if snap.GetCreationTimestamp().Time.Before(cutoff) {
			if err := r.DynClient.Resource(volumeSnapshotGVR).Namespace(policy.Namespace).Delete(ctx, snap.GetName(), metav1.DeleteOptions{}); err != nil {
				logger.Error(err, "failed to delete expired PITR snapshot", "name", snap.GetName())
			} else {
				logger.Info("expired PITR snapshot deleted", "name", snap.GetName())
			}
		}
	}
	return nil
}

// reconcileCrossRegionCopy triggers a Velero backup targeting the cross-region storage location.
func (r *DataProtectionPolicyReconciler) reconcileCrossRegionCopy(ctx context.Context, policy *nestv1.DataProtectionPolicy) error {
	logger := log.FromContext(ctx)
	xr := policy.Spec.Backups.CrossRegionCopy

	lastXRTime := getAnnotation(policy, "nest.penguintech.io/last-xr-backup")
	lastTime, _ := time.Parse(time.RFC3339, lastXRTime)

	// Default cross-region backup frequency: daily
	interval := 24 * time.Hour
	if policy.Spec.Backups.Schedule != "" {
		if d, err := parseDuration(policy.Spec.Backups.Schedule); err == nil {
			interval = d
		}
	}

	now := time.Now()
	if lastTime.IsZero() || now.Sub(lastTime) >= interval {
		backupName := fmt.Sprintf("%s-xr-%d", policy.Name, now.Unix())
		ttl := "2160h0m0s" // 90 days default for cross-region
		storageLocation := xr.Destination
		if storageLocation == "" {
			storageLocation = "nest-cross-region"
		}

		backup := &unstructured.Unstructured{
			Object: map[string]interface{}{
				"apiVersion": "velero.io/v1",
				"kind":       "Backup",
				"metadata": map[string]interface{}{
					"name":      backupName,
					"namespace": "velero",
					"labels": map[string]interface{}{
						"nest.penguintech.io/policy":         policy.Name,
						"nest.penguintech.io/cross-region":   "true",
						"nest.penguintech.io/xr-destination": storageLocation,
					},
				},
				"spec": map[string]interface{}{
					"storageLocation":    storageLocation,
					"includedNamespaces": []interface{}{policy.Namespace},
					"labelSelector": map[string]interface{}{
						"matchLabels": map[string]interface{}{
							"nest.penguintech.io/policy": policy.Name,
						},
					},
					"ttl": ttl,
				},
			},
		}

		_, err := r.DynClient.Resource(veleroBackupGVR).Namespace("velero").Create(ctx, backup, metav1.CreateOptions{FieldManager: "nest-controller"})
		if err != nil {
			logger.Error(err, "failed to create cross-region backup", "name", backupName)
			return err
		}
		logger.Info("cross-region backup created", "name", backupName, "destination", storageLocation)

		setAnnotation(policy, "nest.penguintech.io/last-xr-backup", now.Format(time.RFC3339))
		if err := r.Update(ctx, policy); err != nil {
			return err
		}
	}
	return nil
}

// reconcileVerify triggers a Velero restore into a scratch namespace to validate backup integrity.
func (r *DataProtectionPolicyReconciler) reconcileVerify(ctx context.Context, policy *nestv1.DataProtectionPolicy) error {
	logger := log.FromContext(ctx)
	cfg := policy.Spec.Verify.RestoreTest

	lastVerifyTime := getAnnotation(policy, "nest.penguintech.io/last-verify")
	lastTime, _ := time.Parse(time.RFC3339, lastVerifyTime)

	interval := 7 * 24 * time.Hour // default weekly
	if cfg.Schedule != "" {
		if d, err := parseDuration(cfg.Schedule); err == nil {
			interval = d
		}
	}

	now := time.Now()
	if !lastTime.IsZero() && now.Sub(lastTime) < interval {
		return nil
	}

	// Find the most recent Velero backup for this policy
	backups, err := r.DynClient.Resource(veleroBackupGVR).Namespace("velero").List(ctx, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("nest.penguintech.io/policy=%s", policy.Name),
	})
	if err != nil || len(backups.Items) == 0 {
		logger.Info("no backups available for verification", "policy", policy.Name)
		return nil
	}

	// Pick the most recent
	items := backups.Items
	sort.Slice(items, func(i, j int) bool {
		return items[i].GetCreationTimestamp().After(items[j].GetCreationTimestamp().Time)
	})
	latestBackup := items[0].GetName()

	targetNS := cfg.Target
	if targetNS == "" {
		targetNS = fmt.Sprintf("%s-verify", policy.Namespace)
	}

	restoreName := fmt.Sprintf("%s-verify-%d", policy.Name, now.Unix())
	restore := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "velero.io/v1",
			"kind":       "Restore",
			"metadata": map[string]interface{}{
				"name":      restoreName,
				"namespace": "velero",
				"labels": map[string]interface{}{
					"nest.penguintech.io/policy": policy.Name,
					"nest.penguintech.io/verify": "true",
				},
			},
			"spec": map[string]interface{}{
				"backupName":             latestBackup,
				"includedNamespaces":     []interface{}{policy.Namespace},
				"namespaceMapping":       map[string]interface{}{policy.Namespace: targetNS},
				"restorePVs":             false,
				"existingResourcePolicy": "update",
			},
		},
	}

	_, err = r.DynClient.Resource(veleroRestoreGVR).Namespace("velero").Create(ctx, restore, metav1.CreateOptions{FieldManager: "nest-controller"})
	if err != nil {
		logger.Error(err, "failed to create verify restore", "name", restoreName)
		return err
	}
	logger.Info("verification restore created", "name", restoreName, "backup", latestBackup, "targetNS", targetNS)

	setAnnotation(policy, "nest.penguintech.io/last-verify", now.Format(time.RFC3339))
	return r.Update(ctx, policy)
}

// applyPolicyLabels applies the nest.penguintech.io/policy label to resources covered by the policy scope
// This allows Velero backup selectors and snapshot labels to actually match resources
func (r *DataProtectionPolicyReconciler) applyPolicyLabels(ctx context.Context, policy *nestv1.DataProtectionPolicy) error {
	logger := log.FromContext(ctx)

	if policy.Spec.Scope == nil {
		return nil // No scope defined, nothing to label
	}

	scope := policy.Spec.Scope

	// Determine namespaces to scan
	namespacesToScan := scope.Namespaces
	if len(namespacesToScan) == 0 {
		// If no namespaces specified, use the policy namespace
		namespacesToScan = []string{policy.Namespace}
	}

	// Determine resource types to label
	resourceTypes := scope.ResourceTypes
	if len(resourceTypes) == 0 {
		// Default to common resource types that might contain data
		resourceTypes = []string{"DataResource", "Pod", "StatefulSet", "Deployment", "PersistentVolumeClaim"}
	}

	// Label DataResources matching the selector
	for _, ns := range namespacesToScan {
		var drList nestv1.DataResourceList
		if err := r.List(ctx, &drList, client.InNamespace(ns)); err != nil {
			logger.Error(err, "failed to list DataResources for labeling", "namespace", ns)
			continue
		}

		for _, dr := range drList.Items {
			// Check if this DataResource matches the label selector
			if scope.LabelSelector != nil {
				selector, err := metav1.LabelSelectorAsSelector(scope.LabelSelector)
				if err != nil {
					logger.Error(err, "invalid label selector")
					continue
				}
				if !selector.Matches(labels.Set(dr.Labels)) {
					continue // This resource doesn't match the selector
				}
			}

			// Apply the policy label
			if dr.Labels == nil {
				dr.Labels = make(map[string]string)
			}
			if dr.Labels["nest.penguintech.io/policy"] != policy.Name {
				dr.Labels["nest.penguintech.io/policy"] = policy.Name
				if err := r.Update(ctx, &dr); err != nil {
					logger.Error(err, "failed to label DataResource", "name", dr.Name, "namespace", dr.Namespace)
				} else {
					logger.Info("labeled DataResource with policy", "name", dr.Name, "policy", policy.Name)
				}
			}
		}
	}

	return nil
}

// SetupWithManager registers the reconciler with the controller-runtime manager.
func (r *DataProtectionPolicyReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&nestv1.DataProtectionPolicy{}).
		Complete(r)
}
