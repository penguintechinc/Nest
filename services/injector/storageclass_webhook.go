package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"go.uber.org/zap"
)

// PVCMeta holds the minimal PVC metadata we need for storage class rewriting.
type PVCMeta struct {
	Metadata struct {
		Name      string            `json:"name"`
		Namespace string            `json:"namespace"`
		Labels    map[string]string `json:"labels"`
	} `json:"metadata"`
	Spec struct {
		StorageClassName string `json:"storageClassName"`
	} `json:"spec"`
}

// StorageClassMapping defines how Nest storage classes map to Rook-Ceph equivalents.
var StorageClassMapping = map[string]string{
	"nest-block":      "rook-ceph-block",
	"nest-filesystem": "rook-cephfs",
	"nest-file":       "rook-cephfs-rwo",
	"nest-bucket":     "rook-ceph-bucket",
}

const (
	labelManagedKey = "nest.penguintech.io/managed"
	labelTenantKey  = "nest.penguintech.io/tenant"
	annotationTenantKey = "nest.penguintech.io/tenant"
)

// MutatePVC handles mutating admission webhook requests for PVCs.
// Rewrites Nest-branded storage classes to Rook-Ceph equivalents and injects labels.
func (h *WebhookHandler) MutatePVC(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var review AdmissionReview
	if err := json.NewDecoder(r.Body).Decode(&review); err != nil {
		h.logger.Error("decode admission review", zap.Error(err))
		http.Error(w, "invalid request", http.StatusBadRequest)
		return
	}

	response := &AdmissionResponse{
		UID:     review.Request.UID,
		Allowed: true,
	}

	patches, err := h.buildPVCPatches(review.Request)
	if err != nil {
		h.logger.Error("build pvc patches", zap.Error(err))
	} else if len(patches) > 0 {
		patchBytes, _ := json.Marshal(patches)
		response.Patch = patchBytes
		pt := patchTypeJSON
		response.PatchType = &pt
		h.logger.Info("pvc storage class patches", zap.String("uid", review.Request.UID), zap.Int("patches", len(patches)))
	}

	review.Response = response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(review)
}

// buildPVCPatches builds JSON patches for PVC storage class rewriting and label injection.
func (h *WebhookHandler) buildPVCPatches(req *AdmissionRequest) ([]JSONPatch, error) {
	var pvc PVCMeta
	if err := json.Unmarshal(req.Object, &pvc); err != nil {
		return nil, fmt.Errorf("unmarshal pvc: %w", err)
	}

	patches := []JSONPatch{}

	// Rewrite storage class if it matches Nest-branded naming
	if pvc.Spec.StorageClassName != "" {
		if newSC, ok := StorageClassMapping[pvc.Spec.StorageClassName]; ok {
			patches = append(patches, JSONPatch{
				Op:    "replace",
				Path:  "/spec/storageClassName",
				Value: newSC,
			})
			h.logger.Info("rewriting storage class",
				zap.String("namespace", pvc.Metadata.Namespace),
				zap.String("pvc", pvc.Metadata.Name),
				zap.String("old", pvc.Spec.StorageClassName),
				zap.String("new", newSC),
			)
		}
	}

	// Ensure metadata.labels exists
	if pvc.Metadata.Labels == nil {
		patches = append(patches, JSONPatch{
			Op:    "add",
			Path:  "/metadata/labels",
			Value: map[string]string{},
		})
	}

	// Inject managed label
	patches = append(patches, JSONPatch{
		Op:    "add",
		Path:  fmt.Sprintf("/metadata/labels/%s", escapeJSONPointer(labelManagedKey)),
		Value: "true",
	})

	// Fetch namespace annotations to check for tenant label injection
	// Note: In a real implementation, we'd fetch the Namespace object to get its annotations.
	// For now, we assume this would be done via a downward API or direct namespace lookup.
	// Placeholder: if namespace annotation exists, inject tenant label.
	// This is a simplified version; production code would query the API server.

	return patches, nil
}

// escapeJSONPointer escapes a string for use in JSON Pointer paths (RFC 6901).
func escapeJSONPointer(s string) string {
	s = strings.ReplaceAll(s, "~", "~0")
	s = strings.ReplaceAll(s, "/", "~1")
	return s
}
