package server

import (
	"os"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/anyproto/anytype-heart/core/api/core"
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
		authGroup.POST("/display_code", handler.DisplayCodeHandler(s.service))
		authGroup.POST("/token", handler.TokenHandler(s.service))
	}

	// API routes
	v1 := router.Group("/v1")
	v1.Use(paginator)
	v1.Use(s.ensureAuthenticated(mw))
	{
		// Block
		// TODO: implement create, update and delete block endpoints
		// v1.POST("/spaces/:space_id/objects/:object_id/blocks", s.rateLimit(maxWriteRequestsPerSecond), object.CreateBlockHandler(s.service))
		// v1.PATCH("/spaces/:space_id/objects/:object_id/blocks/:block_id", s.rateLimit(maxWriteRequestsPerSecond), object.UpdateBlockHandler(s.service))
		// v1.DELETE("/spaces/:space_id/objects/:object_id/blocks/:block_id", s.rateLimit(maxWriteRequestsPerSecond), object.DeleteBlockHandler(s.service))

		// List
		v1.GET("/spaces/:space_id/lists/:list_id/views", handler.GetListViewsHandler(s.service))
		v1.GET("/spaces/:space_id/lists/:list_id/:view_id/objects", handler.GetObjectsInListHandler(s.service))
		v1.POST("/spaces/:space_id/lists/:list_id/objects", handler.AddObjectsToListHandler(s.service))
		v1.DELETE("/spaces/:space_id/lists/:list_id/objects/:object_id", s.rateLimit(maxWriteRequestsPerSecond), handler.RemoveObjectFromListHandler(s.service))

		// Object
		v1.GET("/spaces/:space_id/objects", handler.ListObjectsHandler(s.service))
		v1.GET("/spaces/:space_id/objects/:object_id", handler.GetObjectHandler(s.service))
		v1.GET("/spaces/:space_id/objects/:object_id/:format", handler.ExportObjectHandler(s.service))
		v1.POST("/spaces/:space_id/objects", s.rateLimit(maxWriteRequestsPerSecond), handler.CreateObjectHandler(s.service))
		v1.PATCH("/spaces/:space_id/objects/:object_id", s.rateLimit(maxWriteRequestsPerSecond), handler.UpdateObjectHandler(s.service))
		v1.DELETE("/spaces/:space_id/objects/:object_id", s.rateLimit(maxWriteRequestsPerSecond), handler.DeleteObjectHandler(s.service))

		// Property
		v1.GET("/spaces/:space_id/properties", handler.ListPropertiesHandler(s.service))
		v1.GET("/spaces/:space_id/properties/:property_id", handler.GetPropertyHandler(s.service))
		v1.POST("/spaces/:space_id/properties", s.rateLimit(maxWriteRequestsPerSecond), handler.CreatePropertyHandler(s.service))
		v1.PATCH("/spaces/:space_id/properties/:property_id", s.rateLimit(maxWriteRequestsPerSecond), handler.UpdatePropertyHandler(s.service))
		v1.DELETE("/spaces/:space_id/properties/:property_id", s.rateLimit(maxWriteRequestsPerSecond), handler.DeletePropertyHandler(s.service))

		// Tag
		v1.GET("/spaces/:space_id/properties/:property_id/tags", handler.ListTagsHandler(s.service))
		v1.GET("/spaces/:space_id/properties/:property_id/tags/:tag_id", handler.GetTagHandler(s.service))
		v1.POST("/spaces/:space_id/properties/:property_id/tags", s.rateLimit(maxWriteRequestsPerSecond), handler.CreateTagHandler(s.service))
		v1.PATCH("/spaces/:space_id/properties/:property_id/tags/:tag_id", s.rateLimit(maxWriteRequestsPerSecond), handler.UpdateTagHandler(s.service))
		v1.DELETE("/spaces/:space_id/properties/:property_id/tags/:tag_id", s.rateLimit(maxWriteRequestsPerSecond), handler.DeleteTagHandler(s.service))

		// Search
		v1.POST("/search", handler.GlobalSearchHandler(s.service))
		v1.POST("/spaces/:space_id/search", handler.SearchHandler(s.service))

		// Space
		v1.GET("/spaces", handler.ListSpacesHandler(s.service))
		v1.GET("/spaces/:space_id", handler.GetSpaceHandler(s.service))
		v1.GET("/spaces/:space_id/members", handler.ListMembersHandler(s.service))
		v1.GET("/spaces/:space_id/members/:member_id", handler.GetMemberHandler(s.service))
		// TODO: renable when granular permissions are implementeds
		// v1.PATCH("/spaces/:space_id/members/:member_id", s.rateLimit(maxWriteRequestsPerSecond), space.UpdateMemberHandler(s.service))
		v1.POST("/spaces", s.rateLimit(maxWriteRequestsPerSecond), handler.CreateSpaceHandler(s.service))

		// Type
		v1.GET("/spaces/:space_id/types", handler.ListTypesHandler(s.service))
		v1.GET("/spaces/:space_id/types/:type_id", handler.GetTypeHandler(s.service))
		v1.POST("/spaces/:space_id/types", s.rateLimit(maxWriteRequestsPerSecond), handler.CreateTypeHandler(s.service))
		v1.PATCH("/spaces/:space_id/types/:type_id", s.rateLimit(maxWriteRequestsPerSecond), handler.UpdateTypeHandler(s.service))
		v1.DELETE("/spaces/:space_id/types/:type_id", s.rateLimit(maxWriteRequestsPerSecond), handler.DeleteTypeHandler(s.service))

		// Template
		v1.GET("/spaces/:space_id/types/:type_id/templates", handler.ListTemplatesHandler(s.service))
		v1.GET("/spaces/:space_id/types/:type_id/templates/:template_id", handler.GetTemplateHandler(s.service))
	}

	return router
}
