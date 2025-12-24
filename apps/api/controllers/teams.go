package controllers

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/penguintechinc/project-template/shared/licensing"
	"gorm.io/gorm"
)

// CreateTeamRequest represents the request body for creating a team
type CreateTeamRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=255"`
	Description string `json:"description" binding:"max=1000"`
}

// UpdateTeamRequest represents the request body for updating a team
type UpdateTeamRequest struct {
	Name        string `json:"name" binding:"required,min=1,max=255"`
	Description string `json:"description" binding:"max=1000"`
}

// AddMemberRequest represents the request body for adding a team member
type AddMemberRequest struct {
	UserID uint   `json:"user_id" binding:"required"`
	Role   string `json:"role" binding:"required,oneof=team_admin team_maintainer team_viewer"`
}

// TeamResponse represents a team response
type TeamResponse struct {
	ID          uint                 `json:"id"`
	Name        string               `json:"name"`
	Description string               `json:"description"`
	IsGlobal    bool                 `json:"is_global"`
	CreatedAt   string               `json:"created_at"`
	UpdatedAt   string               `json:"updated_at"`
	Members     []TeamMemberResponse `json:"members,omitempty"`
}

// TeamMemberResponse represents a team member response
type TeamMemberResponse struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
}

// TeamsController handles team operations
type TeamsController struct {
	db *gorm.DB
}

// NewTeamsController creates a new teams controller
func NewTeamsController(database *database.Database) *TeamsController {
	return &TeamsController{
		db: database.DB,
	}
}

// ListTeams retrieves all teams (scoped by user permissions)
// GET /api/v1/teams
func (tc *TeamsController) ListTeams(c *gin.Context) {
	userCtx, err := licensing.GetUserContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "User context not found",
		})
		return
	}

	var teams []Team
	query := tc.db

	// Non-global-admins only see teams they're members of
	if userCtx.Role != "global_admin" {
		query = query.Joins("INNER JOIN team_members ON teams.id = team_members.team_id").
			Where("team_members.user_id = ?", userCtx.UserID).
			Group("teams.id")
	}

	if err := query.Preload("Members").Find(&teams).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to retrieve teams",
		})
		return
	}

	responses := make([]TeamResponse, len(teams))
	for i, team := range teams {
		responses[i] = teamToResponse(team)
	}

	c.JSON(http.StatusOK, gin.H{
		"teams": responses,
		"count": len(responses),
	})
}

// GetTeam retrieves a specific team
// GET /api/v1/teams/:id
func (tc *TeamsController) GetTeam(c *gin.Context) {
	teamID, err := parseTeamID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_team_id",
			"message": "Team ID must be a valid number",
		})
		return
	}

	userCtx, err := licensing.GetUserContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "User context not found",
		})
		return
	}

	var team Team
	if err := tc.db.Preload("Members").First(&team, teamID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Team not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to retrieve team",
		})
		return
	}

	// Check user has access to this team
	if !userCtx.IsGlobalAdmin() && !userIsMemberOfTeam(tc.db, teamID, userCtx.UserID) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "insufficient_permissions",
			"message": "User does not have access to this team",
		})
		return
	}

	c.JSON(http.StatusOK, teamToResponse(team))
}

// CreateTeam creates a new team (GlobalAdmin only)
// POST /api/v1/teams
func (tc *TeamsController) CreateTeam(c *gin.Context) {
	userCtx, err := licensing.GetUserContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "User context not found",
		})
		return
	}

	if !userCtx.IsGlobalAdmin() {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "insufficient_permissions",
			"message": "Only global admins can create teams",
		})
		return
	}

	var req CreateTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": err.Error(),
		})
		return
	}

	// Check if team name already exists
	var existingTeam Team
	if err := tc.db.Where("name = ?", req.Name).First(&existingTeam).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "duplicate_name",
			"message": "Team name already exists",
		})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to check team name uniqueness",
		})
		return
	}

	team := Team{
		Name:        req.Name,
		Description: req.Description,
		IsGlobal:    false,
	}

	if err := tc.db.Create(&team).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to create team",
		})
		return
	}

	c.JSON(http.StatusCreated, teamToResponse(team))
}

// UpdateTeam updates a team (TeamAdmin or GlobalAdmin)
// PUT /api/v1/teams/:id
func (tc *TeamsController) UpdateTeam(c *gin.Context) {
	teamID, err := parseTeamID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_team_id",
			"message": "Team ID must be a valid number",
		})
		return
	}

	userCtx, err := licensing.GetUserContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "User context not found",
		})
		return
	}

	var team Team
	if err := tc.db.First(&team, teamID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Team not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to retrieve team",
		})
		return
	}

	// Check permissions
	if !userCtx.IsGlobalAdmin() {
		if !userIsTeamAdminOfTeam(tc.db, teamID, userCtx.UserID) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "insufficient_permissions",
				"message": "User does not have admin rights in this team",
			})
			return
		}
	}

	var req UpdateTeamRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": err.Error(),
		})
		return
	}

	// Check if new name conflicts with existing team (excluding current team)
	if req.Name != team.Name {
		var existingTeam Team
		if err := tc.db.Where("name = ? AND id != ?", req.Name, teamID).First(&existingTeam).Error; err == nil {
			c.JSON(http.StatusConflict, gin.H{
				"error":   "duplicate_name",
				"message": "Team name already exists",
			})
			return
		} else if !errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusInternalServerError, gin.H{
				"error":   "database_error",
				"message": "Failed to check team name uniqueness",
			})
			return
		}
	}

	// Update team fields
	team.Name = req.Name
	team.Description = req.Description

	if err := tc.db.Save(&team).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to update team",
		})
		return
	}

	c.JSON(http.StatusOK, teamToResponse(team))
}

// DeleteTeam deletes a team (GlobalAdmin only)
// DELETE /api/v1/teams/:id
func (tc *TeamsController) DeleteTeam(c *gin.Context) {
	teamID, err := parseTeamID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_team_id",
			"message": "Team ID must be a valid number",
		})
		return
	}

	userCtx, err := licensing.GetUserContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "User context not found",
		})
		return
	}

	if !userCtx.IsGlobalAdmin() {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "insufficient_permissions",
			"message": "Only global admins can delete teams",
		})
		return
	}

	var team Team
	if err := tc.db.First(&team, teamID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Team not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to retrieve team",
		})
		return
	}

	// Prevent deletion of global team
	if team.IsGlobal {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "cannot_delete_global",
			"message": "Cannot delete the global team",
		})
		return
	}

	// Delete associated team members
	if err := tc.db.Where("team_id = ?", teamID).Delete(&TeamMember{}).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to delete team members",
		})
		return
	}

	// Delete team
	if err := tc.db.Delete(&team).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to delete team",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Team deleted successfully",
	})
}

// ListTeamMembers lists all members of a team
// GET /api/v1/teams/:id/members
func (tc *TeamsController) ListTeamMembers(c *gin.Context) {
	teamID, err := parseTeamID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_team_id",
			"message": "Team ID must be a valid number",
		})
		return
	}

	userCtx, err := licensing.GetUserContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "User context not found",
		})
		return
	}

	// Check if team exists
	var team Team
	if err := tc.db.First(&team, teamID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Team not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to retrieve team",
		})
		return
	}

	// Check user has access to this team
	if !userCtx.IsGlobalAdmin() && !userIsMemberOfTeam(tc.db, teamID, userCtx.UserID) {
		c.JSON(http.StatusForbidden, gin.H{
			"error":   "insufficient_permissions",
			"message": "User does not have access to this team",
		})
		return
	}

	var members []TeamMember
	if err := tc.db.Preload("User").Where("team_id = ?", teamID).Find(&members).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to retrieve team members",
		})
		return
	}

	responses := make([]TeamMemberResponse, len(members))
	for i, member := range members {
		responses[i] = teamMemberToResponse(member)
	}

	c.JSON(http.StatusOK, gin.H{
		"members": responses,
		"count":   len(responses),
	})
}

// AddTeamMember adds a member to a team (TeamAdmin or GlobalAdmin)
// POST /api/v1/teams/:id/members
func (tc *TeamsController) AddTeamMember(c *gin.Context) {
	teamID, err := parseTeamID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_team_id",
			"message": "Team ID must be a valid number",
		})
		return
	}

	userCtx, err := licensing.GetUserContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "User context not found",
		})
		return
	}

	// Check if team exists
	var team Team
	if err := tc.db.First(&team, teamID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Team not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to retrieve team",
		})
		return
	}

	// Check permissions
	if !userCtx.IsGlobalAdmin() {
		if !userIsTeamAdminOfTeam(tc.db, teamID, userCtx.UserID) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "insufficient_permissions",
				"message": "User does not have admin rights in this team",
			})
			return
		}
	}

	var req AddMemberRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_request",
			"message": err.Error(),
		})
		return
	}

	// Check if user exists
	var user User
	if err := tc.db.First(&user, req.UserID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "User not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to retrieve user",
		})
		return
	}

	// Check if user is already a member
	var existingMember TeamMember
	if err := tc.db.Where("team_id = ? AND user_id = ?", teamID, req.UserID).First(&existingMember).Error; err == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error":   "already_member",
			"message": "User is already a member of this team",
		})
		return
	} else if !errors.Is(err, gorm.ErrRecordNotFound) {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to check membership status",
		})
		return
	}

	member := TeamMember{
		TeamID: teamID,
		UserID: req.UserID,
		Role:   req.Role,
	}

	if err := tc.db.Create(&member).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to add team member",
		})
		return
	}

	c.JSON(http.StatusCreated, teamMemberToResponse(member))
}

// RemoveTeamMember removes a member from a team (TeamAdmin or GlobalAdmin)
// DELETE /api/v1/teams/:id/members/:user_id
func (tc *TeamsController) RemoveTeamMember(c *gin.Context) {
	teamID, err := parseTeamID(c)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_team_id",
			"message": "Team ID must be a valid number",
		})
		return
	}

	userIDStr := c.Param("user_id")
	userID, err := strconv.ParseUint(userIDStr, 10, 32)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error":   "invalid_user_id",
			"message": "User ID must be a valid number",
		})
		return
	}

	userCtx, err := licensing.GetUserContext(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":   "unauthorized",
			"message": "User context not found",
		})
		return
	}

	// Check if team exists
	var team Team
	if err := tc.db.First(&team, teamID).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Team not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to retrieve team",
		})
		return
	}

	// Check permissions
	if !userCtx.IsGlobalAdmin() {
		if !userIsTeamAdminOfTeam(tc.db, teamID, userCtx.UserID) {
			c.JSON(http.StatusForbidden, gin.H{
				"error":   "insufficient_permissions",
				"message": "User does not have admin rights in this team",
			})
			return
		}
	}

	// Check if member exists
	var member TeamMember
	if err := tc.db.Where("team_id = ? AND user_id = ?", teamID, uint(userID)).First(&member).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error":   "not_found",
				"message": "Team member not found",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to retrieve team member",
		})
		return
	}

	if err := tc.db.Delete(&member).Error; err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error":   "database_error",
			"message": "Failed to remove team member",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"message": "Team member removed successfully",
	})
}

// Helper functions

func teamToResponse(team Team) TeamResponse {
	members := make([]TeamMemberResponse, len(team.Members))
	for i, member := range team.Members {
		members[i] = teamMemberToResponse(member)
	}

	return TeamResponse{
		ID:          team.ID,
		Name:        team.Name,
		Description: team.Description,
		IsGlobal:    team.IsGlobal,
		CreatedAt:   team.CreatedAt.Format("2006-01-02T15:04:05Z07:00"),
		UpdatedAt:   team.UpdatedAt.Format("2006-01-02T15:04:05Z07:00"),
		Members:     members,
	}
}

func teamMemberToResponse(member TeamMember) TeamMemberResponse {
	return TeamMemberResponse{
		UserID:   member.UserID,
		Username: member.User.Username,
		Email:    member.User.Email,
		Role:     member.Role,
	}
}

func parseTeamID(c *gin.Context) (uint, error) {
	teamIDStr := c.Param("id")
	teamID, err := strconv.ParseUint(teamIDStr, 10, 32)
	if err != nil {
		return 0, err
	}
	return uint(teamID), nil
}

func userIsMemberOfTeam(db *gorm.DB, teamID uint, userID uint) bool {
	var count int64
	db.Model(&TeamMember{}).
		Where("team_id = ? AND user_id = ?", teamID, userID).
		Count(&count)
	return count > 0
}

func userIsTeamAdminOfTeam(db *gorm.DB, teamID uint, userID uint) bool {
	var count int64
	db.Model(&TeamMember{}).
		Where("team_id = ? AND user_id = ? AND role = ?", teamID, userID, "team_admin").
		Count(&count)
	return count > 0
}
