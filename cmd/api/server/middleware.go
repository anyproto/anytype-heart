package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/anytype/account"
)

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

		s.exportService.AccountInfo = accInfo
		s.objectService.AccountInfo = accInfo
		s.spaceService.AccountInfo = accInfo
		s.searchService.AccountInfo = accInfo
		c.Next()
	}
}

// ensureAuthenticated is a middleware that ensures the request is authenticated.
func (s *Server) ensureAuthenticated() gin.HandlerFunc {
	return func(c *gin.Context) {
		// token := c.GetHeader("Authorization")
		// if token == "" {
		// 	c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Unauthorized"})
		// 	return
		// }

		// TODO: Validate the token and retrieve user information; this is mock example
		c.Next()
	}
}
