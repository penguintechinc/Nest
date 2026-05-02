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
	"os"
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
	ID          string    `json:"id"`
	IQN         string    `json:"iqn"`
	Name        string    `json:"name"`
	Tenant      string    `json:"tenant"`
	RBDImage    string    `json:"rbdImage"`
	RBDPool     string    `json:"rbdPool"`
	SizeBytes   int64     `json:"sizeBytes"`
	InitiatorIQ string    `json:"initiatorIqn,omitempty"`
	CreatedAt   time.Time `json:"createdAt"`
	Status      string    `json:"status"` // provisioning | active | error
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
	apiURL := os.Getenv("CEPH_ISCSI_API_URL")
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
	Name        string `json:"name" binding:"required"`
	Tenant      string `json:"tenant" binding:"required"`
	RBDImage    string `json:"rbdImage" binding:"required"`
	RBDPool     string `json:"rbdPool"`
	SizeBytes   int64  `json:"sizeBytes"`
	InitiatorIQ string `json:"initiatorIqn"`
}

// iqnFromTenant generates a deterministic IQN from tenant and name.
// Format: iqn.2024-01.io.penguintech.nest:<tenant>:<name>
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

// CreateTarget handles POST /api/v1/targets
func (gw *Gateway) CreateTarget(c *gin.Context) {
	var req CreateTargetRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"code": "nest.iscsi.invalid", "message": err.Error()})
		return
	}
	if req.RBDPool == "" {
		req.RBDPool = "nest-rbd-pool"
	}

	id := uuid.New().String()
	iqn := iqnFromTenant(req.Tenant, req.Name)
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
		gw.callCephISCSIAPI("DELETE", "/api/target/"+iqn, nil)
		c.JSON(http.StatusInternalServerError, gin.H{"code": "nest.iscsi.api_error", "message": err.Error()})
		return
	}

	// Store in in-memory map as cache
	gw.mu.Lock()
	gw.targets[id] = target
	gw.mu.Unlock()

	gw.cfg.Logger.Printf("iSCSI target created: %s (IQN: %s)", id, target.IQN)

	// Async: mark as active after a brief delay
	go func() {
		time.Sleep(100 * time.Millisecond)
		gw.mu.Lock()
		if t, ok := gw.targets[id]; ok {
			t.Status = "active"
		}
		gw.mu.Unlock()
	}()

	c.Header("Location", "/api/v1/targets/"+id)
	c.JSON(http.StatusAccepted, target)
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

// ListTargets handles GET /api/v1/targets
func (gw *Gateway) ListTargets(c *gin.Context) {
	tenant := c.Query("tenant")
	gw.mu.RLock()
	defer gw.mu.RUnlock()
	result := make([]*Target, 0, len(gw.targets))
	for _, t := range gw.targets {
		if tenant == "" || t.Tenant == tenant {
			result = append(result, t)
		}
	}
	c.JSON(http.StatusOK, gin.H{"items": result, "count": len(result)})
}

// GetTarget handles GET /api/v1/targets/:targetId
func (gw *Gateway) GetTarget(c *gin.Context) {
	targetID := c.Param("targetId")
	gw.mu.RLock()
	defer gw.mu.RUnlock()
	target, ok := gw.targets[targetID]
	if !ok {
		c.JSON(http.StatusNotFound, gin.H{"code": "nest.iscsi.not_found", "message": "target not found"})
		return
	}
	c.JSON(http.StatusOK, target)
}
