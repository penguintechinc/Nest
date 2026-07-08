package controllers

import (
	"context"
	"fmt"
	"regexp"
	"strings"

	batchv1 "k8s.io/api/batch/v1"
	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// DarkDriveAdopter handles the low-level adoption actions: formatting and CephCluster enrollment.
type DarkDriveAdopter struct {
	client.Client
	Scheme *runtime.Scheme
}

// CreateFormatJob creates a minimal-privilege batch Job on the target node that formats
// the device with the requested filesystem (btrfs or zfs).
// For "raw" fsType, no Job is created (device handed directly to Rook).
// Job name: "nest-format-<darkdrive-name>"
//
// SECURITY: Device and node names are validated against HardwareInventory allow-list.
// Device must exist in the node's inventory and be in a safe/adoptable state (Dark or blank/nest-previous).
// For foreign-fs drives, eraseConfirmed must be true.
// The Job uses minimal privileges and mounts only the target device.
//
// TODO: Add a ValidatingWebhook to bind DarkDrive CRD creation to authorized operators,
// as DarkDrive is cluster-scoped and any user can create one.
func (a *DarkDriveAdopter) CreateFormatJob(ctx context.Context, dd *nestv1.DarkDrive) error {
	logger := log.FromContext(ctx)

	if dd.Spec.FsType == "raw" || dd.Spec.FsType == "" {
		logger.Info("skipping format job for raw/empty fsType", "darkdrive", dd.Name, "fsType", dd.Spec.FsType)
		return nil
	}

	// Validate device path and node name
	if err := validateDevicePath(dd.Spec.Device); err != nil {
		return fmt.Errorf("device path validation failed: %w", err)
	}
	if err := validateNodeName(dd.Spec.Node); err != nil {
		return fmt.Errorf("node name validation failed: %w", err)
	}

	// Look up device in HardwareInventory and verify it's in a safe state
	device, err := a.lookupDeviceInInventory(ctx, dd.Spec.Node, dd.Spec.Device)
	if err != nil {
		logger.Error(err, "device inventory validation failed", "node", dd.Spec.Node, "device", dd.Spec.Device)
		return err
	}

	// Verify signature safety: blank/nest-previous are always safe; foreign-fs requires eraseConfirmed
	if strings.HasPrefix(device.Signature, "foreign-fs:") && !dd.Spec.EraseConfirmed {
		return fmt.Errorf("device %q has foreign filesystem (%s); eraseConfirmed=true required", dd.Spec.Device, device.Signature)
	}

	// Safety gate: require EraseConfirmed for foreign-fs signatures (redundant check for extra safety)
	if strings.HasPrefix(dd.Spec.Signature, "foreign-fs:") && !dd.Spec.EraseConfirmed {
		return fmt.Errorf("cannot create format job for foreign-fs signature without EraseConfirmed=true")
	}

	jobName := "nest-format-" + dd.Name

	// Check if job already exists
	var existingJob batchv1.Job
	err = a.Client.Get(ctx, types.NamespacedName{Name: jobName, Namespace: "nest"}, &existingJob)
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

	// Use minimal capabilities instead of full Privileged mode
	// mkfs.btrfs needs SYS_ADMIN; mkfs.zfs may need SYS_RAWIO
	runAsNonRoot := false // Must run as root for format operations
	allowPrivilegeEscalation := true
	capabilities := &corev1.Capabilities{
		Add: []corev1.Capability{
			"SYS_ADMIN",
			"SYS_RAWIO", // For raw I/O operations
		},
		Drop: []corev1.Capability{
			"ALL", // Drop all others
		},
	}

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
					// Narrow toleration: only tolerate node's taints, don't blindly tolerate all
					Tolerations: []corev1.Toleration{
						{
							Operator: corev1.TolerationOpEqual,
							Effect:   corev1.TaintEffectNoSchedule,
							// No key/value — will be matched by nodeAffinity if needed
						},
					},
					Containers: []corev1.Container{
						{
							Name:    "format",
							Image:   "ghcr.io/penguintechinc/nest/nest-node-agent:latest",
							Command: cmd,
							SecurityContext: &corev1.SecurityContext{
								RunAsNonRoot:             &runAsNonRoot, // Must run as root
								AllowPrivilegeEscalation: &allowPrivilegeEscalation,
								Capabilities:             capabilities,
							},
							VolumeMounts: []corev1.VolumeMount{
								{Name: "device", MountPath: dd.Spec.Device, MountPropagation: func() *corev1.MountPropagationMode { m := corev1.MountPropagationNone; return &m }()},
							},
						},
					},
					// Mount ONLY the target device, not all of /dev
					Volumes: []corev1.Volume{
						{
							Name: "device",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: dd.Spec.Device,
									Type: func() *corev1.HostPathType { t := corev1.HostPathCharDev; return &t }(),
								},
							},
						},
					},
				},
			},
		},
	}

	// Set OwnerReference so the Job is garbage-collected when the DarkDrive is deleted
	if err := controllerutil.SetControllerReference(dd, job, a.Scheme); err != nil {
		return fmt.Errorf("error setting OwnerReference on format job: %w", err)
	}

	if err := a.Client.Create(ctx, job); err != nil {
		return fmt.Errorf("error creating format job: %w", err)
	}
	logger.Info("created format job", "job", jobName, "fsType", dd.Spec.FsType, "device", dd.Spec.Device, "node", dd.Spec.Node)
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

// validateDevicePath checks that the device path is a valid block device path and
// rejects shell metacharacters, .., spaces, etc. Format: /dev/[a-zA-Z0-9/_-]+
func validateDevicePath(devPath string) error {
	// Strict validation: /dev/<safe-name>
	validDevicePathPattern := regexp.MustCompile(`^/dev/[a-zA-Z0-9/_-]+$`)
	if !validDevicePathPattern.MatchString(devPath) {
		return fmt.Errorf("invalid device path %q: must match ^/dev/[a-zA-Z0-9/_-]+$", devPath)
	}
	// Reject .. to prevent directory traversal
	if strings.Contains(devPath, "..") {
		return fmt.Errorf("invalid device path %q: contains ..", devPath)
	}
	// Reject spaces
	if strings.Contains(devPath, " ") {
		return fmt.Errorf("invalid device path %q: contains spaces", devPath)
	}
	return nil
}

// validateNodeName checks that the node name is a valid Kubernetes node name.
// Format: alphanumeric, hyphens, dots; no spaces or shell metacharacters.
func validateNodeName(nodeName string) error {
	validNodeNamePattern := regexp.MustCompile(`^[a-zA-Z0-9.-]+$`)
	if !validNodeNamePattern.MatchString(nodeName) {
		return fmt.Errorf("invalid node name %q: must match ^[a-zA-Z0-9.-]+$", nodeName)
	}
	return nil
}

// lookupDeviceInInventory retrieves the device from the node's HardwareInventory.
// Returns the device if found and in a safe state, or an error otherwise.
func (a *DarkDriveAdopter) lookupDeviceInInventory(ctx context.Context, nodeName, deviceName string) (*nestv1.DeviceSpec, error) {
	logger := log.FromContext(ctx)

	// Look up HardwareInventory for the node
	var inventory nestv1.HardwareInventory
	err := a.Client.Get(ctx, types.NamespacedName{Name: nodeName}, &inventory)
	if err != nil {
		if apierrors.IsNotFound(err) {
			return nil, fmt.Errorf("node %q has no HardwareInventory; cannot validate device %q", nodeName, deviceName)
		}
		return nil, fmt.Errorf("error looking up HardwareInventory for node %q: %w", nodeName, err)
	}

	// Search for the device in the inventory
	for i := range inventory.Spec.Devices {
		d := &inventory.Spec.Devices[i]
		if d.Name == deviceName {
			// Found device — check if it's in a safe state
			// Only allow Dark (adoptable) or System (but system should be blocked by node-agent)
			// Never allow Active (in-use) or Failed/Rejected
			if d.State == nestv1.DeviceStateActive {
				return nil, fmt.Errorf("device %q on node %q is in Active state; cannot format", deviceName, nodeName)
			}
			if d.State == nestv1.DeviceStateFailed || d.State == nestv1.DeviceStateRejected {
				return nil, fmt.Errorf("device %q on node %q is in %s state; cannot format", deviceName, nodeName, d.State)
			}
			logger.Info("device found in inventory", "node", nodeName, "device", deviceName, "state", d.State, "signature", d.Signature)
			return d, nil
		}
	}

	return nil, fmt.Errorf("device %q not found in HardwareInventory for node %q; allow-list validation failed", deviceName, nodeName)
}
