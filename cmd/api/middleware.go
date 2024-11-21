package api

import (
	"context"
	"net/http"

	"github.com/gin-gonic/gin"
)

// AccountInfoMiddleware retrieves the account information from the middleware service
func (a *ApiServer) AccountInfoMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.accountInfo.TechSpaceId == "" {
			accountInfo, err := a.mwInternal.GetAccountInfo(context.Background())
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to get account info"})
				return
			}
			a.accountInfo = *accountInfo
		}
		c.Next()
	}
}

// PortsMiddleware retrieves the open ports from the middleware service
func (a *ApiServer) PortsMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		if len(a.ports) == 0 {
			ports, err := getPorts()
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "Failed to get open ports"})
				return
			}
			a.ports = ports
		}
		c.Next()
	}
}

// TODO: AuthMiddleware to ensure the user is authenticated
func (a *ApiServer) AuthMiddleware() gin.HandlerFunc {
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

// TODO: PermissionMiddleware to ensure the user has the required permissions
func (a *ApiServer) PermissionMiddleware(requiredPermission string) gin.HandlerFunc {
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
