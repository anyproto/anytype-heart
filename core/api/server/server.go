package server

import (
	"github.com/anyproto/any-sync/app"
	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/auth"
	"github.com/anyproto/anytype-heart/core/api/export"
	"github.com/anyproto/anytype-heart/core/api/object"
	"github.com/anyproto/anytype-heart/core/api/search"
	"github.com/anyproto/anytype-heart/core/api/space"
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
}

// NewServer constructs a new Server with default config and sets up the routes.
func NewServer(a *app.App, mw service.ClientCommandsServer) *Server {
	s := &Server{
		authService:   auth.NewService(mw),
		exportService: export.NewService(mw),
		objectService: object.NewService(mw),
		spaceService:  space.NewService(mw),
	}

	s.searchService = search.NewService(mw, s.spaceService, s.objectService)
	s.engine = s.NewRouter(a)

	return s
}

// Engine returns the underlying gin.Engine.
func (s *Server) Engine() *gin.Engine {
	return s.engine
}
