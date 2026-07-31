package server

import (
	"net/http"
	"os"

	"github.com/gin-gonic/gin"

	apicore "github.com/anyproto/anytype-heart/core/api/core"
	_ "github.com/anyproto/anytype-heart/core/api/docs"
	"github.com/anyproto/anytype-heart/core/api/handler"
	"github.com/anyproto/anytype-heart/core/api/pagination"
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
	v1.Use(srv.ensureAuthenticated(mw))

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

	srv.registerV2Routes(router, mw, eventService, writeRateLimitMW)

	return router
}

// v2DefaultPageSize is the C10 default list page size for /v2.
const v2DefaultPageSize = 25

// registerV2Routes registers the /v2 route group (APIV2.md §8): same
// middleware stack and auth as /v1, C10 pagination defaults, plus the C8
// idempotency and C9 dry-run plumbing. Skipped when the v2 service has no
// dependencies (v1-only construction, e.g. in isolated tests).
func (srv *Server) registerV2Routes(router *gin.Engine, mw apicore.ClientCommands, eventService apicore.EventService, writeRateLimitMW gin.HandlerFunc) {
	if srv.v2Service == nil {
		return
	}

	v2 := router.Group("/v2")
	v2.Use(pagination.New(pagination.Config{
		DefaultPage:     defaultPage,
		DefaultPageSize: v2DefaultPageSize,
		MinPageSize:     minPageSize,
		MaxPageSize:     maxPageSize,
	}))
	v2.Use(srv.ensureCacheInitialized())
	v2.Use(srv.ensureAuthenticated(mw))
	v2.Use(ensureDryRun())
	idempotencyMW := ensureIdempotency(newIdempotencyStore(idempotencyMaxEntries))

	v2.POST("/validate",
		idempotencyMW,
		ensureAnalyticsEvent("V2Validate", eventService),
		handler.ValidateV2Handler(srv.v2Service),
	)
	v2.GET("/spaces",
		ensureAnalyticsEvent("V2ListSpaces", eventService),
		handler.ListSpacesV2Handler(srv.v2Service),
	)
	v2.GET("/spaces/:space_id/objects",
		ensureAnalyticsEvent("V2ListObjects", eventService),
		handler.ListObjectsV2Handler(srv.v2Service),
	)
	v2.GET("/spaces/:space_id/objects/:object_id",
		ensureAnalyticsEvent("V2GetObject", eventService),
		handler.GetObjectV2Handler(srv.v2Service),
	)
	v2.GET("/spaces/:space_id/members",
		ensureAnalyticsEvent("V2ListMembers", eventService),
		handler.ListMembersV2Handler(srv.v2Service),
	)
	v2.GET("/spaces/:space_id/types",
		ensureAnalyticsEvent("V2ListTypes", eventService),
		handler.ListTypesV2Handler(srv.v2Service),
	)
	v2.GET("/spaces/:space_id/types/:type",
		ensureAnalyticsEvent("V2GetType", eventService),
		handler.GetTypeV2Handler(srv.v2Service),
	)
	v2.GET("/spaces/:space_id/types/:type/schema",
		ensureAnalyticsEvent("V2GetTypeSchema", eventService),
		handler.GetTypeSchemaV2Handler(srv.v2Service),
	)
	v2.GET("/spaces/:space_id/properties",
		ensureAnalyticsEvent("V2ListProperties", eventService),
		handler.ListPropertiesV2Handler(srv.v2Service),
	)
	v2.GET("/spaces/:space_id/properties/:key/options",
		ensureAnalyticsEvent("V2ListPropertyOptions", eventService),
		handler.ListPropertyOptionsV2Handler(srv.v2Service),
	)
	// Phase-4 query surface. Search is a READ (POST only because the request
	// needs a body): no idempotency middleware, no write rate limit, and the
	// group-level dry-run middleware's flag is ignored by the handlers.
	v2.POST("/search",
		ensureAnalyticsEvent("V2GlobalSearch", eventService),
		handler.GlobalSearchObjectsV2Handler(srv.v2Service),
	)
	v2.POST("/spaces/:space_id/search",
		ensureAnalyticsEvent("V2Search", eventService),
		handler.SearchObjectsV2Handler(srv.v2Service),
	)
	v2.GET("/spaces/:space_id/sets/:set_id/objects",
		ensureAnalyticsEvent("V2GetSetObjects", eventService),
		handler.GetSetObjectsV2Handler(srv.v2Service),
	)
	v2.GET("/spaces/:space_id/sets/:set_id/views",
		ensureAnalyticsEvent("V2GetSetViews", eventService),
		handler.GetSetViewsV2Handler(srv.v2Service),
	)
	v2.GET("/spaces/:space_id/collections/:collection_id/objects",
		ensureAnalyticsEvent("V2GetCollectionObjects", eventService),
		handler.GetCollectionObjectsV2Handler(srv.v2Service),
	)
	v2.GET("/spaces/:space_id/collections/:collection_id/views",
		ensureAnalyticsEvent("V2GetCollectionViews", eventService),
		handler.GetCollectionViewsV2Handler(srv.v2Service),
	)
	v2.GET("/schemas",
		ensureAnalyticsEvent("V2ListSchemas", eventService),
		handler.SchemaIndexV2Handler(srv.v2Service),
	)
	v2.GET("/schemas/:kind",
		ensureAnalyticsEvent("V2GetSchema", eventService),
		handler.SchemaKindV2Handler(srv.v2Service),
	)
	v2.GET("/schemas/ops/:op",
		ensureAnalyticsEvent("V2GetOpSchema", eventService),
		handler.SchemaOpV2Handler(srv.v2Service),
	)

	srv.registerV2CreateRoutes(v2, eventService, idempotencyMW, writeRateLimitMW)
	srv.registerV2EditRoutes(v2, eventService, idempotencyMW, writeRateLimitMW)
}

// registerV2EditRoutes registers the Phase-3 edit surface (APIV2.md §2
// Phase 3). Concurrency safety is the If-Match header (C7); Idempotency-Key
// additionally covers PATCH/PUT (C8, v0.3.5) because agents auto-retry on
// timeout and a blind PATCH retry duplicates inserted blocks or 404s a
// re-deleted one. Skipped when no mutator dependency was provided.
func (srv *Server) registerV2EditRoutes(v2 *gin.RouterGroup, eventService apicore.EventService, idempotencyMW, writeRateLimitMW gin.HandlerFunc) {
	if srv.v2EditDisabled {
		return
	}
	v2.PATCH("/spaces/:space_id/objects/:object_id",
		writeRateLimitMW,
		idempotencyMW,
		ensureAnalyticsEvent("V2PatchObject", eventService),
		handler.PatchObjectV2Handler(srv.v2Service),
	)
	v2.PUT("/spaces/:space_id/objects/:object_id",
		writeRateLimitMW,
		idempotencyMW,
		ensureAnalyticsEvent("V2PutObject", eventService),
		handler.PutObjectV2Handler(srv.v2Service),
	)
}

// registerV2CreateRoutes registers the Phase-2 create surface (APIV2.md §2).
// Every POST and PATCH runs behind the C8 idempotency middleware (v0.3.5:
// C8 covers all mutations, not only POST); all mutations parse ?dry_run=true
// via the group-level dry-run middleware. Skipped when no creator dependency
// was provided (read-only construction).
func (srv *Server) registerV2CreateRoutes(v2 *gin.RouterGroup, eventService apicore.EventService, idempotencyMW, writeRateLimitMW gin.HandlerFunc) {
	if srv.v2CreateDisabled {
		return
	}
	v2.POST("/spaces/:space_id/objects",
		writeRateLimitMW,
		idempotencyMW,
		ensureAnalyticsEvent("V2CreateObject", eventService),
		handler.CreateObjectV2Handler(srv.v2Service),
	)
	v2.POST("/spaces/:space_id/templates",
		writeRateLimitMW,
		idempotencyMW,
		ensureAnalyticsEvent("V2CreateTemplate", eventService),
		handler.CreateTemplateV2Handler(srv.v2Service),
	)
	v2.POST("/spaces/:space_id/types",
		writeRateLimitMW,
		idempotencyMW,
		ensureAnalyticsEvent("V2CreateType", eventService),
		handler.CreateTypeV2Handler(srv.v2Service),
	)
	v2.PATCH("/spaces/:space_id/types/:type",
		writeRateLimitMW,
		idempotencyMW,
		ensureAnalyticsEvent("V2UpdateType", eventService),
		handler.UpdateTypeV2Handler(srv.v2Service),
	)
	v2.DELETE("/spaces/:space_id/types/:type",
		writeRateLimitMW,
		ensureAnalyticsEvent("V2DeleteType", eventService),
		handler.DeleteTypeV2Handler(srv.v2Service),
	)
	v2.POST("/spaces/:space_id/properties",
		writeRateLimitMW,
		idempotencyMW,
		ensureAnalyticsEvent("V2CreateProperty", eventService),
		handler.CreatePropertyV2Handler(srv.v2Service),
	)
	v2.PATCH("/spaces/:space_id/properties/:key",
		writeRateLimitMW,
		idempotencyMW,
		ensureAnalyticsEvent("V2UpdateProperty", eventService),
		handler.UpdatePropertyV2Handler(srv.v2Service),
	)
	v2.DELETE("/spaces/:space_id/properties/:key",
		writeRateLimitMW,
		ensureAnalyticsEvent("V2DeleteProperty", eventService),
		handler.DeletePropertyV2Handler(srv.v2Service),
	)
	v2.POST("/spaces/:space_id/sets",
		writeRateLimitMW,
		idempotencyMW,
		ensureAnalyticsEvent("V2CreateSet", eventService),
		handler.CreateSetV2Handler(srv.v2Service),
	)
	v2.POST("/spaces/:space_id/collections",
		writeRateLimitMW,
		idempotencyMW,
		ensureAnalyticsEvent("V2CreateCollection", eventService),
		handler.CreateCollectionV2Handler(srv.v2Service),
	)
	v2.POST("/spaces/:space_id/files",
		writeRateLimitMW,
		idempotencyMW,
		ensureAnalyticsEvent("V2UploadFile", eventService),
		handler.UploadFileV2Handler(srv.v2Service),
	)
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
