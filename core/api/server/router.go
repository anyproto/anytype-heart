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

	paginator := pagination.New(pagination.Config{
		DefaultPage:     defaultPage,
		DefaultPageSize: defaultPageSize,
		MinPageSize:     minPageSize,
		MaxPageSize:     maxPageSize,
	})

	// Shared ratelimiter with the option to disable it through env var
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
		authGroup.POST("/auth/display_code", handler.DisplayCodeHandler(srv.service))
		authGroup.POST("/auth/token", handler.TokenHandler(srv.service))
		// UPDATED ROUTES
		authGroup.POST("/auth/challenges", handler.CreateChallengeHandler(srv.service))
		authGroup.POST("/auth/api_keys", handler.CreateApiKeyHandler(srv.service))
	}

	// API routes
	v1 := router.Group("/v1")
	v1.Use(paginator)
	v1.Use(srv.ensureCacheInitialized())
	v1.Use(srv.ensureAuthenticated(mw))
	{
		// Block
		// TODO: implement create, update and delete block endpoints
		// v1.POST("/spaces/:space_id/objects/:object_id/blocks", writeRateLimitMW, ensureAnalyticsEvent("CreateBlock", eventService), object.CreateBlockHandler(s.service))
		// v1.PATCH("/spaces/:space_id/objects/:object_id/blocks/:block_id", writeRateLimitMW, ensureAnalyticsEvent("UpdateBlock", eventService), object.UpdateBlockHandler(s.service))
		// v1.DELETE("/spaces/:space_id/objects/:object_id/blocks/:block_id", writeRateLimitMW, ensureAnalyticsEvent("DeleteBlock", eventService), object.DeleteBlockHandler(s.service))

		// List
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

		// Member
		v1.GET("/spaces/:space_id/members",
			srv.ensureFilters(),
			ensureAnalyticsEvent("ListMembers", eventService),
			handler.ListMembersHandler(srv.service),
		)
		v1.GET("/spaces/:space_id/members/:member_id",
			ensureAnalyticsEvent("OpenMember", eventService),
			handler.GetMemberHandler(srv.service),
		)
		// TODO: renable when granular permissions are implementeds
		// v1.PATCH("/spaces/:space_id/members/:member_id",
		// 	writeRateLimitMW,
		// 	ensureAnalyticsEvent("UpdateMember", eventService),
		// 	handler.UpdateMemberHandler(s.service),
		// )

		// File
		v1.GET("/spaces/:space_id/files",
			ensureFilters(),
			ensureAnalyticsEvent("ListFiles", eventService),
			handler.ListFilesHandler(s.service),
		)
		v1.GET("/spaces/:space_id/files/:file_id",
			ensureAnalyticsEvent("OpenFile", eventService),
			handler.GetFileHandler(s.service),
		)

		// Object
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

		// Property
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

		// Search
		v1.POST("/search",
			ensureAnalyticsEvent("GlobalSearch", eventService),
			handler.GlobalSearchHandler(srv.service),
		)
		v1.POST("/spaces/:space_id/search",
			ensureAnalyticsEvent("SpaceSearch", eventService),
			handler.SearchHandler(srv.service),
		)

		// Space
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

		// Tag
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

		// Template
		v1.GET("/spaces/:space_id/types/:type_id/templates",
			srv.ensureFilters(),
			ensureAnalyticsEvent("ListTemplates", eventService),
			handler.ListTemplatesHandler(srv.service),
		)
		v1.GET("/spaces/:space_id/types/:type_id/templates/:template_id",
			ensureAnalyticsEvent("OpenTemplate", eventService),
			handler.GetTemplateHandler(srv.service),
		)

		// Type
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

	return router
}
