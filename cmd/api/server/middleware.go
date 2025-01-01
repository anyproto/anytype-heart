package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/anytype/account"
)

// TODO: User represents an authenticated user with permissions
type User struct {
	ID          string
	Permissions string // "read-only" or "read-write"
}

// initAccountInfo retrieves the account information from the account service.
func (s *Server) initAccountInfo() gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: consider not fetching account info on every request; currently used to avoid inconsistencies on logout/login
		app := s.mwInternal.GetApp()
		if app == nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "failed to get app instance"})
			return
		}

		accInfo, err := app.Component(account.CName).(account.Service).GetInfo(context.Background())
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get account info: %v", err)})
			return
		}

		s.objectService.AccountInfo = accInfo
		s.spaceService.AccountInfo = accInfo
		s.searchService.AccountInfo = accInfo
		c.Next()
	}
}

// TODO: AuthMiddleware ensures the user is authenticated.
func (s *Server) AuthMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		token := c.GetHeader("Authorization")
		if token == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		// TODO: Validate the token and retrieve user information; this is mock example
		user := &User{
			ID:          "user123",
			Permissions: "read-only", // or "read-only"
		}

		// Add the user to the context
		c.Set("user", user)
		c.Next()
	}
}

// TODO: PermissionMiddleware ensures the user has the required permissions.
func (s *Server) PermissionMiddleware(requiredPermission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		user, exists := c.Get("user")
		if !exists {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
			return
		}

		u := user.(*User)
		if requiredPermission == "read-write" && u.Permissions != "read-write" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "Forbidden: write access required"})
			return
		}

		// For read-only access, both "read-only" and "read-write" permissions are acceptable
		c.Next()
	}
}
