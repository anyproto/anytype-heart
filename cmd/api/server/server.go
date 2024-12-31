package server

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/cmd/api/auth"
	"github.com/anyproto/anytype-heart/cmd/api/object"
	"github.com/anyproto/anytype-heart/cmd/api/search"
	"github.com/anyproto/anytype-heart/cmd/api/space"
	"github.com/anyproto/anytype-heart/core"
	"github.com/anyproto/anytype-heart/pb/service"
)

const (
	httpPort          = ":31009"
	readHeaderTimeout = 5 * time.Second
)

// Server wraps the HTTP server logic.
type Server struct {
	engine *gin.Engine
	srv    *http.Server

	mwInternal    core.MiddlewareInternal
	authService   *auth.AuthService
	objectService *object.ObjectService
	spaceService  *space.SpaceService
	searchService *search.SearchService
}

// NewServer constructs a new Server with default config
// and sets up routes via your router.go
func NewServer(mw service.ClientCommandsServer, mwInternal core.MiddlewareInternal) *Server {
	s := &Server{
		mwInternal:    mwInternal,
		authService:   auth.NewService(mw),
		objectService: object.NewService(mw),
		spaceService:  space.NewService(mw),
	}

	s.searchService = search.NewService(mw, s.spaceService, s.objectService)
	s.engine = s.NewRouter()
	s.srv = &http.Server{
		Addr:              httpPort,
		Handler:           s.engine,
		ReadHeaderTimeout: readHeaderTimeout,
	}

	return s
}

// ListenAndServe starts the HTTP server
func (s *Server) ListenAndServe() error {
	fmt.Printf("Starting API server on %s\n", httpPort)
	return s.srv.ListenAndServe()
}

// Shutdown gracefully stops the server
func (s *Server) Shutdown(ctx context.Context) error {
	fmt.Println("Shutting down API server...")
	return s.srv.Shutdown(ctx)
}
