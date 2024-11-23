package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/anytype/account"
)

// initAccountInfo retrieves the account information from the account service
func (a *ApiServer) initAccountInfo() gin.HandlerFunc {
	return func(c *gin.Context) {
		if a.app == nil && a.accountInfo == nil {
			app := a.mwInternal.GetApp()
			if app == nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "failed to get app instance"})
				return
			}

			accInfo, err := app.Component(account.CName).(account.Service).GetInfo(context.Background())
			if err != nil {
				c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("failed to get account info: %v", err)})
				return
			}

			a.app = app
			a.accountInfo = accInfo
			c.Next()
		}
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
