package apiv2

// authz.go is the /v2 space-grant enforcement: the per-route authorization
// registry (read/write classification plus the global-route classes) and
// the ensureSpaceGrant middleware that applies a key's grant to every
// request. The registry is deliberately an explicit table, not an inference
// from the HTTP method or the path shape: classification is an
// authorization decision, and the conformance test in core/api/server pins
// that every registered route appears here — a new route that skips
// classification fails CI instead of shipping as a silent hole.

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/anyproto/anytype-heart/core/api/util"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// SpaceParam is the ONE route-param name under which a /v2 route may
// address a space. The gate reads exactly this name; a space-addressing
// route registered under any other param (`:workspace_id`, `:spaceId`)
// would present an empty space id here and fall into the global branch,
// where the natural-looking classes wave it through with no space check at
// all. The conformance walk therefore refuses unknown param names outright.
const SpaceParam = "space_id"

// RouteVerb classifies what a route DOES to data, not which HTTP method it
// uses: POST /v2/search and POST /v2/validate are reads that need a body;
// chat POST …/read is a write (it mutates the synced read watermark).
type RouteVerb string

const (
	RouteVerbRead  RouteVerb = "read"
	RouteVerbWrite RouteVerb = "write"
)

// GlobalRouteClass classifies a route that carries no :space_id param —
// the closed set of ways such a route may exist under a space-scoped grant.
type GlobalRouteClass string

const (
	// GlobalAuthExempt marks routes served OUTSIDE the authenticated /v2
	// group (public documents); the gate never runs on them. Listed so the
	// conformance walk stays a closed inventory — and the walk verifies the
	// precondition behaviorally: a route with this class must answer a
	// credential-less request, every other /v2 route must 401. A route
	// registered INSIDE the authenticated group (an authenticated
	// /v2/auth/* surface, say) can therefore never carry this class.
	GlobalAuthExempt GlobalRouteClass = "auth-exempt"
	// GlobalDataFreeAllow marks routes that touch no space data at all
	// (schema/validation surfaces) — any granted key passes.
	GlobalDataFreeAllow GlobalRouteClass = "data-free-allow"
	// GlobalServiceFiltered marks routes whose handlers pick their own
	// space set INSIDE the service (the fan-out surfaces). The gate lets
	// them through; the service intersects its space set with the ctx
	// grant (V2Service.ListSpaces, spaceRefs).
	GlobalServiceFiltered GlobalRouteClass = "service-filtered"
	// GlobalScopedDenied marks routes deliberately refused for every
	// granted key: POST /v2/spaces — a key that can mint spaces it then
	// owns is not meaningfully scoped.
	GlobalScopedDenied GlobalRouteClass = "scoped-denied"
)

// RouteAuthz is one route's authorization classification. Global is empty
// for space-scoped routes (those carry :space_id and are checked against
// the grant's space list).
type RouteAuthz struct {
	Verb   RouteVerb
	Global GlobalRouteClass
}

// routeKey builds the registry key for a route.
func routeKey(method, path string) string {
	return method + " " + path
}

// v2RouteAuthz classifies EVERY /v2 route. Kept in registration order of
// router.go so a diff of the two files reads side by side.
//
// The non-obvious verb calls, recorded:
//   - POST /v2/validate and both search POSTs are READS: POST only because
//     the request needs a body; nothing is persisted.
//   - POST …/chats/:chat_id/read is a WRITE: it advances the synced read
//     watermark that every device sees.
//   - GET …/types/:type/schema is a read (it currently answers 501, and
//     when it lands it stays a derived-artifact read).
//   - POST /v2/spaces is a write AND scoped-denied: even a readwrite grant
//     must not mint spaces outside its list.
//   - GET /v2/auth/whoami is a read, classified service-filtered by
//     reasoning, not by reflex: it is AUTHENTICATED (auth-exempt is
//     impossible inside the gated group — the conformance walk enforces
//     that behaviorally), it addresses no single space, and it discloses
//     nothing the holder cannot already enumerate — the grant echo IS the
//     enforcement boundary, and the space names in its body come from the
//     service's own grant-intersected ListSpaces path, the exact pattern
//     the service-filtered class names. data-free-allow would be wrong:
//     the body carries space data (names).
var v2RouteAuthz = map[string]RouteAuthz{
	// core group (router.go RegisterRoutes)
	routeKey(http.MethodGet, "/v2/auth/whoami"):                                         {Verb: RouteVerbRead, Global: GlobalServiceFiltered},
	routeKey(http.MethodPost, "/v2/validate"):                                           {Verb: RouteVerbRead, Global: GlobalDataFreeAllow},
	routeKey(http.MethodGet, "/v2/spaces"):                                              {Verb: RouteVerbRead, Global: GlobalServiceFiltered},
	routeKey(http.MethodGet, "/v2/spaces/:space_id"):                                    {Verb: RouteVerbRead},
	routeKey(http.MethodPost, "/v2/spaces"):                                             {Verb: RouteVerbWrite, Global: GlobalScopedDenied},
	routeKey(http.MethodPatch, "/v2/spaces/:space_id"):                                  {Verb: RouteVerbWrite},
	routeKey(http.MethodGet, "/v2/spaces/:space_id/objects"):                            {Verb: RouteVerbRead},
	routeKey(http.MethodGet, "/v2/spaces/:space_id/objects/:object_id"):                 {Verb: RouteVerbRead},
	routeKey(http.MethodGet, "/v2/spaces/:space_id/members"):                            {Verb: RouteVerbRead},
	routeKey(http.MethodGet, "/v2/spaces/:space_id/members/me"):                         {Verb: RouteVerbRead},
	routeKey(http.MethodGet, "/v2/spaces/:space_id/types"):                              {Verb: RouteVerbRead},
	routeKey(http.MethodGet, "/v2/spaces/:space_id/types/:type"):                        {Verb: RouteVerbRead},
	routeKey(http.MethodGet, "/v2/spaces/:space_id/types/:type/schema"):                 {Verb: RouteVerbRead},
	routeKey(http.MethodGet, "/v2/spaces/:space_id/properties"):                         {Verb: RouteVerbRead},
	routeKey(http.MethodGet, "/v2/spaces/:space_id/properties/:key/options"):            {Verb: RouteVerbRead},
	routeKey(http.MethodPost, "/v2/search"):                                             {Verb: RouteVerbRead, Global: GlobalServiceFiltered},
	routeKey(http.MethodPost, "/v2/spaces/:space_id/search"):                            {Verb: RouteVerbRead},
	routeKey(http.MethodGet, "/v2/spaces/:space_id/sets/:set_id/objects"):               {Verb: RouteVerbRead},
	routeKey(http.MethodGet, "/v2/spaces/:space_id/sets/:set_id/views"):                 {Verb: RouteVerbRead},
	routeKey(http.MethodGet, "/v2/spaces/:space_id/collections/:collection_id/objects"): {Verb: RouteVerbRead},
	routeKey(http.MethodGet, "/v2/spaces/:space_id/collections/:collection_id/views"):   {Verb: RouteVerbRead},
	routeKey(http.MethodGet, "/v2/schemas"):                                             {Verb: RouteVerbRead, Global: GlobalDataFreeAllow},
	routeKey(http.MethodGet, "/v2/schemas/:kind"):                                       {Verb: RouteVerbRead, Global: GlobalDataFreeAllow},
	routeKey(http.MethodGet, "/v2/schemas/ops/:op"):                                     {Verb: RouteVerbRead, Global: GlobalDataFreeAllow},
	// create surface (registerCreateRoutes)
	routeKey(http.MethodPost, "/v2/spaces/:space_id/objects"):           {Verb: RouteVerbWrite},
	routeKey(http.MethodPost, "/v2/spaces/:space_id/templates"):         {Verb: RouteVerbWrite},
	routeKey(http.MethodPost, "/v2/spaces/:space_id/types"):             {Verb: RouteVerbWrite},
	routeKey(http.MethodPatch, "/v2/spaces/:space_id/types/:type"):      {Verb: RouteVerbWrite},
	routeKey(http.MethodDelete, "/v2/spaces/:space_id/types/:type"):     {Verb: RouteVerbWrite},
	routeKey(http.MethodPost, "/v2/spaces/:space_id/properties"):        {Verb: RouteVerbWrite},
	routeKey(http.MethodPatch, "/v2/spaces/:space_id/properties/:key"):  {Verb: RouteVerbWrite},
	routeKey(http.MethodDelete, "/v2/spaces/:space_id/properties/:key"): {Verb: RouteVerbWrite},
	routeKey(http.MethodPost, "/v2/spaces/:space_id/sets"):              {Verb: RouteVerbWrite},
	routeKey(http.MethodPost, "/v2/spaces/:space_id/collections"):       {Verb: RouteVerbWrite},
	routeKey(http.MethodPost, "/v2/spaces/:space_id/files"):             {Verb: RouteVerbWrite},
	// edit surface (registerEditRoutes)
	routeKey(http.MethodPatch, "/v2/spaces/:space_id/objects/:object_id"): {Verb: RouteVerbWrite},
	routeKey(http.MethodPut, "/v2/spaces/:space_id/objects/:object_id"):   {Verb: RouteVerbWrite},
	// chat surface (registerChatRoutes)
	routeKey(http.MethodGet, "/v2/spaces/:space_id/chats"):                                          {Verb: RouteVerbRead},
	routeKey(http.MethodPost, "/v2/spaces/:space_id/chats"):                                         {Verb: RouteVerbWrite},
	routeKey(http.MethodGet, "/v2/spaces/:space_id/chats/:chat_id/messages"):                        {Verb: RouteVerbRead},
	routeKey(http.MethodPost, "/v2/spaces/:space_id/chats/:chat_id/messages"):                       {Verb: RouteVerbWrite},
	routeKey(http.MethodPatch, "/v2/spaces/:space_id/chats/:chat_id/messages/:message_id"):          {Verb: RouteVerbWrite},
	routeKey(http.MethodDelete, "/v2/spaces/:space_id/chats/:chat_id/messages/:message_id"):         {Verb: RouteVerbWrite},
	routeKey(http.MethodPost, "/v2/spaces/:space_id/chats/:chat_id/messages/:message_id/reactions"): {Verb: RouteVerbWrite},
	routeKey(http.MethodPost, "/v2/spaces/:space_id/chats/:chat_id/read"):                           {Verb: RouteVerbWrite},
	// public documents, registered on the engine root (server
	// registerDocumentationRoutes) — the gate never sees them
	routeKey(http.MethodGet, "/v2/docs/openapi.yaml"): {Verb: RouteVerbRead, Global: GlobalAuthExempt},
	routeKey(http.MethodGet, "/v2/docs/openapi.json"): {Verb: RouteVerbRead, Global: GlobalAuthExempt},
}

// V2RouteAuthz returns a copy of the authorization registry for the
// conformance test in core/api/server (both directions: every registered
// route classified, every classified route registered).
func V2RouteAuthz() map[string]RouteAuthz {
	out := make(map[string]RouteAuthz, len(v2RouteAuthz))
	for key, authz := range v2RouteAuthz {
		out[key] = authz
	}
	return out
}

// neededPerms names the permission a route needs, for the RFC 6750 scope
// string and the 403 messages.
func neededPerms(verb RouteVerb) string {
	if verb == RouteVerbWrite {
		return util.GrantPermsReadWrite
	}
	return util.GrantPermsRead
}

// ensureSpaceGrant enforces the key's space grant on every /v2 request. It
// runs directly after the key-scope gate (deps.KeyScope) and BEFORE the
// service's ensureSpace — which deliberately admits the tech space as an
// ordinary space id, so the tech space is denied here unless explicitly
// granted, like any other space.
//
//   - Grant == nil → pass through: an unscoped/legacy key keeps today's
//     behavior. (An EMPTY grant space list is not "all spaces": AllowsSpace
//     denies everything then — see util.ApiGrant.)
//   - :space_id present → it must be in the grant's space list, else 403
//     space_not_granted naming the grant.
//   - no :space_id → the route must appear in v2RouteAuthz with an explicit
//     global class; an UNREGISTERED route is refused, not allowed (fail
//     closed — the conformance test makes that a CI failure before it can
//     become a runtime 403).
//   - Perms == read on a write-classified route → 403 write_not_granted.
//     A route missing a verb classification counts as write (fail closed).
//
// The route middleware gives the clean 403; V2Service.ensureSpace consults
// the ctx grant again as the backstop for a future route that forgets this
// middleware or resolves ids unusually.
func ensureSpaceGrant() gin.HandlerFunc {
	return func(c *gin.Context) {
		grant := util.ApiGrantFromCtx(c.Request.Context())
		if grant == nil {
			c.Next()
			return
		}

		authz, classified := v2RouteAuthz[routeKey(c.Request.Method, c.FullPath())]
		spaceId := c.Param(SpaceParam)
		if spaceId == "" {
			if !classified || authz.Global == "" || authz.Global == GlobalScopedDenied {
				c.Header(util.WwwAuthenticateHeader, util.BearerChallengeInsufficientScope(""))
				respondV2Error(c, v2model.SpaceNotGranted(globalRouteRefusal(c, classified, authz)))
				return
			}
		} else if !grant.AllowsSpace(spaceId) {
			needed := neededPerms(effectiveVerb(authz, classified))
			c.Header(util.WwwAuthenticateHeader,
				util.BearerChallengeInsufficientScope(util.SpaceScope(spaceId, needed)))
			respondV2Error(c, v2model.SpaceNotGranted(fmt.Sprintf(
				"key not granted space %q; granted: %s", spaceId, grant.Describe())))
			return
		}

		if effectiveVerb(authz, classified) == RouteVerbWrite && !grant.CanWrite() {
			// A global write route (allowed through the branch above) has no
			// space to name: the challenge takes its empty-scope form rather
			// than rendering a malformed "space::readwrite".
			scope := ""
			if spaceId != "" {
				scope = util.SpaceScope(spaceId, util.GrantPermsReadWrite)
			}
			c.Header(util.WwwAuthenticateHeader, util.BearerChallengeInsufficientScope(scope))
			respondV2Error(c, v2model.WriteNotGranted(fmt.Sprintf(
				"%s %s is a write and the key's grant is read-only; granted: %s",
				c.Request.Method, c.FullPath(), grant.Describe())))
			return
		}
		c.Next()
	}
}

// effectiveVerb treats an unclassified route as a write: for a read-only
// grant the unknown route is then refused, and a widening can only come
// from an explicit registry entry.
func effectiveVerb(authz RouteAuthz, classified bool) RouteVerb {
	if !classified {
		return RouteVerbWrite
	}
	return authz.Verb
}

// globalRouteRefusal words the 403 for a no-space route a scoped key cannot
// use — the deliberate deny (POST /v2/spaces) and the fail-closed default
// for a route the registry does not know.
func globalRouteRefusal(c *gin.Context, classified bool, authz RouteAuthz) string {
	route := c.Request.Method + " " + c.FullPath()
	if classified && authz.Global == GlobalScopedDenied {
		return fmt.Sprintf("%s is not available to space-scoped keys: a key that can create spaces it then owns is not meaningfully scoped", route)
	}
	return fmt.Sprintf("%s addresses no single space and is not classified for space-scoped keys — refused fail-closed", route)
}
