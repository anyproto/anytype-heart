package server

import (
	"context"
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/apicore"
	"github.com/anyproto/anytype-heart/core/api/internal"
)

// Server wraps the HTTP server and service logic.
type Server struct {
	engine  *gin.Engine
	service *internal.Service

	mu         sync.Mutex
	KeyToToken map[string]string // appKey -> token
}

// NewServer constructs a new Server with default config and sets up the routes.
func NewServer(mw apicore.ClientCommands, accountService apicore.AccountService, exportService apicore.ExportService) *Server {
	gatewayUrl, techSpaceId, err := getAccountInfo(accountService)
	if err != nil {
		panic(err)
	}

	s := &Server{service: internal.NewService(mw, exportService, gatewayUrl, techSpaceId)}
	s.engine = s.NewRouter(mw)
	s.KeyToToken = make(map[string]string)

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

// Engine returns the underlying gin.Engine.
func (s *Server) Engine() *gin.Engine {
	return s.engine
}
