package main

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/rest"

	"go.uber.org/zap"
)

var nonAlnum = regexp.MustCompile(`[^a-z0-9]+`)

// CRClient abstracts the k8s dynamic client operations needed by CRPublisher,
// allowing the publisher to be tested without a real Kubernetes API server.
type CRClient interface {
	// Patch applies a server-side apply patch to the named resource.
	Patch(ctx context.Context, gvr schema.GroupVersionResource, name string, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error)
	// Create creates the given resource object.
	Create(ctx context.Context, gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.CreateOptions) (*unstructured.Unstructured, error)
}

// dynamicCRClient wraps k8s dynamic.Interface to satisfy CRClient.
type dynamicCRClient struct {
	client dynamic.Interface
}

func (d *dynamicCRClient) Patch(ctx context.Context, gvr schema.GroupVersionResource, name string, data []byte, opts metav1.PatchOptions) (*unstructured.Unstructured, error) {
	return d.client.Resource(gvr).Patch(ctx, name, types.ApplyPatchType, data, opts)
}

func (d *dynamicCRClient) Create(ctx context.Context, gvr schema.GroupVersionResource, obj *unstructured.Unstructured, opts metav1.CreateOptions) (*unstructured.Unstructured, error) {
	return d.client.Resource(gvr).Create(ctx, obj, opts)
}

// CRPublisher publishes HardwareInventory and DarkDrive CRs to Kubernetes.
type CRPublisher struct {
	client   CRClient
	nodeName string
	logger   *zap.Logger
}

var (
	hardwareInventoryGVR = schema.GroupVersionResource{
		Group:    "nest.penguintech.io",
		Version:  "v1",
		Resource: "hardwareinventories",
	}
	darkDriveGVR = schema.GroupVersionResource{
		Group:    "nest.penguintech.io",
		Version:  "v1",
		Resource: "darkdrives",
	}
)

// NewCRPublisher builds an in-cluster dynamic client. Returns nil, nil when not
// running inside a cluster (publisher is simply disabled).
func NewCRPublisher(nodeName string, logger *zap.Logger) (*CRPublisher, error) {
	cfg, err := rest.InClusterConfig()
	if err != nil {
		logger.Info("not running in-cluster, CR publishing disabled", zap.Error(err))
		return nil, nil
	}
	dynClient, err := dynamic.NewForConfig(cfg)
	if err != nil {
		return nil, err
	}
	return &CRPublisher{
		client:   &dynamicCRClient{client: dynClient},
		nodeName: nodeName,
		logger:   logger,
	}, nil
}

// newCRPublisherWithClient creates a CRPublisher with an injected CRClient — for testing only.
func newCRPublisherWithClient(nodeName string, logger *zap.Logger, client CRClient) *CRPublisher {
	return &CRPublisher{client: client, nodeName: nodeName, logger: logger}
}

// UpsertHardwareInventory creates or updates the HardwareInventory CR for this node
// using server-side apply.
func (p *CRPublisher) UpsertHardwareInventory(ctx context.Context, devices []*DeviceInfo) error {
	darkCount := 0
	for _, d := range devices {
		if d.State == "Dark" {
			darkCount++
		}
	}

	name := sanitizeName(p.nodeName)
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "nest.penguintech.io/v1",
			"kind":       "HardwareInventory",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"node":    p.nodeName,
				"devices": devicesToUnstructured(devices),
			},
			"status": map[string]interface{}{
				"lastScanTime":   time.Now().UTC().Format(time.RFC3339),
				"darkDriveCount": int64(darkCount),
			},
		},
	}

	data, err := json.Marshal(obj)
	if err != nil {
		return err
	}

	_, err = p.client.Patch(ctx, hardwareInventoryGVR, name, data,
		metav1.PatchOptions{FieldManager: "nest-node-agent", Force: boolPtr(true)},
	)
	if err != nil {
		p.logger.Error("failed to upsert HardwareInventory", zap.String("name", name), zap.Error(err))
		return err
	}
	p.logger.Debug("HardwareInventory upserted", zap.String("name", name), zap.Int("devices", len(devices)))
	return nil
}

// EnsureDarkDriveCR creates a DarkDrive CR for the given device if it does not
// already exist (idempotent).
func (p *CRPublisher) EnsureDarkDriveCR(ctx context.Context, d *DeviceInfo) error {
	name := sanitizeName(p.nodeName + "-" + d.Name)
	obj := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "nest.penguintech.io/v1",
			"kind":       "DarkDrive",
			"metadata": map[string]interface{}{
				"name": name,
			},
			"spec": map[string]interface{}{
				"node":   p.nodeName,
				"device": d.Name,
				"serial": d.Serial,
				"size":   formatBytes(d.CapacityBytes),
				"class":  d.Class,
			},
			"status": map[string]interface{}{
				"state": "Discovered",
			},
		},
	}

	_, err := p.client.Create(ctx, darkDriveGVR, obj, metav1.CreateOptions{FieldManager: "nest-node-agent"})
	if err != nil {
		if k8serrors.IsAlreadyExists(err) {
			return nil
		}
		p.logger.Error("failed to create DarkDrive CR", zap.String("name", name), zap.Error(err))
		return err
	}
	p.logger.Info("DarkDrive CR created", zap.String("name", name), zap.String("device", d.Name))
	return nil
}

// formatBytes converts a byte count to a human-readable string (e.g. "1.92TB", "500GB").
func formatBytes(b int64) string {
	const tb = int64(1e12)
	const gb = int64(1e9)
	switch {
	case b >= tb:
		return fmt.Sprintf("%.2fTB", float64(b)/float64(tb))
	case b >= gb:
		return fmt.Sprintf("%.2fGB", float64(b)/float64(gb))
	default:
		return fmt.Sprintf("%dB", b)
	}
}

// sanitizeName converts an arbitrary string into a valid DNS subdomain name
// (lowercase alphanumeric + hyphens, max 63 chars).
func sanitizeName(s string) string {
	s = strings.ToLower(s)
	s = strings.TrimPrefix(s, "/dev/")
	s = nonAlnum.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if len(s) > 63 {
		s = s[:63]
		s = strings.TrimRight(s, "-")
	}
	return s
}

// devicesToUnstructured converts []*DeviceInfo to []interface{} suitable for
// embedding in an unstructured Kubernetes object.
func devicesToUnstructured(devices []*DeviceInfo) []interface{} {
	out := make([]interface{}, 0, len(devices))
	for _, d := range devices {
		m := map[string]interface{}{
			"name":          d.Name,
			"serial":        d.Serial,
			"model":         d.Model,
			"capacityBytes": d.CapacityBytes,
			"class":         d.Class,
			"state":         d.State,
		}
		if d.SMART != nil {
			m["smart"] = map[string]interface{}{
				"health":             d.SMART.Health,
				"wearPercent":        int64(d.SMART.WearPercent),
				"hoursOn":            d.SMART.HoursOn,
				"temperatureCelsius": int64(d.SMART.TemperatureCelsius),
				"reallocatedSectors": d.SMART.ReallocatedSectors,
			}
		}
		if d.Signature != "" {
			m["signature"] = d.Signature
		}
		out = append(out, m)
	}
	return out
}

func boolPtr(b bool) *bool { return &b }
