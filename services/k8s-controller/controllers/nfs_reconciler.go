package controllers

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// reconcileNFS reconciles an NFS DataResource by registering an export with nest-nfs-gateway.
// The gateway endpoint is read from NFS_GATEWAY_ENDPOINT env var (default: http://nest-nfs-gateway:8082).
func (r *DataResourceReconciler) reconcileNFS(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	endpoint := os.Getenv("NFS_GATEWAY_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://nest-nfs-gateway:8082"
	}

	// Build export request with tenant-scoped client restrictions
	// Only allow access from pods in the tenant namespace
	exportReq := map[string]interface{}{
		"name":       dr.Name,
		"tenant":     dr.Spec.Tenant,
		"path":       fmt.Sprintf("/cephfs/nest/%s/%s", dr.Spec.Tenant, dr.Name),
		"accessMode": "rw",
		// Restrict to clients in the tenant namespace (pod CIDR or specific subnet)
		// TODO: Configure based on cluster networking (e.g., pod CIDR for tenant namespace)
		"clients":          "10.0.0.0/8", // Placeholder: actual pod CIDR needed
		"idempotencyToken": dr.Name + "-" + string(dr.UID)[:8],  // Prevent duplicate exports on retry
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
	req, err := http.NewRequestWithContext(ctx, "POST",
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

	resp, err := client.Do(req)
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
	nfsEndpoint := fmt.Sprintf("nfs://%s.%s.svc.cluster.local:/nest/%s/%s",
		dr.Name, dr.Spec.Tenant, dr.Spec.Tenant, dr.Name)
	dr.Status.Endpoints = &nestv1.ResourceEndpoints{
		Native: nfsEndpoint,
	}
	r.setPhase(dr, nestv1.PhaseReady, "NFS export provisioned")
	return r.Status().Update(ctx, dr)
}

// reconcileNFSDelete removes the NFS export from nest-nfs-gateway.
func (r *DataResourceReconciler) reconcileNFSDelete(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	endpoint := os.Getenv("NFS_GATEWAY_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://nest-nfs-gateway:8082"
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

	req, err := http.NewRequestWithContext(ctx, "DELETE",
		fmt.Sprintf("%s/api/v1/exports/%s", endpoint, exportID),
		nil,
	)
	if err != nil {
		logger.Error(err, "failed to create DELETE request for NFS export", "exportID", exportID)
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.Error(err, "failed to call NFS gateway DELETE", "endpoint", endpoint, "exportID", exportID)
		return err
	}
	defer func() {
		io.ReadAll(resp.Body)
		resp.Body.Close()
	}()

	// Ignore 404 (idempotent)
	if resp.StatusCode != 200 && resp.StatusCode != 404 {
		logger.Error(nil, "unexpected NFS gateway DELETE response", "status", resp.StatusCode)
		return fmt.Errorf("NFS gateway DELETE returned status %d", resp.StatusCode)
	}

	return nil
}
