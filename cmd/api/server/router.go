package server

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/webstradev/gin-pagination/v2/pkg/pagination"

	"github.com/anyproto/anytype-heart/cmd/api/auth"
	"github.com/anyproto/anytype-heart/cmd/api/object"
	"github.com/anyproto/anytype-heart/cmd/api/search"
	"github.com/anyproto/anytype-heart/cmd/api/space"
)

// NewRouter builds and returns a *gin.Engine with all routes configured.
func (s *Server) NewRouter() *gin.Engine {
	router := gin.Default()
	router.Use(s.initAccountInfo())

	// Pagination middleware setup
	paginator := pagination.New(
		pagination.WithPageText("offset"),
		pagination.WithSizeText("limit"),
		pagination.WithDefaultPage(0),
		pagination.WithDefaultPageSize(100),
		pagination.WithMinPageSize(1),
		pagination.WithMaxPageSize(1000),
	)

	// Swagger route
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Unprotected routes
	authRouter := router.Group("/v1/auth")
	{
		authRouter.POST("/displayCode", auth.AuthDisplayCodeHandler(s.authService))
		authRouter.GET("/token", auth.AuthTokenHandler(s.authService))
	}

	// Read-only group
	readOnly := router.Group("/v1")
	// readOnly.Use(a.AuthMiddleware())
	// readOnly.Use(a.PermissionMiddleware("read-only"))
	{
		readOnly.GET("/spaces", paginator, space.GetSpacesHandler(s.spaceService))
		readOnly.GET("/spaces/:space_id/members", paginator, space.GetMembersHandler(s.spaceService))
		readOnly.GET("/spaces/:space_id/objects", paginator, object.GetObjectsHandler(s.objectService))
		readOnly.GET("/spaces/:space_id/objects/:object_id", object.GetObjectHandler(s.objectService))
		readOnly.GET("/spaces/:space_id/objectTypes", paginator, object.GetTypesHandler(s.objectService))
		readOnly.GET("/spaces/:space_id/objectTypes/:typeId/templates", paginator, object.GetTemplatesHandler(s.objectService))
		readOnly.GET("/search", paginator, search.SearchHandler(s.searchService))
	}

	// Read-write group
	readWrite := router.Group("/v1")
	// readWrite.Use(a.AuthMiddleware())
	// readWrite.Use(a.PermissionMiddleware("read-write"))
	{
		readWrite.POST("/spaces", space.CreateSpaceHandler(s.spaceService))
		readWrite.POST("/spaces/:space_id/objects", object.CreateObjectHandler(s.objectService))
		readWrite.PUT("/spaces/:space_id/objects/:object_id", object.UpdateObjectHandler(s.objectService))
	}

	return router
}
