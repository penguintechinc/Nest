package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestStorageClassRewriting_NestBlock(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	pvcMeta := PVCMeta{}
	pvcMeta.Metadata.Name = "test-pvc"
	pvcMeta.Metadata.Namespace = "default"
	pvcMeta.Metadata.Labels = map[string]string{}
	pvcMeta.Spec.StorageClassName = "nest-block"

	pvcBytes, _ := json.Marshal(pvcMeta)

	review := AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Request: &AdmissionRequest{
			UID:    "test-uid-nestblock",
			Object: pvcBytes,
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate-pvc", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePVC(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var respReview AdmissionReview
	json.NewDecoder(w.Body).Decode(&respReview)

	if !respReview.Response.Allowed {
		t.Errorf("expected allowed=true")
	}

	if len(respReview.Response.Patch) == 0 {
		t.Errorf("expected patches to be generated")
	}

	var patches []JSONPatch
	json.Unmarshal(respReview.Response.Patch, &patches)

	// Find storage class replacement patch
	foundReplace := false
	for _, p := range patches {
		if p.Op == "replace" && strings.Contains(p.Path, "storageClassName") {
			foundReplace = true
			if v, ok := p.Value.(string); !ok || v != "nest-block" {
				t.Errorf("expected storage class to be rewritten to nest-block, got %v", p.Value)
			}
		}
	}
	if !foundReplace {
		t.Errorf("expected to find storageClassName replace patch")
	}
}

func TestStorageClassRewriting_NestFilesystem(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	pvcMeta := PVCMeta{}
	pvcMeta.Metadata.Name = "test-pvc-fs"
	pvcMeta.Metadata.Namespace = "default"
	pvcMeta.Metadata.Labels = map[string]string{}
	pvcMeta.Spec.StorageClassName = "nest-filesystem"

	pvcBytes, _ := json.Marshal(pvcMeta)

	review := AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Request: &AdmissionRequest{
			UID:    "test-uid-nestfs",
			Object: pvcBytes,
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate-pvc", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePVC(w, req)

	var respReview AdmissionReview
	json.NewDecoder(w.Body).Decode(&respReview)

	if !respReview.Response.Allowed {
		t.Errorf("expected allowed=true")
	}

	var patches []JSONPatch
	json.Unmarshal(respReview.Response.Patch, &patches)

	foundReplace := false
	for _, p := range patches {
		if p.Op == "replace" && strings.Contains(p.Path, "storageClassName") {
			foundReplace = true
			if v, ok := p.Value.(string); !ok || v != "nest-fs" {
				t.Errorf("expected storage class to be rewritten to nest-fs, got %v", p.Value)
			}
		}
	}
	if !foundReplace {
		t.Errorf("expected to find storageClassName replace patch for nest-filesystem")
	}
}

func TestStorageClassRewriting_NonNest(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	pvcMeta := PVCMeta{}
	pvcMeta.Metadata.Name = "test-pvc-standard"
	pvcMeta.Metadata.Namespace = "default"
	pvcMeta.Metadata.Labels = map[string]string{}
	pvcMeta.Spec.StorageClassName = "standard"

	pvcBytes, _ := json.Marshal(pvcMeta)

	review := AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Request: &AdmissionRequest{
			UID:    "test-uid-standard",
			Object: pvcBytes,
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate-pvc", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePVC(w, req)

	var respReview AdmissionReview
	json.NewDecoder(w.Body).Decode(&respReview)

	if !respReview.Response.Allowed {
		t.Errorf("expected allowed=true")
	}

	var patches []JSONPatch
	json.Unmarshal(respReview.Response.Patch, &patches)

	// Verify no replace patches for standard storage class
	for _, p := range patches {
		if p.Op == "replace" && strings.Contains(p.Path, "storageClassName") {
			t.Errorf("expected no storage class replacement for non-Nest class")
		}
	}
}

func TestStorageClassRewriting_ManagedLabel(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	pvcMeta := PVCMeta{}
	pvcMeta.Metadata.Name = "test-pvc-label"
	pvcMeta.Metadata.Namespace = "default"
	pvcMeta.Metadata.Labels = map[string]string{}
	pvcMeta.Spec.StorageClassName = "nest-block"

	pvcBytes, _ := json.Marshal(pvcMeta)

	review := AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Request: &AdmissionRequest{
			UID:    "test-uid-label",
			Object: pvcBytes,
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate-pvc", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePVC(w, req)

	var respReview AdmissionReview
	json.NewDecoder(w.Body).Decode(&respReview)

	var patches []JSONPatch
	json.Unmarshal(respReview.Response.Patch, &patches)

	// Find managed label patch
	foundManagedLabel := false
	for _, p := range patches {
		if p.Op == "add" && strings.Contains(p.Path, "managed") {
			foundManagedLabel = true
			if v, ok := p.Value.(string); !ok || v != "true" {
				t.Errorf("expected managed label to be 'true', got %v", p.Value)
			}
		}
	}
	if !foundManagedLabel {
		t.Errorf("expected to find managed label patch")
	}
}

func TestStorageClassRewriting_NoStorageClass(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	pvcMeta := PVCMeta{}
	pvcMeta.Metadata.Name = "test-pvc-no-sc"
	pvcMeta.Metadata.Namespace = "default"
	pvcMeta.Metadata.Labels = map[string]string{}
	pvcMeta.Spec.StorageClassName = ""

	pvcBytes, _ := json.Marshal(pvcMeta)

	review := AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Request: &AdmissionRequest{
			UID:    "test-uid-no-sc",
			Object: pvcBytes,
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate-pvc", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePVC(w, req)

	var respReview AdmissionReview
	json.NewDecoder(w.Body).Decode(&respReview)

	if !respReview.Response.Allowed {
		t.Errorf("expected allowed=true even without storageClassName")
	}

	var patches []JSONPatch
	json.Unmarshal(respReview.Response.Patch, &patches)

	// Should only have label patches, no storage class replacement
	for _, p := range patches {
		if p.Op == "replace" && strings.Contains(p.Path, "storageClassName") {
			t.Errorf("expected no storage class replacement when SC is empty")
		}
	}
}

func TestStorageClassRewriting_AllMappings(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	testCases := []struct {
		input    string
		expected string
	}{
		{"nest-block", "nest-block"},
		{"nest-filesystem", "nest-fs"},
		{"nest-file", "nest-fs-rwo"},
		{"nest-bucket", "nest-bucket"},
	}

	for _, tc := range testCases {
		pvcMeta := PVCMeta{}
		pvcMeta.Metadata.Name = "test-pvc-" + tc.input
		pvcMeta.Metadata.Namespace = "default"
		pvcMeta.Metadata.Labels = map[string]string{}
		pvcMeta.Spec.StorageClassName = tc.input

		pvcBytes, _ := json.Marshal(pvcMeta)

		review := AdmissionReview{
			APIVersion: "admission.k8s.io/v1",
			Kind:       "AdmissionReview",
			Request: &AdmissionRequest{
				UID:    "test-uid-" + tc.input,
				Object: pvcBytes,
			},
		}

		body, _ := json.Marshal(review)
		req, _ := http.NewRequest("POST", "/mutate-pvc", bytes.NewReader(body))
		w := httptest.NewRecorder()

		handler.MutatePVC(w, req)

		var respReview AdmissionReview
		json.NewDecoder(w.Body).Decode(&respReview)

		var patches []JSONPatch
		json.Unmarshal(respReview.Response.Patch, &patches)

		foundReplace := false
		for _, p := range patches {
			if p.Op == "replace" && strings.Contains(p.Path, "storageClassName") {
				foundReplace = true
				if v, ok := p.Value.(string); !ok || v != tc.expected {
					t.Errorf("for %s: expected %s, got %v", tc.input, tc.expected, p.Value)
				}
			}
		}
		if !foundReplace {
			t.Errorf("expected replace patch for %s", tc.input)
		}
	}
}

func TestMutatePVCMethodNotAllowed(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	req, _ := http.NewRequest("GET", "/mutate-pvc", nil)
	w := httptest.NewRecorder()

	handler.MutatePVC(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestMutatePVCInvalidJSON(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	req, _ := http.NewRequest("POST", "/mutate-pvc", bytes.NewReader([]byte("invalid json")))
	w := httptest.NewRecorder()

	handler.MutatePVC(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestMutatePVCResponseContentType(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	pvcMeta := PVCMeta{}
	pvcMeta.Metadata.Name = "test"
	pvcMeta.Metadata.Namespace = "default"
	pvcBytes, _ := json.Marshal(pvcMeta)

	review := AdmissionReview{
		Request: &AdmissionRequest{
			UID:    "test-uid",
			Object: pvcBytes,
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate-pvc", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePVC(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}
}

func TestEscapeJSONPointer(t *testing.T) {
	testCases := []struct {
		input    string
		expected string
	}{
		{"simple", "simple"},
		{"with/slash", "with~1slash"},
		{"with~tilde", "with~0tilde"},
		{"both/and~mixed", "both~1and~0mixed"},
	}

	for _, tc := range testCases {
		result := escapeJSONPointer(tc.input)
		if result != tc.expected {
			t.Errorf("escapeJSONPointer(%q): expected %q, got %q", tc.input, tc.expected, result)
		}
	}
}

func TestBuildPVCPatches_NilLabels(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	pvcMeta := PVCMeta{}
	pvcMeta.Metadata.Name = "test-pvc"
	pvcMeta.Metadata.Namespace = "default"
	pvcMeta.Metadata.Labels = nil
	pvcMeta.Spec.StorageClassName = "nest-block"

	pvcBytes, _ := json.Marshal(pvcMeta)
	req := &AdmissionRequest{
		UID:    "test-uid",
		Object: pvcBytes,
	}

	patches, err := handler.buildPVCPatches(req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(patches) < 2 {
		t.Errorf("expected at least 2 patches (labels add + managed label), got %d", len(patches))
	}

	// Should have add patch for labels creation
	foundLabelAdd := false
	for _, p := range patches {
		if p.Op == "add" && strings.Contains(p.Path, "/labels") && !strings.Contains(p.Path, "managed") {
			foundLabelAdd = true
		}
	}
	if !foundLabelAdd {
		t.Errorf("expected labels add patch")
	}
}

func TestBuildPVCPatches_InvalidPVC(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	req := &AdmissionRequest{
		UID:    "test-uid",
		Object: []byte("invalid pvc structure"),
	}

	_, err := handler.buildPVCPatches(req)
	if err == nil {
		t.Errorf("expected error for invalid PVC")
	}
}
