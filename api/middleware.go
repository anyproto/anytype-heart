package api

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Middleware to authenticate requests and add user info to context
func AuthMiddleware() gin.HandlerFunc {
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

// Middleware to check permissions
func PermissionMiddleware(requiredPermission string) gin.HandlerFunc {
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
