package server

import (
	"os"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/anyproto/anytype-heart/core/api/apicore"
	_ "github.com/anyproto/anytype-heart/core/api/docs"
	"github.com/anyproto/anytype-heart/core/api/handler"

	"github.com/anyproto/anytype-heart/core/api/pagination"
)

const (
	defaultPage               = 0
	defaultPageSize           = 100
	minPageSize               = 1
	maxPageSize               = 1000
	maxWriteRequestsPerSecond = 1
)

// NewRouter builds and returns a *gin.Engine with all routes configured.
func (s *Server) NewRouter(mw apicore.ClientCommands) *gin.Engine {
	debug := os.Getenv("ANYTYPE_API_DEBUG") == "1"
	if !debug {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(s.ensureMetadataHeader())

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
		authGroup.POST("/display_code", handler.DisplayCodeHandler(s.authService))
		authGroup.POST("/token", handler.TokenHandler(s.authService))
	}

	// API routes
	v1 := router.Group("/v1")
	v1.Use(paginator)
	v1.Use(s.ensureAuthenticated(mw))
	{
		// Block
		// TODO: implement create, update and delete block endpoints
		// v1.POST("/spaces/:space_id/objects/:object_id/blocks", s.rateLimit(maxWriteRequestsPerSecond), object.CreateBlockHandler(s.objectService))
		// v1.PATCH("/spaces/:space_id/objects/:object_id/blocks/:block_id", s.rateLimit(maxWriteRequestsPerSecond), object.UpdateBlockHandler(s.objectService))
		// v1.DELETE("/spaces/:space_id/objects/:object_id/blocks/:block_id", s.rateLimit(maxWriteRequestsPerSecond), object.DeleteBlockHandler(s.objectService))

		// List
		v1.GET("/spaces/:space_id/lists/:list_id/views", handler.GetListViewsHandler(s.listService))
		v1.GET("/spaces/:space_id/lists/:list_id/:view_id/objects", handler.GetObjectsInListHandler(s.listService))
		v1.POST("/spaces/:space_id/lists/:list_id/objects", handler.AddObjectsToListHandler(s.listService))
		v1.DELETE("/spaces/:space_id/lists/:list_id/objects/:object_id", s.rateLimit(maxWriteRequestsPerSecond), handler.RemoveObjectFromListHandler(s.listService))

		// Object
		v1.GET("/spaces/:space_id/objects", handler.ListObjectsHandler(s.objectService))
		v1.GET("/spaces/:space_id/objects/:object_id", handler.GetObjectHandler(s.objectService))
		v1.GET("/spaces/:space_id/objects/:object_id/:format", handler.ExportObjectHandler(s.objectService))
		v1.POST("/spaces/:space_id/objects", s.rateLimit(maxWriteRequestsPerSecond), handler.CreateObjectHandler(s.objectService))
		v1.PATCH("/spaces/:space_id/objects/:object_id", s.rateLimit(maxWriteRequestsPerSecond), handler.UpdateObjectHandler(s.objectService))
		v1.DELETE("/spaces/:space_id/objects/:object_id", s.rateLimit(maxWriteRequestsPerSecond), handler.DeleteObjectHandler(s.objectService))

		// Property
		v1.GET("/spaces/:space_id/properties", handler.ListPropertiesHandler(s.objectService))
		v1.GET("/spaces/:space_id/properties/:property_id", handler.GetPropertyHandler(s.objectService))
		v1.POST("/spaces/:space_id/properties", s.rateLimit(maxWriteRequestsPerSecond), handler.CreatePropertyHandler(s.objectService))
		v1.PATCH("/spaces/:space_id/properties/:property_id", s.rateLimit(maxWriteRequestsPerSecond), handler.UpdatePropertyHandler(s.objectService))
		v1.DELETE("/spaces/:space_id/properties/:property_id", s.rateLimit(maxWriteRequestsPerSecond), handler.DeletePropertyHandler(s.objectService))

		// Tag
		v1.GET("/spaces/:space_id/properties/:property_id/tags", handler.ListTagsHandler(s.objectService))
		v1.GET("/spaces/:space_id/properties/:property_id/tags/:tag_id", handler.GetTagHandler(s.objectService))
		v1.POST("/spaces/:space_id/properties/:property_id/tags", s.rateLimit(maxWriteRequestsPerSecond), handler.CreateTagHandler(s.objectService))
		v1.PATCH("/spaces/:space_id/properties/:property_id/tags/:tag_id", s.rateLimit(maxWriteRequestsPerSecond), handler.UpdateTagHandler(s.objectService))
		v1.DELETE("/spaces/:space_id/properties/:property_id/tags/:tag_id", s.rateLimit(maxWriteRequestsPerSecond), handler.DeleteTagHandler(s.objectService))

		// Search
		v1.POST("/search", handler.GlobalSearchHandler(s.searchService))
		v1.POST("/spaces/:space_id/search", handler.SearchHandler(s.searchService))

		// Space
		v1.GET("/spaces", handler.ListSpacesHandler(s.spaceService))
		v1.GET("/spaces/:space_id", handler.GetSpaceHandler(s.spaceService))
		v1.GET("/spaces/:space_id/members", handler.ListMembersHandler(s.spaceService))
		v1.GET("/spaces/:space_id/members/:member_id", handler.GetMemberHandler(s.spaceService))
		// TODO: renable when granular permissions are implementeds
		// v1.PATCH("/spaces/:space_id/members/:member_id", s.rateLimit(maxWriteRequestsPerSecond), space.UpdateMemberHandler(s.spaceService))
		v1.POST("/spaces", s.rateLimit(maxWriteRequestsPerSecond), handler.CreateSpaceHandler(s.spaceService))

		// Type
		v1.GET("/spaces/:space_id/types", handler.ListTypesHandler(s.objectService))
		v1.GET("/spaces/:space_id/types/:type_id", handler.GetTypeHandler(s.objectService))
		v1.POST("/spaces/:space_id/types", s.rateLimit(maxWriteRequestsPerSecond), handler.CreateTypeHandler(s.objectService))
		v1.PATCH("/spaces/:space_id/types/:type_id", s.rateLimit(maxWriteRequestsPerSecond), handler.UpdateTypeHandler(s.objectService))
		v1.DELETE("/spaces/:space_id/types/:type_id", s.rateLimit(maxWriteRequestsPerSecond), handler.DeleteTypeHandler(s.objectService))

		// Template
		v1.GET("/spaces/:space_id/types/:type_id/templates", handler.ListTemplatesHandler(s.objectService))
		v1.GET("/spaces/:space_id/types/:type_id/templates/:template_id", handler.GetTemplateHandler(s.objectService))
	}

	return router
}
