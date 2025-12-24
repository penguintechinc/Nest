package middleware

import (
	"fmt"
	"net/http"
	"os"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/golang-jwt/jwt/v5"
	"github.com/penguintechinc/project-template/apps/api/models"
)

const (
	// ContextKeyUser is the key for storing user in Gin context
	ContextKeyUser = "user"

	// AuthorizationHeader is the name of the authorization header
	AuthorizationHeader = "Authorization"

	// BearerScheme is the bearer token scheme
	BearerScheme = "Bearer"
)

// CustomClaims represents the JWT claims structure
type CustomClaims struct {
	UserID   uint   `json:"user_id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	jwt.RegisteredClaims
}

// AuthMiddleware validates JWT token from Authorization header and stores user in context
func AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from Authorization header
		authHeader := c.GetHeader(AuthorizationHeader)
		if authHeader == "" {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "missing authorization header",
			})
			c.Abort()
			return
		}

		// Parse Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != BearerScheme {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid authorization header format",
			})
			c.Abort()
			return
		}

		tokenString := parts[1]

		// Parse and validate JWT
		claims := &CustomClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			// Validate the signing method
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			jwtSecret := os.Getenv("JWT_SECRET")
			if jwtSecret == "" {
				return nil, fmt.Errorf("JWT_SECRET environment variable not set")
			}
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			c.JSON(http.StatusUnauthorized, gin.H{
				"error": "invalid or expired token",
			})
			c.Abort()
			return
		}

		// Extract user claims
		user := &models.UserClaims{
			UserID:   claims.UserID,
			Username: claims.Username,
			Email:    claims.Email,
		}

		// Store user in context
		c.Set(ContextKeyUser, user)
		c.Next()
	}
}

// OptionalAuthMiddleware validates JWT token if present, but does not require it
func OptionalAuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Get token from Authorization header
		authHeader := c.GetHeader(AuthorizationHeader)
		if authHeader == "" {
			c.Next()
			return
		}

		// Parse Bearer token
		parts := strings.SplitN(authHeader, " ", 2)
		if len(parts) != 2 || parts[0] != BearerScheme {
			c.Next()
			return
		}

		tokenString := parts[1]

		// Parse and validate JWT
		claims := &CustomClaims{}
		token, err := jwt.ParseWithClaims(tokenString, claims, func(token *jwt.Token) (interface{}, error) {
			if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
				return nil, fmt.Errorf("unexpected signing method: %v", token.Header["alg"])
			}
			jwtSecret := os.Getenv("JWT_SECRET")
			if jwtSecret == "" {
				return nil, fmt.Errorf("JWT_SECRET environment variable not set")
			}
			return []byte(jwtSecret), nil
		})

		if err != nil || !token.Valid {
			c.Next()
			return
		}

		// Extract user claims
		user := &models.UserClaims{
			UserID:   claims.UserID,
			Username: claims.Username,
			Email:    claims.Email,
		}

		// Store user in context
		c.Set(ContextKeyUser, user)
		c.Next()
	}
}

// GetUserClaims retrieves user claims from context
func GetUserClaims(c *gin.Context) (*models.UserClaims, error) {
	user, exists := c.Get(ContextKeyUser)
	if !exists {
		return nil, fmt.Errorf("user not found in context")
	}

	claims, ok := user.(*models.UserClaims)
	if !ok {
		return nil, fmt.Errorf("invalid user claims type")
	}

	return claims, nil
}
