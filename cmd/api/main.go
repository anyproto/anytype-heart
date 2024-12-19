package api

import (
	"context"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/webstradev/gin-pagination/v2/pkg/pagination"

	_ "github.com/anyproto/anytype-heart/cmd/api/docs"
	"github.com/anyproto/anytype-heart/core"
	"github.com/anyproto/anytype-heart/pb/service"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	httpPort           = ":31009"
	serverShutdownTime = 5 * time.Second
)

type ApiServer struct {
	mw         service.ClientCommandsServer
	mwInternal core.MiddlewareInternal
	router     *gin.Engine
	server     *http.Server

	// init after app start
	accountInfo *model.AccountInfo
}

// TODO: User represents an authenticated user with permissions
type User struct {
	ID          string
	Permissions string // "read-only" or "read-write"
}

func newApiServer(mw service.ClientCommandsServer, mwInternal core.MiddlewareInternal) *ApiServer {
	a := &ApiServer{
		mw:         mw,
		mwInternal: mwInternal,
		router:     gin.Default(),
	}

	a.server = &http.Server{
		Addr:              httpPort,
		Handler:           a.router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	return a
}

// RunApiServer starts the HTTP server and registers the API routes.
//
//	@title						Anytype API
//	@version					1.0
//	@description				This API allows interaction with Anytype resources such as spaces, objects, and object types.
//	@termsOfService				https://anytype.io/terms_of_use
//	@contact.name				Anytype Support
//	@contact.url				https://anytype.io/contact
//	@contact.email				support@anytype.io
//	@license.name				Any Source Available License 1.0
//	@license.url				https://github.com/anyproto/anytype-ts/blob/main/LICENSE.md
//	@host						localhost:31009
//	@BasePath					/v1
//	@securityDefinitions.basic	BasicAuth
//	@externalDocs.description	OpenAPI
//	@externalDocs.url			https://swagger.io/resources/open-api/
func RunApiServer(ctx context.Context, mw service.ClientCommandsServer, mwInternal core.MiddlewareInternal) {
	a := newApiServer(mw, mwInternal)
	a.router.Use(a.initAccountInfo())

	// Initialize pagination middleware
	paginator := pagination.New(
		pagination.WithPageText("offset"),
		pagination.WithSizeText("limit"),
		pagination.WithDefaultPage(0),
		pagination.WithDefaultPageSize(100),
		pagination.WithMinPageSize(1),
		pagination.WithMaxPageSize(1000),
	)

	// Swagger route
	a.router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

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
		readOnly.GET("/spaces", paginator, a.getSpacesHandler)
		readOnly.GET("/spaces/:space_id/members", paginator, a.getSpaceMembersHandler)
		readOnly.GET("/spaces/:space_id/objects", paginator, a.getObjectsForSpaceHandler)
		readOnly.GET("/spaces/:space_id/objects/:object_id", a.getObjectHandler)
		readOnly.GET("/spaces/:space_id/objectTypes", paginator, a.getObjectTypesHandler)
		readOnly.GET("/spaces/:space_id/objectTypes/:typeId/templates", paginator, a.getObjectTypeTemplatesHandler)
		readOnly.GET("/objects", paginator, a.getObjectsHandler)
	}

	// Read-write routes
	readWrite := a.router.Group("/v1")
	// readWrite.Use(a.AuthMiddleware())
	// readWrite.Use(a.PermissionMiddleware("read-write"))
	{
		readWrite.POST("/spaces", a.createSpaceHandler)
		readWrite.POST("/spaces/:space_id/objects", a.createObjectHandler)
		readWrite.PUT("/spaces/:space_id/objects/:object_id", a.updateObjectHandler)
	}

	// Chat routes
	chat := a.router.Group("/v1/spaces/:space_id/chat")
	// chat.Use(a.AuthMiddleware())
	// chat.Use(a.PermissionMiddleware("read-write"))
	{
		chat.GET("/messages", paginator, a.getChatMessagesHandler)
		chat.GET("/messages/:message_id", a.getChatMessageHandler)
		chat.POST("/messages", a.addChatMessageHandler)
		chat.PUT("/messages/:message_id", a.updateChatMessageHandler)
		chat.DELETE("/messages/:message_id", a.deleteChatMessageHandler)
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
