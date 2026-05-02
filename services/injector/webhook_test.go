package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestNewWebhookHandler(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	if handler.logger != logger {
		t.Errorf("logger not set correctly")
	}
	if handler.nestEndpoint != "" {
		t.Errorf("expected empty endpoint initially, got %s", handler.nestEndpoint)
	}
}

func TestNewWebhookHandlerWithEnv(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	os.Setenv("NEST_ENDPOINT", "http://test-endpoint:8080")
	defer os.Unsetenv("NEST_ENDPOINT")

	handler := NewWebhookHandler(logger)
	if handler.nestEndpoint != "http://test-endpoint:8080" {
		t.Errorf("expected endpoint from env, got %s", handler.nestEndpoint)
	}
}

func TestMutatePodWithInjectSDKAnnotation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Annotations = map[string]string{
		"nest.penguintech.io/inject-sdk": "true",
	}
	podMeta.Metadata.Namespace = "default"

	podBytes, _ := json.Marshal(podMeta)

	review := AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Request: &AdmissionRequest{
			UID:    "test-uid-123",
			Object: podBytes,
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePod(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	var respReview AdmissionReview
	json.NewDecoder(w.Body).Decode(&respReview)

	if !respReview.Response.Allowed {
		t.Errorf("expected allowed=true")
	}
	if respReview.Response.UID != "test-uid-123" {
		t.Errorf("expected UID test-uid-123, got %s", respReview.Response.UID)
	}
	if len(respReview.Response.Patch) == 0 {
		t.Errorf("expected patches to be generated")
	}
	if respReview.Response.PatchType == nil || *respReview.Response.PatchType != "JSONPatch" {
		t.Errorf("expected PatchType JSONPatch")
	}

	var patches []JSONPatch
	json.Unmarshal(respReview.Response.Patch, &patches)
	if len(patches) < 1 {
		t.Errorf("expected at least 1 patch")
	}

	// Verify patch structure
	foundSDKPatch := false
	for _, p := range patches {
		if p.Op == "add" && strings.Contains(p.Path, "sdk-injected") {
			foundSDKPatch = true
			if v, ok := p.Value.(string); !ok || v != "true" {
				t.Errorf("expected sdk-injected patch value to be 'true'")
			}
		}
	}
	if !foundSDKPatch {
		t.Errorf("expected to find sdk-injected patch")
	}
}

func TestMutatePodWithoutInjectSDKAnnotation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Annotations = map[string]string{
		"some-other": "annotation",
	}
	podMeta.Metadata.Namespace = "default"

	podBytes, _ := json.Marshal(podMeta)

	review := AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Request: &AdmissionRequest{
			UID:    "test-uid-456",
			Object: podBytes,
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePod(w, req)

	var respReview AdmissionReview
	json.NewDecoder(w.Body).Decode(&respReview)

	if respReview.Response.Allowed != true {
		t.Errorf("expected allowed=true")
	}
	if len(respReview.Response.Patch) > 0 {
		// Patches may be empty or minimal
		var patches []JSONPatch
		json.Unmarshal(respReview.Response.Patch, &patches)
		// Should have no inject-sdk patches
		for _, p := range patches {
			if strings.Contains(p.Path, "sdk-injected") {
				t.Errorf("expected no sdk-injected patch when annotation not present")
			}
		}
	}
}

func TestMutatePodWithCredentialsAnnotation(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Annotations = map[string]string{
		"nest.penguintech.io/credentials": "secret-one, secret-two",
	}
	podMeta.Metadata.Namespace = "default"

	podBytes, _ := json.Marshal(podMeta)

	review := AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Request: &AdmissionRequest{
			UID:    "test-uid-789",
			Object: podBytes,
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePod(w, req)

	var respReview AdmissionReview
	json.NewDecoder(w.Body).Decode(&respReview)

	if !respReview.Response.Allowed {
		t.Errorf("expected allowed=true")
	}

	if len(respReview.Response.Patch) == 0 {
		t.Errorf("expected patches for credentials")
	}

	var patches []JSONPatch
	json.Unmarshal(respReview.Response.Patch, &patches)

	foundCredsPath := false
	for _, p := range patches {
		if p.Op == "add" && strings.Contains(p.Path, "credentials-injected") {
			foundCredsPath = true
			v, ok := p.Value.(string)
			if !ok {
				t.Errorf("expected patch value to be string")
			}
			// Should contain sanitized credential names
			if !strings.Contains(v, "secret-one") && !strings.Contains(v, "secret-two") {
				t.Errorf("expected credentials in patch, got %s", v)
			}
		}
	}
	if !foundCredsPath {
		t.Errorf("expected credentials-injected patch")
	}
}

func TestMutatePodWithBothAnnotations(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Annotations = map[string]string{
		"nest.penguintech.io/inject-sdk":   "true",
		"nest.penguintech.io/credentials":  "cred1",
	}
	podMeta.Metadata.Namespace = "prod"

	podBytes, _ := json.Marshal(podMeta)

	review := AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Request: &AdmissionRequest{
			UID:    "test-uid-both",
			Object: podBytes,
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePod(w, req)

	var respReview AdmissionReview
	json.NewDecoder(w.Body).Decode(&respReview)

	if len(respReview.Response.Patch) < 2 {
		t.Errorf("expected at least 2 patches for both annotations")
	}

	var patches []JSONPatch
	json.Unmarshal(respReview.Response.Patch, &patches)

	foundSDK := false
	foundCreds := false
	for _, p := range patches {
		if strings.Contains(p.Path, "sdk-injected") {
			foundSDK = true
		}
		if strings.Contains(p.Path, "credentials-injected") {
			foundCreds = true
		}
	}

	if !foundSDK || !foundCreds {
		t.Errorf("expected both sdk and creds patches")
	}
}

func TestMutatePodMethodNotAllowed(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	req, _ := http.NewRequest("GET", "/mutate", nil)
	w := httptest.NewRecorder()

	handler.MutatePod(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405, got %d", w.Code)
	}
}

func TestMutatePodInvalidJSON(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	req, _ := http.NewRequest("POST", "/mutate", bytes.NewReader([]byte("invalid json")))
	w := httptest.NewRecorder()

	handler.MutatePod(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestMutatePodWithEmptyAnnotations(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Annotations = map[string]string{}
	podMeta.Metadata.Namespace = "default"

	podBytes, _ := json.Marshal(podMeta)

	review := AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Request: &AdmissionRequest{
			UID:    "test-uid-empty",
			Object: podBytes,
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePod(w, req)

	var respReview AdmissionReview
	json.NewDecoder(w.Body).Decode(&respReview)

	if !respReview.Response.Allowed {
		t.Errorf("expected allowed=true even with empty annotations")
	}
}

func TestMutatePodWithNilAnnotations(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Namespace = "default"
	// Annotations field is nil by default

	podBytes, _ := json.Marshal(podMeta)

	review := AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Request: &AdmissionRequest{
			UID:    "test-uid-nil",
			Object: podBytes,
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePod(w, req)

	var respReview AdmissionReview
	json.NewDecoder(w.Body).Decode(&respReview)

	if !respReview.Response.Allowed {
		t.Errorf("expected allowed=true with nil annotations")
	}
}

func TestMutatePodInvalidObjectJSON(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	review := AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Request: &AdmissionRequest{
			UID:    "test-uid-invalid",
			Object: json.RawMessage(`{"invalid": true, "not-a-pod": "yes"}`),
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePod(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}

	// Should still return response with allowed=true even if buildPatches fails
	var respReview AdmissionReview
	json.NewDecoder(w.Body).Decode(&respReview)
	if !respReview.Response.Allowed {
		t.Errorf("expected allowed=true on error")
	}
}

func TestBuildPatchesWithInjectSDK(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)
	os.Setenv("NEST_ENDPOINT", "http://custom-endpoint:9000")
	defer os.Unsetenv("NEST_ENDPOINT")

	podMeta := PodMeta{}
	podMeta.Metadata.Annotations = map[string]string{
		"nest.penguintech.io/inject-sdk": "true",
	}
	podMeta.Metadata.Namespace = "testing"

	podBytes, _ := json.Marshal(podMeta)
	req := &AdmissionRequest{
		UID:    "test-uid",
		Object: podBytes,
	}

	patches, err := handler.buildPatches(req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(patches) < 1 {
		t.Errorf("expected at least 1 patch")
	}

	// Verify JSONPatch structure
	for _, p := range patches {
		if p.Op != "add" {
			t.Errorf("expected op 'add', got %s", p.Op)
		}
		if p.Path == "" {
			t.Errorf("expected non-empty path")
		}
	}
}

func TestBuildPatchesWithCredentials(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Annotations = map[string]string{
		"nest.penguintech.io/credentials": "api-key, db-pass, token",
	}
	podMeta.Metadata.Namespace = "default"

	podBytes, _ := json.Marshal(podMeta)
	req := &AdmissionRequest{
		UID:    "test-uid",
		Object: podBytes,
	}

	patches, err := handler.buildPatches(req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(patches) < 1 {
		t.Errorf("expected at least 1 patch")
	}

	foundCredsPath := false
	for _, p := range patches {
		if p.Op == "add" && strings.Contains(p.Path, "credentials-injected") {
			foundCredsPath = true
			v, ok := p.Value.(string)
			if !ok {
				t.Errorf("expected value to be string")
			}
			// Check that credentials are space-trimmed
			if strings.Contains(v, "  ") {
				t.Errorf("expected spaces to be trimmed in credentials")
			}
		}
	}
	if !foundCredsPath {
		t.Errorf("expected credentials patch")
	}
}

func TestBuildPatchesNoAnnotations(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Namespace = "default"

	podBytes, _ := json.Marshal(podMeta)
	req := &AdmissionRequest{
		UID:    "test-uid",
		Object: podBytes,
	}

	patches, err := handler.buildPatches(req)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}

	if len(patches) > 0 {
		t.Errorf("expected no patches when no annotations present")
	}
}

func TestBuildPatchesInvalidPod(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	req := &AdmissionRequest{
		UID:    "test-uid",
		Object: []byte("invalid pod structure"),
	}

	_, err := handler.buildPatches(req)
	if err == nil {
		t.Errorf("expected error for invalid pod")
	}
}

func TestAdmissionReviewRoundTrip(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Annotations = map[string]string{
		"nest.penguintech.io/inject-sdk": "true",
	}
	podMeta.Metadata.Namespace = "default"

	podBytes, _ := json.Marshal(podMeta)

	originalReview := AdmissionReview{
		APIVersion: "admission.k8s.io/v1",
		Kind:       "AdmissionReview",
		Request: &AdmissionRequest{
			UID:    "roundtrip-uid",
			Object: podBytes,
		},
	}

	body, _ := json.Marshal(originalReview)
	req, _ := http.NewRequest("POST", "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePod(w, req)

	var decodedReview AdmissionReview
	json.NewDecoder(w.Body).Decode(&decodedReview)

	if decodedReview.Response == nil {
		t.Errorf("expected response to be set")
	}
	if decodedReview.Response.UID != "roundtrip-uid" {
		t.Errorf("expected UID to match")
	}
	if !decodedReview.Response.Allowed {
		t.Errorf("expected allowed=true")
	}
}

func TestJSONPatchStructure(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Annotations = map[string]string{
		"nest.penguintech.io/inject-sdk": "true",
	}
	podMeta.Metadata.Namespace = "default"

	podBytes, _ := json.Marshal(podMeta)
	req := &AdmissionRequest{
		UID:    "test-uid",
		Object: podBytes,
	}

	patches, _ := handler.buildPatches(req)

	// Verify each patch has required fields
	for i, p := range patches {
		if p.Op == "" {
			t.Errorf("patch %d missing Op", i)
		}
		if p.Path == "" {
			t.Errorf("patch %d missing Path", i)
		}
		// Value may be optional for some ops
	}
}

func TestMutatePodResponseContentType(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Namespace = "default"
	podBytes, _ := json.Marshal(podMeta)

	review := AdmissionReview{
		Request: &AdmissionRequest{
			UID:    "test-uid",
			Object: podBytes,
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePod(w, req)

	contentType := w.Header().Get("Content-Type")
	if contentType != "application/json" {
		t.Errorf("expected Content-Type application/json, got %s", contentType)
	}
}

func TestMutatePodEmptyRequest(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	req, _ := http.NewRequest("POST", "/mutate", bytes.NewReader([]byte("")))
	w := httptest.NewRecorder()

	handler.MutatePod(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400 for empty request, got %d", w.Code)
	}
}

func TestMutatePodWithEmptyCredentialsField(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Annotations = map[string]string{
		"nest.penguintech.io/credentials": "",
	}
	podMeta.Metadata.Namespace = "default"

	podBytes, _ := json.Marshal(podMeta)

	review := AdmissionReview{
		Request: &AdmissionRequest{
			UID:    "test-uid",
			Object: podBytes,
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePod(w, req)

	var respReview AdmissionReview
	json.NewDecoder(w.Body).Decode(&respReview)

	// Empty credentials annotation should not produce patches
	if len(respReview.Response.Patch) > 0 {
		var patches []JSONPatch
		json.Unmarshal(respReview.Response.Patch, &patches)
		for _, p := range patches {
			if strings.Contains(p.Path, "credentials-injected") {
				t.Errorf("expected no credentials patch for empty annotation")
			}
		}
	}
}

func TestMutatePodLargeBody(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Annotations = make(map[string]string)
	// Add many annotations
	for i := 0; i < 100; i++ {
		podMeta.Metadata.Annotations[string(rune('a'+i%26))] = "value"
	}
	podMeta.Metadata.Namespace = "default"

	podBytes, _ := json.Marshal(podMeta)

	review := AdmissionReview{
		Request: &AdmissionRequest{
			UID:    "test-uid",
			Object: podBytes,
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePod(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200 for large body, got %d", w.Code)
	}
}

func TestBuildPatchesWithWhitespaceInCredentials(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Annotations = map[string]string{
		"nest.penguintech.io/credentials": "  secret1  ,  secret2  , secret3  ",
	}
	podMeta.Metadata.Namespace = "default"

	podBytes, _ := json.Marshal(podMeta)
	req := &AdmissionRequest{
		UID:    "test-uid",
		Object: podBytes,
	}

	patches, _ := handler.buildPatches(req)

	for _, p := range patches {
		if strings.Contains(p.Path, "credentials-injected") {
			v := p.Value.(string)
			// Should have trimmed whitespace
			if strings.HasPrefix(v, " ") || strings.HasSuffix(v, " ") {
				t.Errorf("expected whitespace to be trimmed")
			}
		}
	}
}

func BenchmarkBuildPatches(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Annotations = map[string]string{
		"nest.penguintech.io/inject-sdk":  "true",
		"nest.penguintech.io/credentials": "cred1,cred2",
	}
	podMeta.Metadata.Namespace = "default"

	podBytes, _ := json.Marshal(podMeta)
	req := &AdmissionRequest{
		UID:    "test-uid",
		Object: podBytes,
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		handler.buildPatches(req)
	}
}

func BenchmarkMutatePod(b *testing.B) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Annotations = map[string]string{
		"nest.penguintech.io/inject-sdk": "true",
	}
	podMeta.Metadata.Namespace = "default"

	podBytes, _ := json.Marshal(podMeta)

	review := AdmissionReview{
		Request: &AdmissionRequest{
			UID:    "test-uid",
			Object: podBytes,
		},
	}

	body, _ := json.Marshal(review)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("POST", "/mutate", bytes.NewReader(body))
		w := httptest.NewRecorder()
		handler.MutatePod(w, req)
	}
}

func TestMutatePodResponseBodyNotEmpty(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Namespace = "default"
	podBytes, _ := json.Marshal(podMeta)

	review := AdmissionReview{
		Request: &AdmissionRequest{
			UID:    "test-uid",
			Object: podBytes,
		},
	}

	body, _ := json.Marshal(review)
	req, _ := http.NewRequest("POST", "/mutate", bytes.NewReader(body))
	w := httptest.NewRecorder()

	handler.MutatePod(w, req)

	if w.Body.Len() == 0 {
		t.Errorf("expected non-empty response body")
	}
}

func TestJSONPatchOpValues(t *testing.T) {
	logger, _ := zap.NewDevelopment()
	handler := NewWebhookHandler(logger)

	podMeta := PodMeta{}
	podMeta.Metadata.Annotations = map[string]string{
		"nest.penguintech.io/inject-sdk": "true",
	}
	podMeta.Metadata.Namespace = "default"

	podBytes, _ := json.Marshal(podMeta)
	req := &AdmissionRequest{
		UID:    "test-uid",
		Object: podBytes,
	}

	patches, _ := handler.buildPatches(req)

	// All patches should have 'add' operation
	for _, p := range patches {
		if p.Op != "add" {
			t.Errorf("expected op 'add', got %s", p.Op)
		}
	}
}
