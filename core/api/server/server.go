package server

import (
	"sync"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/api/internal/auth"
	"github.com/anyproto/anytype-heart/core/api/internal/export"
	"github.com/anyproto/anytype-heart/core/api/internal/object"
	"github.com/anyproto/anytype-heart/core/api/internal/search"
	"github.com/anyproto/anytype-heart/core/api/internal/space"
	"github.com/anyproto/anytype-heart/pb/service"
)

// Server wraps the HTTP server and service logic.
type Server struct {
	engine *gin.Engine

	authService   *auth.AuthService
	exportService *export.ExportService
	objectService *object.ObjectService
	spaceService  *space.SpaceService
	searchService *search.SearchService

	mu         sync.Mutex
	KeyToToken map[string]string // appKey -> token
}

// NewServer constructs a new Server with default config and sets up the routes.
func NewServer(accountService account.Service, mw service.ClientCommandsServer) *Server {
	s := &Server{
		authService:   auth.NewService(mw),
		exportService: export.NewService(mw),
		spaceService:  space.NewService(mw),
	}

	s.objectService = object.NewService(mw, s.spaceService)
	s.searchService = search.NewService(mw, s.spaceService, s.objectService)
	s.engine = s.NewRouter(accountService, mw)
	s.KeyToToken = make(map[string]string)

	return s
}

// Engine returns the underlying gin.Engine.
func (s *Server) Engine() *gin.Engine {
	return s.engine
}
