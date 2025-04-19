package server

import (
	"os"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/anyproto/anytype-heart/core/api/apicore"
	_ "github.com/anyproto/anytype-heart/core/api/docs"

	"github.com/anyproto/anytype-heart/core/api/internal/auth"
	"github.com/anyproto/anytype-heart/core/api/internal/export"
	"github.com/anyproto/anytype-heart/core/api/internal/list"
	"github.com/anyproto/anytype-heart/core/api/internal/object"
	"github.com/anyproto/anytype-heart/core/api/internal/search"
	"github.com/anyproto/anytype-heart/core/api/internal/space"
	"github.com/anyproto/anytype-heart/core/api/pagination"
)

const (
	defaultPage               = 0
	defaultPageSize           = 100
	minPageSize               = 1
	maxPageSize               = 1000
	maxWriteRequestsPerSecond = 100
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
		authGroup.POST("/display_code", auth.DisplayCodeHandler(s.authService))
		authGroup.POST("/token", auth.TokenHandler(s.authService))
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

		// Export
		v1.GET("/spaces/:space_id/objects/:object_id/:format", export.GetObjectExportHandler(s.exportService))

		// List
		v1.GET("/spaces/:space_id/lists/:list_id/views", list.GetListViewsHandler(s.listService))
		v1.GET("/spaces/:space_id/lists/:list_id/:view_id/objects", list.GetObjectsInListHandler(s.listService))
		v1.POST("/spaces/:space_id/lists/:list_id/objects", list.AddObjectsToListHandler(s.listService))
		v1.DELETE("/spaces/:space_id/lists/:list_id/objects/:object_id", s.rateLimit(maxWriteRequestsPerSecond), list.RemoveObjectFromListHandler(s.listService))

		// Object
		v1.GET("/spaces/:space_id/objects", object.GetObjectsHandler(s.objectService))
		v1.GET("/spaces/:space_id/objects/:object_id", object.GetObjectHandler(s.objectService))
		v1.POST("/spaces/:space_id/objects", s.rateLimit(maxWriteRequestsPerSecond), object.CreateObjectHandler(s.objectService))
		// TODO: implement update object properties endpoint
		// v1.PATCH("/spaces/:space_id/objects/:object_id", s.rateLimit(maxWriteRequestsPerSecond), object.UpdateObjectHandler(s.objectService))
		v1.DELETE("/spaces/:space_id/objects/:object_id", s.rateLimit(maxWriteRequestsPerSecond), object.DeleteObjectHandler(s.objectService))

		// Property
		v1.GET("/spaces/:space_id/properties", object.GetPropertiesHandler(s.objectService))
		v1.GET("/spaces/:space_id/properties/:property_id", object.GetPropertyHandler(s.objectService))
		// TODO: implement  create, delete and update property endpoints
		// v1.POST("/spaces/:space_id/properties", s.rateLimit(maxWriteRequestsPerSecond), object.CreatePropertyHandler(s.objectService))
		// v1.PATCH("/spaces/:space_id/properties/:property_id", s.rateLimit(maxWriteRequestsPerSecond), object.UpdatePropertyHandler(s.objectService))
		// v1.DELETE("/spaces/:space_id/properties/:property_id", s.rateLimit(maxWriteRequestsPerSecond), object.DeletePropertyHandler(s.objectService))
		v1.GET("/spaces/:space_id/properties/:property_id/options", object.GetPropertyOptionsHandler(s.objectService))
		// TODO: implement create, delete and update property options endpoints
		// v1.GET("/spaces/:space_id/properties/:property_id/options/:option_id", object.GetPropertyOptionHandler(s.objectService))
		// v1.POST("/spaces/:space_id/properties/:property_id/options", s.rateLimit(maxWriteRequestsPerSecond), object.AddPropertyOptionHandler(s.objectService))
		// v1.PATCH("/spaces/:space_id/properties/:property_id/options/:option_id", s.rateLimit(maxWriteRequestsPerSecond), object.UpdatePropertyOptionHandler(s.objectService))
		// v1.DELETE("/spaces/:space_id/properties/:property_id/options/:option_id", s.rateLimit(maxWriteRequestsPerSecond), object.RemovePropertyOptionHandler(s.objectService))

		// Search
		v1.POST("/search", search.GlobalSearchHandler(s.searchService))
		v1.POST("/spaces/:space_id/search", search.SearchHandler(s.searchService))

		// Space
		v1.GET("/spaces", space.GetSpacesHandler(s.spaceService))
		v1.GET("/spaces/:space_id", space.GetSpaceHandler(s.spaceService))
		v1.GET("/spaces/:space_id/members", space.GetMembersHandler(s.spaceService))
		v1.GET("/spaces/:space_id/members/:member_id", space.GetMemberHandler(s.spaceService))
		// TODO: renable when granular permissions are implementeds
		// v1.PATCH("/spaces/:space_id/members/:member_id", s.rateLimit(maxWriteRequestsPerSecond), space.UpdateMemberHandler(s.spaceService))
		v1.POST("/spaces", s.rateLimit(maxWriteRequestsPerSecond), space.CreateSpaceHandler(s.spaceService))

		// Type
		v1.GET("/spaces/:space_id/types", object.GetTypesHandler(s.objectService))
		v1.GET("/spaces/:space_id/types/:type_id", object.GetTypeHandler(s.objectService))
		// TODO: implement add and remove type properties endpoints
		// v1.POST("/spaces/:space_id/types/:type_id/properties", s.rateLimit(maxWriteRequestsPerSecond), object.AddTypePropertyHandler(s.objectService))
		// v1.DELETE("/spaces/:space_id/types/:type_id/properties/:property_id", s.rateLimit(maxWriteRequestsPerSecond), object.DeleteTypePropertyHandler(s.objectService))

		// Template
		v1.GET("/spaces/:space_id/types/:type_id/templates", object.GetTemplatesHandler(s.objectService))
		v1.GET("/spaces/:space_id/types/:type_id/templates/:template_id", object.GetTemplateHandler(s.objectService))
	}

	return router
}
