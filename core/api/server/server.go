package server

import (
	"context"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/service"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
)

type ApiSessionEntry struct {
	Token   string `json:"token"`
	AppName string `json:"appName"`
}

// Server wraps the HTTP server and service logic.
type Server struct {
	engine     *gin.Engine
	service    *service.Service
	v2Service  *service.V2Service
	chatSubSvc apicore.ChatSubscriptionService

	mu         sync.Mutex
	KeyToToken map[string]ApiSessionEntry // appKey -> token

	initOnce sync.Once
}

// V2Deps carries the API v2 dependencies (APIV2.md §8: live smartblock
// reads + objectstore-backed lists/resolvers). With either dependency nil,
// the /v2 route group is not registered — v1 keeps working standalone.
type V2Deps struct {
	Reader apicore.ObjectReader
	Store  objectstore.ObjectStore
}

// NewServer constructs a new Server with the default config and sets up the routes.
func NewServer(mw apicore.ClientCommands, accountService apicore.AccountService, eventService apicore.EventService, crossSpaceSubService apicore.CrossSpaceSubscriptionService, chatSubSvc apicore.ChatSubscriptionService, fileObjectService apicore.FileObjectService, v2Deps V2Deps, apiListenAddr string, openapiYAML []byte, openapiJSON []byte) *Server {
	techSpaceId, err := getTechSpaceId(accountService)
	if err != nil {
		panic(err)
	}

	apiBaseUrl := buildApiBaseUrl(apiListenAddr)
	s := &Server{
		service:    service.NewService(mw, fileObjectService, apiBaseUrl, techSpaceId, crossSpaceSubService),
		chatSubSvc: chatSubSvc,
	}
	if v2Deps.Reader != nil && v2Deps.Store != nil {
		s.v2Service = service.NewV2Service(mw, v2Deps.Reader, v2Deps.Store, techSpaceId)
	}
	s.engine = s.NewRouter(mw, eventService, openapiYAML, openapiJSON)
	s.KeyToToken = make(map[string]ApiSessionEntry)

	return s
}

// getTechSpaceId retrieves the tech space ID from the account service.
func getTechSpaceId(accountService apicore.AccountService) (techSpaceId string, err error) {
	accountInfo, err := accountService.GetInfo(context.Background())
	if err != nil {
		return "", err
	}
	return accountInfo.TechSpaceId, nil
}

// buildApiBaseUrl turns the API listen address into a fully-qualified base URL
// (e.g. "127.0.0.1:31009" -> "http://127.0.0.1:31009"). Wildcard hosts are
// rewritten to 127.0.0.1 so the URL is dialable from clients.
func buildApiBaseUrl(listenAddr string) string {
	addr := listenAddr
	if strings.HasPrefix(addr, ":") {
		addr = "127.0.0.1" + addr
	} else if strings.HasPrefix(addr, "0.0.0.0:") {
		addr = "127.0.0.1:" + strings.TrimPrefix(addr, "0.0.0.0:")
	}
	return "http://" + addr
}

// Stop the service to clean up caches and subscriptions
func (srv *Server) Stop() {
	srv.service.Stop()
}

// RevokeToken removes the cached API key entry associated with the given session token.
func (srv *Server) RevokeToken(token string) {
	srv.mu.Lock()
	defer srv.mu.Unlock()
	for key, entry := range srv.KeyToToken {
		if entry.Token == token {
			delete(srv.KeyToToken, key)
			return
		}
	}
}

// Engine returns the underlying gin.Engine.
func (srv *Server) Engine() *gin.Engine {
	return srv.engine
}
