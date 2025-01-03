package server

import (
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"
	"github.com/webstradev/gin-pagination/v2/pkg/pagination"

	"github.com/anyproto/anytype-heart/cmd/api/auth"
	"github.com/anyproto/anytype-heart/cmd/api/export"
	"github.com/anyproto/anytype-heart/cmd/api/object"
	"github.com/anyproto/anytype-heart/cmd/api/search"
	"github.com/anyproto/anytype-heart/cmd/api/space"
)

// NewRouter builds and returns a *gin.Engine with all routes configured.
func (s *Server) NewRouter() *gin.Engine {
	router := gin.Default()

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

	// API routes
	v1 := router.Group("/v1")
	v1.Use(s.initAccountInfo())
	v1.Use(s.ensureAuthenticated())
	{
		// Auth
		v1.POST("/auth/display_code", auth.DisplayCodeHandler(s.authService))
		v1.GET("/auth/token", auth.TokenHandler(s.authService))

		// Export
		v1.POST("/spaces/:space_id/objects/:object_id/export/:format", export.GetObjectExportHandler(s.exportService))
		v1.GET("/spaces/:space_id/objects/export/:format", export.GetSpaceExportHandler(s.exportService))

		// Object
		v1.GET("/spaces/:space_id/objects", paginator, object.GetObjectsHandler(s.objectService))
		v1.GET("/spaces/:space_id/objects/:object_id", object.GetObjectHandler(s.objectService))
		v1.GET("/spaces/:space_id/object_types", paginator, object.GetTypesHandler(s.objectService))
		v1.GET("/spaces/:space_id/object_types/:typeId/templates", paginator, object.GetTemplatesHandler(s.objectService))
		v1.POST("/spaces/:space_id/objects", object.CreateObjectHandler(s.objectService))
		v1.PUT("/spaces/:space_id/objects/:object_id", object.UpdateObjectHandler(s.objectService))

		// Search
		v1.GET("/search", paginator, search.SearchHandler(s.searchService))

		// Space
		v1.GET("/spaces", paginator, space.GetSpacesHandler(s.spaceService))
		v1.GET("/spaces/:space_id/members", paginator, space.GetMembersHandler(s.spaceService))
		v1.POST("/spaces", space.CreateSpaceHandler(s.spaceService))
	}

	return router
}
