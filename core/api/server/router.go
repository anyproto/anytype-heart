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

func (s *Server) NewRouter(mw apicore.ClientCommands, eventService apicore.EventService, openapiYAML []byte, openapiJSON []byte) *gin.Engine {
	isDebug := os.Getenv("ANYTYPE_API_DEBUG") == "1"
	if !isDebug {
		gin.SetMode(gin.ReleaseMode)
	}

	router := gin.New()
	router.Use(gin.Recovery())
	router.Use(s.ensureMetadataHeader())

	if isDebug {
		router.Use(gin.Logger())
	}

	paginator := pagination.New(pagination.Config{
		DefaultPage:     defaultPage,
		DefaultPageSize: defaultPageSize,
		MinPageSize:     minPageSize,
		MaxPageSize:     maxPageSize,
	})

	// Shared ratelimiter with option to disable it through env var
	isRateLimitDisabled := os.Getenv("ANYTYPE_API_DISABLE_RATE_LIMIT") == "1"
	writeRateLimitMW := ensureRateLimit(maxWriteRequestsPerSecond, maxBurstRequests, isRateLimitDisabled)

	// Swagger route
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

	// Auth routes (no authentication required)
	authGroup := router.Group("/v1")
	{
		// TO BE DEPRECATED
		authGroup.POST("/auth/display_code", handler.DisplayCodeHandler(s.service))
		// TO BE DEPRECATED
		authGroup.POST("/auth/token", handler.TokenHandler(s.service))

		authGroup.POST("/auth/challenges", handler.CreateChallengeHandler(s.service))
		authGroup.POST("/auth/api_keys", handler.CreateApiKeyHandler(s.service))
	}

	// API routes
	v1 := router.Group("/v1")
	v1.Use(paginator)
	v1.Use(s.ensureAuthenticated(mw))
	{
		// Block
		// TODO: implement create, update and delete block endpoints
		// v1.POST("/spaces/:space_id/objects/:object_id/blocks", writeRateLimitMW, object.CreateBlockHandler(s.service))
		// v1.PATCH("/spaces/:space_id/objects/:object_id/blocks/:block_id", writeRateLimitMW, object.UpdateBlockHandler(s.service))
		// v1.DELETE("/spaces/:space_id/objects/:object_id/blocks/:block_id", writeRateLimitMW, object.DeleteBlockHandler(s.service))

		// List
		v1.GET("/spaces/:space_id/lists/:list_id/views", s.ensureAnalyticsEvent("ListGetViews", eventService), handler.GetListViewsHandler(s.service))
		v1.GET("/spaces/:space_id/lists/:list_id/views/:view_id/objects", s.ensureAnalyticsEvent("ListGetObjects", eventService), handler.GetObjectsInListHandler(s.service))
		v1.POST("/spaces/:space_id/lists/:list_id/objects", writeRateLimitMW, s.ensureAnalyticsEvent("ListAddObject", eventService), handler.AddObjectsToListHandler(s.service))
		v1.DELETE("/spaces/:space_id/lists/:list_id/objects/:object_id", writeRateLimitMW, s.ensureAnalyticsEvent("ListRemoveObject", eventService), handler.RemoveObjectFromListHandler(s.service))

		// Member
		v1.GET("/spaces/:space_id/members", s.ensureAnalyticsEvent("MemberList", eventService), handler.ListMembersHandler(s.service))
		v1.GET("/spaces/:space_id/members/:member_id", s.ensureAnalyticsEvent("MemberOpen", eventService), handler.GetMemberHandler(s.service))
		// TODO: renable when granular permissions are implementeds
		// v1.PATCH("/spaces/:space_id/members/:member_id", writeRateLimitMW, space.UpdateMemberHandler(s.service))

		// Object
		v1.GET("/spaces/:space_id/objects", s.ensureAnalyticsEvent("ObjectList", eventService), handler.ListObjectsHandler(s.service))
		v1.GET("/spaces/:space_id/objects/:object_id", s.ensureAnalyticsEvent("ObjectOpen", eventService), handler.GetObjectHandler(s.service))
		v1.POST("/spaces/:space_id/objects", writeRateLimitMW, s.ensureAnalyticsEvent("ObjectCreate", eventService), handler.CreateObjectHandler(s.service))
		v1.PATCH("/spaces/:space_id/objects/:object_id", writeRateLimitMW, s.ensureAnalyticsEvent("ObjectUpdate", eventService), handler.UpdateObjectHandler(s.service))
		v1.DELETE("/spaces/:space_id/objects/:object_id", writeRateLimitMW, s.ensureAnalyticsEvent("ObjectDelete", eventService), handler.DeleteObjectHandler(s.service))

		// Property
		v1.GET("/spaces/:space_id/properties", s.ensureAnalyticsEvent("PropertyList", eventService), handler.ListPropertiesHandler(s.service))
		v1.GET("/spaces/:space_id/properties/:property_id", s.ensureAnalyticsEvent("PropertyOpen", eventService), handler.GetPropertyHandler(s.service))
		v1.POST("/spaces/:space_id/properties", writeRateLimitMW, s.ensureAnalyticsEvent("PropertyCreate", eventService), handler.CreatePropertyHandler(s.service))
		v1.PATCH("/spaces/:space_id/properties/:property_id", writeRateLimitMW, s.ensureAnalyticsEvent("PropertyUpdate", eventService), handler.UpdatePropertyHandler(s.service))
		v1.DELETE("/spaces/:space_id/properties/:property_id", writeRateLimitMW, s.ensureAnalyticsEvent("PropertyDelete", eventService), handler.DeletePropertyHandler(s.service))

		// Search
		v1.POST("/search", s.ensureAnalyticsEvent("SearchGlobal", eventService), handler.GlobalSearchHandler(s.service))
		v1.POST("/spaces/:space_id/search", s.ensureAnalyticsEvent("SearchSpace", eventService), handler.SearchHandler(s.service))

		// Space
		v1.GET("/spaces", s.ensureAnalyticsEvent("SpaceList", eventService), handler.ListSpacesHandler(s.service))
		v1.GET("/spaces/:space_id", s.ensureAnalyticsEvent("SpaceOpen", eventService), handler.GetSpaceHandler(s.service))
		v1.POST("/spaces", writeRateLimitMW, s.ensureAnalyticsEvent("SpaceCreate", eventService), handler.CreateSpaceHandler(s.service))
		v1.PATCH("/spaces/:space_id", writeRateLimitMW, s.ensureAnalyticsEvent("SpaceUpdate", eventService), handler.UpdateSpaceHandler(s.service))

		// Tag
		v1.GET("/spaces/:space_id/properties/:property_id/tags", s.ensureAnalyticsEvent("TagList", eventService), handler.ListTagsHandler(s.service))
		v1.GET("/spaces/:space_id/properties/:property_id/tags/:tag_id", s.ensureAnalyticsEvent("TagOpen", eventService), handler.GetTagHandler(s.service))
		v1.POST("/spaces/:space_id/properties/:property_id/tags", writeRateLimitMW, s.ensureAnalyticsEvent("TagCreate", eventService), handler.CreateTagHandler(s.service))
		v1.PATCH("/spaces/:space_id/properties/:property_id/tags/:tag_id", writeRateLimitMW, s.ensureAnalyticsEvent("TagUpdate", eventService), handler.UpdateTagHandler(s.service))
		v1.DELETE("/spaces/:space_id/properties/:property_id/tags/:tag_id", writeRateLimitMW, s.ensureAnalyticsEvent("TagDelete", eventService), handler.DeleteTagHandler(s.service))

		// Template
		v1.GET("/spaces/:space_id/types/:type_id/templates", s.ensureAnalyticsEvent("TemplateList", eventService), handler.ListTemplatesHandler(s.service))
		v1.GET("/spaces/:space_id/types/:type_id/templates/:template_id", s.ensureAnalyticsEvent("TemplateOpen", eventService), handler.GetTemplateHandler(s.service))

		// Type
		v1.GET("/spaces/:space_id/types", s.ensureAnalyticsEvent("TypeList", eventService), handler.ListTypesHandler(s.service))
		v1.GET("/spaces/:space_id/types/:type_id", s.ensureAnalyticsEvent("TypeOpen", eventService), handler.GetTypeHandler(s.service))
		v1.POST("/spaces/:space_id/types", writeRateLimitMW, s.ensureAnalyticsEvent("TypeCreate", eventService), handler.CreateTypeHandler(s.service))
		v1.PATCH("/spaces/:space_id/types/:type_id", writeRateLimitMW, s.ensureAnalyticsEvent("TypeUpdate", eventService), handler.UpdateTypeHandler(s.service))
		v1.DELETE("/spaces/:space_id/types/:type_id", writeRateLimitMW, s.ensureAnalyticsEvent("TypeDelete", eventService), handler.DeleteTypeHandler(s.service))
	}

	return router
}
