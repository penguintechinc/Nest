package migrate

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"text/template"
)

// LonghornVolume represents a discovered Longhorn volume
type LonghornVolume struct {
	Name         string
	Namespace    string
	SizeBytes    int64
	PVCName      string
	StorageClass string
	AccessMode   string
	Replicas     int
}

// MigrationPlan describes what will be created
type MigrationPlan struct {
	Volumes      []LonghornVolume
	TargetTenant string
	DryRun       bool
}

// MigrationResult describes the outcome
type MigrationResult struct {
	Created []string
	Skipped []string
	Failed  []string
	Errors  map[string]error
}

// PVCItem represents a Kubernetes PVC from kubectl output
type PVCItem struct {
	Metadata struct {
		Name      string `json:"name"`
		Namespace string `json:"namespace"`
	} `json:"metadata"`
	Spec struct {
		StorageClassName string `json:"storageClassName"`
		AccessModes      []string `json:"accessModes"`
		Resources        struct {
			Requests struct {
				Storage string `json:"storage"`
			} `json:"requests"`
		} `json:"resources"`
	} `json:"spec"`
}

// PVCList represents the kubectl output structure
type PVCList struct {
	Items []PVCItem `json:"items"`
}

// DiscoverLonghornVolumes discovers Longhorn PVCs in the cluster.
// Uses kubectl to fetch PVCs and filters for storage class containing "longhorn".
func DiscoverLonghornVolumes(ctx context.Context, namespace string) ([]LonghornVolume, error) {
	var volumes []LonghornVolume

	// Build kubectl command
	cmd := exec.CommandContext(ctx, "kubectl", "get", "pvc", "-o", "json")
	if namespace != "" {
		cmd = exec.CommandContext(ctx, "kubectl", "get", "pvc", "-n", namespace, "-o", "json")
	} else {
		cmd = exec.CommandContext(ctx, "kubectl", "get", "pvc", "--all-namespaces", "-o", "json")
	}

	// Execute kubectl
	output, err := cmd.Output()
	if err != nil {
		// Graceful degradation: if kubectl is not available or fails, return empty slice with warning
		if _, ok := err.(*exec.ExitError); ok {
			fmt.Fprintf(os.Stderr, "Warning: kubectl command failed: %v\n", err)
			return volumes, nil
		}
		// Check if kubectl executable was not found
		errStr := err.Error()
		if strings.Contains(errStr, "executable file not found") || strings.Contains(errStr, "no such file or directory") {
			fmt.Fprintf(os.Stderr, "Warning: kubectl not found in PATH\n")
			return volumes, nil
		}
		return volumes, fmt.Errorf("failed to execute kubectl: %w", err)
	}

	// Parse JSON output
	var pvcList PVCList
	if err := json.Unmarshal(output, &pvcList); err != nil {
		return volumes, fmt.Errorf("failed to parse kubectl output: %w", err)
	}

	// Filter for Longhorn volumes
	for _, pvc := range pvcList.Items {
		if pvc.Spec.StorageClassName == "" {
			continue
		}

		if !strings.Contains(strings.ToLower(pvc.Spec.StorageClassName), "longhorn") {
			continue
		}

		// Parse storage size
		sizeStr := strings.TrimSpace(pvc.Spec.Resources.Requests.Storage)
		sizeBytes := parseSizeBytes(sizeStr)

		// Determine access mode
		accessMode := "ReadWriteOnce"
		if len(pvc.Spec.AccessModes) > 0 {
			accessMode = pvc.Spec.AccessModes[0]
		}

		vol := LonghornVolume{
			Name:         pvc.Metadata.Name,
			Namespace:    pvc.Metadata.Namespace,
			SizeBytes:    sizeBytes,
			PVCName:      pvc.Metadata.Name,
			StorageClass: pvc.Spec.StorageClassName,
			AccessMode:   accessMode,
			Replicas:     3, // Default Longhorn replica count
		}

		volumes = append(volumes, vol)
	}

	return volumes, nil
}

// parseSizeBytes converts Kubernetes storage size strings (e.g., "10Gi") to bytes
func parseSizeBytes(s string) int64 {
	s = strings.TrimSpace(s)

	// Remove the suffix and parse the numeric part
	var multiplier int64 = 1
	if strings.HasSuffix(s, "Gi") {
		multiplier = 1024 * 1024 * 1024
		s = strings.TrimSuffix(s, "Gi")
	} else if strings.HasSuffix(s, "G") {
		multiplier = 1000 * 1000 * 1000
		s = strings.TrimSuffix(s, "G")
	} else if strings.HasSuffix(s, "Mi") {
		multiplier = 1024 * 1024
		s = strings.TrimSuffix(s, "Mi")
	} else if strings.HasSuffix(s, "M") {
		multiplier = 1000 * 1000
		s = strings.TrimSuffix(s, "M")
	} else if strings.HasSuffix(s, "Ki") {
		multiplier = 1024
		s = strings.TrimSuffix(s, "Ki")
	} else if strings.HasSuffix(s, "K") {
		multiplier = 1000
		s = strings.TrimSuffix(s, "K")
	}

	val, err := strconv.ParseInt(strings.TrimSpace(s), 10, 64)
	if err != nil {
		return 0
	}

	return val * multiplier
}

// PlanMigration creates a migration plan from discovered volumes
func PlanMigration(volumes []LonghornVolume, tenant string, dryRun bool) *MigrationPlan {
	return &MigrationPlan{
		Volumes:      volumes,
		TargetTenant: tenant,
		DryRun:       dryRun,
	}
}

// ExecuteMigration executes the plan, creating Nest DataResource YAML manifests.
// Does NOT apply them — writes YAML files to outputDir.
func ExecuteMigration(ctx context.Context, plan *MigrationPlan, outputDir string) (*MigrationResult, error) {
	result := &MigrationResult{
		Created: []string{},
		Skipped: []string{},
		Failed:  []string{},
		Errors:  make(map[string]error),
	}

	if plan.DryRun {
		fmt.Println("DRY RUN: Would create the following DataResources:")
		for _, vol := range plan.Volumes {
			fmt.Printf("  - %s/%s (from PVC: %s, size: %d bytes)\n",
				vol.Namespace, vol.Name, vol.PVCName, vol.SizeBytes)
		}
		return result, nil
	}

	// Create output directory if it doesn't exist
	if err := os.MkdirAll(outputDir, 0755); err != nil {
		return result, fmt.Errorf("failed to create output directory: %w", err)
	}

	// Generate YAML for each volume
	for _, vol := range plan.Volumes {
		yaml := volumeToDataResourceYAML(vol, plan.TargetTenant)

		// Write to file
		filename := filepath.Join(outputDir, fmt.Sprintf("%s-dataresource.yaml", vol.Name))
		if err := os.WriteFile(filename, []byte(yaml), 0644); err != nil {
			result.Failed = append(result.Failed, vol.Name)
			result.Errors[vol.Name] = fmt.Errorf("failed to write YAML: %w", err)
		} else {
			result.Created = append(result.Created, filename)
		}
	}

	return result, nil
}

// volumeToDataResourceYAML converts a LonghornVolume to a Nest DataResource YAML string
func volumeToDataResourceYAML(v LonghornVolume, tenant string) string {
	sizeGi := v.SizeBytes / (1024 * 1024 * 1024)
	if v.SizeBytes%0x40000000 != 0 {
		sizeGi++ // Round up
	}

	tmpl := `apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: {{ .Name }}
  namespace: {{ .Namespace }}
  annotations:
    nest.penguintech.io/migrated-from: longhorn
    nest.penguintech.io/original-pvc: {{ .PVCName }}
spec:
  type: pvc/block
  tenant: {{ .Tenant }}
  origination: managed
  size:
    storage: {{ .SizeGi }}Gi
`

	t, err := template.New("dataresource").Parse(tmpl)
	if err != nil {
		// Fallback to manual string formatting if template fails
		return fmt.Sprintf(`apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: %s
  namespace: %s
  annotations:
    nest.penguintech.io/migrated-from: longhorn
    nest.penguintech.io/original-pvc: %s
spec:
  type: pvc/block
  tenant: %s
  origination: managed
  size:
    storage: %dGi
`, v.Name, v.Namespace, v.PVCName, tenant, sizeGi)
	}

	data := struct {
		Name    string
		Namespace string
		PVCName string
		Tenant  string
		SizeGi  int64
	}{
		Name:      v.Name,
		Namespace: v.Namespace,
		PVCName:   v.PVCName,
		Tenant:    tenant,
		SizeGi:    sizeGi,
	}

	var buf strings.Builder
	if err := t.Execute(&buf, data); err != nil {
		// Fallback if template execution fails
		return fmt.Sprintf(`apiVersion: nest.penguintech.io/v1
kind: DataResource
metadata:
  name: %s
  namespace: %s
  annotations:
    nest.penguintech.io/migrated-from: longhorn
    nest.penguintech.io/original-pvc: %s
spec:
  type: pvc/block
  tenant: %s
  origination: managed
  size:
    storage: %dGi
`, v.Name, v.Namespace, v.PVCName, tenant, sizeGi)
	}

	return buf.String()
}
