package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"gorm.io/driver/sqlite"
	"gorm.io/gorm"
)

// setupTestDB creates an in-memory SQLite database for testing
func setupTestDB() (*gorm.DB, error) {
	db, err := gorm.Open(sqlite.Open(":memory:"), &gorm.Config{})
	if err != nil {
		return nil, err
	}

	// Auto-migrate models
	if err := db.AutoMigrate(&User{}, &Team{}, &TeamMembership{}, &Resource{}); err != nil {
		return nil, err
	}

	return db, nil
}

// setupTestData creates test users, teams, and memberships
func setupTestData(db *gorm.DB) error {
	// Create users
	activeTrue := true
	activeFalse := false
	users := []User{
		{Username: "admin", Email: "admin@test.com", GlobalRole: GlobalAdmin, IsActive: &activeTrue},
		{Username: "viewer", Email: "viewer@test.com", GlobalRole: GlobalViewer, IsActive: &activeTrue},
		{Username: "team_admin", Email: "teamadmin@test.com", GlobalRole: "", IsActive: &activeTrue},
		{Username: "maintainer", Email: "maintainer@test.com", GlobalRole: "", IsActive: &activeTrue},
		{Username: "team_viewer", Email: "teamviewer@test.com", GlobalRole: "", IsActive: &activeTrue},
		{Username: "inactive", Email: "inactive@test.com", GlobalRole: "", IsActive: &activeFalse},
	}

	for _, user := range users {
		if err := db.Create(&user).Error; err != nil {
			return err
		}
	}

	// Create teams
	teams := []Team{
		{Name: "Team Alpha", Description: "Test team alpha", IsActive: &activeTrue},
		{Name: "Team Beta", Description: "Test team beta", IsActive: &activeTrue},
	}

	for _, team := range teams {
		if err := db.Create(&team).Error; err != nil {
			return err
		}
	}

	// Create team memberships
	memberships := []TeamMembership{
		{UserID: 3, TeamID: 1, Role: TeamAdmin},      // team_admin in Team Alpha
		{UserID: 4, TeamID: 1, Role: TeamMaintainer}, // maintainer in Team Alpha
		{UserID: 5, TeamID: 1, Role: TeamViewer},     // team_viewer in Team Alpha
		{UserID: 5, TeamID: 2, Role: TeamViewer},     // team_viewer in Team Beta
	}

	for _, membership := range memberships {
		if err := db.Create(&membership).Error; err != nil {
			return err
		}
	}

	// Create resources
	resources := []Resource{
		{TeamID: 1, Name: "Resource 1", Type: "server", Description: "Test resource 1", IsActive: &activeTrue},
		{TeamID: 1, Name: "Resource 2", Type: "database", Description: "Test resource 2", IsActive: &activeTrue},
		{TeamID: 2, Name: "Resource 3", Type: "server", Description: "Test resource 3", IsActive: &activeTrue},
	}

	for _, resource := range resources {
		if err := db.Create(&resource).Error; err != nil {
			return err
		}
	}

	return nil
}

// TestRequireAuth tests the authentication middleware
func TestRequireAuth(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatal("Failed to setup test database:", err)
	}

	if err := setupTestData(db); err != nil {
		t.Fatal("Failed to setup test data:", err)
	}

	rbac := NewRBACMiddleware(db)
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		userID         string
		expectedStatus int
	}{
		{"Valid user", "1", http.StatusOK},
		{"Invalid user ID", "invalid", http.StatusUnauthorized},
		{"Non-existent user", "999", http.StatusUnauthorized},
		{"Inactive user", "6", http.StatusForbidden},
		{"Missing auth", "", http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(rbac.RequireAuth())
			router.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"status": "ok"})
			})

			req, _ := http.NewRequest("GET", "/test", nil)
			if tt.userID != "" {
				req.Header.Set("X-User-ID", tt.userID)
			}

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// TestRequireRole tests role-based access control
func TestRequireRole(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatal("Failed to setup test database:", err)
	}

	if err := setupTestData(db); err != nil {
		t.Fatal("Failed to setup test data:", err)
	}

	rbac := NewRBACMiddleware(db)
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		userID         string
		requiredRoles  []string
		expectedStatus int
	}{
		{"Global admin access", "1", []string{GlobalAdmin}, http.StatusOK},
		{"Global viewer access", "2", []string{GlobalViewer}, http.StatusOK},
		{"Team admin with team role", "3", []string{TeamAdmin}, http.StatusOK},
		{"Insufficient role", "5", []string{TeamAdmin}, http.StatusForbidden},
		{"Multiple roles - has one", "2", []string{GlobalAdmin, GlobalViewer}, http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(rbac.RequireAuth())
			router.Use(rbac.RequireRole(tt.requiredRoles...))
			router.GET("/test", func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"status": "ok"})
			})

			req, _ := http.NewRequest("GET", "/test", nil)
			req.Header.Set("X-User-ID", tt.userID)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// TestRequireTeamAccess tests team-specific access control
func TestRequireTeamAccess(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatal("Failed to setup test database:", err)
	}

	if err := setupTestData(db); err != nil {
		t.Fatal("Failed to setup test data:", err)
	}

	rbac := NewRBACMiddleware(db)
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		userID         string
		teamID         string
		permission     string
		expectedStatus int
	}{
		{"Global admin read access", "1", "1", PermissionRead, http.StatusOK},
		{"Global admin write access", "1", "1", PermissionWrite, http.StatusOK},
		{"Global viewer read access", "2", "1", PermissionRead, http.StatusOK},
		{"Global viewer write denied", "2", "1", PermissionWrite, http.StatusForbidden},
		{"Team admin full access", "3", "1", PermissionWrite, http.StatusOK},
		{"Team maintainer write access", "4", "1", PermissionWrite, http.StatusOK},
		{"Team maintainer admin denied", "4", "1", PermissionAdmin, http.StatusForbidden},
		{"Team viewer read access", "5", "1", PermissionRead, http.StatusOK},
		{"Team viewer write denied", "5", "1", PermissionWrite, http.StatusForbidden},
		{"No team membership", "5", "99", PermissionRead, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(rbac.RequireAuth())
			router.GET("/teams/:team_id", rbac.RequireTeamAccess(tt.permission), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"status": "ok"})
			})

			req, _ := http.NewRequest("GET", "/teams/"+tt.teamID, nil)
			req.Header.Set("X-User-ID", tt.userID)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// TestCheckResourceAccess tests resource-specific access control
func TestCheckResourceAccess(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatal("Failed to setup test database:", err)
	}

	if err := setupTestData(db); err != nil {
		t.Fatal("Failed to setup test data:", err)
	}

	rbac := NewRBACMiddleware(db)
	gin.SetMode(gin.TestMode)

	tests := []struct {
		name           string
		userID         string
		resourceID     string
		permission     string
		expectedStatus int
	}{
		{"Team admin access to team resource", "3", "1", PermissionRead, http.StatusOK},
		{"Team maintainer write to resource", "4", "1", PermissionWrite, http.StatusOK},
		{"Team viewer read resource", "5", "1", PermissionRead, http.StatusOK},
		{"Team viewer write denied", "5", "1", PermissionWrite, http.StatusForbidden},
		{"Access to other team resource denied", "3", "3", PermissionRead, http.StatusForbidden},
		{"Global admin access any resource", "1", "3", PermissionWrite, http.StatusOK},
		{"Non-existent resource", "1", "999", PermissionRead, http.StatusForbidden},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			router := gin.New()
			router.Use(rbac.RequireAuth())
			router.GET("/resources/:resource_id", rbac.CheckResourceAccess(tt.permission), func(c *gin.Context) {
				c.JSON(http.StatusOK, gin.H{"status": "ok"})
			})

			req, _ := http.NewRequest("GET", "/resources/"+tt.resourceID, nil)
			req.Header.Set("X-User-ID", tt.userID)

			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if w.Code != tt.expectedStatus {
				t.Errorf("Expected status %d, got %d", tt.expectedStatus, w.Code)
			}
		})
	}
}

// TestGetUserTeams tests the GetUserTeams helper function
func TestGetUserTeams(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatal("Failed to setup test database:", err)
	}

	if err := setupTestData(db); err != nil {
		t.Fatal("Failed to setup test data:", err)
	}

	rbac := NewRBACMiddleware(db)

	tests := []struct {
		name          string
		userID        uint
		expectedCount int
	}{
		{"Global admin sees all teams", 1, 2},
		{"Global viewer sees all teams", 2, 2},
		{"Team member sees their teams", 5, 2},
		{"Team admin sees one team", 3, 1},
		{"User with no teams", 6, 0},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			teams, err := rbac.GetUserTeams(tt.userID)
			if err != nil {
				t.Fatal("Failed to get user teams:", err)
			}

			if len(teams) != tt.expectedCount {
				t.Errorf("Expected %d teams, got %d", tt.expectedCount, len(teams))
			}
		})
	}
}

// TestCanManageTeamMembers tests member management permission checking
func TestCanManageTeamMembers(t *testing.T) {
	db, err := setupTestDB()
	if err != nil {
		t.Fatal("Failed to setup test database:", err)
	}

	if err := setupTestData(db); err != nil {
		t.Fatal("Failed to setup test data:", err)
	}

	rbac := NewRBACMiddleware(db)

	tests := []struct {
		name     string
		userID   uint
		teamID   uint
		expected bool
	}{
		{"Global admin can manage members", 1, 1, true},
		{"Team admin can manage members", 3, 1, true},
		{"Team maintainer cannot manage members", 4, 1, false},
		{"Team viewer cannot manage members", 5, 1, false},
		{"User not in team cannot manage", 3, 2, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			canManage, err := rbac.CanManageTeamMembers(tt.userID, tt.teamID)
			if err != nil {
				t.Fatal("Failed to check member management permission:", err)
			}

			if canManage != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, canManage)
			}
		})
	}
}
