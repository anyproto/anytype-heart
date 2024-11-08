package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/anyproto/any-sync/app"
	"github.com/gin-gonic/gin"
)

const CName = "api"

type Service interface {
	app.ComponentRunnable
}

type service struct {
	router *gin.Engine
	server *http.Server
}

// TODO: User represents an authenticated user with permissions
type User struct {
	ID          string
	Permissions string // "read-only" or "read-write"
}

func New() Service {
	return &service{}
}

func (s *service) Init(a *app.App) error {
	gin.SetMode(gin.ReleaseMode)
	s.router = gin.New()

	// Unprotected routes
	auth := s.router.Group("/v1/auth")
	{
		auth.POST("/displayCode", authDisplayCodeHandler)
		auth.GET("/token", authTokenHandler)
	}

	// Read-only routes
	readOnly := s.router.Group("/v1")
	readOnly.Use(AuthMiddleware())
	readOnly.Use(PermissionMiddleware("read-only"))
	{
		readOnly.GET("/spaces", getSpacesHandler)
		readOnly.GET("/spaces/:space_id/members", getSpaceMembersHandler)
		readOnly.GET("/spaces/:space_id/objects", getSpaceObjectsHandler)
		readOnly.GET("/spaces/:space_id/objects/:object_id", getObjectHandler)
		readOnly.GET("/spaces/:space_id/objectTypes", getObjectTypesHandler)
		readOnly.GET("/spaces/:space_id/objectTypes/:typeId/templates", getObjectTypeTemplatesHandler)
		readOnly.GET("/objects", getObjectsHandler)
	}

	// Read-write routes
	readWrite := s.router.Group("/v1")
	readWrite.Use(AuthMiddleware())
	readWrite.Use(PermissionMiddleware("read-write"))
	{
		readWrite.POST("/spaces", createSpaceHandler)
		readWrite.POST("/spaces/:space_id/objects/:object_id", createObjectHandler)
		readWrite.PUT("/spaces/:space_id/objects/:object_id", updateObjectHandler)
	}
	return nil
}

func (s *service) Name() string {
	return CName
}

func (s *service) Run(ctx context.Context) error {
	s.server = &http.Server{
		Addr:    ":31009",
		Handler: s.router,
	}

	// Start the HTTP server
	go func() {
		if err := s.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("failed to start HTTP server: %v\n", err)
		}
	}()

	return nil
}

func (s *service) Close(ctx context.Context) error {
	// Gracefully shut down the server
	ctx, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := s.server.Shutdown(ctx); err != nil {
		return fmt.Errorf("server shutdown failed: %w", err)
	}
	return nil
}
