package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"

	"go.uber.org/zap"
)

// AdmissionReview is the Kubernetes admission webhook request/response envelope.
type AdmissionReview struct {
	APIVersion string             `json:"apiVersion"`
	Kind       string             `json:"kind"`
	Request    *AdmissionRequest  `json:"request,omitempty"`
	Response   *AdmissionResponse `json:"response,omitempty"`
}

type AdmissionRequest struct {
	UID    string          `json:"uid"`
	Object json.RawMessage `json:"object"`
	// UserInfo carries the identity of the requestor
	UserInfo *UserInfo `json:"userInfo,omitempty"`
}

// UserInfo contains the authenticated user's identity and group memberships
type UserInfo struct {
	Username string   `json:"username,omitempty"`
	Groups   []string `json:"groups,omitempty"`
}

// HasGroup checks if the userInfo contains a specific group
func (u *UserInfo) HasGroup(group string) bool {
	if u == nil {
		return false
	}
	for _, g := range u.Groups {
		if g == group {
			return true
		}
	}
	return false
}

type AdmissionResponse struct {
	UID       string  `json:"uid"`
	Allowed   bool    `json:"allowed"`
	Patch     []byte  `json:"patch,omitempty"`
	PatchType *string `json:"patchType,omitempty"`
}

// PodMeta holds the minimal Pod metadata we need for injection decisions.
type PodMeta struct {
	Metadata struct {
		Annotations map[string]string `json:"annotations"`
		Labels      map[string]string `json:"labels"`
		Namespace   string            `json:"namespace"`
	} `json:"metadata"`
}

// WebhookHandler handles mutating admission webhook requests.
type WebhookHandler struct {
	logger       *zap.Logger
	nestEndpoint string
}

func NewWebhookHandler(logger *zap.Logger) *WebhookHandler {
	return &WebhookHandler{
		logger:       logger,
		nestEndpoint: os.Getenv("NEST_ENDPOINT"),
	}
}

const (
	annotationInjectSDK   = "nest.penguintech.io/inject-sdk"
	annotationCredentials = "nest.penguintech.io/credentials"
	patchTypeJSON         = "JSONPatch"
)

func (h *WebhookHandler) MutatePod(w http.ResponseWriter, r *http.Request) {
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
		Allowed: true, // always allow; we only add patches
	}

	patches, err := h.buildPatches(review.Request)
	if err != nil {
		h.logger.Error("build patches", zap.Error(err))
	} else if len(patches) > 0 {
		patchBytes, _ := json.Marshal(patches)
		response.Patch = patchBytes
		pt := patchTypeJSON
		response.PatchType = &pt
		h.logger.Info("injecting patches", zap.String("uid", review.Request.UID), zap.Int("patches", len(patches)))
	}

	review.Response = response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(review)
}

// JSONPatch represents a single RFC6902 patch operation.
type JSONPatch struct {
	Op    string      `json:"op"`
	Path  string      `json:"path"`
	Value interface{} `json:"value,omitempty"`
}

func (h *WebhookHandler) buildPatches(req *AdmissionRequest) ([]JSONPatch, error) {
	var pod PodMeta
	if err := json.Unmarshal(req.Object, &pod); err != nil {
		return nil, fmt.Errorf("unmarshal pod: %w", err)
	}

	annotations := pod.Metadata.Annotations
	patches := []JSONPatch{}

	// Check inject-sdk annotation
	if injectSDK := annotations[annotationInjectSDK]; injectSDK == "true" {
		endpoint := h.nestEndpoint
		if endpoint == "" {
			endpoint = "http://nest-gateway.nest.svc.cluster.local:8080"
		}

		// Add NEST_ENDPOINT env var to all containers via env patch
		// (In production, this would iterate existing containers and add env entries)
		patches = append(patches, JSONPatch{
			Op:    "add",
			Path:  "/metadata/annotations/nest.penguintech.io~1sdk-injected",
			Value: "true",
		})

		h.logger.Info("sdk injection requested",
			zap.String("namespace", pod.Metadata.Namespace),
			zap.String("endpoint", endpoint),
		)
	}

	// Check credentials annotation
	if creds := annotations[annotationCredentials]; creds != "" {
		rawNames := strings.Split(creds, ",")
		trimmed := make([]string, 0, len(rawNames))
		for _, cred := range rawNames {
			cred = strings.TrimSpace(cred)
			if cred != "" {
				h.logger.Info("credential injection requested", zap.String("credential", cred))
				trimmed = append(trimmed, cred)
			}
		}

		patches = append(patches, JSONPatch{
			Op:    "add",
			Path:  "/metadata/annotations/nest.penguintech.io~1credentials-injected",
			Value: strings.Join(trimmed, ","),
		})
	}

	return patches, nil
}
