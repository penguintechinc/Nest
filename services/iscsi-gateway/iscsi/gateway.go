// Package iscsi implements iSCSI target management for Nest.
// It communicates with the ceph-iscsi REST API to create/delete iSCSI targets
// backed by Ceph RBD images.
package iscsi

import (
	"fmt"
	"log"
	"net/http"
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
	cfg     Config
	mu      sync.RWMutex
	targets map[string]*Target
}

// New creates a new iSCSI Gateway
func New(cfg Config) *Gateway {
	return &Gateway{
		cfg:     cfg,
		targets: make(map[string]*Target),
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
	target := &Target{
		ID:          id,
		IQN:         iqnFromTenant(req.Tenant, req.Name),
		Name:        req.Name,
		Tenant:      req.Tenant,
		RBDImage:    req.RBDImage,
		RBDPool:     req.RBDPool,
		SizeBytes:   req.SizeBytes,
		InitiatorIQ: req.InitiatorIQ,
		CreatedAt:   time.Now().UTC(),
		Status:      "provisioning",
	}

	gw.mu.Lock()
	gw.targets[id] = target
	gw.mu.Unlock()

	// In production this would call the ceph-iscsi REST API.
	// For P2: register the target in our in-memory store and report provisioning.
	gw.cfg.Logger.Printf("iSCSI target created: %s (IQN: %s)", id, target.IQN)

	// Async: mark as active after registration
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
	delete(gw.targets, targetID)
	gw.mu.Unlock()
	gw.cfg.Logger.Printf("iSCSI target deleted: %s (IQN: %s)", targetID, target.IQN)
	c.Status(http.StatusNoContent)
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
