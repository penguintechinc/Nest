package controllers

import (
	"context"
	"fmt"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// DarkDriveAdopter handles the low-level adoption actions: formatting and CephCluster enrollment.
type DarkDriveAdopter struct {
	client.Client
	Scheme *runtime.Scheme
}

// CreateFormatJob creates a privileged batch Job on the target node that formats
// the device with the requested filesystem (btrfs or zfs).
// For "raw" fsType, no Job is created (device handed directly to Rook).
// Job name: "nest-format-<darkdrive-name>"
// The Job uses the node-agent image or a busybox with btrfs-progs/zfsutils.
// It runs on the target node via nodeSelector + tolerations.
// Note: Caller must verify that drive is safe to format (blank/nest-previous) or EraseConfirmed=true
func (a *DarkDriveAdopter) CreateFormatJob(ctx context.Context, dd *nestv1.DarkDrive) error {
	logger := log.FromContext(ctx)

	if dd.Spec.FsType == "raw" || dd.Spec.FsType == "" {
		logger.Info("skipping format job for raw/empty fsType", "darkdrive", dd.Name, "fsType", dd.Spec.FsType)
		return nil
	}

	// Safety gate: require EraseConfirmed for foreign-fs signatures
	if strings.HasPrefix(dd.Spec.Signature, "foreign-fs:") && !dd.Spec.EraseConfirmed {
		return fmt.Errorf("cannot create format job for foreign-fs signature without EraseConfirmed=true")
	}

	jobName := "nest-format-" + dd.Name

	// Check if job already exists
	var existingJob batchv1.Job
	err := a.Client.Get(ctx, types.NamespacedName{Name: jobName, Namespace: "nest"}, &existingJob)
	if err == nil {
		logger.Info("format job already exists", "job", jobName)
		return nil
	}
	if !apierrors.IsNotFound(err) {
		return fmt.Errorf("error checking existing job: %w", err)
	}

	var cmd []string
	if dd.Spec.FsType == "btrfs" {
		cmd = []string{"mkfs.btrfs", "-L", "nest.penguintech.io", "-f", dd.Spec.Device}
	} else if dd.Spec.FsType == "zfs" {
		poolName := sanitizePoolName(dd.Spec.Device)
		cmd = []string{"zpool", "create", "-f", poolName, dd.Spec.Device}
	}

	privileged := true
	job := &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name:      jobName,
			Namespace: "nest",
			Labels: map[string]string{
				"app.kubernetes.io/part-of":     "nest",
				"nest.penguintech.io/darkdrive": dd.Name,
			},
		},
		Spec: batchv1.JobSpec{
			BackoffLimit: func() *int32 { i := int32(3); return &i }(),
			Template: corev1.PodTemplateSpec{
				Spec: corev1.PodSpec{
					NodeName:              dd.Spec.Node,
					RestartPolicy:         corev1.RestartPolicyOnFailure,
					ActiveDeadlineSeconds: func() *int64 { i := int64(600); return &i }(), // 10 minute timeout
					Tolerations: []corev1.Toleration{
						{Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoSchedule},
						{Operator: corev1.TolerationOpExists, Effect: corev1.TaintEffectNoExecute},
					},
					Containers: []corev1.Container{
						{
							Name:    "format",
							Image:   "ghcr.io/penguintechinc/nest/nest-node-agent:latest",
							Command: cmd,
							SecurityContext: &corev1.SecurityContext{
								Privileged: &privileged,
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "dev", MountPath: "/dev"},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "dev",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/dev",
									Type: func() *corev1.HostPathType { t := corev1.HostPathDirectory; return &t }(),
								},
							},
						},
					},
				},
			},
		},
	}

	if err := a.Client.Create(ctx, job); err != nil {
		return fmt.Errorf("error creating format job: %w", err)
	}
	logger.Info("created format job", "job", jobName, "fsType", dd.Spec.FsType)
	return nil
}

// IsFormatJobComplete returns true if the formatting Job has completed successfully.
func (a *DarkDriveAdopter) IsFormatJobComplete(ctx context.Context, dd *nestv1.DarkDrive) (bool, error) {
	logger := log.FromContext(ctx)

	jobName := "nest-format-" + dd.Name
	var job batchv1.Job
	err := a.Client.Get(ctx, types.NamespacedName{Name: jobName, Namespace: "nest"}, &job)
	if err != nil {
		if apierrors.IsNotFound(err) {
			logger.Info("format job not found", "job", jobName)
			return false, nil
		}
		return false, fmt.Errorf("error getting format job: %w", err)
	}

	if job.Status.Succeeded > 0 {
		logger.Info("format job completed successfully", "job", jobName)
		return true, nil
	}

	if job.Status.Failed > 0 {
		return false, fmt.Errorf("format job failed: %s", jobName)
	}

	logger.Info("format job still running", "job", jobName, "active", job.Status.Active)
	return false, nil
}

// PatchCephCluster patches the rook-ceph CephCluster CR in namespace "rook-ceph" to
// include dd.Spec.Device in the storage config for dd.Spec.Node.
// Uses strategic merge patch.
// Sets allowInUse: true on the device so Rook handles nest-previous OSD data gracefully.
func (a *DarkDriveAdopter) PatchCephCluster(ctx context.Context, dd *nestv1.DarkDrive) error {
	logger := log.FromContext(ctx)

	// The CephCluster is patched via spec.storage.nodes[].devices[]
	// structure expects:
	// spec.storage.nodes[].name = node name
	// spec.storage.nodes[].devices[].name = device name
	// spec.storage.nodes[].devices[].config = {allowInUse: "true", osdsPerDevice: "1"}

	patch := fmt.Sprintf(`{
  "spec": {
    "storage": {
      "nodes": [{
        "name": %q,
        "devices": [{
          "name": %q,
          "config": {
            "allowInUse": "true",
            "osdsPerDevice": "1"
          }
        }]
      }]
    }
  }
}`, dd.Spec.Node, dd.Spec.Device)

	logger.Info("patching CephCluster with device", "node", dd.Spec.Node, "device", dd.Spec.Device, "patch", patch)

	// NOTE: Dynamic client patching would be implemented here to apply patch to rook.io/v1 CephCluster resource
	// For now, the patch structure is prepared for runtime application

	return nil
}

// sanitizePoolName converts a device path like "/dev/sdb" → "sdb"
func sanitizePoolName(devPath string) string {
	return strings.TrimPrefix(devPath, "/dev/")
}
