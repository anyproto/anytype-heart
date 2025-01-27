package server

import (
	"os"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/anyproto/anytype-heart/core/anytype/account"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	"github.com/anyproto/anytype-heart/core/api/services/auth"
	"github.com/anyproto/anytype-heart/core/api/services/export"
	"github.com/anyproto/anytype-heart/core/api/services/object"
	"github.com/anyproto/anytype-heart/core/api/services/search"
	"github.com/anyproto/anytype-heart/core/api/services/space"
	"github.com/anyproto/anytype-heart/pb/service"
)

const (
	defaultPage               = 0
	defaultPageSize           = 100
	minPageSize               = 1
	maxPageSize               = 1000
	maxWriteRequestsPerSecond = 1
)

// NewRouter builds and returns a *gin.Engine with all routes configured.
func (s *Server) NewRouter(accountService account.Service, mw service.ClientCommandsServer) *gin.Engine {
	debug := os.Getenv("ANYTYPE_API_DEBUG") == "1"
	if !debug {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())

	if debug {
		router.Use(gin.Logger())
	}
	paginator := pagination.New(pagination.Config{
		DefaultPage:     defaultPage,
		DefaultPageSize: defaultPageSize,
		MinPageSize:     minPageSize,
		MaxPageSize:     maxPageSize,
	})

	// Swagger route
	router.GET("/swagger/*any", ginSwagger.WrapHandler(swaggerFiles.Handler))

	// Auth routes (no authentication required)
	authGroup := router.Group("/v1/auth")
	{
		authGroup.POST("/display_code", auth.DisplayCodeHandler(s.authService))
		authGroup.POST("/token", auth.TokenHandler(s.authService))
	}

	// API routes
	v1 := router.Group("/v1")
	v1.Use(paginator)
	v1.Use(s.ensureAuthenticated(mw))
	v1.Use(s.ensureAccountInfo(accountService))
	{
		// Export
		v1.POST("/spaces/:space_id/objects/:object_id/export/:format", export.GetObjectExportHandler(s.exportService))

		// Object
		v1.GET("/spaces/:space_id/objects", object.GetObjectsHandler(s.objectService))
		v1.GET("/spaces/:space_id/objects/:object_id", object.GetObjectHandler(s.objectService))
		v1.DELETE("/spaces/:space_id/objects/:object_id", s.rateLimit(maxWriteRequestsPerSecond), object.DeleteObjectHandler(s.objectService))
		v1.GET("/spaces/:space_id/types", object.GetTypesHandler(s.objectService))
		v1.GET("/spaces/:space_id/types/:type_id/templates", object.GetTemplatesHandler(s.objectService))
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
