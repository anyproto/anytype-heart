package server

// grant_gate_test.go pins the P1 space-grant enforcement end to end through
// the real engine: the /v2 gate (apiv2.ensureSpaceGrant), the /v1 rejection
// of granted keys, the service-layer fan-out constraint, and the
// route-classification conformance that keeps all of it from rotting.

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
	apiv2 "github.com/anyproto/anytype-heart/core/api/v2"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// registerGrantTestSpace adds a spaceView for spaceId to BOTH tech-space
// indexes the fixture wires differently: the store's own tech space
// (objectstore.TestTechSpaceId — what GetSpaceViewDetails/ensureSpace
// resolve against) and the v2 service's configured one (mockedTechSpaceId —
// what ListSpaces/spaceRefs enumerate).
func registerGrantTestSpace(t *testing.T, fx *fixture, spaceId, name string) {
	for _, techSpace := range []string{objectstore.TestTechSpaceId, mockedTechSpaceId} {
		fx.objectStore.AddObjects(t, techSpace, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("spaceView_" + spaceId + "_" + techSpace),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:  domain.String(spaceId),
			bundle.RelationKeyName:           domain.String(name),
		}})
	}
}

// grantedSession caches a JsonAPI session entry carrying the given grant.
func grantedSession(fx *fixture, key string, grant *util.ApiGrant) {
	fx.KeyToToken = map[string]ApiSessionEntry{
		key: {Token: "tok", AppName: "agent", Scope: model.AccountAuth_JsonAPI, Grant: grant},
	}
}

func serveWithKey(fx *fixture, method, path, key string) *httptest.ResponseRecorder {
	return serveWithKeyBody(fx, method, path, key, `{}`)
}

func serveWithKeyBody(fx *fixture, method, path, key, body string) *httptest.ResponseRecorder {
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Host = localApiHost
	req.Header.Set("Authorization", "Bearer "+key)
	fx.Engine().ServeHTTP(w, req)
	return w
}

// knownRouteParams is the closed set of path-param names the /v2 surface
// may use. The gate resolves the addressed space from exactly
// apiv2.SpaceParam, so a space-addressing route under any OTHER name
// (`:workspace_id`, `:spaceId`) would present an empty space id and the
// walk below would then force it into a global class — where the
// natural-looking choices pass the gate with no space check at all. An
// unknown param name is therefore a conformance FAILURE, making the new
// name a review-visible decision instead of a silent reclassification.
var knownRouteParams = map[string]bool{
	apiv2.SpaceParam: true,
	"object_id":      true,
	"type":           true,
	"key":            true,
	"kind":           true,
	"op":             true,
	"set_id":         true,
	"collection_id":  true,
	"chat_id":        true,
	"message_id":     true,
}

// substituteRouteParams replaces every :param / *param segment so gin
// routes a probe to the registered handler.
func substituteRouteParams(path string) string {
	segments := strings.Split(path, "/")
	for i, segment := range segments {
		if strings.HasPrefix(segment, ":") || strings.HasPrefix(segment, "*") {
			segments[i] = "x"
		}
	}
	return strings.Join(segments, "/")
}

func TestV2RouteAuthzConformance(t *testing.T) {
	// The structural guarantee that P1b's coverage does not rot: every
	// registered /v2 route either carries :space_id or appears in the
	// global-route registry, and EVERY route appears in the read/write
	// classification. A new route that skips classification fails here, in
	// CI, instead of shipping as a silent authorization hole.
	fx := newV2ServerFixture(t)
	// The walk covers only what the fixture registers: every conditional
	// route group MUST be enabled here, or its routes would be invisible to
	// both directions of the check. A future conditionally-registered group
	// must add its enablement (and a flag assertion) to this fixture.
	require.False(t, fx.v2CreateDisabled, "the conformance fixture must register the create routes")
	require.False(t, fx.v2EditDisabled, "the conformance fixture must register the edit routes")
	authz := apiv2.RouteAuthzTable()

	registered := map[string]bool{}
	v2Routes := 0
	for _, route := range fx.Engine().Routes() {
		if !strings.HasPrefix(route.Path, "/v2/") && route.Path != "/v2" {
			continue
		}
		v2Routes++
		key := route.Method + " " + route.Path
		registered[key] = true

		for _, segment := range strings.Split(route.Path, "/") {
			if !strings.HasPrefix(segment, ":") && !strings.HasPrefix(segment, "*") {
				continue
			}
			param := strings.TrimPrefix(strings.TrimPrefix(segment, ":"), "*")
			require.True(t, knownRouteParams[param],
				"%s uses the unknown route param %q — if it addresses a space it MUST be named %q (the gate reads exactly that name); otherwise add it to knownRouteParams as a deliberate decision", key, param, apiv2.SpaceParam)
		}

		entry, classified := authz[key]
		require.True(t, classified,
			"%s is not classified in apiv2's v2RouteAuthz — every /v2 route MUST carry an explicit read/write classification (and, without :space_id, a global class); the space-grant gate refuses what it does not know, so an unclassified route is broken for every scoped key", key)

		if strings.Contains(route.Path, ":"+apiv2.SpaceParam) {
			require.Empty(t, entry.Global,
				"%s carries :space_id and must not be classified global", key)
		} else {
			require.NotEmpty(t, entry.Global,
				"%s has no :space_id and must carry an explicit global-route class (auth-exempt, data-free-allow, service-filtered, or scoped-denied)", key)
		}

		// The auth-exempt class states a precondition — "served OUTSIDE the
		// authenticated group" — and this probe is what makes it true: an
		// auth-exempt route must answer a credential-less request, every
		// other /v2 route must 401. Without it, the class is asserted in a
		// registry nothing checks, and a future authenticated route (a
		// /v2/auth/* surface, say) could be talked into carrying it — which
		// would ship the route with neither auth nor the grant gate.
		w := httptest.NewRecorder()
		req := httptest.NewRequest(route.Method, substituteRouteParams(route.Path), strings.NewReader(`{}`))
		req.Host = localApiHost
		fx.Engine().ServeHTTP(w, req)
		if entry.Global == apiv2.GlobalAuthExempt {
			require.NotEqual(t, http.StatusUnauthorized, w.Code,
				"%s is classified auth-exempt but demands credentials — the class is only for routes served outside the authenticated group", key)
		} else {
			require.Equal(t, http.StatusUnauthorized, w.Code,
				"%s answered %d to a credential-less request — every non-exempt /v2 route must sit behind ensureAuthenticated (is it registered on the gated v2 group?)", key, w.Code)
		}
	}
	require.Equal(t, len(authz), v2Routes,
		"the engine and the registry must be a bijection — a count mismatch means a route group the fixture failed to register (or duplicate registration)")

	// the reverse direction: a stale registry entry (a renamed or removed
	// route) is rot too — the registry must mirror the engine exactly
	for key := range authz {
		require.True(t, registered[key],
			"%s is classified in v2RouteAuthz but not registered on the engine — remove or rename the stale entry", key)
	}
}

func TestV2SpaceGrantGate(t *testing.T) {
	readWrite := func(spaces ...string) *util.ApiGrant {
		return &util.ApiGrant{Spaces: spaces, Perms: util.GrantPermsReadWrite}
	}
	readOnly := func(spaces ...string) *util.ApiGrant {
		return &util.ApiGrant{Spaces: spaces, Perms: util.GrantPermsRead}
	}

	t.Run("granted space passes, non-granted space is 403 space_not_granted", func(t *testing.T) {
		// given
		fx := newV2ServerFixture(t)
		registerGrantTestSpace(t, fx, "spaceA", "Work")
		registerGrantTestSpace(t, fx, "spaceB", "Personal")
		grantedSession(fx, "scopedKey", readWrite("spaceA"))
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		// when
		granted := serveWithKey(fx, "GET", "/v2/spaces/spaceA", "scopedKey")
		denied := serveWithKey(fx, "GET", "/v2/spaces/spaceB", "scopedKey")

		// then
		require.Equal(t, http.StatusOK, granted.Code)
		require.Contains(t, granted.Body.String(), `"Work"`)

		require.Equal(t, http.StatusForbidden, denied.Code)
		body := denied.Body.String()
		require.Contains(t, body, `"space_not_granted"`)
		require.Contains(t, body, `key not granted space \"spaceB\"`)
		require.Contains(t, body, "spaces [spaceA] with readwrite access")
		require.Equal(t, `Bearer error="insufficient_scope", scope="space:spaceB:read"`,
			denied.Header().Get("WWW-Authenticate"))
	})

	t.Run("the tech space is denied unless explicitly granted", func(t *testing.T) {
		// the gate runs BEFORE the v2 service's ensureSpace, which
		// deliberately admits the tech space as an ordinary space id — so
		// the deny has to happen at the gate, and an explicit grant of the
		// tech space id opens it like any other space
		fx := newV2ServerFixture(t)
		grantedSession(fx, "scopedKey", readWrite("spaceA"))

		denied := serveWithKey(fx, "GET", "/v2/spaces/"+mockedTechSpaceId+"/types", "scopedKey")
		require.Equal(t, http.StatusForbidden, denied.Code)
		require.Contains(t, denied.Body.String(), `"space_not_granted"`)

		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()
		grantedSession(fx, "scopedKey", readWrite("spaceA", mockedTechSpaceId))
		granted := serveWithKey(fx, "GET", "/v2/spaces/"+mockedTechSpaceId+"/types", "scopedKey")
		require.Equal(t, http.StatusOK, granted.Code)
	})

	t.Run("a read-only grant is refused on EVERY write-classified route", func(t *testing.T) {
		// the walk is driven by the same classification table the gate
		// enforces, and the conformance test pins that table against the
		// engine — so this covers every write route, present and future
		fx := newV2ServerFixture(t)
		grantedSession(fx, "readKey", readOnly("spaceA"))
		registerGrantTestSpace(t, fx, "spaceA", "Work")

		writes := 0
		for key, authz := range apiv2.RouteAuthzTable() {
			if authz.Verb != apiv2.RouteVerbWrite || authz.Global == apiv2.GlobalAuthExempt {
				continue
			}
			writes++
			method, path, _ := strings.Cut(key, " ")
			probe := path
			probe = strings.ReplaceAll(probe, ":space_id", "spaceA")
			for _, param := range []string{":object_id", ":type", ":key", ":chat_id", ":message_id"} {
				probe = strings.ReplaceAll(probe, param, "x")
			}

			w := serveWithKey(fx, method, probe, "readKey")
			require.Equal(t, http.StatusForbidden, w.Code, "%s must refuse a read-only grant", key)
			if authz.Global == apiv2.GlobalScopedDenied {
				// POST /v2/spaces is refused as a global route every granted
				// key is denied, before the verb gate is reached
				require.Contains(t, w.Body.String(), `"space_not_granted"`, key)
			} else {
				require.Contains(t, w.Body.String(), `"write_not_granted"`, key)
			}
		}
		require.GreaterOrEqual(t, writes, 20, "the write walk must cover the mutation surface")
	})

	t.Run("a read-only grant passes on representative reads", func(t *testing.T) {
		fx := newV2ServerFixture(t)
		registerGrantTestSpace(t, fx, "spaceA", "Work")
		grantedSession(fx, "readKey", readOnly("spaceA"))
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		for _, probe := range []struct{ method, path string }{
			{"GET", "/v2/spaces/spaceA"},
			{"GET", "/v2/spaces/spaceA/types"},
			// POST search is classified READ: POST only because it needs a body
			{"POST", "/v2/spaces/spaceA/search"},
			{"POST", "/v2/search"},
			{"GET", "/v2/spaces"},
			{"GET", "/v2/schemas"},
			{"POST", "/v2/validate"},
		} {
			w := httptest.NewRecorder()
			body := `{}`
			if probe.path == "/v2/validate" {
				body = `{"formatVersion":"2.0","blocks":[]}`
			}
			req := httptest.NewRequest(probe.method, probe.path, strings.NewReader(body))
			req.Host = localApiHost
			req.Header.Set("Authorization", "Bearer readKey")
			fx.Engine().ServeHTTP(w, req)
			require.Equal(t, http.StatusOK, w.Code, "%s %s", probe.method, probe.path)
		}
	})

	t.Run("a legacy nil-grant key is unaffected on /v2", func(t *testing.T) {
		fx := newV2ServerFixture(t)
		registerGrantTestSpace(t, fx, "spaceA", "Work")
		fx.KeyToToken = map[string]ApiSessionEntry{
			"legacyKey": {Token: "tok", AppName: "legacy", Scope: model.AccountAuth_JsonAPI},
		}
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		w := serveWithKey(fx, "GET", "/v2/spaces/spaceA", "legacyKey")
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("GET /v2/spaces lists only granted spaces", func(t *testing.T) {
		// given: three live spaces, grant covers one
		fx := newV2ServerFixture(t)
		registerGrantTestSpace(t, fx, "spaceA", "Work")
		registerGrantTestSpace(t, fx, "spaceB", "Personal")
		registerGrantTestSpace(t, fx, "spaceC", "Diary")
		grantedSession(fx, "scopedKey", readOnly("spaceA"))
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		// when
		w := serveWithKey(fx, "GET", "/v2/spaces", "scopedKey")

		// then: the non-granted spaces must not appear ANYWHERE in the
		// response — not as rows, not as names
		require.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		require.Contains(t, body, `"spaceA"`)
		require.NotContains(t, body, "spaceB")
		require.NotContains(t, body, "spaceC")
		require.NotContains(t, body, "Personal")
		require.NotContains(t, body, "Diary")
		require.Contains(t, body, `"total":1`)
	})

	t.Run("global search returns results ONLY from granted spaces and does not warn about others", func(t *testing.T) {
		// given: an object in each of two spaces; the grant covers spaceA.
		// The intersection happens on the INPUT space set (spaceRefs), so
		// the non-granted space contributes no rows, no totals, and — the
		// disclosure channel — no per-space warnings.
		fx := newV2ServerFixture(t)
		registerGrantTestSpace(t, fx, "spaceA", "Work")
		registerGrantTestSpace(t, fx, "spaceB", "Personal")
		for spaceId, objectId := range map[string]string{"spaceA": "docA", "spaceB": "docB"} {
			fx.objectStore.AddObjects(t, spaceId, []objectstore.TestObject{{
				bundle.RelationKeyId:               domain.String(objectId),
				bundle.RelationKeyName:             domain.String("Doc in " + spaceId),
				bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyLastModifiedDate: domain.Int64(1000),
			}})
		}
		grantedSession(fx, "scopedKey", readOnly("spaceA"))
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		// when
		w := serveWithKey(fx, "POST", "/v2/search", "scopedKey")

		// then
		require.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		require.Contains(t, body, `"docA"`)
		require.Contains(t, body, `"total":1`)
		require.NotContains(t, body, "docB")
		require.NotContains(t, body, "spaceB")
		require.NotContains(t, body, "Personal")
		require.NotContains(t, body, "warnings")
	})

	t.Run("a non-granted space's skip warning never reaches the wire", func(t *testing.T) {
		// given: a type that resolves ONLY in the granted space — the exact
		// probe that makes a non-intersected space emit a skip warning
		// naming it ("Personal") into the response body. The empty-search
		// subtest above cannot catch that channel: with no type to resolve,
		// neither space warns whether or not the intersection runs.
		fx := newV2ServerFixture(t)
		registerGrantTestSpace(t, fx, "spaceA", "Work")
		registerGrantTestSpace(t, fx, "spaceB", "Personal")
		fx.objectStore.AddObjects(t, "spaceA", []objectstore.TestObject{
			{
				bundle.RelationKeyId:             domain.String("type-chore"),
				bundle.RelationKeyName:           domain.String("Chore"),
				bundle.RelationKeyUniqueKey:      domain.String("ot-chore"),
				bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_objectType)),
			},
			{
				bundle.RelationKeyId:               domain.String("choreA"),
				bundle.RelationKeyName:             domain.String("A chore"),
				bundle.RelationKeyType:             domain.String("type-chore"),
				bundle.RelationKeyResolvedLayout:   domain.Int64(int64(model.ObjectType_basic)),
				bundle.RelationKeyLastModifiedDate: domain.Int64(2000),
			},
		})
		grantedSession(fx, "scopedKey", readOnly("spaceA"))
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		// when
		w := serveWithKeyBody(fx, "POST", "/v2/search", "scopedKey", `{"type":"chore"}`)

		// then
		require.Equal(t, http.StatusOK, w.Code)
		body := w.Body.String()
		require.Contains(t, body, `"choreA"`)
		require.NotContains(t, body, "warnings")
		require.NotContains(t, body, "Personal")
		require.NotContains(t, body, "spaceB")
	})
}

func TestV1RejectsGrantedKeys(t *testing.T) {
	t.Run("a granted key is refused on /v1 with a pointer to /v2", func(t *testing.T) {
		// the intended asymmetry: a GRANTED key is refused on /v1 (its
		// grant cannot be honored there); a LEGACY key is served on /v1
		// exactly as today. Grant presence decides, never key format.
		fx := newFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{
			"scopedKey": {Token: "tok", AppName: "agent", Scope: model.AccountAuth_JsonAPI,
				Grant: &util.ApiGrant{Spaces: []string{"spaceA"}, Perms: util.GrantPermsReadWrite}},
		}

		w := serveWithKey(fx, "GET", "/v1/spaces", "scopedKey")

		require.Equal(t, http.StatusForbidden, w.Code)
		body := w.Body.String()
		require.Contains(t, body, `"v1_not_available_for_scoped_keys"`)
		require.Contains(t, body, "/v2")
		require.Contains(t, body, "spaces [spaceA] with readwrite access")
		require.Equal(t, `Bearer error="insufficient_scope"`, w.Header().Get("WWW-Authenticate"))
	})

	t.Run("a legacy nil-grant key keeps working on /v1", func(t *testing.T) {
		fx := newFixture(t)
		fx.KeyToToken = map[string]ApiSessionEntry{
			"legacyKey": {Token: "tok", AppName: "legacy", Scope: model.AccountAuth_JsonAPI},
		}
		fx.mwMock.On("ObjectSearch", mock.Anything, mock.Anything).
			Return(&pb.RpcObjectSearchResponse{
				Error: &pb.RpcObjectSearchResponseError{Code: pb.RpcObjectSearchResponseError_NULL},
			}, nil).Once()
		fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

		w := serveWithKey(fx, "GET", "/v1/spaces", "legacyKey")

		require.Equal(t, http.StatusOK, w.Code)
	})
}

func TestGrantEditTakesEffectOnNextRequest(t *testing.T) {
	// The safety-critical coupling from P1a: LinkLocalUpdateApp evicts the
	// key's cached HTTP entries via RevokeToken, so an in-place grant
	// NARROWING is enforced on the very next request — a stale cached grant
	// would be a silent authorization bypass.
	fx := newV2ServerFixture(t)
	registerGrantTestSpace(t, fx, "spaceA", "Work")
	registerGrantTestSpace(t, fx, "spaceB", "Personal")
	grantedSession(fx, "editedKey", &util.ApiGrant{
		Spaces: []string{"spaceA", "spaceB"}, Perms: util.GrantPermsReadWrite})
	fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

	// the wide grant serves spaceB
	before := serveWithKey(fx, "GET", "/v2/spaces/spaceB", "editedKey")
	require.Equal(t, http.StatusOK, before.Code)

	// the grant is narrowed to spaceA in place: LinkLocalUpdateApp persists
	// the new grant and calls RevokeToken with the key's session token —
	// this test performs exactly that eviction, then serves the re-mint
	// with the NARROWED grant the wallet now holds
	fx.RevokeToken("tok")
	fx.mwMock.
		On("WalletCreateSession", mock.Anything, &pb.RpcWalletCreateSessionRequest{
			Auth: &pb.RpcWalletCreateSessionRequestAuthOfAppKey{AppKey: "editedKey"},
		}).
		Return(&pb.RpcWalletCreateSessionResponse{
			Token:        "tok2",
			AccountScope: model.AccountAuth_JsonAPI,
			AppName:      "agent",
			Grant: &model.AccountAuthAppGrant{
				SpaceIds: []string{"spaceA"},
				Perm:     model.AccountAuthAppGrant_ReadWrite,
			},
			Error: &pb.RpcWalletCreateSessionResponseError{
				Code: pb.RpcWalletCreateSessionResponseError_NULL,
			},
		}, nil).Once()

	// the VERY NEXT request enforces the new grant, not the cached old one
	after := serveWithKey(fx, "GET", "/v2/spaces/spaceB", "editedKey")
	require.Equal(t, http.StatusForbidden, after.Code)
	require.Contains(t, after.Body.String(), `"space_not_granted"`)
	require.Contains(t, after.Body.String(), "spaces [spaceA] with readwrite access")

	// the still-granted space keeps working through the re-minted session
	granted := serveWithKey(fx, "GET", "/v2/spaces/spaceA", "editedKey")
	require.Equal(t, http.StatusOK, granted.Code)
}

func TestGrantEditDuringMintIsNotLost(t *testing.T) {
	// The eviction sweep can only evict entries that EXIST: a RevokeToken
	// landing while the very first mint for a key is in flight sweeps
	// nothing, and without the eviction-generation check the mint would then
	// cache the pre-edit WIDE grant permanently — every later request is a
	// cache hit, and a cached entry is re-validated only against ExpireAt.
	// The mock's Run hook makes the interleaving deterministic: the sweep
	// fires INSIDE WalletCreateSession, between the cache read and the cache
	// write.
	fx := newV2ServerFixture(t)
	registerGrantTestSpace(t, fx, "spaceA", "Work")
	registerGrantTestSpace(t, fx, "spaceB", "Personal")
	fx.eventMock.On("Broadcast", mock.Anything).Return(nil).Maybe()

	mintRequest := &pb.RpcWalletCreateSessionRequest{
		Auth: &pb.RpcWalletCreateSessionRequestAuthOfAppKey{AppKey: "raceKey"},
	}
	// the first mint returns the WIDE grant, and the narrowing lands
	// MID-MINT: LinkLocalUpdateApp persists the narrow grant and calls
	// RevokeToken while the mint is still in flight
	fx.mwMock.On("WalletCreateSession", mock.Anything, mintRequest).
		Run(func(args mock.Arguments) { fx.RevokeToken("raceTok") }).
		Return(&pb.RpcWalletCreateSessionResponse{
			Token:        "raceTok",
			AccountScope: model.AccountAuth_JsonAPI,
			AppName:      "agent",
			Grant: &model.AccountAuthAppGrant{
				SpaceIds: []string{"spaceA", "spaceB"},
				Perm:     model.AccountAuthAppGrant_ReadWrite,
			},
			Error: &pb.RpcWalletCreateSessionResponseError{
				Code: pb.RpcWalletCreateSessionResponseError_NULL,
			},
		}, nil).Once()

	// the racing request itself may still see the wide grant — it began
	// before the edit landed; what it must NOT do is cache it
	first := serveWithKey(fx, "GET", "/v2/spaces/spaceB", "raceKey")
	require.Equal(t, http.StatusOK, first.Code)

	// the NEXT request must be a cache MISS (nothing was cached), re-mint,
	// and be enforced against the grant the wallet holds NOW
	fx.mwMock.On("WalletCreateSession", mock.Anything, mintRequest).
		Return(&pb.RpcWalletCreateSessionResponse{
			Token:        "raceTok2",
			AccountScope: model.AccountAuth_JsonAPI,
			AppName:      "agent",
			Grant: &model.AccountAuthAppGrant{
				SpaceIds: []string{"spaceA"},
				Perm:     model.AccountAuthAppGrant_ReadWrite,
			},
			Error: &pb.RpcWalletCreateSessionResponseError{
				Code: pb.RpcWalletCreateSessionResponseError_NULL,
			},
		}, nil).Once()

	after := serveWithKey(fx, "GET", "/v2/spaces/spaceB", "raceKey")
	require.Equal(t, http.StatusForbidden, after.Code)
	require.Contains(t, after.Body.String(), `"space_not_granted"`)
	require.Contains(t, after.Body.String(), "spaces [spaceA] with readwrite access")

	// the narrowed entry (an ordinary mint, no eviction racing it) IS cached
	granted := serveWithKey(fx, "GET", "/v2/spaces/spaceA", "raceKey")
	require.Equal(t, http.StatusOK, granted.Code)
}
