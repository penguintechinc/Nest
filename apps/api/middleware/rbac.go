package middleware

import (
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

// Role definitions (constants)
const (
	// Global roles - can access all teams
	GlobalAdmin  = "global_admin"
	GlobalViewer = "global_viewer"

	// Team-specific roles - can only access their teams
	TeamAdmin      = "team_admin"
	TeamMaintainer = "team_maintainer"
	TeamViewer     = "team_viewer"
)

// Permission types for resource access
const (
	PermissionRead   = "read"
	PermissionWrite  = "write"
	PermissionDelete = "delete"
	PermissionAdmin  = "admin"
)

// Context keys
const (
	UserIDKey   = "user_id"
	UserRoleKey = "user_role"
)

// Database models

// BaseModel provides common fields for all models
type BaseModel struct {
	ID        uint           `gorm:"primarykey" json:"id"`
	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"deleted_at,omitempty"`
}

// User represents a user in the system
type User struct {
	BaseModel
	Username     string           `gorm:"uniqueIndex;not null" json:"username"`
	Email        string           `gorm:"uniqueIndex;not null" json:"email"`
	PasswordHash string           `gorm:"not null" json:"-"`
	FirstName    string           `json:"first_name"`
	LastName     string           `json:"last_name"`
	GlobalRole   string           `gorm:"default:''" json:"global_role"` // GlobalAdmin, GlobalViewer, or empty
	IsActive     *bool            `gorm:"default:true" json:"is_active"` // Pointer to bool to preserve false values (GORM skips bool zero-values)
	LastLoginAt  *time.Time       `json:"last_login_at,omitempty"`
	Memberships  []TeamMembership `gorm:"foreignKey:UserID" json:"memberships,omitempty"`
}

// Team represents a team/organization
type Team struct {
	BaseModel
	Name        string           `gorm:"uniqueIndex;not null" json:"name"`
	Description string           `json:"description"`
	IsActive    *bool            `gorm:"default:true" json:"is_active"` // Pointer to bool to preserve false values (GORM skips bool zero-values)
	Memberships []TeamMembership `gorm:"foreignKey:TeamID" json:"memberships,omitempty"`
	Resources   []Resource       `gorm:"foreignKey:TeamID" json:"resources,omitempty"`
}

// TeamMembership represents a user's membership in a team with a specific role
type TeamMembership struct {
	BaseModel
	UserID uint   `gorm:"not null;index:idx_user_team,unique" json:"user_id"`
	TeamID uint   `gorm:"not null;index:idx_user_team,unique" json:"team_id"`
	Role   string `gorm:"not null" json:"role"` // TeamAdmin, TeamMaintainer, TeamViewer
	User   User   `gorm:"foreignKey:UserID" json:"user,omitempty"`
	Team   Team   `gorm:"foreignKey:TeamID" json:"team,omitempty"`
}

// Resource represents a resource that belongs to a team
type Resource struct {
	BaseModel
	TeamID      uint   `gorm:"not null;index" json:"team_id"`
	Name        string `gorm:"not null" json:"name"`
	Type        string `gorm:"not null" json:"type"`
	Description string `json:"description"`
	IsActive    *bool  `gorm:"default:true" json:"is_active"` // Pointer to bool to preserve false values (GORM skips bool zero-values)
	Team        Team   `gorm:"foreignKey:TeamID" json:"team,omitempty"`
}

// RBACMiddleware holds the database connection for RBAC operations
type RBACMiddleware struct {
	db *gorm.DB
}

// NewRBACMiddleware creates a new RBAC middleware instance
func NewRBACMiddleware(db *gorm.DB) *RBACMiddleware {
	return &RBACMiddleware{db: db}
}

// RequireAuth checks if the user is authenticated
// Sets user_id in the context if authentication succeeds
func (r *RBACMiddleware) RequireAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Extract user ID from authentication token/session
		// This is a placeholder - implement your actual authentication logic
		userIDStr := c.GetHeader("X-User-ID")
		if userIDStr == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
			})
			c.Abort()
			return
		}

		userID, err := strconv.ParseUint(userIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Invalid user ID",
			})
			c.Abort()
			return
		}

		// Verify user exists and is active
		var user User
		if err := r.db.First(&user, userID).Error; err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "User not found",
			})
			c.Abort()
			return
		}

		// Explicitly reject any user who is not active (IsActive must be true)
		// IsActive is *bool, so check both nil and false cases
		if user.IsActive == nil || !*user.IsActive {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "User account is inactive",
			})
			c.Abort()
			return
		}

		// Set user ID and role in context
		c.Set(UserIDKey, uint(userID))
		c.Set(UserRoleKey, user.GlobalRole)
		c.Next()
	}
}

// RequireRole requires the user to have one of the specified roles
func (r *RBACMiddleware) RequireRole(roles ...string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get(UserIDKey)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
			})
			c.Abort()
			return
		}

		uid := userID.(uint)
		userRoles, err := r.getUserRoles(uid)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to retrieve user roles",
			})
			c.Abort()
			return
		}

		// Check if user has any of the required roles
		for _, requiredRole := range roles {
			for _, userRole := range userRoles {
				if userRole == requiredRole {
					c.Next()
					return
				}
			}
		}

		c.JSON(http.StatusForbidden, gin.H{
			"error": "Insufficient permissions",
		})
		c.Abort()
	}
}

// RequireTeamAccess checks if the user has access to the specified team with the given permission
// Expects team_id as a URL parameter
func (r *RBACMiddleware) RequireTeamAccess(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get(UserIDKey)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
			})
			c.Abort()
			return
		}

		teamIDStr := c.Param("team_id")
		if teamIDStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Team ID required",
			})
			c.Abort()
			return
		}

		teamID, err := strconv.ParseUint(teamIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid team ID",
			})
			c.Abort()
			return
		}

		uid := userID.(uint)
		hasAccess, err := r.checkTeamPermission(uid, uint(teamID), permission)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to check permissions",
			})
			c.Abort()
			return
		}

		if !hasAccess {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Insufficient team permissions",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// CheckResourceAccess verifies user can access a specific resource
// Expects resource_id as a URL parameter
func (r *RBACMiddleware) CheckResourceAccess(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID, exists := c.Get(UserIDKey)
		if !exists {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "Authentication required",
			})
			c.Abort()
			return
		}

		resourceIDStr := c.Param("resource_id")
		if resourceIDStr == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Resource ID required",
			})
			c.Abort()
			return
		}

		resourceID, err := strconv.ParseUint(resourceIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "Invalid resource ID",
			})
			c.Abort()
			return
		}

		uid := userID.(uint)
		hasAccess, err := r.canAccessResource(uid, uint(resourceID), permission)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error": "Failed to check resource access",
			})
			c.Abort()
			return
		}

		if !hasAccess {
			c.JSON(http.StatusForbidden, gin.H{
				"error": "Insufficient permissions to access this resource",
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// Helper functions

// getUserRoles retrieves all roles for a user (global + team roles)
func (r *RBACMiddleware) getUserRoles(userID uint) ([]string, error) {
	roles := make([]string, 0)

	// Get user's global role
	var user User
	if err := r.db.First(&user, userID).Error; err != nil {
		return nil, err
	}

	if user.GlobalRole != "" {
		roles = append(roles, user.GlobalRole)
	}

	// Get user's team roles
	var memberships []TeamMembership
	if err := r.db.Where("user_id = ?", userID).Find(&memberships).Error; err != nil {
		return nil, err
	}

	for _, membership := range memberships {
		roles = append(roles, membership.Role)
	}

	return roles, nil
}

// hasGlobalRole checks if user has a specific global role
func (r *RBACMiddleware) hasGlobalRole(userID uint, role string) (bool, error) {
	var user User
	if err := r.db.First(&user, userID).Error; err != nil {
		return false, err
	}

	return user.GlobalRole == role, nil
}

// hasTeamRole checks if user has a specific role in a team
func (r *RBACMiddleware) hasTeamRole(userID uint, teamID uint, role string) (bool, error) {
	var membership TeamMembership
	err := r.db.Where("user_id = ? AND team_id = ? AND role = ?", userID, teamID, role).
		First(&membership).Error

	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	return true, nil
}

// checkTeamPermission checks if user has permission to perform action on team
func (r *RBACMiddleware) checkTeamPermission(userID uint, teamID uint, permission string) (bool, error) {
	// Check if user has global admin role (can access all teams)
	hasGlobalAdmin, err := r.hasGlobalRole(userID, GlobalAdmin)
	if err != nil {
		return false, err
	}
	if hasGlobalAdmin {
		return true, nil
	}

	// Check if user has global viewer role (read-only access to all teams)
	hasGlobalViewer, err := r.hasGlobalRole(userID, GlobalViewer)
	if err != nil {
		return false, err
	}
	if hasGlobalViewer && permission == PermissionRead {
		return true, nil
	}

	// Check team-specific role
	var membership TeamMembership
	err = r.db.Where("user_id = ? AND team_id = ?", userID, teamID).First(&membership).Error
	if err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	// Apply permission matrix
	return r.hasPermission(membership.Role, permission), nil
}

// hasPermission checks if a role has a specific permission
func (r *RBACMiddleware) hasPermission(role string, permission string) bool {
	// Permission matrix
	switch role {
	case GlobalAdmin:
		// Global admin has all permissions
		return true

	case GlobalViewer:
		// Global viewer has read-only access
		return permission == PermissionRead

	case TeamAdmin:
		// Team admin has all permissions on team resources
		return true

	case TeamMaintainer:
		// Team maintainer can read, write, and delete resources but not manage members
		return permission == PermissionRead || permission == PermissionWrite || permission == PermissionDelete

	case TeamViewer:
		// Team viewer has read-only access
		return permission == PermissionRead

	default:
		return false
	}
}

// canAccessResource verifies if user can access a specific resource
func (r *RBACMiddleware) canAccessResource(userID uint, resourceID uint, permission string) (bool, error) {
	// Get resource to find its team
	var resource Resource
	if err := r.db.First(&resource, resourceID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			return false, nil
		}
		return false, err
	}

	// Check team permission for the resource's team
	return r.checkTeamPermission(userID, resource.TeamID, permission)
}

// GetUserTeams returns all teams the user has access to
func (r *RBACMiddleware) GetUserTeams(userID uint) ([]Team, error) {
	// Check if user has global role
	hasGlobalAdmin, err := r.hasGlobalRole(userID, GlobalAdmin)
	if err != nil {
		return nil, err
	}

	hasGlobalViewer, err := r.hasGlobalRole(userID, GlobalViewer)
	if err != nil {
		return nil, err
	}

	var teams []Team

	// Global roles can access all teams
	if hasGlobalAdmin || hasGlobalViewer {
		if err := r.db.Find(&teams).Error; err != nil {
			return nil, err
		}
		return teams, nil
	}

	// Get teams through memberships
	var memberships []TeamMembership
	if err := r.db.Where("user_id = ?", userID).Preload("Team").Find(&memberships).Error; err != nil {
		return nil, err
	}

	for _, membership := range memberships {
		teams = append(teams, membership.Team)
	}

	return teams, nil
}

// CanManageTeamMembers checks if user can manage team members
func (r *RBACMiddleware) CanManageTeamMembers(userID uint, teamID uint) (bool, error) {
	// Check if user has global admin role
	hasGlobalAdmin, err := r.hasGlobalRole(userID, GlobalAdmin)
	if err != nil {
		return false, err
	}
	if hasGlobalAdmin {
		return true, nil
	}

	// Only team admins can manage members
	return r.hasTeamRole(userID, teamID, TeamAdmin)
}

// Migrate runs database migrations for RBAC models
func (r *RBACMiddleware) Migrate() error {
	return r.db.AutoMigrate(&User{}, &Team{}, &TeamMembership{}, &Resource{})
}
