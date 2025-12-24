package controllers

import (
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/penguintechinc/project-template/apps/api/middleware"
	"github.com/penguintechinc/project-template/apps/api/models"
	"gorm.io/gorm"
)

// LoginRequest represents a login request payload
type LoginRequest struct {
	Username string `json:"username" binding:"required"`
	Password string `json:"password" binding:"required"`
}

// LoginResponse represents a login response with JWT token
type LoginResponse struct {
	Token     string        `json:"token"`
	ExpiresAt int64         `json:"expires_at"`
	User      *UserResponse `json:"user"`
}

// UserResponse represents a user response
type UserResponse struct {
	ID       uint   `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
}

// LogoutResponse represents a logout response
type LogoutResponse struct {
	Message string `json:"message"`
}

// MeResponse represents current user response
type MeResponse struct {
	User *UserResponse `json:"user"`
}

// AuthController handles authentication endpoints
type AuthController struct {
	db *gorm.DB
}

// NewAuthController creates a new auth controller
func NewAuthController(db *gorm.DB) *AuthController {
	return &AuthController{
		db: db,
	}
}

// Login handles user login and returns JWT token
// POST /api/v1/auth/login
func (ac *AuthController) Login(c *gin.Context) {
	var req LoginRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	// Find user by username
	user := &models.User{}
	result := ac.db.Where("username = ?", req.Username).First(user)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid username or password",
			})
			return
		}
		log.Printf("Database error: %v", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal server error",
		})
		return
	}

	// Verify password
	if !user.VerifyPassword(req.Password) {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "invalid username or password",
		})
		return
	}

	// Generate JWT token
	expiresAt := time.Now().Add(24 * time.Hour)
	claims := middleware.CustomClaims{
		UserID:   user.ID,
		Username: user.Username,
		Email:    user.Email,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(expiresAt),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}

	jwtSecret := os.Getenv("JWT_SECRET")
	if jwtSecret == "" {
		log.Println("JWT_SECRET environment variable not set")
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal server error",
		})
		return
	}

	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	tokenString, err := token.SignedString([]byte(jwtSecret))
	if err != nil {
		log.Printf("Failed to sign token: %v", err)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to generate token",
		})
		return
	}

	// Return token and user info
	c.JSON(http.StatusOK, LoginResponse{
		Token:     tokenString,
		ExpiresAt: expiresAt.Unix(),
		User: &UserResponse{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
		},
	})
}

// Logout handles user logout
// POST /api/v1/auth/logout
func (ac *AuthController) Logout(c *gin.Context) {
	// Get user from context to verify authentication
	_, err := middleware.GetUserClaims(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "unauthorized",
		})
		return
	}

	// Logout is primarily client-side (token removal)
	// Server can log the logout event if needed
	c.JSON(http.StatusOK, LogoutResponse{
		Message: "successfully logged out",
	})
}

// Me returns the current authenticated user
// GET /api/v1/auth/me
func (ac *AuthController) Me(c *gin.Context) {
	// Get user claims from context
	userClaims, err := middleware.GetUserClaims(c)
	if err != nil {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error": "unauthorized",
		})
		return
	}

	// Fetch full user details from database
	user := &models.User{}
	result := ac.db.First(user, userClaims.UserID)
	if result.Error != nil {
		if result.Error == gorm.ErrRecordNotFound {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "user not found",
			})
			return
		}
		log.Printf("Database error: %v", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal server error",
		})
		return
	}

	// Return user info
	c.JSON(http.StatusOK, MeResponse{
		User: &UserResponse{
			ID:       user.ID,
			Username: user.Username,
			Email:    user.Email,
		},
	})
}

// CreateUser creates a new user (for testing/initial setup)
// POST /api/v1/auth/register
func (ac *AuthController) CreateUser(c *gin.Context) {
	var user models.User
	if err := c.ShouldBindJSON(&user); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
		})
		return
	}

	// Validate input
	if user.Username == "" || user.Email == "" || user.Password == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "username, email, and password are required",
		})
		return
	}

	// Check if user already exists
	existingUser := &models.User{}
	result := ac.db.Where("username = ? OR email = ?", user.Username, user.Email).First(existingUser)
	if result.Error == nil {
		c.JSON(http.StatusConflict, gin.H{
			"error": "username or email already exists",
		})
		return
	}
	if result.Error != gorm.ErrRecordNotFound {
		log.Printf("Database error: %v", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "internal server error",
		})
		return
	}

	// Create user
	result = ac.db.Create(&user)
	if result.Error != nil {
		log.Printf("Failed to create user: %v", result.Error)
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("failed to create user: %v", result.Error),
		})
		return
	}

	// Return created user (password not included)
	c.JSON(http.StatusCreated, UserResponse{
		ID:       user.ID,
		Username: user.Username,
		Email:    user.Email,
	})
}
