package apiv2

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
)

// newGrantEngine builds a bare engine with a stub auth middleware carrying
// the given grant on the request context (the way server's
// ensureAuthenticated does) and the gate under test, then a set of probe
// routes mirroring the real registration shapes.
func newGrantEngine(grant *util.ApiGrant) *gin.Engine {
	router := gin.New()
	ok := func(c *gin.Context) { c.String(http.StatusOK, "OK") }
	group := router.Group("/v2")
	group.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(util.CtxWithApiGrant(c.Request.Context(), grant))
		c.Next()
	})
	group.Use(ensureSpaceGrant())
	// registered probes (each present in v2RouteAuthz)
	group.GET("/spaces/:space_id", ok)
	group.GET("/spaces/:space_id/objects", ok)
	group.POST("/spaces/:space_id/objects", ok)
	group.POST("/spaces/:space_id/search", ok)
	group.POST("/spaces/:space_id/chats/:chat_id/read", ok)
	group.POST("/validate", ok)
	group.GET("/schemas", ok)
	group.GET("/spaces", ok)
	group.POST("/search", ok)
	group.POST("/spaces", ok)
	// an UNREGISTERED no-space route: not in v2RouteAuthz — must be refused
	// for any granted key, fail closed
	group.GET("/bogus", ok)
	// an unclassified space route: the verb gate must treat it as write
	group.GET("/spaces/:space_id/bogus", ok)
	return router
}

func serveGrant(t *testing.T, grant *util.ApiGrant, method, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, nil)
	newGrantEngine(grant).ServeHTTP(w, req)
	return w
}

func readGrant(spaces ...string) *util.ApiGrant {
	return &util.ApiGrant{Spaces: spaces, Perms: util.GrantPermsRead}
}

func readWriteGrant(spaces ...string) *util.ApiGrant {
	return &util.ApiGrant{Spaces: spaces, Perms: util.GrantPermsReadWrite}
}

func TestEnsureSpaceGrant(t *testing.T) {
	t.Run("a nil grant passes everywhere: legacy keys keep today's behavior", func(t *testing.T) {
		for _, probe := range []struct{ method, path string }{
			{"GET", "/v2/spaces/space1"},
			{"POST", "/v2/spaces/space1/objects"},
			{"POST", "/v2/spaces"},
			{"GET", "/v2/bogus"},
		} {
			w := serveGrant(t, nil, probe.method, probe.path)
			require.Equal(t, http.StatusOK, w.Code, "%s %s", probe.method, probe.path)
		}
	})

	t.Run("granted space passes", func(t *testing.T) {
		w := serveGrant(t, readWriteGrant("space1", "space2"), "GET", "/v2/spaces/space2/objects")
		require.Equal(t, http.StatusOK, w.Code)
	})

	t.Run("non-granted space is 403 space_not_granted naming the grant", func(t *testing.T) {
		// when
		w := serveGrant(t, readWriteGrant("space1"), "GET", "/v2/spaces/other/objects")

		// then
		require.Equal(t, http.StatusForbidden, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, `"space_not_granted"`)
		assert.Contains(t, body, `key not granted space \"other\"`)
		assert.Contains(t, body, "spaces [space1] with readwrite access")
		assert.Equal(t, `Bearer error="insufficient_scope", scope="space:other:read"`,
			w.Header().Get("WWW-Authenticate"))
	})

	t.Run("the WWW-Authenticate scope names readwrite when the denied route is a write", func(t *testing.T) {
		w := serveGrant(t, readWriteGrant("space1"), "POST", "/v2/spaces/other/objects")
		require.Equal(t, http.StatusForbidden, w.Code)
		assert.Equal(t, `Bearer error="insufficient_scope", scope="space:other:readwrite"`,
			w.Header().Get("WWW-Authenticate"))
	})

	t.Run("an EMPTY granted-space list denies every space — never all spaces", func(t *testing.T) {
		// persist-time validation makes an empty list impossible; if one is
		// ever encountered the gate must deny, not widen
		w := serveGrant(t, &util.ApiGrant{Spaces: []string{}, Perms: util.GrantPermsReadWrite}, "GET", "/v2/spaces/space1")
		require.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), `"space_not_granted"`)
	})

	t.Run("a read grant is refused on writes with 403 write_not_granted", func(t *testing.T) {
		tests := []struct{ method, path string }{
			{"POST", "/v2/spaces/space1/objects"},
			// chat read watermark: POST …/read is classified WRITE — it
			// mutates the synced read state every device sees
			{"POST", "/v2/spaces/space1/chats/chat1/read"},
		}
		for _, tt := range tests {
			t.Run(tt.method+" "+tt.path, func(t *testing.T) {
				w := serveGrant(t, readGrant("space1"), tt.method, tt.path)
				require.Equal(t, http.StatusForbidden, w.Code)
				body := w.Body.String()
				assert.Contains(t, body, `"write_not_granted"`)
				assert.Contains(t, body, "read-only")
				assert.Contains(t, body, "spaces [space1] with read access")
				assert.Equal(t, `Bearer error="insufficient_scope", scope="space:space1:readwrite"`,
					w.Header().Get("WWW-Authenticate"))
			})
		}
	})

	t.Run("a read grant passes on reads, POST search and validate included", func(t *testing.T) {
		for _, probe := range []struct{ method, path string }{
			{"GET", "/v2/spaces/space1"},
			{"GET", "/v2/spaces/space1/objects"},
			// POST only because the request needs a body — classified READ
			{"POST", "/v2/spaces/space1/search"},
			{"POST", "/v2/validate"},
			{"GET", "/v2/schemas"},
			// service-filtered: allowed through the gate, constrained in the
			// service layer
			{"GET", "/v2/spaces"},
			{"POST", "/v2/search"},
		} {
			w := serveGrant(t, readGrant("space1"), probe.method, probe.path)
			require.Equal(t, http.StatusOK, w.Code, "%s %s", probe.method, probe.path)
		}
	})

	t.Run("POST /v2/spaces is refused for every granted key, readwrite included", func(t *testing.T) {
		// a key that can mint spaces it then owns is not meaningfully scoped
		w := serveGrant(t, readWriteGrant("space1"), "POST", "/v2/spaces")
		require.Equal(t, http.StatusForbidden, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, `"space_not_granted"`)
		assert.Contains(t, body, "not available to space-scoped keys")
		assert.Equal(t, `Bearer error="insufficient_scope"`, w.Header().Get("WWW-Authenticate"))
	})

	t.Run("an unregistered no-space route is refused, not allowed", func(t *testing.T) {
		// fail closed: the registry is the allowlist; the conformance test
		// turns this runtime 403 into a CI failure before it can ship
		w := serveGrant(t, readWriteGrant("space1"), "GET", "/v2/bogus")
		require.Equal(t, http.StatusForbidden, w.Code)
		body := w.Body.String()
		assert.Contains(t, body, `"space_not_granted"`)
		assert.Contains(t, body, "not classified for space-scoped keys")
	})

	t.Run("an unclassified space route counts as write for a read grant", func(t *testing.T) {
		// the verb table is the allowlist too: no entry → write → refused
		// for read-only grants
		w := serveGrant(t, readGrant("space1"), "GET", "/v2/spaces/space1/bogus")
		require.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), `"write_not_granted"`)
	})

	t.Run("a global write route yields the scope-less insufficient_scope challenge", func(t *testing.T) {
		// unreachable through today's registry (the only global write is
		// scoped-denied and refused earlier), but the moment a global route
		// is classified write the challenge must take its empty-scope form,
		// never the malformed `scope="space::readwrite"`
		key := routeKey(http.MethodPost, "/v2/global-write")
		v2RouteAuthz[key] = RouteAuthz{Verb: RouteVerbWrite, Global: GlobalDataFreeAllow}
		defer delete(v2RouteAuthz, key)

		router := gin.New()
		group := router.Group("/v2")
		group.Use(func(c *gin.Context) {
			c.Request = c.Request.WithContext(util.CtxWithApiGrant(c.Request.Context(), readGrant("space1")))
			c.Next()
		})
		group.Use(ensureSpaceGrant())
		group.POST("/global-write", func(c *gin.Context) { c.String(http.StatusOK, "OK") })

		w := httptest.NewRecorder()
		router.ServeHTTP(w, httptest.NewRequest(http.MethodPost, "/v2/global-write", nil))

		require.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), `"write_not_granted"`)
		assert.Equal(t, `Bearer error="insufficient_scope"`, w.Header().Get("WWW-Authenticate"))
	})

	t.Run("the tech space is denied unless explicitly granted", func(t *testing.T) {
		// this gate runs BEFORE the service's ensureSpace, which admits the
		// tech space as an ordinary space id — so a grant without it must be
		// stopped here
		w := serveGrant(t, readWriteGrant("space1"), "GET", "/v2/spaces/techSpace1/objects")
		require.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), `"space_not_granted"`)

		granted := serveGrant(t, readWriteGrant("space1", "techSpace1"), "GET", "/v2/spaces/techSpace1/objects")
		require.Equal(t, http.StatusOK, granted.Code)
	})
}

func TestV2RouteAuthzTable(t *testing.T) {
	t.Run("every no-space entry carries an explicit global class", func(t *testing.T) {
		for key, authz := range V2RouteAuthz() {
			// the registry key is "METHOD /path"
			if !containsSpaceParam(key) {
				assert.NotEmpty(t, authz.Global, "%s has no :space_id and must carry a global class", key)
			} else {
				assert.Empty(t, authz.Global, "%s is space-scoped and must not carry a global class", key)
			}
		}
	})

	t.Run("the non-obvious verb calls hold", func(t *testing.T) {
		want := map[string]RouteVerb{
			"POST /v2/validate":                             RouteVerbRead,
			"POST /v2/search":                               RouteVerbRead,
			"POST /v2/spaces/:space_id/search":              RouteVerbRead,
			"POST /v2/spaces/:space_id/chats/:chat_id/read": RouteVerbWrite,
			"POST /v2/spaces":                               RouteVerbWrite,
		}
		table := V2RouteAuthz()
		for key, verb := range want {
			entry, ok := table[key]
			require.True(t, ok, "%s must be classified", key)
			assert.Equal(t, verb, entry.Verb, key)
		}
	})
}

func containsSpaceParam(routeKey string) bool {
	return strings.Contains(routeKey, "/:"+SpaceParam+"/") || strings.HasSuffix(routeKey, "/:"+SpaceParam)
}
