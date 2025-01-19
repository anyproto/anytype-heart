package server

import (
	"github.com/anyproto/any-sync/app"
	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/services/auth"
	"github.com/anyproto/anytype-heart/core/api/services/export"
	"github.com/anyproto/anytype-heart/core/api/services/object"
	"github.com/anyproto/anytype-heart/core/api/services/search"
	"github.com/anyproto/anytype-heart/core/api/services/space"
	"github.com/anyproto/anytype-heart/core/interfaces"
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

	KeyToToken map[string]string // appKey -> token
}

// NewServer constructs a new Server with default config and sets up the routes.
func NewServer(a *app.App, mw service.ClientCommandsServer, tv interfaces.TokenValidator) *Server {
	s := &Server{
		authService:   auth.NewService(mw),
		exportService: export.NewService(mw),
		spaceService:  space.NewService(mw),
	}

	s.objectService = object.NewService(mw, s.spaceService)
	s.searchService = search.NewService(mw, s.spaceService, s.objectService)
	s.engine = s.NewRouter(a, mw, tv)
	s.KeyToToken = make(map[string]string)

	return s
}

// Engine returns the underlying gin.Engine.
func (s *Server) Engine() *gin.Engine {
	return s.engine
}
