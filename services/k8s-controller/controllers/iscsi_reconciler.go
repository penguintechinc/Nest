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

	"k8s.io/apimachinery/pkg/api/resource"
	"sigs.k8s.io/controller-runtime/pkg/log"

	nestv1 "github.com/penguintechinc/nest/apis/v1"
)

// reconcileISCSI reconciles an iSCSI DataResource by creating a target on nest-iscsi-gateway.
// The gateway endpoint is read from ISCSI_GATEWAY_ENDPOINT env var (default: http://nest-iscsi-gateway:8083).
func (r *DataResourceReconciler) reconcileISCSI(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	endpoint := os.Getenv("ISCSI_GATEWAY_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://nest-iscsi-gateway:8083"
	}

	// Get storage size and convert to bytes
	storageSize := iscsiStorageSize(dr)
	q, err := resource.ParseQuantity(storageSize)
	if err != nil {
		logger.Error(err, "failed to parse storage size", "size", storageSize)
		r.setPhase(dr, nestv1.PhaseFailed, fmt.Sprintf("Failed to parse storage size: %v", err))
		_ = r.Status().Update(ctx, dr)
		return err
	}
	sizeBytes := q.Value()

	// Build target request with tenant-scoped ACL and CHAP authentication
	targetReq := map[string]interface{}{
		"name":      dr.Name,
		"tenant":    dr.Spec.Tenant,
		"rbdImage":  fmt.Sprintf("%s-%s", dr.Spec.Tenant, dr.Name),
		"rbdPool":   "nest-rbd-pool",
		"sizeBytes": sizeBytes,
		// Tenant-scoped ACL: restrict to initiators in the tenant namespace
		"acl": map[string]interface{}{
			// TODO: Configure based on actual initiator IQNs in the tenant
			"initiators": []string{}, // Empty = deny all; populate with tenant pod initiators
		},
		// CHAP authentication credentials
		"chap": map[string]interface{}{
			"username": fmt.Sprintf("tenant-%s", dr.Spec.Tenant),
			"password": "generated-secret", // TODO: Use generated secret from K8s Secret
		},
		"idempotencyToken": dr.Name + "-" + string(dr.UID)[:8], // Prevent duplicate targets on retry
	}

	reqBody, err := json.Marshal(targetReq)
	if err != nil {
		logger.Error(err, "failed to marshal iSCSI target request", "name", dr.Name)
		r.setPhase(dr, nestv1.PhaseFailed, fmt.Sprintf("Failed to marshal request: %v", err))
		_ = r.Status().Update(ctx, dr)
		return err
	}

	// Call nest-iscsi-gateway API with context and timeout
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "POST",
		fmt.Sprintf("%s/api/v1/targets", endpoint),
		bytes.NewReader(reqBody),
	)
	if err != nil {
		logger.Error(err, "failed to create iSCSI gateway request", "endpoint", endpoint)
		r.setPhase(dr, nestv1.PhaseFailed, fmt.Sprintf("Failed to create request: %v", err))
		_ = r.Status().Update(ctx, dr)
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		logger.Error(err, "failed to call iSCSI gateway", "endpoint", endpoint)
		r.setPhase(dr, nestv1.PhaseFailed, fmt.Sprintf("iSCSI gateway call failed: %v", err))
		_ = r.Status().Update(ctx, dr)
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != 201 {
		logger.Error(nil, "unexpected iSCSI gateway response", "status", resp.StatusCode)
		r.setPhase(dr, nestv1.PhaseFailed, fmt.Sprintf("iSCSI gateway returned status %d", resp.StatusCode))
		_ = r.Status().Update(ctx, dr)
		return fmt.Errorf("iSCSI gateway returned status %d", resp.StatusCode)
	}

	// Parse response to get target ID and IQN
	var respData map[string]string
	if err := json.NewDecoder(resp.Body).Decode(&respData); err != nil {
		logger.Error(err, "failed to parse iSCSI gateway response", "name", dr.Name)
		r.setPhase(dr, nestv1.PhaseFailed, fmt.Sprintf("Failed to parse response: %v", err))
		_ = r.Status().Update(ctx, dr)
		return err
	}

	targetID, ok := respData["id"]
	if !ok {
		logger.Error(nil, "iSCSI gateway response missing 'id' field", "name", dr.Name)
		r.setPhase(dr, nestv1.PhaseFailed, "iSCSI gateway response missing target ID")
		_ = r.Status().Update(ctx, dr)
		return fmt.Errorf("iSCSI gateway response missing 'id' field")
	}

	iqn, ok := respData["iqn"]
	if !ok {
		logger.Error(nil, "iSCSI gateway response missing 'iqn' field", "name", dr.Name)
		r.setPhase(dr, nestv1.PhaseFailed, "iSCSI gateway response missing IQN")
		_ = r.Status().Update(ctx, dr)
		return fmt.Errorf("iSCSI gateway response missing 'iqn' field")
	}

	// Store target ID and IQN in annotations
	if dr.Annotations == nil {
		dr.Annotations = make(map[string]string)
	}
	dr.Annotations["nest.penguintech.io/iscsi-target-id"] = targetID
	dr.Annotations["nest.penguintech.io/iscsi-iqn"] = iqn

	// Update annotations
	if err := r.Update(ctx, dr); err != nil {
		logger.Error(err, "failed to update DataResource annotations", "name", dr.Name)
		return err
	}

	// Set endpoints and phase
	iscsiEndpoint := fmt.Sprintf("iscsi://%s", iqn)
	dr.Status.Endpoints = &nestv1.ResourceEndpoints{
		Native: iscsiEndpoint,
	}
	r.setPhase(dr, nestv1.PhaseReady, "iSCSI target provisioned")
	return r.Status().Update(ctx, dr)
}

// reconcileISCSIDelete removes the iSCSI target from nest-iscsi-gateway.
func (r *DataResourceReconciler) reconcileISCSIDelete(ctx context.Context, dr *nestv1.DataResource) error {
	logger := log.FromContext(ctx)

	endpoint := os.Getenv("ISCSI_GATEWAY_ENDPOINT")
	if endpoint == "" {
		endpoint = "http://nest-iscsi-gateway:8083"
	}

	// Get target ID from annotations
	if dr.Annotations == nil {
		logger.Info("no annotations found; skipping iSCSI delete", "name", dr.Name)
		return nil
	}

	targetID, ok := dr.Annotations["nest.penguintech.io/iscsi-target-id"]
	if !ok {
		logger.Info("iSCSI target ID not found in annotations; skipping delete", "name", dr.Name)
		return nil
	}

	// Call DELETE on gateway with context and timeout
	client := &http.Client{Timeout: 30 * time.Second}

	req, err := http.NewRequestWithContext(ctx, "DELETE",
		fmt.Sprintf("%s/api/v1/targets/%s", endpoint, targetID),
		nil,
	)
	if err != nil {
		logger.Error(err, "failed to create DELETE request for iSCSI target", "targetID", targetID)
		return err
	}

	resp, err := client.Do(req)
	if err != nil {
		logger.Error(err, "failed to call iSCSI gateway DELETE", "endpoint", endpoint, "targetID", targetID)
		return err
	}
	defer func() {
		io.ReadAll(resp.Body)
		resp.Body.Close()
	}()

	// Ignore 404 (idempotent)
	if resp.StatusCode != 200 && resp.StatusCode != 404 {
		logger.Error(nil, "unexpected iSCSI gateway DELETE response", "status", resp.StatusCode)
		return fmt.Errorf("iSCSI gateway DELETE returned status %d", resp.StatusCode)
	}

	return nil
}

// Helper function for iSCSI storage size
func iscsiStorageSize(dr *nestv1.DataResource) string {
	if dr.Spec.Size != nil && dr.Spec.Size.Storage != "" {
		q, err := resource.ParseQuantity(dr.Spec.Size.Storage)
		if err == nil {
			return q.String()
		}
	}
	return "100Gi"
}
