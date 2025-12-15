package server

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	_ "github.com/anyproto/anytype-heart/core/api/docs"
	"github.com/anyproto/anytype-heart/core/api/handler"
	"github.com/anyproto/anytype-heart/core/api/pagination"
)

const (
	defaultPage               = 0
	defaultPageSize           = 100
	minPageSize               = 1
	maxPageSize               = 1000
	maxWriteRequestsPerSecond = 1  // allow sustained 1 request per second
	maxBurstRequests          = 60 // allow all requests in the first second
)

// NewRouter builds and returns a *gin.Engine with all routes configured.
func (srv *Server) NewRouter(mw apicore.ClientCommands, eventService apicore.EventService, openapiYAML []byte, openapiJSON []byte) *gin.Engine {
	router := srv.setupMiddleware()

	srv.registerDocumentationRoutes(router, openapiYAML, openapiJSON)
	srv.registerAuthRoutes(router)

	paginator := createPaginationMiddleware()
	writeRateLimitMW := createRateLimitMiddleware()

	v1 := router.Group("/v1")
	v1.Use(paginator)
	v1.Use(srv.ensureCacheInitialized())
	v1.Use(srv.ensureAuthenticated(mw))

	srv.registerListRoutes(v1, eventService, writeRateLimitMW)
	srv.registerMemberRoutes(v1, eventService)
	srv.registerObjectRoutes(v1, eventService, writeRateLimitMW)
	srv.registerPropertyRoutes(v1, eventService, writeRateLimitMW)
	srv.registerSearchRoutes(v1, eventService)
	srv.registerSpaceRoutes(v1, eventService, writeRateLimitMW)
	srv.registerTagRoutes(v1, eventService, writeRateLimitMW)
	srv.registerTemplateRoutes(v1, eventService)
	srv.registerTypeRoutes(v1, eventService, writeRateLimitMW)

	return router
}

// setupMiddleware configures the base middleware for the router
func (srv *Server) setupMiddleware() *gin.Engine {
	isDebug := os.Getenv("ANYTYPE_API_DEBUG") == "1"
	if !isDebug {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(ensureMetadataHeader())

	if isDebug {
		router.Use(gin.Logger())
	}

	return router
}

// createPaginationMiddleware creates and returns pagination middleware
func createPaginationMiddleware() gin.HandlerFunc {
	return pagination.New(pagination.Config{
		DefaultPage:     defaultPage,
		DefaultPageSize: defaultPageSize,
		MinPageSize:     minPageSize,
		MaxPageSize:     maxPageSize,
	})
}

// createRateLimitMiddleware creates and returns rate limit middleware
func createRateLimitMiddleware() gin.HandlerFunc {
	isRateLimitDisabled := os.Getenv("ANYTYPE_API_DISABLE_RATE_LIMIT") == "1"
	return ensureRateLimit(maxWriteRequestsPerSecond, maxBurstRequests, isRateLimitDisabled)
}

// registerDocumentationRoutes registers Swagger and OpenAPI documentation routes
func (srv *Server) registerDocumentationRoutes(router *gin.Engine, openapiYAML []byte, openapiJSON []byte) {
	router.GET("/swagger/*any", func(c *gin.Context) {
		target := "https://developers.anytype.io/docs/reference"
		c.Redirect(http.StatusMovedPermanently, target)
	})

	router.GET("/docs/openapi.yaml", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/x-yaml", openapiYAML)
	})

	router.GET("/docs/openapi.json", func(c *gin.Context) {
		c.Data(http.StatusOK, "application/json", openapiJSON)
	})
}

// registerAuthRoutes registers authentication routes (no auth required)
func (srv *Server) registerAuthRoutes(router *gin.Engine) {
	authGroup := router.Group("/v1")
	{
		authGroup.POST("/auth/challenges", handler.CreateChallengeHandler(srv.service))
		authGroup.POST("/auth/api_keys", handler.CreateApiKeyHandler(srv.service))
	}
}

// registerListRoutes registers list-related routes
func (srv *Server) registerListRoutes(v1 *gin.RouterGroup, eventService apicore.EventService, writeRateLimitMW gin.HandlerFunc) {
	v1.GET("/spaces/:space_id/lists/:list_id/views",
		ensureAnalyticsEvent("GetListViews", eventService),
		handler.GetListViewsHandler(srv.service),
	)
	v1.GET("/spaces/:space_id/lists/:list_id/views/:view_id/objects",
		srv.ensureFilters(),
		ensureAnalyticsEvent("GetListObjects", eventService),
		handler.GetObjectsInListHandler(srv.service),
	)
	v1.POST("/spaces/:space_id/lists/:list_id/objects",
		writeRateLimitMW,
		ensureAnalyticsEvent("AddObjectToList", eventService),
		handler.AddObjectsToListHandler(srv.service),
	)
	v1.DELETE("/spaces/:space_id/lists/:list_id/objects/:object_id",
		writeRateLimitMW,
		ensureAnalyticsEvent("RemoveObjectFromList", eventService),
		handler.RemoveObjectFromListHandler(srv.service),
	)
}

// registerMemberRoutes registers member-related routes
func (srv *Server) registerMemberRoutes(v1 *gin.RouterGroup, eventService apicore.EventService) {
	v1.GET("/spaces/:space_id/members",
		srv.ensureFilters(),
		ensureAnalyticsEvent("ListMembers", eventService),
		handler.ListMembersHandler(srv.service),
	)
	v1.GET("/spaces/:space_id/members/:member_id",
		ensureAnalyticsEvent("OpenMember", eventService),
		handler.GetMemberHandler(srv.service),
	)
	// TODO: renable when granular permissions are implemented
	// v1.PATCH("/spaces/:space_id/members/:member_id",
	// 	writeRateLimitMW,
	// 	ensureAnalyticsEvent("UpdateMember", eventService),
	// 	handler.UpdateMemberHandler(srv.service),
	// )
}

// registerObjectRoutes registers object-related routes
func (srv *Server) registerObjectRoutes(v1 *gin.RouterGroup, eventService apicore.EventService, writeRateLimitMW gin.HandlerFunc) {
	v1.GET("/spaces/:space_id/objects",
		srv.ensureFilters(),
		ensureAnalyticsEvent("ListObjects", eventService),
		handler.ListObjectsHandler(srv.service),
	)
	v1.GET("/spaces/:space_id/objects/:object_id",
		ensureAnalyticsEvent("OpenObject", eventService),
		handler.GetObjectHandler(srv.service),
	)
	v1.POST("/spaces/:space_id/objects",
		writeRateLimitMW,
		ensureAnalyticsEvent("CreateObject", eventService),
		handler.CreateObjectHandler(srv.service),
	)
	v1.PATCH("/spaces/:space_id/objects/:object_id",
		writeRateLimitMW,
		ensureAnalyticsEvent("UpdateObject", eventService),
		handler.UpdateObjectHandler(srv.service),
	)
	v1.DELETE("/spaces/:space_id/objects/:object_id",
		writeRateLimitMW,
		ensureAnalyticsEvent("DeleteObject", eventService),
		handler.DeleteObjectHandler(srv.service),
	)
}

// registerPropertyRoutes registers property-related routes
func (srv *Server) registerPropertyRoutes(v1 *gin.RouterGroup, eventService apicore.EventService, writeRateLimitMW gin.HandlerFunc) {
	v1.GET("/spaces/:space_id/properties",
		srv.ensureFilters(),
		ensureAnalyticsEvent("ListProperties", eventService),
		handler.ListPropertiesHandler(srv.service),
	)
	v1.GET("/spaces/:space_id/properties/:property_id",
		ensureAnalyticsEvent("OpenProperty", eventService),
		handler.GetPropertyHandler(srv.service),
	)
	v1.POST("/spaces/:space_id/properties",
		writeRateLimitMW,
		ensureAnalyticsEvent("CreateProperty", eventService),
		handler.CreatePropertyHandler(srv.service),
	)
	v1.PATCH("/spaces/:space_id/properties/:property_id",
		writeRateLimitMW,
		ensureAnalyticsEvent("UpdateProperty", eventService),
		handler.UpdatePropertyHandler(srv.service),
	)
	v1.DELETE("/spaces/:space_id/properties/:property_id",
		writeRateLimitMW,
		ensureAnalyticsEvent("DeleteProperty", eventService),
		handler.DeletePropertyHandler(srv.service),
	)
}

// registerSearchRoutes registers search-related routes
func (srv *Server) registerSearchRoutes(v1 *gin.RouterGroup, eventService apicore.EventService) {
	v1.POST("/search",
		ensureAnalyticsEvent("GlobalSearch", eventService),
		handler.GlobalSearchHandler(srv.service),
	)
	v1.POST("/spaces/:space_id/search",
		ensureAnalyticsEvent("SpaceSearch", eventService),
		handler.SearchHandler(srv.service),
	)
}

// registerSpaceRoutes registers space-related routes
func (srv *Server) registerSpaceRoutes(v1 *gin.RouterGroup, eventService apicore.EventService, writeRateLimitMW gin.HandlerFunc) {
	v1.GET("/spaces",
		srv.ensureFilters(),
		ensureAnalyticsEvent("ListSpaces", eventService),
		handler.ListSpacesHandler(srv.service),
	)
	v1.GET("/spaces/:space_id",
		ensureAnalyticsEvent("OpenSpace", eventService),
		handler.GetSpaceHandler(srv.service),
	)
	v1.POST("/spaces",
		writeRateLimitMW,
		ensureAnalyticsEvent("CreateSpace", eventService),
		handler.CreateSpaceHandler(srv.service),
	)
	v1.PATCH("/spaces/:space_id",
		writeRateLimitMW,
		ensureAnalyticsEvent("UpdateSpace", eventService),
		handler.UpdateSpaceHandler(srv.service),
	)
}

// registerTagRoutes registers tag-related routes
func (srv *Server) registerTagRoutes(v1 *gin.RouterGroup, eventService apicore.EventService, writeRateLimitMW gin.HandlerFunc) {
	v1.GET("/spaces/:space_id/properties/:property_id/tags",
		srv.ensureFilters(),
		ensureAnalyticsEvent("ListTags", eventService),
		handler.ListTagsHandler(srv.service),
	)
	v1.GET("/spaces/:space_id/properties/:property_id/tags/:tag_id",
		ensureAnalyticsEvent("OpenTag", eventService),
		handler.GetTagHandler(srv.service),
	)
	v1.POST("/spaces/:space_id/properties/:property_id/tags",
		writeRateLimitMW,
		ensureAnalyticsEvent("CreateTag", eventService),
		handler.CreateTagHandler(srv.service),
	)
	v1.PATCH("/spaces/:space_id/properties/:property_id/tags/:tag_id",
		writeRateLimitMW,
		ensureAnalyticsEvent("UpdateTag", eventService),
		handler.UpdateTagHandler(srv.service),
	)
	v1.DELETE("/spaces/:space_id/properties/:property_id/tags/:tag_id",
		writeRateLimitMW,
		ensureAnalyticsEvent("DeleteTag", eventService),
		handler.DeleteTagHandler(srv.service),
	)
}

// registerTemplateRoutes registers template-related routes
func (srv *Server) registerTemplateRoutes(v1 *gin.RouterGroup, eventService apicore.EventService) {
	v1.GET("/spaces/:space_id/types/:type_id/templates",
		srv.ensureFilters(),
		ensureAnalyticsEvent("ListTemplates", eventService),
		handler.ListTemplatesHandler(srv.service),
	)
	v1.GET("/spaces/:space_id/types/:type_id/templates/:template_id",
		ensureAnalyticsEvent("OpenTemplate", eventService),
		handler.GetTemplateHandler(srv.service),
	)
}

// registerTypeRoutes registers type-related routes
func (srv *Server) registerTypeRoutes(v1 *gin.RouterGroup, eventService apicore.EventService, writeRateLimitMW gin.HandlerFunc) {
	v1.GET("/spaces/:space_id/types",
		srv.ensureFilters(),
		ensureAnalyticsEvent("ListTypes", eventService),
		handler.ListTypesHandler(srv.service),
	)
	v1.GET("/spaces/:space_id/types/:type_id",
		ensureAnalyticsEvent("OpenType", eventService),
		handler.GetTypeHandler(srv.service),
	)
	v1.POST("/spaces/:space_id/types",
		writeRateLimitMW,
		ensureAnalyticsEvent("CreateType", eventService),
		handler.CreateTypeHandler(srv.service),
	)
	v1.PATCH("/spaces/:space_id/types/:type_id",
		writeRateLimitMW,
		ensureAnalyticsEvent("UpdateType", eventService),
		handler.UpdateTypeHandler(srv.service),
	)
	v1.DELETE("/spaces/:space_id/types/:type_id",
		writeRateLimitMW,
		ensureAnalyticsEvent("DeleteType", eventService),
		handler.DeleteTypeHandler(srv.service),
	)
}
