package main

import (
	"database/sql"
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"gorm.io/datatypes"
	"gorm.io/gorm"
)

// ResourceController handles resource-related HTTP requests
type ResourceController struct {
	db *gorm.DB
}

// NewResourceController creates a new resource controller
func NewResourceController(db *gorm.DB) *ResourceController {
	return &ResourceController{db: db}
}

// ListResources retrieves all resources visible to the current user
// GET /api/v1/resources
func (rc *ResourceController) ListResources(c *gin.Context) {
	// Extract user context (would be set by auth middleware)
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	userIDUint := userID.(uint)

	// Get pagination parameters
	page := 1
	if p := c.Query("page"); p != "" {
		if parsed, err := strconv.Atoi(p); err == nil && parsed > 0 {
			page = parsed
		}
	}

	pageSize := 20
	if ps := c.Query("page_size"); ps != "" {
		if parsed, err := strconv.Atoi(ps); err == nil && parsed > 0 && parsed <= 100 {
			pageSize = parsed
		}
	}

	// Get filter parameters
	teamID := c.Query("team_id")
	status := c.Query("status")
	resourceTypeID := c.Query("resource_type_id")

	// Build query - resources scoped by user's team membership
	query := rc.db.Where("resources.deleted_at IS NULL").
		Joins("INNER JOIN team_members ON resources.team_id = team_members.team_id").
		Where("team_members.user_id = ?", userIDUint).
		Preload("ResourceType").
		Preload("Team")

	// Apply filters
	if teamID != "" {
		if tid, err := strconv.ParseUint(teamID, 10, 32); err == nil {
			query = query.Where("resources.team_id = ?", uint(tid))
		}
	}

	if status != "" {
		query = query.Where("resources.status = ?", status)
	}

	if resourceTypeID != "" {
		if rtid, err := strconv.ParseUint(resourceTypeID, 10, 32); err == nil {
			query = query.Where("resources.resource_type_id = ?", uint(rtid))
		}
	}

	// Count total
	var total int64
	countQuery := query
	if err := countQuery.Model(&Resource{}).Count(&total).Error; err != nil {
		log.Printf("Error counting resources: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to count resources",
		})
		return
	}

	// Paginate
	offset := (page - 1) * pageSize
	query = query.Offset(offset).Limit(pageSize).Order("created_at DESC")

	var resources []*Resource
	if err := query.Find(&resources).Error; err != nil {
		log.Printf("Error listing resources: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to list resources",
		})
		return
	}

	// Convert to response format
	responses := make([]*ResourceResponse, 0, len(resources))
	for _, r := range resources {
		responses = append(responses, resourceToResponse(r))
	}

	c.JSON(http.StatusOK, ResourceListResponse{
		Resources: responses,
		Total:     total,
		Page:      page,
		PageSize:  pageSize,
	})
}

// CreateResource creates a new resource
// POST /api/v1/resources
func (rc *ResourceController) CreateResource(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	userRole, _ := c.Get("user_role")
	teamRole, _ := c.Get("team_role")

	// Check authorization - must be TeamMaintainer or higher
	if !hasMinimumRole(userRole, "admin") && !hasMinimumRole(teamRole, "maintainer") {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "Insufficient permissions to create resources",
		})
		return
	}

	var req CreateResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid request body",
			Details: err.Error(),
		})
		return
	}

	// Validate lifecycle_mode
	validModes := map[string]bool{"full": true, "partial": true, "monitor_only": true}
	if !validModes[req.LifecycleMode] {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_lifecycle_mode",
			Message: "lifecycle_mode must be one of: full, partial, monitor_only",
		})
		return
	}

	// Verify team exists and user has access
	var team Team
	if err := rc.db.Where("id = ? AND deleted_at IS NULL", req.TeamID).First(&team).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "team_not_found",
				Message: "Team not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "database_error",
				Message: "Failed to verify team",
			})
		}
		return
	}

	// Verify user has access to team
	var teamMember TeamMember
	if err := rc.db.Where("team_id = ? AND user_id = ?", req.TeamID, userID.(uint)).
		First(&teamMember).Error; err != nil {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "You do not have access to this team",
		})
		return
	}

	// Verify resource type exists
	var resourceType ResourceType
	if err := rc.db.Where("id = ? AND deleted_at IS NULL", req.ResourceTypeID).
		First(&resourceType).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "resource_type_not_found",
				Message: "Resource type not found",
			})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "database_error",
				Message: "Failed to verify resource type",
			})
		}
		return
	}

	// Check unique constraint - name must be unique within team
	var existing Resource
	if err := rc.db.Where("team_id = ? AND name = ? AND deleted_at IS NULL",
		req.TeamID, req.Name).First(&existing).Error; err == nil {
		c.JSON(http.StatusConflict, ErrorResponse{
			Error:   "resource_exists",
			Message: "A resource with this name already exists in this team",
		})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to check existing resources",
		})
		return
	}

	// Marshal connection info and config to JSON
	connInfo, _ := json.Marshal(req.ConnectionInfo)
	creds, _ := json.Marshal(req.Credentials)
	cfg, _ := json.Marshal(req.Config)

	// Set capabilities
	canBackup := false
	canModifyConfig := false
	canModifyUsers := false
	canScale := false
	if req.Capabilities != nil {
		canBackup = req.Capabilities["can_backup"]
		canModifyConfig = req.Capabilities["can_modify_config"]
		canModifyUsers = req.Capabilities["can_modify_users"]
		canScale = req.Capabilities["can_scale"]
	}

	// Create resource
	resource := &Resource{
		Name:               req.Name,
		ResourceTypeID:     req.ResourceTypeID,
		TeamID:             req.TeamID,
		Status:             "pending",
		LifecycleMode:      req.LifecycleMode,
		ProvisioningMethod: req.ProvisioningMethod,
		ConnectionInfo:     datatypes.JSON(connInfo),
		Credentials:        datatypes.JSON(creds),
		Config:             datatypes.JSON(cfg),
		TLSEnabled:         req.TLSEnabled,
		CanBackup:          canBackup,
		CanModifyConfig:    canModifyConfig,
		CanModifyUsers:     canModifyUsers,
		CanScale:           canScale,
		CreatedBy:          userID.(uint),
	}

	if err := rc.db.Create(resource).Error; err != nil {
		log.Printf("Error creating resource: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to create resource",
		})
		return
	}

	// Preload associations for response
	rc.db.Preload("ResourceType").Preload("Team").First(resource)

	c.JSON(http.StatusCreated, resourceToResponse(resource))
}

// GetResource retrieves a single resource by ID
// GET /api/v1/resources/:id
func (rc *ResourceController) GetResource(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	resourceID := c.Param("id")

	var resource Resource
	// Verify user has access to this resource's team
	query := rc.db.Where("resources.id = ? AND resources.deleted_at IS NULL", resourceID).
		Joins("INNER JOIN team_members ON resources.team_id = team_members.team_id").
		Where("team_members.user_id = ?", userID.(uint)).
		Preload("ResourceType").
		Preload("Team")

	if err := query.First(&resource).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "resource_not_found",
				Message: "Resource not found or you do not have access",
			})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "database_error",
				Message: "Failed to retrieve resource",
			})
		}
		return
	}

	c.JSON(http.StatusOK, resourceToResponse(&resource))
}

// UpdateResource updates a resource
// PUT /api/v1/resources/:id
func (rc *ResourceController) UpdateResource(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	userRole, _ := c.Get("user_role")
	teamRole, _ := c.Get("team_role")

	// Check authorization - must be TeamMaintainer or higher
	if !hasMinimumRole(userRole, "admin") && !hasMinimumRole(teamRole, "maintainer") {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "Insufficient permissions to update resources",
		})
		return
	}

	resourceID := c.Param("id")

	var resource Resource
	// Verify user has access
	if err := rc.db.Where("id = ? AND deleted_at IS NULL", resourceID).
		Joins("INNER JOIN team_members ON resources.team_id = team_members.team_id").
		Where("team_members.user_id = ?", userID.(uint)).
		Preload("ResourceType").
		Preload("Team").
		First(&resource).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "resource_not_found",
				Message: "Resource not found or you do not have access",
			})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "database_error",
				Message: "Failed to retrieve resource",
			})
		}
		return
	}

	var req UpdateResourceRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, ErrorResponse{
			Error:   "invalid_request",
			Message: "Invalid request body",
			Details: err.Error(),
		})
		return
	}

	// Apply updates
	if req.Name != nil {
		// Check uniqueness in team
		var existing Resource
		if err := rc.db.Where("team_id = ? AND name = ? AND id != ? AND deleted_at IS NULL",
			resource.TeamID, *req.Name, resource.ID).First(&existing).Error; err == nil {
			c.JSON(http.StatusConflict, ErrorResponse{
				Error:   "resource_exists",
				Message: "A resource with this name already exists in this team",
			})
			return
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "database_error",
				Message: "Failed to check existing resources",
			})
			return
		}
		resource.Name = *req.Name
	}

	if req.Status != nil {
		validStatuses := map[string]bool{
			"pending": true, "provisioning": true, "active": true,
			"updating": true, "paused": true, "error": true, "deleted": true,
		}
		if !validStatuses[*req.Status] {
			c.JSON(http.StatusBadRequest, ErrorResponse{
				Error:   "invalid_status",
				Message: "Invalid status value",
			})
			return
		}
		resource.Status = *req.Status
	}

	if req.Config != nil {
		cfg, _ := json.Marshal(req.Config)
		resource.Config = datatypes.JSON(cfg)
	}

	// Save updates
	if err := rc.db.Save(&resource).Error; err != nil {
		log.Printf("Error updating resource: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to update resource",
		})
		return
	}

	c.JSON(http.StatusOK, resourceToResponse(&resource))
}

// DeleteResource soft-deletes a resource
// DELETE /api/v1/resources/:id
func (rc *ResourceController) DeleteResource(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	userRole, _ := c.Get("user_role")
	teamRole, _ := c.Get("team_role")

	// Check authorization - must be TeamAdmin or GlobalAdmin
	if !hasMinimumRole(userRole, "admin") && !hasMinimumRole(teamRole, "admin") {
		c.JSON(http.StatusForbidden, ErrorResponse{
			Error:   "forbidden",
			Message: "Insufficient permissions to delete resources",
		})
		return
	}

	resourceID := c.Param("id")

	var resource Resource
	// Verify user has access
	if err := rc.db.Where("id = ? AND deleted_at IS NULL", resourceID).
		Joins("INNER JOIN team_members ON resources.team_id = team_members.team_id").
		Where("team_members.user_id = ?", userID.(uint)).
		First(&resource).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "resource_not_found",
				Message: "Resource not found or you do not have access",
			})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "database_error",
				Message: "Failed to retrieve resource",
			})
		}
		return
	}

	// Soft delete
	if err := rc.db.Delete(&resource).Error; err != nil {
		log.Printf("Error deleting resource: %v", err)
		c.JSON(http.StatusInternalServerError, ErrorResponse{
			Error:   "database_error",
			Message: "Failed to delete resource",
		})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

// GetResourceStats retrieves statistics for a resource
// GET /api/v1/resources/:id/stats
func (rc *ResourceController) GetResourceStats(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	resourceID := c.Param("id")

	// Verify user has access to resource
	var resource Resource
	if err := rc.db.Where("id = ? AND deleted_at IS NULL", resourceID).
		Joins("INNER JOIN team_members ON resources.team_id = team_members.team_id").
		Where("team_members.user_id = ?", userID.(uint)).
		First(&resource).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "resource_not_found",
				Message: "Resource not found or you do not have access",
			})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "database_error",
				Message: "Failed to retrieve resource",
			})
		}
		return
	}

	// Get latest stats
	var stats ResourceStats
	if err := rc.db.Where("resource_id = ?", resourceID).
		Order("timestamp DESC").
		First(&stats).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "stats_not_found",
				Message: "No statistics available for this resource",
			})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "database_error",
				Message: "Failed to retrieve statistics",
			})
		}
		return
	}

	// Parse JSON fields
	var metrics, riskFactors map[string]interface{}
	json.Unmarshal(stats.Metrics, &metrics)
	json.Unmarshal(stats.RiskFactors, &riskFactors)

	c.JSON(http.StatusOK, ResourceStatsResponse{
		ResourceID:  stats.ResourceID,
		Timestamp:   stats.Timestamp,
		Metrics:     metrics,
		RiskLevel:   stats.RiskLevel,
		RiskFactors: riskFactors,
	})
}

// GetConnectionInfo retrieves connection information for a resource
// GET /api/v1/resources/:id/connection-info
func (rc *ResourceController) GetConnectionInfo(c *gin.Context) {
	userID, exists := c.Get("user_id")
	if !exists {
		c.JSON(http.StatusUnauthorized, ErrorResponse{
			Error:   "unauthorized",
			Message: "User context not found",
		})
		return
	}

	userRole, _ := c.Get("user_role")
	teamRole, _ := c.Get("team_role")

	resourceID := c.Param("id")

	var resource Resource
	// Verify user has access to resource
	if err := rc.db.Where("id = ? AND deleted_at IS NULL", resourceID).
		Joins("INNER JOIN team_members ON resources.team_id = team_members.team_id").
		Where("team_members.user_id = ?", userID.(uint)).
		First(&resource).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, ErrorResponse{
				Error:   "resource_not_found",
				Message: "Resource not found or you do not have access",
			})
		} else {
			c.JSON(http.StatusInternalServerError, ErrorResponse{
				Error:   "database_error",
				Message: "Failed to retrieve resource",
			})
		}
		return
	}

	// Parse connection info
	var connInfo map[string]interface{}
	json.Unmarshal(resource.ConnectionInfo, &connInfo)

	response := &ConnectionInfoResponse{
		ConnectionInfo: connInfo,
		TLSEnabled:     resource.TLSEnabled,
		TLSCertID:      resource.TLSCertID,
	}

	// Only expose credentials to TeamMaintainer+ roles
	if hasMinimumRole(userRole, "admin") || hasMinimumRole(teamRole, "maintainer") {
		var creds map[string]interface{}
		json.Unmarshal(resource.Credentials, &creds)
		response.Credentials = creds
		response.AccessLevel = "full"
	} else {
		response.AccessLevel = "restricted"
	}

	c.JSON(http.StatusOK, response)
}

// Helper functions

// resourceToResponse converts a Resource model to ResourceResponse DTO
func resourceToResponse(r *Resource) *ResourceResponse {
	var connInfo, cfg map[string]interface{}
	json.Unmarshal(r.ConnectionInfo, &connInfo)
	json.Unmarshal(r.Config, &cfg)

	resp := &ResourceResponse{
		ID:                 r.ID,
		Name:               r.Name,
		ResourceTypeID:     r.ResourceTypeID,
		TeamID:             r.TeamID,
		Status:             r.Status,
		LifecycleMode:      r.LifecycleMode,
		ProvisioningMethod: r.ProvisioningMethod,
		ConnectionInfo:     connInfo,
		TLSEnabled:         r.TLSEnabled,
		Config:             cfg,
		CanModifyUsers:     r.CanModifyUsers,
		CanModifyConfig:    r.CanModifyConfig,
		CanBackup:          r.CanBackup,
		CanScale:           r.CanScale,
		CreatedBy:          r.CreatedBy,
		CreatedAt:          r.CreatedAt,
		UpdatedAt:          r.UpdatedAt,
	}

	if r.ResourceType != nil {
		resp.ResourceType = r.ResourceType
	}
	if r.Team != nil {
		resp.Team = r.Team
	}

	if !r.DeletedAt.Time.IsZero() {
		resp.DeletedAt = sql.NullTime{Time: r.DeletedAt.Time, Valid: true}
	}

	return resp
}

// hasMinimumRole checks if a role meets or exceeds the minimum required role
func hasMinimumRole(role interface{}, minRequired string) bool {
	if role == nil {
		return false
	}

	roleStr, ok := role.(string)
	if !ok {
		return false
	}

	roleHierarchy := map[string]int{
		"viewer":      1,
		"contributor": 2,
		"maintainer":  3,
		"admin":       4,
	}

	return roleHierarchy[roleStr] >= roleHierarchy[minRequired]
}
