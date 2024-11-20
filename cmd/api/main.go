package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	httpPort           = ":31009"
	serverShutdownTime = 5 * time.Second
)

type ApiServer struct {
	mw          service.ClientCommandsServer
	mwInternal  core.MiddlewareInternal
	router      *gin.Engine
	server      *http.Server
	accountInfo model.AccountInfo
}

// TODO: User represents an authenticated user with permissions
type User struct {
	ID          string
	Permissions string // "read-only" or "read-write"
}

func newApiServer(mw service.ClientCommandsServer, mwInternal core.MiddlewareInternal) *ApiServer {
	a := &ApiServer{
		mw:          mw,
		mwInternal:  mwInternal,
		router:      gin.New(),
		accountInfo: model.AccountInfo{},
	}

	a.server = &http.Server{
		Addr:              httpPort,
		Handler:           a.router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return a
}

func RunApiServer(ctx context.Context, mw service.ClientCommandsServer, mwInternal core.MiddlewareInternal) {
	a := newApiServer(mw, mwInternal)
	a.router.Use(a.EnsureAccountInfoMiddleware())

	// Unprotected routes
	auth := a.router.Group("/v1/auth")
	{
		auth.POST("/displayCode", a.authDisplayCodeHandler)
		auth.GET("/token", a.authTokenHandler)
	}

	// Read-only routes
	readOnly := a.router.Group("/v1")
	// readOnly.Use(a.AuthMiddleware())
	// readOnly.Use(a.PermissionMiddleware("read-only"))
	{
		readOnly.GET("/spaces", a.getSpacesHandler)
		readOnly.GET("/spaces/:space_id/members", a.getSpaceMembersHandler)
		readOnly.GET("/spaces/:space_id/objects", a.getSpaceObjectsHandler)
		readOnly.GET("/spaces/:space_id/objects/:object_id", a.getObjectHandler)
		readOnly.GET("/spaces/:space_id/objectTypes", a.getObjectTypesHandler)
		readOnly.GET("/spaces/:space_id/objectTypes/:typeId/templates", a.getObjectTypeTemplatesHandler)
		readOnly.GET("/objects", a.getObjectsHandler)
	}

	// Read-write routes
	readWrite := a.router.Group("/v1")
	// readWrite.Use(a.AuthMiddleware())
	// readWrite.Use(a.PermissionMiddleware("read-write"))
	{
		readWrite.POST("/spaces", a.createSpaceHandler)
		readWrite.POST("/spaces/:space_id/objects/:object_id", a.createObjectHandler)
		readWrite.PUT("/spaces/:space_id/objects/:object_id", a.updateObjectHandler)
	}

	// Start the HTTP server
	go func() {
		if err := a.server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Printf("failed to start HTTP server: %v\n", err)
		}
	}()

	// Wait for the context to be done and then shut down the server
	<-ctx.Done()

	// Create a new context with a timeout to shut down the server
	shutdownCtx, cancel := context.WithTimeout(context.Background(), serverShutdownTime)
	defer cancel()
	if err := a.server.Shutdown(shutdownCtx); err != nil {
		fmt.Println("server shutdown failed: %w", err)
	}
}
