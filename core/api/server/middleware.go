package server

import (
	"context"
	"fmt"
	"net/http"

	"github.com/anyproto/any-sync/app"
	"github.com/didip/tollbooth/v8"
	"github.com/didip/tollbooth/v8/limiter"
	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/anytype/account"
)

// rateLimit is a middleware that limits the number of requests per second.
func (s *Server) rateLimit(max float64) gin.HandlerFunc {
	lmt := tollbooth.NewLimiter(max, nil)
	lmt.SetIPLookup(limiter.IPLookup{
		Name:           "RemoteAddr",
		IndexFromRight: 0,
	})

	return func(c *gin.Context) {
		httpError := tollbooth.LimitByRequest(lmt, c.Writer, c.Request)
		if httpError != nil {
			c.AbortWithStatusJSON(httpError.StatusCode, gin.H{"error": httpError.Message})
			return
		}
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

// ensureAccountInfo is a middleware that ensures the account info is available in the services.
func (s *Server) ensureAccountInfo(a *app.App) gin.HandlerFunc {
	return func(c *gin.Context) {
		// TODO: consider not fetching account info on every request; currently used to avoid inconsistencies on logout/login
		if a == nil {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "failed to get app instance"})
			return
		}

		accInfo, err := a.Component(account.CName).(account.Service).GetInfo(context.Background())
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
