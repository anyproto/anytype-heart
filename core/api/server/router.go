package server

import (
	"github.com/anyproto/any-sync/app"
	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/services/auth"
	"github.com/anyproto/anytype-heart/core/api/services/export"
	"github.com/anyproto/anytype-heart/core/api/services/object"
	"github.com/anyproto/anytype-heart/core/api/services/search"
	"github.com/anyproto/anytype-heart/core/api/services/space"
)

const (
	defaultPage               = 0
	defaultPageSize           = 100
	minPageSize               = 1
	maxPageSize               = 1000
	maxWriteRequestsPerSecond = 1
)

// NewRouter builds and returns a *gin.Engine with all routes configured.
func (s *Server) NewRouter(a *app.App) *gin.Engine {
	router := gin.Default()

	paginator := pagination.New(pagination.Config{
		DefaultPage:     defaultPage,
		DefaultPageSize: defaultPageSize,
		MinPageSize:     minPageSize,
		MaxPageSize:     maxPageSize,
	})

	// Swagger route
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// API routes
	v1 := router.Group("/v1")
	v1.Use(paginator)
	v1.Use(s.ensureAuthenticated())
	v1.Use(s.ensureAccountInfo(a))
	{
		// Auth
		v1.POST("/auth/display_code", auth.DisplayCodeHandler(s.authService))
		v1.POST("/auth/token", auth.TokenHandler(s.authService))

		// Export
		v1.POST("/spaces/:space_id/objects/:object_id/export/:format", export.GetObjectExportHandler(s.exportService))

		// Object
		v1.GET("/spaces/:space_id/objects", object.GetObjectsHandler(s.objectService))
		v1.GET("/spaces/:space_id/objects/:object_id", object.GetObjectHandler(s.objectService))
		v1.DELETE("/spaces/:space_id/objects/:object_id", s.rateLimit(maxWriteRequestsPerSecond), object.DeleteObjectHandler(s.objectService))
		v1.GET("/spaces/:space_id/object_types", object.GetTypesHandler(s.objectService))
		v1.GET("/spaces/:space_id/object_types/:typeId/templates", object.GetTemplatesHandler(s.objectService))
		v1.POST("/spaces/:space_id/objects", s.rateLimit(maxWriteRequestsPerSecond), object.CreateObjectHandler(s.objectService))

		// Search
		v1.GET("/search", search.SearchHandler(s.searchService))

		// Space
		v1.GET("/spaces", space.GetSpacesHandler(s.spaceService))
		v1.GET("/spaces/:space_id/members", space.GetMembersHandler(s.spaceService))
		v1.POST("/spaces", s.rateLimit(maxWriteRequestsPerSecond), space.CreateSpaceHandler(s.spaceService))
	}

	return router
}
