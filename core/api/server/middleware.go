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
	"github.com/anyproto/anytype-heart/core/event"
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
			c.AbortWithStatusJSON(httpError.StatusCode, gin.H{"error": httpError.Message})
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
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Missing Authorization header"})
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid Authorization header format"})
			return
		}
		key := strings.TrimPrefix(authHeader, "Bearer ")

		// Validate the key - if the key exists in the KeyToToken map, it is considered valid.
		// Otherwise, attempt to create a new session using the key and add it to the map upon successful validation.
		s.mu.Lock()
		apiSession, exists := s.KeyToToken[key]
		s.mu.Unlock()

		if !exists {
			response := mw.WalletCreateSession(context.Background(), &pb.RpcWalletCreateSessionRequest{Auth: &pb.RpcWalletCreateSessionRequestAuthOfAppKey{AppKey: key}})
			if response.Error.Code != pb.RpcWalletCreateSessionResponseError_NULL {
				c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{"error": "Invalid token"})
				return
			}
			apiSession = ApiSessionEntry{
				Token: response.Token,
				// TODO: enable once app name is returned
				// AppName: response.AppName,
			}

			s.mu.Lock()
			s.KeyToToken[key] = apiSession
			s.mu.Unlock()
		}

		// Add token to request context for downstream services (subscriptions, events, etc.)
		c.Set("token", apiSession.Token)
		c.Set("apiAppName", apiSession.AppName)
		c.Next()
	}
}

// ensureAccountInfo is a middleware that ensures the account info is available in the services.
func (s *Server) ensureAccountInfo(accountService account.Service) gin.HandlerFunc {
	return func(c *gin.Context) {
		accInfo, err := accountService.GetInfo(context.Background())
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

// ensureAnalyticsEvent is a middleware that ensures broadcasting an analytics event after a successful request.
func (s *Server) ensureAnalyticsEvent(code string, eventService event.Sender) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		if c.Writer.Status() != http.StatusOK {
			return
		}

		payload := util.NewAnalyticsEventForApi(c.Request.Context(), code)
		eventService.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfPayloadBroadcast{
			PayloadBroadcast: &pb.EventPayloadBroadcast{
				Payload: payload,
			},
		}))
	}
}
