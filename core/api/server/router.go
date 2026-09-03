package server

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	"github.com/anyproto/anytype-heart/core/api/handler"
	"github.com/anyproto/anytype-heart/core/api/pagination"
	apiv2 "github.com/anyproto/anytype-heart/core/api/v2"
	"github.com/anyproto/anytype-heart/util/localorigin"
)

const (
	defaultPage               = 0
	defaultPageSize           = 100
	minPageSize               = 1
	maxPageSize               = 1000
	maxWriteRequestsPerSecond = 1  // allow sustained 1 request per second
	maxBurstRequests          = 60 // allow all requests in the first second
)

// envApiAllowedOrigins adds comma-separated exact origins to the allowlist, for
// local clients that are not served from a loopback host.
const envApiAllowedOrigins = "ANYTYPE_API_ALLOWED_ORIGINS"

// envApiAllowedHosts adds comma-separated Host header values to the allowlist,
// for operators who bind the API to a routable interface and reach it by name.
const envApiAllowedHosts = "ANYTYPE_API_ALLOWED_HOSTS"

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
	// No scope gate on /v1: keys minted without a scope carry Limited
	// (anytype-cli's CreateApp historically sent none) and must keep working
	// here — the JSON-API scope gate (ensureJsonApiScope) is /v2-only.
	v1.Use(srv.ensureAuthenticated(mw))
	// GRANTED keys are the one exception to /v1's grandfathering: their
	// grant can only be honored on /v2, so /v1 refuses them with a pointer
	// there. Legacy (nil-grant) keys pass untouched.
	v1.Use(ensureUngrantedKey())

	srv.registerChatRoutes(v1, eventService, writeRateLimitMW)
	srv.registerFileRoutes(v1, eventService, writeRateLimitMW)
	srv.registerListRoutes(v1, eventService, writeRateLimitMW)
	srv.registerMemberRoutes(v1, eventService)
	srv.registerObjectRoutes(v1, eventService, writeRateLimitMW)
	srv.registerPropertyRoutes(v1, eventService, writeRateLimitMW)
	srv.registerSearchRoutes(v1, eventService)
	srv.registerSpaceRoutes(v1, eventService, writeRateLimitMW)
	srv.registerTagRoutes(v1, eventService, writeRateLimitMW)
	srv.registerTemplateRoutes(v1, eventService)
	srv.registerTypeRoutes(v1, eventService, writeRateLimitMW)

	apiv2.RegisterRoutes(router, apiv2.RouteDeps{
		Service:        srv.v2Service,
		CreateDisabled: srv.v2CreateDisabled,
		EditDisabled:   srv.v2EditDisabled,
		StreamDisabled: srv.v2StreamDisabled,
		Auth:           srv.ensureAuthenticated(mw),
		KeyScope:       ensureJsonApiScope(),
		CacheInit:      srv.ensureCacheInitialized(),
		WriteRateLimit: writeRateLimitMW,
		AnalyticsEvent: func(code string) gin.HandlerFunc {
			return ensureAnalyticsEvent(code, eventService)
		},
	})

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
	// Before every route, including the unauthenticated /v1/auth ones.
	// Only native clients talk to this API, so file:// is not trusted here.
	router.Use(ensureTrustedOrigin(localorigin.New(
		os.Getenv(envApiAllowedOrigins),
		localorigin.AllowHosts(os.Getenv(envApiAllowedHosts)),
	)))

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

	// /docs/* keeps serving v1 unchanged: it is the path developers.anytype.io
	// and existing integrations use, so repointing it at v2 would break them
	// and repointing it at nothing would be worse. The versioned paths are the
	// ones to link to from here on.
	serveDoc := func(path, contentType string, body []byte) {
		router.GET(path, func(c *gin.Context) {
			c.Data(http.StatusOK, contentType, body)
		})
	}
	serveDoc("/docs/openapi.yaml", "application/x-yaml", openapiYAML)
	serveDoc("/docs/openapi.json", "application/json", openapiJSON)
	serveDoc("/v1/docs/openapi.yaml", "application/x-yaml", openapiYAML)
	serveDoc("/v1/docs/openapi.json", "application/json", openapiJSON)
	serveDoc("/v2/docs/openapi.yaml", "application/x-yaml", srv.docs.V2YAML)
	serveDoc("/v2/docs/openapi.json", "application/json", srv.docs.V2JSON)
}

// registerAuthRoutes registers authentication routes (no auth required)
func (srv *Server) registerAuthRoutes(router *gin.Engine) {
	authGroup := router.Group("/v1")
	{
		authGroup.POST("/auth/challenges", handler.CreateChallengeHandler(srv.service))
		authGroup.POST("/auth/api_keys", handler.CreateApiKeyHandler(srv.service))
	}
}

// registerChatRoutes registers chat message routes
func (srv *Server) registerChatRoutes(v1 *gin.RouterGroup, eventService apicore.EventService, writeRateLimitMW gin.HandlerFunc) {
	v1.GET("/spaces/:space_id/chats",
		srv.ensureFilters(),
		ensureAnalyticsEvent("ListChats", eventService),
		handler.ListChatsHandler(srv.service),
	)
	v1.POST("/spaces/:space_id/chats",
		writeRateLimitMW,
		ensureAnalyticsEvent("CreateChat", eventService),
		handler.CreateChatHandler(srv.service),
	)
	v1.GET("/spaces/:space_id/chats/:chat_id/messages/stream",
		ensureAnalyticsEvent("ChatMessageStream", eventService),
		handler.ChatStreamHandler(srv.service, srv.chatSubSvc),
	)
	v1.GET("/spaces/:space_id/chats/:chat_id/messages",
		ensureAnalyticsEvent("GetChatMessages", eventService),
		handler.GetChatMessagesHandler(srv.service),
	)
	v1.GET("/spaces/:space_id/chats/:chat_id/messages/search",
		ensureAnalyticsEvent("SearchChatMessages", eventService),
		handler.SearchChatMessagesHandler(srv.service),
	)
	v1.GET("/spaces/:space_id/chats/:chat_id/messages/:message_id",
		ensureAnalyticsEvent("GetChatMessage", eventService),
		handler.GetChatMessageHandler(srv.service),
	)
	v1.POST("/spaces/:space_id/chats/:chat_id/messages",
		writeRateLimitMW,
		ensureAnalyticsEvent("AddChatMessage", eventService),
		handler.AddChatMessageHandler(srv.service),
	)
	v1.PATCH("/spaces/:space_id/chats/:chat_id/messages/:message_id",
		writeRateLimitMW,
		ensureAnalyticsEvent("EditChatMessage", eventService),
		handler.EditChatMessageHandler(srv.service),
	)
	v1.DELETE("/spaces/:space_id/chats/:chat_id/messages/:message_id",
		writeRateLimitMW,
		ensureAnalyticsEvent("DeleteChatMessage", eventService),
		handler.DeleteChatMessageHandler(srv.service),
	)
	v1.POST("/spaces/:space_id/chats/:chat_id/messages/:message_id/reactions",
		writeRateLimitMW,
		ensureAnalyticsEvent("ToggleChatReaction", eventService),
		handler.ToggleChatReactionHandler(srv.service),
	)
	v1.POST("/spaces/:space_id/chats/:chat_id/read_all",
		writeRateLimitMW,
		ensureAnalyticsEvent("ReadAllChatMessages", eventService),
		handler.ReadAllChatMessagesHandler(srv.service),
	)
	v1.POST("/spaces/:space_id/chats/:chat_id/messages/read",
		writeRateLimitMW,
		ensureAnalyticsEvent("ReadChatMessages", eventService),
		handler.ReadChatMessagesHandler(srv.service),
	)
	v1.POST("/spaces/:space_id/chats/:chat_id/reactions/read",
		writeRateLimitMW,
		ensureAnalyticsEvent("ReadChatReactions", eventService),
		handler.ReadChatReactionsHandler(srv.service),
	)
}

// registerFileRoutes registers file-related routes
func (srv *Server) registerFileRoutes(v1 *gin.RouterGroup, eventService apicore.EventService, writeRateLimitMW gin.HandlerFunc) {
	v1.POST("/spaces/:space_id/files",
		writeRateLimitMW,
		ensureAnalyticsEvent("UploadFile", eventService),
		handler.UploadFileHandler(srv.service),
	)
	v1.GET("/spaces/:space_id/files/:file_id",
		ensureAnalyticsEvent("DownloadFile", eventService),
		handler.DownloadFileHandler(srv.service),
	)
	// HEAD reuses the same handler; http.ServeContent omits the body and
	// returns just the status line + headers, which is what HEAD requires.
	v1.HEAD("/spaces/:space_id/files/:file_id",
		handler.DownloadFileHandler(srv.service),
	)
	v1.DELETE("/spaces/:space_id/files/:file_id",
		writeRateLimitMW,
		ensureAnalyticsEvent("DeleteFile", eventService),
		handler.DeleteFileHandler(srv.service),
	)
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
