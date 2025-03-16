package server

import (
	"context"
	"fmt"
	"net/http"
	"strings"

	"github.com/didip/tollbooth/v8"
	"github.com/didip/tollbooth/v8/limiter"
	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pb/service"
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
			apiErr := util.CodeToAPIError(httpError.StatusCode, httpError.Message)
			c.AbortWithStatusJSON(httpError.StatusCode, apiErr)
			return
		}
		c.Next()
	}
}

// ensureAuthenticated is a middleware that ensures the request is authenticated.
func (s *Server) ensureAuthenticated(mw service.ClientCommandsServer) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			apiErr := util.CodeToAPIError(http.StatusUnauthorized, "Missing Authorization header")
			c.AbortWithStatusJSON(http.StatusUnauthorized, apiErr)
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			apiErr := util.CodeToAPIError(http.StatusUnauthorized, "Invalid Authorization header format")
			c.AbortWithStatusJSON(http.StatusUnauthorized, apiErr)
			return
		}
		key := strings.TrimPrefix(authHeader, "Bearer ")

		// Validate the key - if the key exists in the KeyToToken map, it is considered valid.
		// Otherwise, attempt to create a new session using the key and add it to the map upon successful validation.
		s.mu.Lock()
		token, exists := s.KeyToToken[key]
		s.mu.Unlock()

		if !exists {
			response := mw.WalletCreateSession(context.Background(), &pb.RpcWalletCreateSessionRequest{Auth: &pb.RpcWalletCreateSessionRequestAuthOfAppKey{AppKey: key}})
			if response.Error.Code != pb.RpcWalletCreateSessionResponseError_NULL {
				apiErr := util.CodeToAPIError(http.StatusUnauthorized, "Invalid token")
				c.AbortWithStatusJSON(http.StatusUnauthorized, apiErr)
				return
			}
			token = response.Token

			s.mu.Lock()
			s.KeyToToken[key] = token
			s.mu.Unlock()
		}

		// Add token to request context for downstream services (subscriptions, events, etc.)
		c.Set("token", token)
		c.Next()
	}
}

// ensureAccountInfo is a middleware that ensures the account info is available in the services.
func (s *Server) ensureAccountInfo(accountService account.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		accInfo, err := accountService.GetInfo(context.Background())
		if err != nil {
			apiErr := util.CodeToAPIError(http.StatusInternalServerError, fmt.Sprintf("failed to get account info: %v", err))
			c.AbortWithStatusJSON(http.StatusInternalServerError, apiErr)
			return
		}

		s.objectService.AccountInfo = accInfo
		s.spaceService.AccountInfo = accInfo
		s.searchService.AccountInfo = accInfo

		c.Next()
	}
}

// ensureMetadataHeader is a middleware that ensures the metadata header is set.
func (s *Server) ensureMetadataHeader() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Anytype-Version", "2025-03-17")
		c.Next()
	}
}
