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

	"github.com/anyproto/anytype-heart/cmd/api/auth"
	_ "github.com/anyproto/anytype-heart/cmd/api/docs"
	"github.com/anyproto/anytype-heart/cmd/api/object"
	"github.com/anyproto/anytype-heart/cmd/api/search"
	"github.com/anyproto/anytype-heart/cmd/api/space"
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

	accountInfo   *model.AccountInfo
	authService   *auth.AuthService
	objectService *object.ObjectService
	spaceService  *space.SpaceService
	searchService *search.SearchService
}

// TODO: User represents an authenticated user with permissions
type User struct {
	ID          string
	Permissions string // "read-only" or "read-write"
}

func newApiServer(mw service.ClientCommandsServer, mwInternal core.MiddlewareInternal) *ApiServer {
	a := &ApiServer{
		mw:            mw,
		mwInternal:    mwInternal,
		router:        gin.Default(),
		authService:   auth.NewService(mw),
		objectService: object.NewService(mw),
		spaceService:  space.NewService(mw),
		searchService: search.NewService(mw),
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
	authRouter := a.router.Group("/v1/auth")
	{
		authRouter.POST("/displayCode", auth.AuthDisplayCodeHandler(a.authService))
		authRouter.GET("/token", auth.AuthTokenHandler(a.authService))
	}

	// Read-only routes
	readOnly := a.router.Group("/v1")
	// readOnly.Use(a.AuthMiddleware())
	// readOnly.Use(a.PermissionMiddleware("read-only"))
	{
		readOnly.GET("/spaces", paginator, space.GetSpacesHandler(a.spaceService))
		readOnly.GET("/spaces/:space_id/members", paginator, space.GetMembersHandler(a.spaceService))
		readOnly.GET("/spaces/:space_id/objects", paginator, object.GetObjectsHandler(a.objectService))
		readOnly.GET("/spaces/:space_id/objects/:object_id", object.GetObjectHandler(a.objectService))
		readOnly.GET("/spaces/:space_id/objectTypes", paginator, object.GetObjectTypesHandler(a.objectService))
		readOnly.GET("/spaces/:space_id/objectTypes/:typeId/templates", paginator, object.GetObjectTypeTemplatesHandler(a.objectService))
		readOnly.GET("/search", paginator, search.SearchHandler(a.searchService))
	}

	// Read-write routes
	readWrite := a.router.Group("/v1")
	// readWrite.Use(a.AuthMiddleware())
	// readWrite.Use(a.PermissionMiddleware("read-write"))
	{
		// readWrite.POST("/spaces", a.createSpaceHandler)
		readWrite.POST("/spaces/:space_id/objects", object.CreateObjectHandler(a.objectService))
		readWrite.PUT("/spaces/:space_id/objects/:object_id", object.UpdateObjectHandler(a.objectService))
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
