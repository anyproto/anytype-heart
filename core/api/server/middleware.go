package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/didip/tollbooth/v8"
	"github.com/didip/tollbooth/v8/limiter"
	"github.com/gin-gonic/gin"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/util"
	"github.com/anyproto/anytype-heart/core/event"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/logging"
)

const ApiVersion = "2025-05-20"

var log = logging.Logger("api-server")

var (
	ErrMissingAuthorizationHeader = errors.New("missing authorization header")
	ErrInvalidAuthorizationHeader = errors.New("invalid authorization header format")
	ErrInvalidApiKey              = errors.New("invalid api key")
)

// ensureRateLimit creates a shared write-rate limiter middleware.
func ensureRateLimit(rate float64, burst int, isRateLimitDisabled bool) gin.HandlerFunc {
	lmt := tollbooth.NewLimiter(rate, nil)
	lmt.SetBurst(burst)
	lmt.SetIPLookup(limiter.IPLookup{
		Name:           "RemoteAddr",
		IndexFromRight: 0,
	})

	return func(c *gin.Context) {
		if isRateLimitDisabled {
			c.Next()
			return
		}
		if httpError := tollbooth.LimitByRequest(lmt, c.Writer, c.Request); httpError != nil {
			apiErr := util.CodeToAPIError(httpError.StatusCode, httpError.Message)
			c.AbortWithStatusJSON(httpError.StatusCode, apiErr)
			return
		}
		c.Next()
	}
}

// ensureAuthenticated is a middleware that ensures the request is authenticated.
func (s *Server) ensureAuthenticated(mw apicore.ClientCommands) gin.HandlerFunc {
	return func(c *gin.Context) {
		authHeader := c.GetHeader("Authorization")
		if authHeader == "" {
			apiErr := util.CodeToAPIError(http.StatusUnauthorized, ErrMissingAuthorizationHeader.Error())
			c.AbortWithStatusJSON(http.StatusUnauthorized, apiErr)
			return
		}

		if !strings.HasPrefix(authHeader, "Bearer ") {
			apiErr := util.CodeToAPIError(http.StatusUnauthorized, ErrInvalidAuthorizationHeader.Error())
			c.AbortWithStatusJSON(http.StatusUnauthorized, apiErr)
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
				apiErr := util.CodeToAPIError(http.StatusUnauthorized, ErrInvalidApiKey.Error())
				c.AbortWithStatusJSON(http.StatusUnauthorized, apiErr)
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

// ensureAnalyticsEvent is a middleware that ensures broadcasting an analytics event after a successful request.
func (s *Server) ensureAnalyticsEvent(code string, eventService apicore.EventService) gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Next()

		status := c.Writer.Status()
		payload, err := util.NewAnalyticsEventForApi(c.Request.Context(), code, status)
		if err != nil {
			log.Errorf("failed to create api analytics event: %v", err)
			return
		}

		eventService.Broadcast(event.NewEventSingleMessage("", &pb.EventMessageValueOfPayloadBroadcast{
			PayloadBroadcast: &pb.EventPayloadBroadcast{
				Payload: payload,
			},
		}))
	}
}

// ensureMetadataHeader is a middleware that ensures the metadata header is set.
func (s *Server) ensureMetadataHeader() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Writer.Header().Set("Anytype-Version", ApiVersion)
		c.Next()
	}
}
