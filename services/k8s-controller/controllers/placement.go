package controllers

import (
	"context"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// PlacementEngine resolves hardware pool → node affinity for DataResource scheduling.
type PlacementEngine struct {
	client client.Client
}

// NewPlacementEngine creates a PlacementEngine using the given controller-runtime client.
func NewPlacementEngine(c client.Client) *PlacementEngine {
	return &PlacementEngine{client: c}
}

// NodeAffinity builds a *corev1.NodeAffinity for DataResources that require a specific
// hardware class. Returns nil if no HardwareInventory CRs exist with active devices in
// that class (no constraint applied).
func (p *PlacementEngine) NodeAffinity(ctx context.Context, hardwareClass string) (*corev1.NodeAffinity, error) {
	logger := log.FromContext(ctx)

	// List all HardwareInventory CRs
	inventoryList := &nestv1.HardwareInventoryList{}
	if err := p.client.List(ctx, inventoryList); err != nil {
		logger.Error(err, "failed to list HardwareInventory CRs")
		return nil, fmt.Errorf("list HardwareInventory: %w", err)
	}

	// Collect unique node names with active devices in the given hardware class
	nodeSet := make(map[string]struct{})
	for _, inventory := range inventoryList.Items {
		for _, device := range inventory.Spec.Devices {
			if device.Class == hardwareClass && device.State == nestv1.DeviceStateActive {
				nodeSet[inventory.Spec.Node] = struct{}{}
				break // Found at least one active device in this class on this node
			}
		}
	}

	// If no nodes found, return nil (no constraint)
	if len(nodeSet) == 0 {
		logger.Info("no nodes with active devices in hardware class", "hardwareClass", hardwareClass)
		return nil, nil
	}

	// Convert set to slice
	nodes := make([]string, 0, len(nodeSet))
	for node := range nodeSet {
		nodes = append(nodes, node)
	}

	// Build NodeAffinity with RequiredDuringScheduling constraint
	affinity := &corev1.NodeAffinity{
		RequiredDuringSchedulingIgnoredDuringExecution: &corev1.NodeSelector{
			NodeSelectorTerms: []corev1.NodeSelectorTerm{
				{
					MatchExpressions: []corev1.NodeSelectorRequirement{
						{
							Key:      "kubernetes.io/hostname",
							Operator: corev1.NodeSelectorOpIn,
							Values:   nodes,
						},
					},
				},
			},
		},
	}

	logger.Info("resolved hardware class to node affinity", "hardwareClass", hardwareClass, "nodeCount", len(nodes))
	return affinity, nil
}

// NodesWithCapacity returns the list of node names that have at least minFreeBytes
// available in the given hardware class. "free" is calculated as the total capacity
// of Active+Dark devices in that class, minus a 20% overhead factor for Ceph.
func (p *PlacementEngine) NodesWithCapacity(ctx context.Context, hardwareClass string, minFreeBytes int64) ([]string, error) {
	logger := log.FromContext(ctx)

	// List all HardwareInventory CRs
	inventoryList := &nestv1.HardwareInventoryList{}
	if err := p.client.List(ctx, inventoryList); err != nil {
		logger.Error(err, "failed to list HardwareInventory CRs")
		return nil, fmt.Errorf("list HardwareInventory: %w", err)
	}

	// Map of node name → total effective capacity for the hardware class
	nodeCapacity := make(map[string]int64)

	for _, inventory := range inventoryList.Items {
		totalCapacity := int64(0)
		for _, device := range inventory.Spec.Devices {
			// Include Active and Dark devices in capacity calculation
			if device.Class == hardwareClass && (device.State == nestv1.DeviceStateActive || device.State == nestv1.DeviceStateDark) {
				totalCapacity += device.CapacityBytes
			}
		}

		// Apply 80% factor (20% Ceph overhead)
		effectiveCapacity := int64(float64(totalCapacity) * 0.8)
		if effectiveCapacity > 0 {
			nodeCapacity[inventory.Spec.Node] = effectiveCapacity
		}
	}

	// Collect nodes that meet the minimum capacity requirement
	var result []string
	for node, capacity := range nodeCapacity {
		if capacity >= minFreeBytes {
			result = append(result, node)
		}
	}

	logger.Info("resolved nodes with capacity", "hardwareClass", hardwareClass, "minFreeBytes", minFreeBytes, "nodeCount", len(result))
	return result, nil
}

// PoolSummary returns total and free capacity statistics for each hardware class
// across all nodes. Used for pre-flight checks before provisioning.
func (p *PlacementEngine) PoolSummary(ctx context.Context) (map[string]PoolStats, error) {
	logger := log.FromContext(ctx)

	// List all HardwareInventory CRs
	inventoryList := &nestv1.HardwareInventoryList{}
	if err := p.client.List(ctx, inventoryList); err != nil {
		logger.Error(err, "failed to list HardwareInventory CRs")
		return nil, fmt.Errorf("list HardwareInventory: %w", err)
	}

	// Map of hardware class → PoolStats
	summary := make(map[string]PoolStats)

	for _, inventory := range inventoryList.Items {
		for _, device := range inventory.Spec.Devices {
			hwClass := device.Class
			if hwClass == "" {
				continue // Skip devices without a class
			}

			stats := summary[hwClass]

			// Accumulate total capacity
			stats.TotalBytes += device.CapacityBytes

			// Count devices by state
			switch device.State {
			case nestv1.DeviceStateActive:
				stats.UsedBytes += device.CapacityBytes
				stats.ActiveDevices++
			case nestv1.DeviceStateDark:
				stats.DarkDevices++
			case nestv1.DeviceStateFailed:
				stats.FailedDevices++
			}

			summary[hwClass] = stats
		}
	}

	logger.Info("computed pool summary", "classCount", len(summary))
	return summary, nil
}

// HardwareClassFromDataResourceClass extracts the hardware class annotation
// from a DataResource. Checks the annotation "nest.penguintech.io/hardware-class";
// returns empty string if absent.
func HardwareClassFromDataResourceClass(dr *nestv1.DataResource) string {
	if dr == nil || dr.Spec.Annotations == nil {
		return ""
	}
	return dr.Spec.Annotations["nest.penguintech.io/hardware-class"]
}

// PoolStats holds capacity and device counts for a hardware pool.
type PoolStats struct {
	TotalBytes    int64
	UsedBytes     int64
	ActiveDevices int
	DarkDevices   int
	FailedDevices int
}
