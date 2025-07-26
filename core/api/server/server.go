package server

import (
	"context"
	"sync"

	"github.com/cheggaaa/mb/v3"
	"github.com/gin-gonic/gin"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/core/subscription"
	"github.com/anyproto/anytype-heart/pb"
)

type ApiSessionEntry struct {
	Token   string `json:"token"`
	AppName string `json:"appName"`
}

// Server wraps the HTTP server and service logic.
type Server struct {
	engine       *gin.Engine
	service      *service.Service
	eventService apicore.EventService

	mu         sync.Mutex
	KeyToToken map[string]ApiSessionEntry // appKey -> token

	initOnce sync.Once
}

// NewServer constructs a new Server with the default config and sets up the routes.
func NewServer(mw apicore.ClientCommands, accountService apicore.AccountService, eventService apicore.EventService, openapiYAML []byte, openapiJSON []byte) *Server {
	gatewayUrl, techSpaceId, err := getAccountInfo(accountService)
	if err != nil {
		panic(err)
	}

	s := &Server{
		service:      service.NewService(mw, gatewayUrl, techSpaceId),
		eventService: eventService,
	}
	s.engine = s.NewRouter(mw, eventService, openapiYAML, openapiJSON)
	s.KeyToToken = make(map[string]ApiSessionEntry)

	return s
}

// SetEventQueue sets the event queue for the service to receive real-time updates
func (s *Server) SetEventQueue(queue *mb.MB[*pb.EventMessage]) {
	if s.service != nil {
		s.service.SetEventQueue(queue)
	}
}

// SetSubscriptionService sets the subscription service for internal subscriptions
func (s *Server) SetSubscriptionService(svc subscription.Service) {
	if s.service != nil {
		s.service.SetSubscriptionService(svc)
	}
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

// Start initializes the server
func (s *Server) Start() {
	// Event processing is handled automatically when caches are initialized
}

// ProcessEvent processes events from the event system to update caches
// This method should be called by the parent application when events are received
func (s *Server) ProcessEvent(event *pb.Event) {
	if s.service != nil {
		s.service.ProcessSubscriptionEvent(event)
	}
}

// Stop the service to clean up caches and subscriptions
func (s *Server) Stop() {
	if s.service != nil {
		s.service.Stop()
	}
}

// Engine returns the underlying gin.Engine.
func (s *Server) Engine() *gin.Engine {
	return s.engine
}
