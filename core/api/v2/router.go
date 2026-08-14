package apiv2

// router.go registers the /v2 route group (APIV2.md §8). It lives in the v2
// package, not in core/api/server, so that nothing under core/api/v2 can
// reach v1 code and nothing in v1 can reach v2's: the dependency points one
// way, server → v2. The shared middleware stack (auth, cache init, write
// rate limit, analytics) stays in server — one gin engine and one
// ensureAuthenticated serve both versions — and arrives here through
// RouteDeps. The key-scope gate also arrives through RouteDeps but is
// installed only on this group: it is a /v2-only refusal by design.

import (
	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/pagination"
	v2handler "github.com/anyproto/anytype-heart/core/api/v2/handler"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
)

// C10 pagination bounds for /v2. The default page size is v2's own contract
// (C10: 25, against v1's 100); the rest match the shared bounds.
const (
	defaultPage     = 0
	defaultPageSize = 25
	minPageSize     = 1
	maxPageSize     = 1000
)

// RouteDeps carries what the /v2 group needs from the server: the constructed
// v2 service, the two capability flags, and the shared middleware the server
// owns. Passing the middleware as values (rather than importing server) is
// what keeps the dependency one-directional.
type RouteDeps struct {
	Service *v2service.V2Service
	// CreateDisabled skips the Phase-2 create routes when no creator
	// dependency was provided (read-only construction, e.g. in tests).
	CreateDisabled bool
	// EditDisabled skips the Phase-3 edit routes when no mutator dependency
	// was provided.
	EditDisabled bool

	// Auth is the shared bearer-token middleware (the same one /v1 uses).
	Auth gin.HandlerFunc
	// KeyScope is the JSON-API key-scope gate — it decides on the key's KIND
	// (Limited/JsonAPI/Full), not on which spaces the key may touch — run
	// directly after Auth (it needs the session Auth resolves). It is
	// installed on /v2 only: keys minted without a scope carry Limited and
	// are grandfathered on /v1, while /v2 has no shipped clients to break.
	KeyScope gin.HandlerFunc
	// CacheInit is the shared lazy cache-initialization middleware.
	CacheInit gin.HandlerFunc
	// WriteRateLimit is the shared write-rate limiter.
	WriteRateLimit gin.HandlerFunc
	// AnalyticsEvent builds the analytics middleware for one event code.
	AnalyticsEvent func(code string) gin.HandlerFunc
}

// RegisterRoutes registers the /v2 route group (APIV2.md §8): same
// middleware stack and auth as /v1 plus the /v2-only key-scope gate, C10
// pagination defaults, and the C8 idempotency and C9 dry-run plumbing.
// Skipped when the v2 service has no dependencies (v1-only construction,
// e.g. in isolated tests).
func RegisterRoutes(router *gin.Engine, deps RouteDeps) {
	if deps.Service == nil {
		return
	}

	v2 := router.Group("/v2")
	v2.Use(pagination.New(pagination.Config{
		DefaultPage:     defaultPage,
		DefaultPageSize: defaultPageSize,
		MinPageSize:     minPageSize,
		MaxPageSize:     maxPageSize,
	}))
	v2.Use(deps.CacheInit)
	v2.Use(deps.Auth)
	v2.Use(deps.KeyScope)
	// `?ids=` (C4) is parsed once, for every route: it picks the shape ids
	// are SERVED in, on both axes a response has — block ids in a document
	// and space ids anywhere a space is named (§8.36). It runs before
	// resolution so that an ambiguous reference's candidate list is spelled
	// the way this request asked for — idshape.go.
	v2.Use(ensureIdsShape())
	// Short space references (§8.35) resolve BEFORE the grant gate: grants
	// are keyed by full space id, so a short reference has to become one
	// before anything compares it against the grant. Resolution itself can
	// only land inside the caller's visible (grant-intersected) spaces —
	// spaceref.go.
	v2.Use(resolveSpaceRef(deps.Service))
	// The space-grant gate runs directly after the key-scope gate: KeyScope
	// decides the key's KIND, ensureSpaceGrant decides which spaces and
	// which verbs the key's grant covers (authz.go). It must run before any
	// handler resolves a space — the service's ensureSpace admits the tech
	// space, this gate denies it unless explicitly granted.
	v2.Use(ensureSpaceGrant())
	v2.Use(ensureDryRun())
	idempotencyMW := ensureIdempotency(newIdempotencyStore(idempotencyMaxEntries))

	// P1c introspection: the credential's self-description, derived from the
	// same ctx carriers the grant gate reads. Registered INSIDE the
	// authenticated group — its registry class is service-filtered, never
	// auth-exempt (the conformance walk fails an authenticated route
	// carrying that class), and the shared auth middleware is what keeps the
	// token Authorization-header-only.
	v2.GET("/auth/whoami",
		deps.AnalyticsEvent("V2Whoami"),
		v2handler.WhoamiV2Handler(deps.Service),
	)
	v2.POST("/validate",
		idempotencyMW,
		deps.AnalyticsEvent("V2Validate"),
		v2handler.ValidateV2Handler(deps.Service),
	)
	v2.GET("/spaces",
		deps.AnalyticsEvent("V2ListSpaces"),
		v2handler.ListSpacesV2Handler(deps.Service),
	)
	// Phase-7 space surface: the read is a tech-space store query (no
	// WorkspaceOpen/ObjectShow); both mutations carry C8 — a retried space
	// create without idempotency duplicates an entire space.
	v2.GET("/spaces/:space_id",
		deps.AnalyticsEvent("V2GetSpace"),
		v2handler.GetSpaceV2Handler(deps.Service),
	)
	v2.POST("/spaces",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2CreateSpace"),
		v2handler.CreateSpaceV2Handler(deps.Service),
	)
	v2.PATCH("/spaces/:space_id",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2UpdateSpace"),
		v2handler.UpdateSpaceV2Handler(deps.Service),
	)
	v2.GET("/spaces/:space_id/objects",
		deps.AnalyticsEvent("V2ListObjects"),
		v2handler.ListObjectsV2Handler(deps.Service),
	)
	v2.GET("/spaces/:space_id/objects/:object_id",
		deps.AnalyticsEvent("V2GetObject"),
		v2handler.GetObjectV2Handler(deps.Service),
	)
	v2.GET("/spaces/:space_id/members",
		deps.AnalyticsEvent("V2ListMembers"),
		v2handler.ListMembersV2Handler(deps.Service),
	)
	v2.GET("/spaces/:space_id/members/me",
		deps.AnalyticsEvent("V2GetMemberMe"),
		v2handler.GetMemberMeV2Handler(deps.Service),
	)
	v2.GET("/spaces/:space_id/types",
		deps.AnalyticsEvent("V2ListTypes"),
		v2handler.ListTypesV2Handler(deps.Service),
	)
	v2.GET("/spaces/:space_id/types/:type",
		deps.AnalyticsEvent("V2GetType"),
		v2handler.GetTypeV2Handler(deps.Service),
	)
	v2.GET("/spaces/:space_id/types/:type/schema",
		deps.AnalyticsEvent("V2GetTypeSchema"),
		v2handler.GetTypeSchemaV2Handler(deps.Service),
	)
	v2.GET("/spaces/:space_id/properties",
		deps.AnalyticsEvent("V2ListProperties"),
		v2handler.ListPropertiesV2Handler(deps.Service),
	)
	v2.GET("/spaces/:space_id/properties/:key/options",
		deps.AnalyticsEvent("V2ListPropertyOptions"),
		v2handler.ListPropertyOptionsV2Handler(deps.Service),
	)
	// Phase-4 query surface. Search is a READ (POST only because the request
	// needs a body): no idempotency middleware, no write rate limit, and the
	// group-level dry-run middleware's flag is ignored by the handlers.
	v2.POST("/search",
		deps.AnalyticsEvent("V2GlobalSearch"),
		v2handler.GlobalSearchObjectsV2Handler(deps.Service),
	)
	v2.POST("/spaces/:space_id/search",
		deps.AnalyticsEvent("V2Search"),
		v2handler.SearchObjectsV2Handler(deps.Service),
	)
	v2.GET("/spaces/:space_id/sets/:set_id/objects",
		deps.AnalyticsEvent("V2GetSetObjects"),
		v2handler.GetSetObjectsV2Handler(deps.Service),
	)
	v2.GET("/spaces/:space_id/sets/:set_id/views",
		deps.AnalyticsEvent("V2GetSetViews"),
		v2handler.GetSetViewsV2Handler(deps.Service),
	)
	v2.GET("/spaces/:space_id/collections/:collection_id/objects",
		deps.AnalyticsEvent("V2GetCollectionObjects"),
		v2handler.GetCollectionObjectsV2Handler(deps.Service),
	)
	v2.GET("/spaces/:space_id/collections/:collection_id/views",
		deps.AnalyticsEvent("V2GetCollectionViews"),
		v2handler.GetCollectionViewsV2Handler(deps.Service),
	)
	v2.GET("/schemas",
		deps.AnalyticsEvent("V2ListSchemas"),
		v2handler.SchemaIndexV2Handler(deps.Service),
	)
	v2.GET("/schemas/:kind",
		deps.AnalyticsEvent("V2GetSchema"),
		v2handler.SchemaKindV2Handler(deps.Service),
	)
	v2.GET("/schemas/ops/:op",
		deps.AnalyticsEvent("V2GetOpSchema"),
		v2handler.SchemaOpV2Handler(deps.Service),
	)

	registerCreateRoutes(v2, deps, idempotencyMW)
	registerEditRoutes(v2, deps, idempotencyMW)
	registerChatRoutes(v2, deps, idempotencyMW)
}

// registerChatRoutes registers the Phase-6 chat surface (APIV2.md §8.7).
// Every mutation — including DELETE, a Phase-6 widening of C8's method set
// that now covers every v2 DELETE — runs behind the idempotency middleware:
// a double-sent chat message is user-visible damage, and a blindly retried
// delete 404s misleadingly. C7 etag/If-Match deliberately does not apply
// (order ids and lastStateId are the chat's native concurrency vocabulary).
func registerChatRoutes(v2 *gin.RouterGroup, deps RouteDeps, idempotencyMW gin.HandlerFunc) {
	v2.GET("/spaces/:space_id/chats",
		deps.AnalyticsEvent("V2ListChats"),
		v2handler.ListChatsV2Handler(deps.Service),
	)
	v2.POST("/spaces/:space_id/chats",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2CreateChat"),
		v2handler.CreateChatV2Handler(deps.Service),
	)
	v2.GET("/spaces/:space_id/chats/:chat_id/messages",
		deps.AnalyticsEvent("V2GetChatMessages"),
		v2handler.GetChatMessagesV2Handler(deps.Service),
	)
	v2.POST("/spaces/:space_id/chats/:chat_id/messages",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2AddChatMessage"),
		v2handler.AddChatMessageV2Handler(deps.Service),
	)
	v2.PATCH("/spaces/:space_id/chats/:chat_id/messages/:message_id",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2EditChatMessage"),
		v2handler.EditChatMessageV2Handler(deps.Service),
	)
	v2.DELETE("/spaces/:space_id/chats/:chat_id/messages/:message_id",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2DeleteChatMessage"),
		v2handler.DeleteChatMessageV2Handler(deps.Service),
	)
	v2.POST("/spaces/:space_id/chats/:chat_id/messages/:message_id/reactions",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2ToggleChatReaction"),
		v2handler.ToggleChatReactionV2Handler(deps.Service),
	)
	v2.POST("/spaces/:space_id/chats/:chat_id/read",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2ReadChat"),
		v2handler.ReadChatV2Handler(deps.Service),
	)
}

// registerEditRoutes registers the Phase-3 edit surface (APIV2.md §2
// Phase 3): PATCH alone — snapshots are for creates, edits are ops (§8.27),
// so there is no full-document replace route. Concurrency safety is the
// If-Match header (C7); Idempotency-Key additionally covers PATCH (C8,
// v0.3.5) because agents auto-retry on timeout and a blind PATCH retry
// duplicates inserted blocks or 404s a re-deleted one. Skipped when no
// mutator dependency was provided.
func registerEditRoutes(v2 *gin.RouterGroup, deps RouteDeps, idempotencyMW gin.HandlerFunc) {
	if deps.EditDisabled {
		return
	}
	v2.PATCH("/spaces/:space_id/objects/:object_id",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2PatchObject"),
		v2handler.PatchObjectV2Handler(deps.Service),
	)
	// Plan 3.3 (APIV2_OBJECT_DELETE.md): archive, own-output-only — the
	// creator-provenance gate runs in the service; C8 idempotency because a
	// blindly retried delete after an already-archived answer must stay a
	// clean replay, C9 dry-run is the deletability probe.
	v2.DELETE("/spaces/:space_id/objects/:object_id",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2DeleteObject"),
		v2handler.DeleteObjectV2Handler(deps.Service),
	)
}

// registerCreateRoutes registers the Phase-2 create surface (APIV2.md §2).
// EVERY mutation — POST, PATCH and, since the Phase-6 review, the DELETEs
// too — runs behind the C8 idempotency middleware (v0.3.5: C8 covers all
// mutations; a route-dependent exception would be an invisible contract);
// all mutations parse ?dry_run=true via the group-level dry-run middleware.
// Skipped when no creator dependency was provided (read-only construction).
func registerCreateRoutes(v2 *gin.RouterGroup, deps RouteDeps, idempotencyMW gin.HandlerFunc) {
	if deps.CreateDisabled {
		return
	}
	v2.POST("/spaces/:space_id/objects",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2CreateObject"),
		v2handler.CreateObjectV2Handler(deps.Service),
	)
	v2.POST("/spaces/:space_id/templates",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2CreateTemplate"),
		v2handler.CreateTemplateV2Handler(deps.Service),
	)
	v2.POST("/spaces/:space_id/types",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2CreateType"),
		v2handler.CreateTypeV2Handler(deps.Service),
	)
	v2.PATCH("/spaces/:space_id/types/:type",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2UpdateType"),
		v2handler.UpdateTypeV2Handler(deps.Service),
	)
	v2.DELETE("/spaces/:space_id/types/:type",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2DeleteType"),
		v2handler.DeleteTypeV2Handler(deps.Service),
	)
	v2.POST("/spaces/:space_id/properties",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2CreateProperty"),
		v2handler.CreatePropertyV2Handler(deps.Service),
	)
	v2.PATCH("/spaces/:space_id/properties/:key",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2UpdateProperty"),
		v2handler.UpdatePropertyV2Handler(deps.Service),
	)
	v2.DELETE("/spaces/:space_id/properties/:key",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2DeleteProperty"),
		v2handler.DeletePropertyV2Handler(deps.Service),
	)
	v2.POST("/spaces/:space_id/sets",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2CreateSet"),
		v2handler.CreateSetV2Handler(deps.Service),
	)
	v2.POST("/spaces/:space_id/collections",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2CreateCollection"),
		v2handler.CreateCollectionV2Handler(deps.Service),
	)
	v2.POST("/spaces/:space_id/files",
		deps.WriteRateLimit,
		idempotencyMW,
		deps.AnalyticsEvent("V2UploadFile"),
		v2handler.UploadFileV2Handler(deps.Service),
	)
}
