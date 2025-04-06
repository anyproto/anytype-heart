package server

import (
	"os"

	"github.com/gin-gonic/gin"
	swaggerFiles "github.com/swaggo/files"
	ginSwagger "github.com/swaggo/gin-swagger"

	"github.com/anyproto/anytype-heart/core/api/apicore"
	_ "github.com/anyproto/anytype-heart/core/api/docs"
	"github.com/anyproto/anytype-heart/core/event"

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
	maxWriteRequestsPerSecond = 1
)

// NewRouter builds and returns a *gin.Engine with all routes configured.
func (s *Server) NewRouter(mw apicore.ClientCommands, accountService apicore.AccountService, eventService event.Sender) *gin.Engine {
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
	v1.Use(s.ensureAccountInfo(accountService))
	{
		// Export
		v1.GET("/spaces/:space_id/objects/:object_id/:format", s.ensureAnalyticsEvent("ObjectExport", eventService), export.GetObjectExportHandler(s.exportService))

		// List
		v1.GET("/spaces/:space_id/lists/:list_id/views", s.ensureAnalyticsEvent("ListGetViews", eventService), list.GetListViewsHandler(s.listService))
		v1.GET("/spaces/:space_id/lists/:list_id/:view_id/objects", s.ensureAnalyticsEvent("ListGetObjects", eventService), list.GetObjectsInListHandler(s.listService))
		v1.POST("/spaces/:space_id/lists/:list_id/objects", s.ensureAnalyticsEvent("ListAddObject", eventService), list.AddObjectsToListHandler(s.listService))
		v1.DELETE("/spaces/:space_id/lists/:list_id/objects/:object_id", s.rateLimit(maxWriteRequestsPerSecond), s.ensureAnalyticsEvent("ListRemoveObject", eventService), list.RemoveObjectFromListHandler(s.listService))

		// Object
		v1.GET("/spaces/:space_id/objects", s.ensureAnalyticsEvent("ObjectList", eventService), object.GetObjectsHandler(s.objectService))
		v1.GET("/spaces/:space_id/objects/:object_id", s.ensureAnalyticsEvent("ObjectOpen", eventService), object.GetObjectHandler(s.objectService))
		v1.DELETE("/spaces/:space_id/objects/:object_id", s.rateLimit(maxWriteRequestsPerSecond), s.ensureAnalyticsEvent("ObjectDelete", eventService), object.DeleteObjectHandler(s.objectService))
		v1.POST("/spaces/:space_id/objects", s.rateLimit(maxWriteRequestsPerSecond), s.ensureAnalyticsEvent("ObjectCreate", eventService), object.CreateObjectHandler(s.objectService))

		// Search
		v1.POST("/search", s.ensureAnalyticsEvent("GlobalSearch", eventService), search.GlobalSearchHandler(s.searchService))
		v1.POST("/spaces/:space_id/search", s.ensureAnalyticsEvent("Search", eventService), search.SearchHandler(s.searchService))

		// Space
		v1.GET("/spaces", s.ensureAnalyticsEvent("SpaceList", eventService), space.GetSpacesHandler(s.spaceService))
		v1.GET("/spaces/:space_id", s.ensureAnalyticsEvent("SpaceOpen", eventService), space.GetSpaceHandler(s.spaceService))
		v1.GET("/spaces/:space_id/members", s.ensureAnalyticsEvent("MemberList", eventService), space.GetMembersHandler(s.spaceService))
		v1.GET("/spaces/:space_id/members/:member_id", s.ensureAnalyticsEvent("MemberOpen", eventService), space.GetMemberHandler(s.spaceService))
		// v1.PATCH("/spaces/:space_id/members/:member_id", s.rateLimit(maxWriteRequestsPerSecond), s.ensureAnalyticsEvent("MemberUpdate", eventService), space.UpdateMemberHandler(s.spaceService))
		v1.POST("/spaces", s.rateLimit(maxWriteRequestsPerSecond), s.ensureAnalyticsEvent("SpaceCreate", eventService), space.CreateSpaceHandler(s.spaceService))

		// Type
		v1.GET("/spaces/:space_id/types", s.ensureAnalyticsEvent("TypeList", eventService), object.GetTypesHandler(s.objectService))
		v1.GET("/spaces/:space_id/types/:type_id", s.ensureAnalyticsEvent("TypeOpen", eventService), object.GetTypeHandler(s.objectService))
		v1.GET("/spaces/:space_id/types/:type_id/templates", s.ensureAnalyticsEvent("TemplateList", eventService), object.GetTemplatesHandler(s.objectService))
		v1.GET("/spaces/:space_id/types/:type_id/templates/:template_id", s.ensureAnalyticsEvent("TemplateOpen", eventService), object.GetTemplateHandler(s.objectService))
	}

	return router
}
