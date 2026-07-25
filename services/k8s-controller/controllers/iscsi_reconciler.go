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
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"
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

	// Validate endpoint is a safe, operator-controlled config value
	if err := validateGatewayEndpoint(endpoint); err != nil {
		logger.Error(err, "invalid iSCSI gateway endpoint", "endpoint", endpoint)
		r.setPhase(dr, nestv1.PhaseFailed, fmt.Sprintf("Invalid endpoint config: %v", err))
		_ = r.Status().Update(ctx, dr)
		return err
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

	// The initiator IQN is the ACL: the gateway admits only this initiator to the
	// target. Without it the LUN would be attachable by anyone who can reach the
	// portal, so a missing IQN fails the resource rather than provisioning an open
	// target.
	initiatorIQN := iscsiInitiatorIQN(dr)
	if initiatorIQN == "" {
		err := fmt.Errorf("iSCSI DataResource requires spec.annotations[%q] to scope the target ACL to an initiator", iscsiInitiatorIQNAnnotation)
		logger.Error(err, "missing initiator IQN", "name", dr.Name)
		r.setPhase(dr, nestv1.PhaseFailed, err.Error())
		_ = r.Status().Update(ctx, dr)
		return err
	}

	// CHAP credentials live in a per-resource Secret so each tenant target gets a
	// distinct password that survives reconciles.
	chapPassword, err := r.ensureISCSICHAPSecret(ctx, dr)
	if err != nil {
		logger.Error(err, "failed to ensure iSCSI CHAP secret", "name", dr.Name)
		r.setPhase(dr, nestv1.PhaseFailed, fmt.Sprintf("Failed to provision CHAP credentials: %v", err))
		_ = r.Status().Update(ctx, dr)
		return err
	}

	// The gateway has no server-side idempotency and mints a fresh target ID per
	// POST, so a retry after a partial failure would strand a duplicate target.
	// Short-circuit if the target we recorded is still present.
	if existingID, ok := dr.Annotations["nest.penguintech.io/iscsi-target-id"]; ok {
		exists, err := iscsiTargetExists(ctx, endpoint, existingID)
		if err != nil {
			logger.Error(err, "failed to check existing iSCSI target", "targetID", existingID)
			return err
		}
		if exists {
			iqn := dr.Annotations["nest.penguintech.io/iscsi-iqn"]
			dr.Status.Endpoints = &nestv1.ResourceEndpoints{Native: fmt.Sprintf("iscsi://%s", iqn)}
			r.setPhase(dr, nestv1.PhaseReady, "iSCSI target provisioned")
			return r.Status().Update(ctx, dr)
		}
		logger.Info("recorded iSCSI target no longer exists; recreating", "targetID", existingID)
	}

	// Field names must match the gateway's CreateTargetRequest exactly — it binds
	// JSON and silently ignores unknown keys, so a nested shape would drop the
	// ACL and CHAP credentials and provision an unauthenticated target.
	targetReq := map[string]interface{}{
		"name":         dr.Name,
		"tenant":       dr.Spec.Tenant,
		"rbdImage":     fmt.Sprintf("%s-%s", dr.Spec.Tenant, dr.Name),
		"rbdPool":      "nest-rbd-pool",
		"sizeBytes":    sizeBytes,
		"initiatorIqn": initiatorIQN,
		"chapUsername": iscsiCHAPUsername(dr),
		"chapPassword": chapPassword,
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

	// endpoint is validated operator config (env), path is static; user data is in request body, not URL
	req, err := http.NewRequestWithContext(ctx, "POST", //#nosec G704
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

	resp, err := client.Do(req) //#nosec G704
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

	// Validate endpoint is a safe, operator-controlled config value
	if err := validateGatewayEndpoint(endpoint); err != nil {
		logger.Error(err, "invalid iSCSI gateway endpoint", "endpoint", endpoint)
		return err
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

	// Validate target ID to prevent SSRF
	if !validResourceID(targetID) {
		logger.Error(nil, "invalid target ID format", "targetID", targetID)
		return fmt.Errorf("invalid target ID format")
	}

	// Build URL safely using url.URL with url.PathEscape - targetID is validated via validResourceID()
	u, _ := url.Parse(endpoint)
	u.Path = "/api/v1/targets/" + url.PathEscape(targetID)

	req, err := http.NewRequestWithContext(ctx, "DELETE", u.String(), nil) //#nosec G704
	if err != nil {
		logger.Error(err, "failed to create DELETE request for iSCSI target", "targetID", targetID)
		return err
	}

	resp, err := client.Do(req) //#nosec G704
	if err != nil {
		logger.Error(err, "failed to call iSCSI gateway DELETE", "endpoint", endpoint, "targetID", targetID)
		return err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()
	}()

	// Ignore 404 (idempotent)
	if resp.StatusCode != 200 && resp.StatusCode != 404 {
		logger.Error(nil, "unexpected iSCSI gateway DELETE response", "status", resp.StatusCode)
		return fmt.Errorf("iSCSI gateway DELETE returned status %d", resp.StatusCode)
	}

	return nil
}

// iscsiInitiatorIQNAnnotation names the spec annotation carrying the initiator IQN
// permitted to attach the target. It is the target's ACL.
const iscsiInitiatorIQNAnnotation = "nest.penguintech.io/iscsi-initiator-iqn"

// iscsiInitiatorIQN reads the permitted initiator IQN from the resource spec.
func iscsiInitiatorIQN(dr *nestv1.DataResource) string {
	if dr.Spec.Annotations == nil {
		return ""
	}
	return dr.Spec.Annotations[iscsiInitiatorIQNAnnotation]
}

// iscsiCHAPUsername derives the CHAP username for a target.
func iscsiCHAPUsername(dr *nestv1.DataResource) string {
	return fmt.Sprintf("tenant-%s-%s", dr.Spec.Tenant, dr.Name)
}

func iscsiCHAPSecretName(dr *nestv1.DataResource) string {
	return fmt.Sprintf("%s-%s-iscsi-chap", dr.Spec.Tenant, dr.Name)
}

// iscsiCHAPPasswordLength is capped at 16 because the Windows iSCSI initiator
// rejects CHAP secrets outside 12-16 characters.
const iscsiCHAPPasswordLength = 16

// ensureISCSICHAPSecret returns the target's CHAP password, generating and storing
// it on first reconcile. The Secret is owned by the DataResource so it is garbage
// collected with it, and re-reads on later reconciles keep the password stable.
func (r *DataResourceReconciler) ensureISCSICHAPSecret(ctx context.Context, dr *nestv1.DataResource) (string, error) {
	name := iscsiCHAPSecretName(dr)
	namespace := dr.Spec.Tenant

	secret := &corev1.Secret{}
	err := r.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, secret)
	switch {
	case err == nil:
		pw := string(secret.Data["password"])
		if pw == "" {
			return "", fmt.Errorf("iSCSI CHAP secret %s/%s has no password field", namespace, name)
		}
		return pw, nil
	case !errors.IsNotFound(err):
		return "", fmt.Errorf("getting iSCSI CHAP secret: %w", err)
	}

	pw, err := generateRandomPassword(iscsiCHAPPasswordLength)
	if err != nil {
		return "", fmt.Errorf("generating iSCSI CHAP password: %w", err)
	}

	secret = &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"nest.penguintech.io/tenant":       dr.Spec.Tenant,
				"nest.penguintech.io/dataresource": dr.Name,
			},
			OwnerReferences: []metav1.OwnerReference{
				{
					APIVersion:         "nest.penguintech.io/v1",
					Kind:               "DataResource",
					Name:               dr.Name,
					UID:                dr.UID,
					BlockOwnerDeletion: boolPtr(true),
				},
			},
		},
		Type: corev1.SecretTypeOpaque,
		Data: map[string][]byte{
			"username": []byte(iscsiCHAPUsername(dr)),
			"password": []byte(pw),
		},
	}
	if err := r.Create(ctx, secret); err != nil {
		// A concurrent reconcile may have won the race; adopt its password.
		if errors.IsAlreadyExists(err) {
			existing := &corev1.Secret{}
			if getErr := r.Get(ctx, client.ObjectKey{Name: name, Namespace: namespace}, existing); getErr == nil {
				return string(existing.Data["password"]), nil
			}
		}
		return "", fmt.Errorf("creating iSCSI CHAP secret: %w", err)
	}
	return pw, nil
}

// iscsiTargetExists reports whether the gateway still holds the given target.
func iscsiTargetExists(ctx context.Context, endpoint, targetID string) (bool, error) {
	if !validResourceID(targetID) {
		return false, fmt.Errorf("invalid target ID format")
	}

	u, err := url.Parse(endpoint)
	if err != nil {
		return false, fmt.Errorf("parsing gateway endpoint: %w", err)
	}
	u.Path = "/api/v1/targets/" + url.PathEscape(targetID)

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
		return false, fmt.Errorf("iSCSI gateway GET returned status %d", resp.StatusCode)
	}
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
