package licensing

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
)

// UserContext represents the authenticated user in the request context
type UserContext struct {
	UserID   uint
	Username string
	Email    string
	Role     string // global_admin, team_admin, user, etc.
}

// IsGlobalAdmin checks if the user is a global admin
func (uc *UserContext) IsGlobalAdmin() bool {
	return uc.Role == "global_admin"
}

// RequireRole middleware checks if user has the required role
func RequireRole(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userCtx, err := GetUserContext(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "User context not found",
			})
			c.Abort()
			return
		}

		if !HasRole(userCtx.Role, requiredRole) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":    "insufficient_permissions",
				"message":  "User does not have required role",
				"required": requiredRole,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// RequireTeamRole middleware checks if user has the required role in a specific team
func RequireTeamRole(requiredRole string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userCtx, err := GetUserContext(c)
		if err != nil {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error":   "unauthorized",
				"message": "User context not found",
			})
			c.Abort()
			return
		}

		// Global admins can access anything
		if userCtx.Role == "global_admin" {
			c.Next()
			return
		}

		teamIDStr := c.Param("id")
		teamID, err := strconv.ParseUint(teamIDStr, 10, 32)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error":   "invalid_team_id",
				"message": "Team ID must be a valid number",
			})
			c.Abort()
			return
		}

		// Check if user has required role in team
		hasAccess, err := UserHasTeamRole(c, uint(teamID), userCtx.UserID, requiredRole)
		if err != nil || !hasAccess {
			c.JSON(http.StatusForbidden, gin.H{
				"error":    "insufficient_permissions",
				"message":  "User does not have required role in team",
				"required": requiredRole,
			})
			c.Abort()
			return
		}

		c.Next()
	}
}

// SetUserContext sets the user context in the request
func SetUserContext(c *gin.Context, userCtx *UserContext) {
	c.Set("user_context", userCtx)
}

// GetUserContext retrieves the user context from the request
func GetUserContext(c *gin.Context) (*UserContext, error) {
	userCtx, exists := c.Get("user_context")
	if !exists {
		return nil, errors.New("user context not found")
	}

	ctx, ok := userCtx.(*UserContext)
	if !ok {
		return nil, errors.New("invalid user context type")
	}

	return ctx, nil
}

// HasRole checks if a user role satisfies a required role
func HasRole(userRole, requiredRole string) bool {
	// Role hierarchy: global_admin > team_admin > team_maintainer > team_viewer
	roleHierarchy := map[string]int{
		"global_admin":    4,
		"team_admin":      3,
		"team_maintainer": 2,
		"team_viewer":     1,
	}

	userLevel, userExists := roleHierarchy[userRole]
	requiredLevel, requiredExists := roleHierarchy[requiredRole]

	if !userExists || !requiredExists {
		return userRole == requiredRole
	}

	return userLevel >= requiredLevel
}

// UserHasTeamRole checks if a user has a specific role in a team
// Note: This is a placeholder that should be implemented with database queries
func UserHasTeamRole(c *gin.Context, teamID uint, userID uint, requiredRole string) (bool, error) {
	// This should be implemented with actual database query
	// For now, returning false as placeholder
	return false, errors.New("not implemented")
}
