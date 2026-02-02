package server

import (
	"context"
	"sync"

	"github.com/gin-gonic/gin"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/service"
)

type ApiSessionEntry struct {
	Token   string `json:"token"`
	AppName string `json:"appName"`
}

// Server wraps the HTTP server and service logic.
type Server struct {
	engine  *gin.Engine
	service *service.Service

	mu         sync.Mutex
	KeyToToken map[string]ApiSessionEntry // appKey -> token

	sseManager         *service.SSESessionManager
	unregisterCallback func() // To unregister event callback on shutdown

	initOnce sync.Once
}

// NewServer constructs a new Server with the default config and sets up the routes.
func NewServer(mw apicore.ClientCommands, accountService apicore.AccountService, eventService apicore.EventService, crossSpaceSubService apicore.CrossSpaceSubscriptionService, openapiYAML []byte, openapiJSON []byte) *Server {
	gatewayUrl, techSpaceId, err := getAccountInfo(accountService)
	if err != nil {
		panic(err)
	}

	svc := service.NewService(mw, gatewayUrl, techSpaceId, crossSpaceSubService)
	sseManager := service.NewSSESessionManager()
	sseDispatcher := service.NewSSEEventDispatcher(sseManager, svc)

	// Register callback to receive all events
	unregisterCallback := eventService.RegisterCallback(sseDispatcher.HandleEvent)

	s := &Server{
		service:            svc,
		sseManager:         sseManager,
		unregisterCallback: unregisterCallback,
		KeyToToken:         make(map[string]ApiSessionEntry),
	}
	_ = sseDispatcher // Used for event registration above
	s.engine = s.NewRouter(mw, eventService, openapiYAML, openapiJSON)

	return s
}

// getAccountInfo retrieves the account information from the account service and returns the gateway URL and tech space ID.
func getAccountInfo(accountService apicore.AccountService) (gatewayUrl string, techSpaceId string, err error) {
	accountInfo, err := accountService.GetInfo(context.Background())
	if err != nil {
		return "", "", err
	}
	gatewayUrl = accountInfo.GatewayUrl
	techSpaceId = accountInfo.TechSpaceId
	return gatewayUrl, techSpaceId, nil
}

// Stop the service to clean up caches and subscriptions
func (srv *Server) Stop() {
	// Unregister event callback
	if srv.unregisterCallback != nil {
		srv.unregisterCallback()
	}
	// Close all SSE sessions
	if srv.sseManager != nil {
		srv.sseManager.CloseAll()
	}
	srv.service.Stop()
}

// Engine returns the underlying gin.Engine.
func (srv *Server) Engine() *gin.Engine {
	return srv.engine
}
