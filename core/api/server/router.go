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
func (s *Server) NewRouter(mw apicore.ClientCommands) *gin.Engine {
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
	writeRateLimitMW := newWriteRateLimitMiddleware(maxWriteRequestsPerSecond, maxBurstRequests, isRateLimitDisabled)

	// Swagger route
	router.GET("/swagger/*any", func(c *gin.Context) {
		target := "https://developers.anytype.io/docs/reference"
		c.Redirect(http.StatusMovedPermanently, target)
	})

	router.GET("/docs/openapi.yaml", func(c *gin.Context) {
		data, err := os.ReadFile("./core/api/docs/openapi.yaml")
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to read OpenAPI spec")
			return
		}
		c.Data(http.StatusOK, "application/x-yaml", data)
	})

	router.GET("/docs/openapi.json", func(c *gin.Context) {
		data, err := os.ReadFile("./core/api/docs/openapi.json")
		if err != nil {
			c.String(http.StatusInternalServerError, "Failed to read OpenAPI spec")
			return
		}
		c.Data(http.StatusOK, "application/json", data)
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
		v1.GET("/spaces/:space_id/lists/:list_id/views", handler.GetListViewsHandler(s.service))
		v1.GET("/spaces/:space_id/lists/:list_id/views/:view_id/objects", handler.GetObjectsInListHandler(s.service))
		v1.POST("/spaces/:space_id/lists/:list_id/objects", writeRateLimitMW, handler.AddObjectsToListHandler(s.service))
		v1.DELETE("/spaces/:space_id/lists/:list_id/objects/:object_id", writeRateLimitMW, handler.RemoveObjectFromListHandler(s.service))

		// Member
		v1.GET("/spaces/:space_id/members", handler.ListMembersHandler(s.service))
		v1.GET("/spaces/:space_id/members/:member_id", handler.GetMemberHandler(s.service))
		// TODO: renable when granular permissions are implementeds
		// v1.PATCH("/spaces/:space_id/members/:member_id", writeRateLimitMW, space.UpdateMemberHandler(s.service))

		// Object
		v1.GET("/spaces/:space_id/objects", handler.ListObjectsHandler(s.service))
		v1.GET("/spaces/:space_id/objects/:object_id", handler.GetObjectHandler(s.service))
		v1.POST("/spaces/:space_id/objects", writeRateLimitMW, handler.CreateObjectHandler(s.service))
		v1.PATCH("/spaces/:space_id/objects/:object_id", writeRateLimitMW, handler.UpdateObjectHandler(s.service))
		v1.DELETE("/spaces/:space_id/objects/:object_id", writeRateLimitMW, handler.DeleteObjectHandler(s.service))

		// Property
		v1.GET("/spaces/:space_id/properties", handler.ListPropertiesHandler(s.service))
		v1.GET("/spaces/:space_id/properties/:property_id", handler.GetPropertyHandler(s.service))
		v1.POST("/spaces/:space_id/properties", writeRateLimitMW, handler.CreatePropertyHandler(s.service))
		v1.PATCH("/spaces/:space_id/properties/:property_id", writeRateLimitMW, handler.UpdatePropertyHandler(s.service))
		v1.DELETE("/spaces/:space_id/properties/:property_id", writeRateLimitMW, handler.DeletePropertyHandler(s.service))

		// Search
		v1.POST("/search", handler.GlobalSearchHandler(s.service))
		v1.POST("/spaces/:space_id/search", handler.SearchHandler(s.service))

		// Space
		v1.GET("/spaces", handler.ListSpacesHandler(s.service))
		v1.GET("/spaces/:space_id", handler.GetSpaceHandler(s.service))
		v1.POST("/spaces", writeRateLimitMW, handler.CreateSpaceHandler(s.service))
		v1.PATCH("/spaces/:space_id", writeRateLimitMW, handler.UpdateSpaceHandler(s.service))

		// Tag
		v1.GET("/spaces/:space_id/properties/:property_id/tags", handler.ListTagsHandler(s.service))
		v1.GET("/spaces/:space_id/properties/:property_id/tags/:tag_id", handler.GetTagHandler(s.service))
		v1.POST("/spaces/:space_id/properties/:property_id/tags", writeRateLimitMW, handler.CreateTagHandler(s.service))
		v1.PATCH("/spaces/:space_id/properties/:property_id/tags/:tag_id", writeRateLimitMW, handler.UpdateTagHandler(s.service))
		v1.DELETE("/spaces/:space_id/properties/:property_id/tags/:tag_id", writeRateLimitMW, handler.DeleteTagHandler(s.service))

		// Template
		v1.GET("/spaces/:space_id/types/:type_id/templates", handler.ListTemplatesHandler(s.service))
		v1.GET("/spaces/:space_id/types/:type_id/templates/:template_id", handler.GetTemplateHandler(s.service))

		// Type
		v1.GET("/spaces/:space_id/types", handler.ListTypesHandler(s.service))
		v1.GET("/spaces/:space_id/types/:type_id", handler.GetTypeHandler(s.service))
		v1.POST("/spaces/:space_id/types", writeRateLimitMW, handler.CreateTypeHandler(s.service))
		v1.PATCH("/spaces/:space_id/types/:type_id", writeRateLimitMW, handler.UpdateTypeHandler(s.service))
		v1.DELETE("/spaces/:space_id/types/:type_id", writeRateLimitMW, handler.DeleteTypeHandler(s.service))
	}

	return router
}
