package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// validResourceID validates that a resource ID is alphanumeric/UUID-safe (prevents SSRF).
func validResourceID(id string) bool {
	return regexp.MustCompile(`^[a-zA-Z0-9_-]+$`).MatchString(id)
}

// validateGatewayEndpoint validates that a gateway endpoint URL has a valid scheme and host.
// Returns error if the endpoint is malformed or unsafe (prevents SSRF via config injection).
func validateGatewayEndpoint(endpoint string) error {
	u, err := url.Parse(endpoint)
	if err != nil {
		return fmt.Errorf("invalid endpoint URL: %w", err)
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("endpoint scheme must be http or https, got: %s", u.Scheme)
	}
	if u.Host == "" {
		return fmt.Errorf("endpoint must have a non-empty host")
	}
	return nil
}

// resourceIdempotencyToken builds a stable per-resource token that is safe even
// when the object has no UID yet (e.g. in unit tests). Real K8s objects always
// carry a UID, so this normally appends the first 8 UID chars.
func resourceIdempotencyToken(dr *nestv1.DataResource) string {
	uid := string(dr.UID)
	if len(uid) > 8 {
		uid = uid[:8]
	}
	if uid == "" {
		return dr.Name
	}
	return dr.Name + "-" + uid
}

// reconcileNFS reconciles an NFS DataResource by registering an export with nest-nfs-gateway.
// The gateway endpoint is read from NFS_GATEWAY_ENDPOINT env var (default: http://nest-nfs-gateway:8082).
func (r *DataResourceReconciler) reconcileNFS(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	endpoint := os.Getenv("NFS_GATEWAY_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://nest-nfs-gateway:8082"
	}

	// Validate endpoint is a safe, operator-controlled config value
	if err := validateGatewayEndpoint(endpoint); err != nil {
		logger.Error(err, "invalid NFS gateway endpoint", "endpoint", endpoint)
		r.setPhase(dr, nestv1.PhaseFailed, fmt.Sprintf("Invalid endpoint config: %v", err))
		_ = r.Status().Update(ctx, dr)
		return err
	}

	// The client CIDR is the export's only access control. Defaulting it to a broad
	// range would expose every tenant's export to the whole cluster, so an
	// unspecified CIDR fails the resource rather than provisioning an open export.
	allowedClients := nfsAllowedClients(dr)
	if allowedClients == "" {
		err := fmt.Errorf("NFS DataResource requires spec.annotations[%q] to scope export access to a client CIDR", nfsAllowedClientsAnnotation)
		logger.Error(err, "missing allowed clients", "name", dr.Name)
		r.setPhase(dr, nestv1.PhaseFailed, err.Error())
		_ = r.Status().Update(ctx, dr)
		return err
	}

	// The gateway has no server-side idempotency and mints a fresh export ID per
	// POST, so a retry after a partial failure would strand a duplicate export.
	// Short-circuit if the export we recorded is still present.
	if existingID, ok := dr.Annotations["nest.penguintech.io/nfs-export-id"]; ok {
		exists, err := nfsExportExists(ctx, endpoint, existingID)
		if err != nil {
			logger.Error(err, "failed to check existing NFS export", "exportID", existingID)
			return err
		}
		if exists {
			dr.Status.Endpoints = &nestv1.ResourceEndpoints{Native: nfsEndpointFor(dr)}
			r.setPhase(dr, nestv1.PhaseReady, "NFS export provisioned")
			return r.Status().Update(ctx, dr)
		}
		logger.Info("recorded NFS export no longer exists; recreating", "exportID", existingID)
	}

	// Field names must match the gateway's CreateExportRequest exactly — it binds
	// JSON and silently ignores unknown keys, so an unrecognised field would be
	// dropped rather than rejected.
	exportReq := map[string]interface{}{
		"name":       dr.Name,
		"tenant":     dr.Spec.Tenant,
		"path":       fmt.Sprintf("/cephfs/nest/%s/%s", dr.Spec.Tenant, dr.Name),
		"accessMode": "rw",
		"clients":    allowedClients,
	}

	reqBody, err := json.Marshal(exportReq)
	if err != nil {
		logger.Error(err, "failed to marshal NFS export request", "name", dr.Name)
		r.setPhase(dr, nestv1.PhaseFailed, fmt.Sprintf("Failed to marshal request: %v", err))
		_ = r.Status().Update(ctx, dr)
		return err
	}

	// Call nest-nfs-gateway API with context and timeout
	// Create HTTP client with timeout to prevent blocking forever
	client := &http.Client{Timeout: 30 * time.Second}

	// Create request with context for cancellation
	// endpoint is validated operator config (env), path is static; user data is in request body, not URL
	req, err := http.NewRequestWithContext(ctx, "POST", //#nosec G704
		fmt.Sprintf("%s/api/v1/exports", endpoint),
		bytes.NewReader(reqBody),
	)
	if err != nil {
		logger.Error(err, "failed to create NFS gateway request", "endpoint", endpoint)
		r.setPhase(dr, nestv1.PhaseFailed, fmt.Sprintf("Failed to create request: %v", err))
		_ = r.Status().Update(ctx, dr)
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req) //#nosec G704
	if err != nil {
		logger.Error(err, "failed to call NFS gateway", "endpoint", endpoint)
		r.setPhase(dr, nestv1.PhaseFailed, fmt.Sprintf("NFS gateway call failed: %v", err))
		_ = r.Status().Update(ctx, dr)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		logger.Error(nil, "unexpected NFS gateway response", "status", resp.StatusCode)
		r.setPhase(dr, nestv1.PhaseFailed, fmt.Sprintf("NFS gateway returned status %d", resp.StatusCode))
		_ = r.Status().Update(ctx, dr)
		return fmt.Errorf("NFS gateway returned status %d", resp.StatusCode)
	}

	// Parse response to get export ID
	var respData map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		logger.Error(err, "failed to parse NFS gateway response", "name", dr.Name)
		r.setPhase(dr, nestv1.PhaseFailed, fmt.Sprintf("Failed to parse response: %v", err))
		_ = r.Status().Update(ctx, dr)
		return err
	}

	exportID, ok := respData["id"]
	if !ok {
		logger.Error(nil, "NFS gateway response missing 'id' field", "name", dr.Name)
		r.setPhase(dr, nestv1.PhaseFailed, "NFS gateway response missing export ID")
		_ = r.Status().Update(ctx, dr)
		return fmt.Errorf("NFS gateway response missing 'id' field")
	}

	// Store export ID in annotations
	if dr.Annotations == nil {
		dr.Annotations = make(map[string]string)
	}
	dr.Annotations["nest.penguintech.io/nfs-export-id"] = exportID

	// Update annotations
	if err := r.Update(ctx, dr); err != nil {
		logger.Error(err, "failed to update DataResource annotations", "name", dr.Name)
		return err
	}

	// Set endpoints and phase
	dr.Status.Endpoints = &nestv1.ResourceEndpoints{Native: nfsEndpointFor(dr)}
	r.setPhase(dr, nestv1.PhaseReady, "NFS export provisioned")
	return r.Status().Update(ctx, dr)
}

// nfsAllowedClientsAnnotation names the spec annotation carrying the CIDR (or client
// list) permitted to mount the export. It is the export's access control.
const nfsAllowedClientsAnnotation = "nest.penguintech.io/nfs-allowed-clients"

// nfsAllowedClients reads the permitted client CIDR from the resource spec.
func nfsAllowedClients(dr *nestv1.DataResource) string {
	if dr.Spec.Annotations == nil {
		return ""
	}
	return dr.Spec.Annotations[nfsAllowedClientsAnnotation]
}

// nfsEndpointFor builds the native NFS endpoint for a resource.
func nfsEndpointFor(dr *nestv1.DataResource) string {
	return fmt.Sprintf("nfs://%s.%s.svc.cluster.local:/nest/%s/%s",
		dr.Name, dr.Spec.Tenant, dr.Spec.Tenant, dr.Name)
}

// nfsExportExists reports whether the gateway still holds the given export.
func nfsExportExists(ctx context.Context, endpoint, exportID string) (bool, error) {
	if !validResourceID(exportID) {
		return false, fmt.Errorf("invalid export ID format")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return false, fmt.Errorf("parsing gateway endpoint: %w", err)
	}
	u.Path = "/api/v1/exports/" + url.PathEscape(exportID)

	req, err := http.NewRequestWithContext(ctx, "GET", u.String(), nil) //#nosec G704
	if err != nil {
		return false, err
	}

	httpClient := &http.Client{Timeout: 30 * time.Second}
	resp, err := httpClient.Do(req) //#nosec G704
	if err != nil {
		return false, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	switch resp.StatusCode {
	case http.StatusOK:
		return true, nil
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("NFS gateway GET returned status %d", resp.StatusCode)
	}
}

// reconcileNFSDelete removes the NFS export from nest-nfs-gateway.
func (r *DataResourceReconciler) reconcileNFSDelete(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	endpoint := os.Getenv("NFS_GATEWAY_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://nest-nfs-gateway:8082"
	}

	// Validate endpoint is a safe, operator-controlled config value
	if err := validateGatewayEndpoint(endpoint); err != nil {
		logger.Error(err, "invalid NFS gateway endpoint", "endpoint", endpoint)
		return err
	}

	// Get export ID from annotations
	if dr.Annotations == nil {
		logger.Info("no annotations found; skipping NFS delete", "name", dr.Name)
		return nil
	}

	exportID, ok := dr.Annotations["nest.penguintech.io/nfs-export-id"]
	if !ok {
		logger.Info("NFS export ID not found in annotations; skipping delete", "name", dr.Name)
		return nil
	}

	// Call DELETE on gateway with context and timeout
	client := &http.Client{Timeout: 30 * time.Second}

	// Validate export ID to prevent SSRF
	if !validResourceID(exportID) {
		logger.Error(nil, "invalid export ID format", "exportID", exportID)
		return fmt.Errorf("invalid export ID format")
	}

	// Build URL safely using url.URL with url.PathEscape - exportID is validated via validResourceID()
	u, _ := url.Parse(endpoint)
	u.Path = "/api/v1/exports/" + url.PathEscape(exportID)

	req, err := http.NewRequestWithContext(ctx, "DELETE", u.String(), nil) //#nosec G704
	if err != nil {
		logger.Error(err, "failed to create DELETE request for NFS export", "exportID", exportID)
		return err
	}

	resp, err := client.Do(req) //#nosec G704
	if err != nil {
		logger.Error(err, "failed to call NFS gateway DELETE", "endpoint", endpoint, "exportID", exportID)
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	// Ignore 404 (idempotent)
	if resp.StatusCode != 200 && resp.StatusCode != 404 {
		logger.Error(nil, "unexpected NFS gateway DELETE response", "status", resp.StatusCode)
		return fmt.Errorf("NFS gateway DELETE returned status %d", resp.StatusCode)
	}

	return nil
}
