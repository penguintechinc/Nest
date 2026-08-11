// Package iscsi implements iSCSI target management for Nest.
// It communicates with the ceph-iscsi REST API to create/delete iSCSI targets
// backed by Ceph RBD images.
package iscsi

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Config holds iSCSI gateway configuration
type Config struct {
	CephISCSIEndpoint string
	CephISCSIUser     string
	CephISCSIPassword string
	Logger            *log.Logger
}

// Target represents a managed iSCSI target
type Target struct {
	ID          string        `json:"id"`
	IQN         string        `json:"iqn"`
	Name        string        `json:"name"`
	Tenant      string        `json:"tenant"`
	RBDImage    string        `json:"rbdImage"`
	RBDPool     string        `json:"rbdPool"`
	SizeBytes   int64         `json:"sizeBytes"`
	InitiatorIQ string        `json:"initiatorIqn,omitempty"`
	CreatedAt   time.Time     `json:"createdAt"`
	Status      string        `json:"status"` // provisioning | active | degraded | failed | error
	statusMu    sync.RWMutex  `json:"-"`
	pollDone    chan struct{} `json:"-"`
}

// TargetResponse is a safe JSON-serializable representation of Target without internal fields
type TargetResponse struct {
	ID          string    `json:"id"`
	IQN         string    `json:"iqn"`
	Name        string    `json:"name"`
	Tenant      string    `json:"tenant"`
	RBDImage    string    `json:"rbdImage"`
	RBDPool     string    `json:"rbdPool"`
	SizeBytes   int64     `json:"sizeBytes"`
	InitiatorIQ string    `json:"initiatorIqn,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	Status      string    `json:"status"`
}

// toResponse converts Target to TargetResponse, safely reading Status
func (t *Target) toResponse() *TargetResponse {
	t.statusMu.RLock()
	status := t.Status
	t.statusMu.RUnlock()

	return &TargetResponse{
		ID:          t.ID,
		IQN:         t.IQN,
		Name:        t.Name,
		Tenant:      t.Tenant,
		RBDImage:    t.RBDImage,
		RBDPool:     t.RBDPool,
		SizeBytes:   t.SizeBytes,
		InitiatorIQ: t.InitiatorIQ,
		CreatedAt:   t.CreatedAt,
		Status:      status,
	}
}

// Gateway manages iSCSI targets via the ceph-iscsi API
type Gateway struct {
	cfg        Config
	mu         sync.RWMutex
	targets    map[string]*Target
	apiURL     string
	httpClient *http.Client
}

// New creates a new iSCSI Gateway
func New(cfg Config) *Gateway {
	// Use Config.CephISCSIEndpoint if provided, fall back to env var, then default
	apiURL := cfg.CephISCSIEndpoint
	if apiURL == "" {
		apiURL = os.Getenv("CEPH_ISCSI_API_URL")
	}
	if apiURL == "" {
		apiURL = "http://localhost:5001"
	}

	return &Gateway{
		cfg:        cfg,
		targets:    make(map[string]*Target),
		apiURL:     apiURL,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// CreateTargetRequest is the request body for creating an iSCSI target
type CreateTargetRequest struct {
	Name         string `json:"name" binding:"required"`
	Tenant       string `json:"tenant" binding:"required"`
	RBDImage     string `json:"rbdImage" binding:"required"`
	RBDPool      string `json:"rbdPool"`
	SizeBytes    int64  `json:"sizeBytes"`
	InitiatorIQ  string `json:"initiatorIqn"`
	CHAPUsername string `json:"chapUsername,omitempty"`
	CHAPPassword string `json:"chapPassword,omitempty"`
}

// validateTenantAndName ensures tenant and name are safe identifiers (no path injection).
// Allowed: alphanumeric, -, _ (no / : or other special chars that could be used for injection).
func validateTenantAndName(tenant, name string) error {
	validPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

	if !validPattern.MatchString(tenant) {
		return fmt.Errorf("tenant contains invalid characters (only alphanumeric, -, _ allowed)")
	}
	if !validPattern.MatchString(name) {
		return fmt.Errorf("name contains invalid characters (only alphanumeric, -, _ allowed)")
	}

	return nil
}

// validateRBDImage ensures RBD image path is safe (pool/image format).
func validateRBDImage(image string) error {
	// Format should be simple image name or pool/image; no slashes in pool or image parts
	parts := strings.Split(image, "/")
	if len(parts) != 2 {
		return fmt.Errorf("RBD image must be in format 'pool/image'")
	}

	validPattern := regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)
	if !validPattern.MatchString(parts[0]) || !validPattern.MatchString(parts[1]) {
		return fmt.Errorf("RBD pool and image must be alphanumeric with - or _")
	}

	return nil
}

// validateIQN ensures an iSCSI Qualified Name (IQN) is safe for use in REST paths.
// RFC 3720: iqn.yyyy-mm.{reverse-domain}:{unique-id}
// Restrictions: max 223 chars, no path traversal chars (/, \, .., ?, #, whitespace)
func validateIQN(iqn string) error {
	if iqn == "" {
		return fmt.Errorf("IQN cannot be empty")
	}
	if len(iqn) > 223 {
		return fmt.Errorf("IQN exceeds maximum length of 223 characters")
	}
	// Reject path traversal and special characters dangerous in REST paths
	if strings.Contains(iqn, "/") || strings.Contains(iqn, "\\") || strings.Contains(iqn, "..") ||
		strings.Contains(iqn, "?") || strings.Contains(iqn, "#") || strings.ContainsAny(iqn, "\t\n\r ") {
		return fmt.Errorf("IQN contains invalid characters")
	}
	// Basic IQN format check: iqn.YYYY-MM.xxx:xxx
	iqnPattern := regexp.MustCompile(`^iqn\.\d{4}-\d{2}\.[A-Za-z0-9.\-:]+$`)
	if !iqnPattern.MatchString(iqn) {
		return fmt.Errorf("IQN does not match RFC 3720 format")
	}
	return nil
}

// iqnFromTenant generates a deterministic IQN from tenant and name.
// Format: iqn.2024-01.io.penguintech.nest:<tenant>:<name>
// Note: tenant and name must be validated before calling this function.
func iqnFromTenant(tenant, name string) string {
	return fmt.Sprintf("iqn.2024-01.io.penguintech.nest:%s:%s", tenant, name)
}

// cephISCSICreateRequest is the request body for Ceph-iSCSI API POST /api/target
type cephISCSICreateRequest struct {
	TargetIQN string `json:"target_iqn"`
}

// cephISCSICreateTargetResponse is the response from Ceph-iSCSI API POST /api/target
type cephISCSICreateTargetResponse struct {
	TargetIQN string `json:"target_iqn"`
	Status    string `json:"status"`
}

// cephISCSIAttachDiskRequest is the request body for Ceph-iSCSI API POST /api/target/{iqn}/disk
type cephISCSIAttachDiskRequest struct {
	Pool  string `json:"pool"`
	Image string `json:"image"`
	Size  int64  `json:"size,omitempty"`
}

// cephISCSIClientRequest is the request body for Ceph-iSCSI API PUT /api/client/{target_iqn}/{client_iqn}
type cephISCSIClientRequest struct {
	ClientIQN string `json:"client_iqn"`
}

// cephISCSIClientAuthRequest is the request body for Ceph-iSCSI API PUT /api/clientauth/{target_iqn}/{client_iqn}
type cephISCSIClientAuthRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

// cephISCSITargetStatusResponse is the response from Ceph-iSCSI API GET /api/target/{iqn}
type cephISCSITargetStatusResponse struct {
	TargetIQN string `json:"target_iqn"`
	Status    string `json:"status"`
	State     string `json:"state,omitempty"`
}

// callCephISCSIAPI makes a request to the Ceph-iSCSI REST API
func (gw *Gateway) callCephISCSIAPI(method, path string, body interface{}) ([]byte, error) {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	url := gw.apiURL + path
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := gw.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("api request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("ceph-iscsi api returned %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// CreateTarget handles POST /api/v1/targets.
// Validates tenant, name, and RBD image to prevent path injection.
func (gw *Gateway) CreateTarget(c *gin.Context) {
	var req CreateTargetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "nest.iscsi.invalid", "message": err.Error()})
		return
	}

	// Validate tenant and name to prevent path injection in IQN
	if err := validateTenantAndName(req.Tenant, req.Name); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "nest.iscsi.invalid_tenant_name", "message": err.Error()})
		return
	}

	// Validate RBD image format
	if err := validateRBDImage(req.RBDImage); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "nest.iscsi.invalid_image", "message": err.Error()})
		return
	}

	// Validate InitiatorIQ if provided
	if req.InitiatorIQ != "" {
		if err := validateIQN(req.InitiatorIQ); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"code": "nest.iscsi.invalid_initiator", "message": err.Error()})
			return
		}
	}

	if req.RBDPool == "" {
		req.RBDPool = "nest-rbd-pool"
	}

	id := uuid.New().String()
	iqn := iqnFromTenant(req.Tenant, req.Name)

	// Validate the generated target IQN
	if err := validateIQN(iqn); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"code": "nest.iscsi.invalid_iqn", "message": err.Error()})
		return
	}
	target := &Target{
		ID:          id,
		IQN:         iqn,
		Name:        req.Name,
		Tenant:      req.Tenant,
		RBDImage:    req.RBDImage,
		RBDPool:     req.RBDPool,
		SizeBytes:   req.SizeBytes,
		InitiatorIQ: req.InitiatorIQ,
		CreatedAt:   time.Now().UTC(),
		Status:      "provisioning",
		pollDone:    make(chan struct{}),
	}

	// Call Ceph-iSCSI API to create the target
	createReq := cephISCSICreateRequest{TargetIQN: iqn}
	_, err := gw.callCephISCSIAPI("POST", "/api/target", createReq)
	if err != nil {
		gw.cfg.Logger.Printf("failed to create target in Ceph-iSCSI: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": "nest.iscsi.api_error", "message": err.Error()})
		return
	}

	// Attach the RBD image to the target
	attachReq := cephISCSIAttachDiskRequest{
		Pool:  req.RBDPool,
		Image: req.RBDImage,
		Size:  req.SizeBytes,
	}
	_, err = gw.callCephISCSIAPI("POST", "/api/target/"+iqn+"/disk", attachReq)
	if err != nil {
		gw.cfg.Logger.Printf("failed to attach disk to target in Ceph-iSCSI: %v", err)
		// Try to clean up the created target
		if cleanupErr := gw.callCephISCSIAPIDirect("DELETE", "/api/target/"+iqn, nil); cleanupErr != nil {
			gw.cfg.Logger.Printf("cleanup failed after disk attachment error: %v", cleanupErr)
		}
		c.JSON(http.StatusInternalServerError, gin.H{"code": "nest.iscsi.api_error", "message": err.Error()})
		return
	}

	// Apply client ACL and CHAP authentication if InitiatorIQ is set
	if req.InitiatorIQ != "" {
		// Register the initiator ACL for the target
		err = gw.applyClientACL(iqn, req.InitiatorIQ)
		if err != nil {
			gw.cfg.Logger.Printf("failed to apply client ACL: %v", err)
			// Clean up the created target and disk
			if cleanupErr := gw.callCephISCSIAPIDirect("DELETE", "/api/target/"+iqn, nil); cleanupErr != nil {
				gw.cfg.Logger.Printf("cleanup failed after ACL error: %v", cleanupErr)
			}
			c.JSON(http.StatusInternalServerError, gin.H{"code": "nest.iscsi.api_error", "message": "failed to apply initiator ACL"})
			return
		}

		// Apply CHAP authentication if credentials are provided
		if req.CHAPUsername != "" && req.CHAPPassword != "" {
			err = gw.applyCHAPAuth(iqn, req.InitiatorIQ, req.CHAPUsername, req.CHAPPassword)
			if err != nil {
				gw.cfg.Logger.Printf("failed to apply CHAP auth: %v", err)
				// Clean up the created target and disk
				if cleanupErr := gw.callCephISCSIAPIDirect("DELETE", "/api/target/"+iqn, nil); cleanupErr != nil {
					gw.cfg.Logger.Printf("cleanup failed after CHAP error: %v", cleanupErr)
				}
				c.JSON(http.StatusInternalServerError, gin.H{"code": "nest.iscsi.api_error", "message": "failed to apply CHAP authentication"})
				return
			}
		}
	}

	// Store in in-memory map as cache
	gw.mu.Lock()
	gw.targets[id] = target
	gw.mu.Unlock()

	// Start polling for real target status
	go gw.pollTargetStatus(target)

	gw.cfg.Logger.Printf("iSCSI target created: %s (IQN: %s)", id, target.IQN)

	c.Header("Location", "/api/v1/targets/"+id)
	c.JSON(http.StatusAccepted, target.toResponse())
}

// DeleteTarget handles DELETE /api/v1/targets/:targetId
func (gw *Gateway) DeleteTarget(c *gin.Context) {
	targetID := c.Param("targetId")
	gw.mu.Lock()
	target, ok := gw.targets[targetID]
	if !ok {
		gw.mu.Unlock()
		c.JSON(http.StatusNotFound, gin.H{"code": "nest.iscsi.not_found", "message": "target not found"})
		return
	}
	gw.mu.Unlock()

	// Call Ceph-iSCSI API to delete the target
	err := gw.callCephISCSIAPIDirect("DELETE", "/api/target/"+target.IQN, nil)
	if err != nil {
		gw.cfg.Logger.Printf("failed to delete target from Ceph-iSCSI: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{"code": "nest.iscsi.api_error", "message": err.Error()})
		return
	}

	// Stop polling goroutine for this target
	close(target.pollDone)

	// Remove from in-memory map on successful API deletion
	gw.mu.Lock()
	delete(gw.targets, targetID)
	gw.mu.Unlock()

	gw.cfg.Logger.Printf("iSCSI target deleted: %s (IQN: %s)", targetID, target.IQN)
	c.Status(http.StatusNoContent)
}

// callCephISCSIAPIDirect makes a request to the Ceph-iSCSI REST API and returns only error (for DELETE ops)
func (gw *Gateway) callCephISCSIAPIDirect(method, path string, body interface{}) error {
	var reqBody io.Reader
	if body != nil {
		jsonData, err := json.Marshal(body)
		if err != nil {
			return fmt.Errorf("failed to marshal request body: %w", err)
		}
		reqBody = bytes.NewReader(jsonData)
	}

	url := gw.apiURL + path
	req, err := http.NewRequest(method, url, reqBody)
	if err != nil {
		return fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := gw.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("api request failed: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("failed to read response: %w", err)
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("ceph-iscsi api returned %d: %s", resp.StatusCode, string(respBody))
	}

	return nil
}

// applyClientACL registers an initiator ACL for a target via Ceph-iSCSI API
// Calls PUT /api/client/{target_iqn}/{client_iqn}
func (gw *Gateway) applyClientACL(targetIQN, clientIQN string) error {
	// Validate IQNs before building paths (path injection protection)
	if err := validateIQN(targetIQN); err != nil {
		return fmt.Errorf("invalid target IQN: %w", err)
	}
	if err := validateIQN(clientIQN); err != nil {
		return fmt.Errorf("invalid client IQN: %w", err)
	}

	// Use URL path escaping as defense in depth
	path := fmt.Sprintf("/api/client/%s/%s", url.PathEscape(targetIQN), url.PathEscape(clientIQN))
	req := cephISCSIClientRequest{ClientIQN: clientIQN}
	_, err := gw.callCephISCSIAPI("PUT", path, req)
	if err != nil {
		return fmt.Errorf("failed to apply client ACL: %w", err)
	}
	return nil
}

// applyCHAPAuth sets CHAP authentication for a client via Ceph-iSCSI API
// Calls PUT /api/clientauth/{target_iqn}/{client_iqn}
// Password is masked in logs; never logged directly
func (gw *Gateway) applyCHAPAuth(targetIQN, clientIQN, username, password string) error {
	// Validate IQNs before building paths (path injection protection)
	if err := validateIQN(targetIQN); err != nil {
		return fmt.Errorf("invalid target IQN: %w", err)
	}
	if err := validateIQN(clientIQN); err != nil {
		return fmt.Errorf("invalid client IQN: %w", err)
	}

	// Use URL path escaping as defense in depth
	path := fmt.Sprintf("/api/clientauth/%s/%s", url.PathEscape(targetIQN), url.PathEscape(clientIQN))
	req := cephISCSIClientAuthRequest{
		Username: username,
		Password: password,
	}
	_, err := gw.callCephISCSIAPI("PUT", path, req)
	if err != nil {
		// Log without revealing password
		gw.cfg.Logger.Printf("failed to apply CHAP auth for client %s: %v", clientIQN, err)
		return fmt.Errorf("failed to apply CHAP auth: %w", err)
	}
	// Log successful CHAP application without revealing password
	gw.cfg.Logger.Printf("CHAP auth applied for client %s (username: %s, password: ****)", clientIQN, username)
	return nil
}

// pollTargetStatus polls the Ceph-iSCSI API for target status
// Updates the target's status field based on the actual API response
// Runs in a goroutine and updates status periodically
func (gw *Gateway) pollTargetStatus(target *Target) {
	ticker := time.NewTicker(1 * time.Second)
	defer ticker.Stop()

	// Make initial status check immediately
	pollOnce := func() {
		path := fmt.Sprintf("/api/target/%s", target.IQN)
		respBody, err := gw.callCephISCSIAPI("GET", path, nil)
		if err != nil {
			// Log error but don't fail; keep polling
			gw.cfg.Logger.Printf("failed to poll target status for %s: %v", target.IQN, err)
			target.statusMu.Lock()
			target.Status = "failed"
			target.statusMu.Unlock()
			return
		}

		// Guard against nil respBody (should not happen but be defensive)
		if respBody == nil {
			gw.cfg.Logger.Printf("empty response from status poll for %s", target.IQN)
			target.statusMu.Lock()
			target.Status = "failed"
			target.statusMu.Unlock()
			return
		}

		var statusResp cephISCSITargetStatusResponse
		if err := json.Unmarshal(respBody, &statusResp); err != nil {
			gw.cfg.Logger.Printf("failed to parse target status response for %s: %v", target.IQN, err)
			target.statusMu.Lock()
			target.Status = "failed"
			target.statusMu.Unlock()
			return
		}

		// Guard against empty status response
		if statusResp.Status == "" {
			gw.cfg.Logger.Printf("empty status in response for %s", target.IQN)
			target.statusMu.Lock()
			target.Status = "failed"
			target.statusMu.Unlock()
			return
		}

		// Map ceph-iscsi status to our status values
		target.statusMu.Lock()
		switch statusResp.Status {
		case "ready", "active":
			target.Status = "active"
		case "degraded":
			target.Status = "degraded"
		case "failed":
			target.Status = "failed"
		case "provisioning":
			target.Status = "provisioning"
		default:
			// For unknown statuses, use the value as-is but guard against injection
			if len(statusResp.Status) > 50 {
				gw.cfg.Logger.Printf("suspiciously long status value for %s: %d chars", target.IQN, len(statusResp.Status))
				target.Status = "failed"
			} else {
				target.Status = statusResp.Status
			}
		}
		target.statusMu.Unlock()
	}

	// Poll immediately first
	pollOnce()

	for {
		select {
		case <-target.pollDone:
			return
		case <-ticker.C:
			pollOnce()
		}
	}
}

// ListTargets handles GET /api/v1/targets
func (gw *Gateway) ListTargets(c *gin.Context) {
	tenant := c.Query("tenant")
	gw.mu.RLock()
	defer gw.mu.RUnlock()
	result := make([]*TargetResponse, 0, len(gw.targets))
	for _, t := range gw.targets {
		if tenant == "" || t.Tenant == tenant {
			// Convert to response DTO safely (avoids copying mutex)
			result = append(result, t.toResponse())
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": result, "count": len(result)})
}

// GetTarget handles GET /api/v1/targets/:targetId
func (gw *Gateway) GetTarget(c *gin.Context) {
	targetID := c.Param("targetId")
	gw.mu.RLock()
	target, ok := gw.targets[targetID]
	gw.mu.RUnlock()
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": "nest.iscsi.not_found", "message": "target not found"})
		return
	}
	c.JSON(http.StatusOK, target.toResponse())
}
