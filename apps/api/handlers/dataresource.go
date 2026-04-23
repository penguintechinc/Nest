package handlers

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/penguintechinc/nest/apps/api/middleware"
	"github.com/penguintechinc/nest/apps/api/store"
)

// ListDataResources handles GET /api/v1/tenants/:tenantId/data-resources
func ListDataResources(s store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenant := c.Param("tenantId")
		if t := middleware.GetTenant(c); t != tenant {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    "nest.auth.tenant_mismatch",
				"message": "Token tenant does not match path tenant",
			})
			return
		}
		resources, err := s.ListDataResources(c.Request.Context(), tenant)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "nest.internal", "message": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"items": resources,
			"meta": gin.H{
				"count":   len(resources),
				"version": 1,
			},
		})
	}
}

// CreateDataResource handles POST /api/v1/tenants/:tenantId/data-resources
func CreateDataResource(s store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenant := c.Param("tenantId")
		if t := middleware.GetTenant(c); t != tenant {
			c.JSON(http.StatusForbidden, gin.H{
				"code":    "nest.auth.tenant_mismatch",
				"message": "Token tenant does not match path tenant",
			})
			return
		}

		var req CreateDataResourceRequest
		if err := c.ShouldBindJSON(&req); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "nest.dataresource.invalid",
				"message": err.Error(),
			})
			return
		}

		// Validate required fields
		if req.Type == "" || req.Class == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"code":    "nest.dataresource.invalid",
				"message": "type and class are required",
			})
			return
		}

		// Check free-tier limit (5 DataResources)
		claims := middleware.GetClaims(c)
		if claims != nil && claims.Tier == "free" {
			count, err := s.CountDataResources(c.Request.Context(), tenant)
			if err == nil && count >= 5 {
				c.JSON(http.StatusPaymentRequired, gin.H{
					"code":    "nest.license.limit_exceeded",
					"message": "Free tier DataResource limit reached (5/5). Upgrade to Pro to provision more.",
					"details": []gin.H{{"type": "quota_failure", "quota": "data_resources", "limit": 5, "used": count}},
				})
				return
			}
		}

		dr := &store.DataResourceRecord{
			ID:          uuid.New().String(),
			Name:        req.Name,
			Tenant:      tenant,
			Type:        req.Type,
			Class:       req.Class,
			Origination: "managed",
			Phase:       "Pending",
			HA:          req.HA,
			Protocols:   req.Protocols,
			CreatedAt:   time.Now().UTC(),
		}
		if req.Origination != "" {
			dr.Origination = req.Origination
		}

		if err := s.CreateDataResource(c.Request.Context(), dr); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"code": "nest.internal", "message": err.Error()})
			return
		}

		// Return 202 Accepted with operation location (LRO pattern §17.1)
		opID := uuid.New().String()
		c.Header("Location", "/api/v1/tenants/"+tenant+"/data-resources/"+dr.Name)
		c.Header("X-Operation-ID", opID)
		c.JSON(http.StatusAccepted, gin.H{
			"name":        dr.Name,
			"id":          dr.ID,
			"tenant":      tenant,
			"type":        dr.Type,
			"class":       dr.Class,
			"phase":       dr.Phase,
			"operationId": opID,
		})
	}
}

// GetDataResource handles GET /api/v1/tenants/:tenantId/data-resources/:name
func GetDataResource(s store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenant := c.Param("tenantId")
		name := c.Param("name")
		if t := middleware.GetTenant(c); t != tenant {
			c.JSON(http.StatusForbidden, gin.H{"code": "nest.auth.tenant_mismatch", "message": "tenant mismatch"})
			return
		}
		dr, err := s.GetDataResource(c.Request.Context(), tenant, name)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": "nest.dataresource.not_found", "message": "DataResource not found"})
			return
		}
		c.JSON(http.StatusOK, dr)
	}
}

// DeleteDataResource handles DELETE /api/v1/tenants/:tenantId/data-resources/:name
func DeleteDataResource(s store.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		tenant := c.Param("tenantId")
		name := c.Param("name")
		if t := middleware.GetTenant(c); t != tenant {
			c.JSON(http.StatusForbidden, gin.H{"code": "nest.auth.tenant_mismatch", "message": "tenant mismatch"})
			return
		}
		if err := s.DeleteDataResource(c.Request.Context(), tenant, name); err != nil {
			c.JSON(http.StatusNotFound, gin.H{"code": "nest.dataresource.not_found", "message": "DataResource not found"})
			return
		}
		c.Status(http.StatusNoContent)
	}
}

type CreateDataResourceRequest struct {
	Name        string   `json:"name" binding:"required"`
	Type        string   `json:"type" binding:"required"`
	Class       string   `json:"class" binding:"required"`
	HA          bool     `json:"ha"`
	Protocols   []string `json:"protocols"`
	Origination string   `json:"origination"`
	Size        *struct {
		Storage string `json:"storage"`
	} `json:"size"`
}
