package apiv2

// spaceref_test.go pins the route half of the short space reference
// (APIV2.md §8.35): the param rewrite, its position in front of the grant
// gate, and the refusals.
//
// The space ids here are real ones off the eval account — 59-character
// base32 CIDs sharing ONE base36 replication key, which is how a real
// account looks and the only fixture in which the mechanism is observable.

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/api/util"
	v2handler "github.com/anyproto/anytype-heart/core/api/v2/handler"
	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	v2service "github.com/anyproto/anytype-heart/core/api/v2/service"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const (
	refSpaceEval    = "bafyreihwvsaekzzyb54o7um4hdpvpn5b2invn75lmijhhtghblvphxwz2i.28y6mgnwgodt7"
	refSpaceTracker = "bafyreia4znhhjvxek2iux7enfzilekj5vwlgddgogpfgnx5qnim7bugaxa.28y6mgnwgodt7"
	// the twins share their last six CID characters
	refSpaceTwinA = "bafyreiay4rdeleruyuy6x575hvhtifedmjq4g3ojpffvpbgmuackq3oake.28y6mgnwgodt7"
	refSpaceTwinB = "bafyreiby4rdeleruyuy6x575hvhtifedmjq4g3ojpffvpbgmuackq3oake.28y6mgnwgodt7"
)

// newSpaceRefEngine builds the /v2 middleware chain as router.go orders it —
// grant on the context, the `?ids=` shape, THEN resolution, THEN the grant
// gate — over a store holding the given spaces. The probe handler answers
// with whatever :space_id the handlers would see, which is what makes the
// rewrite observable; the second probe raises a not-found quoting that same
// value, which is what makes the echo observable; the third is the real
// GET-one handler, which is what makes the SERVED spelling observable.
func newSpaceRefEngine(t *testing.T, grant *util.ApiGrant, spaceIds ...string) *gin.Engine {
	t.Helper()
	store := objectstore.NewStoreFixture(t)
	for i, spaceId := range spaceIds {
		store.AddObjects(t, objectstore.TestTechSpaceId, []objectstore.TestObject{{
			bundle.RelationKeyId:             domain.String("spaceView_" + spaceId),
			bundle.RelationKeyResolvedLayout: domain.Int64(int64(model.ObjectType_spaceView)),
			bundle.RelationKeyTargetSpaceId:  domain.String(spaceId),
			bundle.RelationKeyName:           domain.String("Space " + string(rune('A'+i))),
		}})
	}
	svc := v2service.NewV2Service(nil, nil, nil, nil, store, objectstore.TestTechSpaceId, "")

	router := gin.New()
	group := router.Group("/v2")
	group.Use(func(c *gin.Context) {
		c.Request = c.Request.WithContext(util.CtxWithApiGrant(c.Request.Context(), grant))
		c.Next()
	})
	group.Use(ensureIdsShape())
	group.Use(resolveSpaceRef(svc))
	group.Use(ensureSpaceGrant())
	group.GET("/spaces/:space_id", v2handler.GetSpaceV2Handler(svc))
	group.GET("/spaces/:space_id/objects", func(c *gin.Context) {
		c.String(http.StatusOK, c.Param(SpaceParam))
	})
	group.GET("/spaces/:space_id/types/:type", func(c *gin.Context) {
		v2handler.RespondV2Error(c, v2model.NotFound(
			"type \"page\" not found in space \""+c.Param(SpaceParam)+"\"",
			v2model.Issue{Hint: "list all with GET /v2/spaces/" + c.Param(SpaceParam) + "/types"}))
	})
	return router
}

func serveSpaceRef(t *testing.T, engine *gin.Engine, path string) *httptest.ResponseRecorder {
	t.Helper()
	w := httptest.NewRecorder()
	engine.ServeHTTP(w, httptest.NewRequest(http.MethodGet, path, nil))
	return w
}

func TestResolveSpaceRefMiddleware(t *testing.T) {
	t.Run("a short reference in the path becomes the full id the handler sees", func(t *testing.T) {
		// given
		engine := newSpaceRefEngine(t, nil, refSpaceEval, refSpaceTracker)

		// when
		w := serveSpaceRef(t, engine, "/v2/spaces/hxwz2i/objects")

		// then
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, refSpaceEval, w.Body.String())
	})

	t.Run("the full id keeps working, untouched", func(t *testing.T) {
		engine := newSpaceRefEngine(t, nil, refSpaceEval, refSpaceTracker)

		w := serveSpaceRef(t, engine, "/v2/spaces/"+refSpaceEval+"/objects")

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, refSpaceEval, w.Body.String())
	})

	t.Run("the §8.34 truncation resolves instead of 404ing", func(t *testing.T) {
		// given: the mistake gemma4:e4b made on 83 of 93 find calls — the id
		// cut at the dot. The CID half is a suffix of itself, so the rule
		// that serves short references also repairs the truncation.
		engine := newSpaceRefEngine(t, nil, refSpaceEval, refSpaceTracker)

		w := serveSpaceRef(t, engine, "/v2/spaces/bafyreihwvsaekzzyb54o7um4hdpvpn5b2invn75lmijhhtghblvphxwz2i/objects")

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, refSpaceEval, w.Body.String())
	})

	t.Run("an unresolvable reference reaches the handler unchanged", func(t *testing.T) {
		engine := newSpaceRefEngine(t, nil, refSpaceEval)

		w := serveSpaceRef(t, engine, "/v2/spaces/zzzzzz/objects")

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "zzzzzz", w.Body.String(), "the ordinary 404 must quote the caller's own value")
	})

	t.Run("an ambiguous reference is refused 400 before the handler runs", func(t *testing.T) {
		// given: two spaces whose tails collide — neither has a short form,
		// and the tail they share addresses neither
		engine := newSpaceRefEngine(t, nil, refSpaceTwinA, refSpaceTwinB)

		// when
		w := serveSpaceRef(t, engine, "/v2/spaces/q3oake/objects")

		// then
		require.Equal(t, http.StatusBadRequest, w.Code)
		var body v2model.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, v2model.CodeAmbiguousInput, body.Code)
		require.Len(t, body.Issues, 1)
		assert.Contains(t, body.Issues[0].Message, refSpaceTwinA)
		assert.Contains(t, body.Issues[0].Message, refSpaceTwinB)
	})

	t.Run("a refusal echoes the caller's own spelling, not the id it resolved to", func(t *testing.T) {
		// given
		engine := newSpaceRefEngine(t, nil, refSpaceEval)

		// when: a short reference on a route whose handler refuses
		w := serveSpaceRef(t, engine, "/v2/spaces/hxwz2i/types/page")

		// then: neither the message nor the hint hands back the full id
		require.Equal(t, http.StatusNotFound, w.Code)
		assert.NotContains(t, w.Body.String(), refSpaceEval)
		assert.Contains(t, w.Body.String(), "hxwz2i")
	})

	t.Run("a full-id caller is echoed the full id (nothing is rewritten)", func(t *testing.T) {
		engine := newSpaceRefEngine(t, nil, refSpaceEval)

		w := serveSpaceRef(t, engine, "/v2/spaces/"+refSpaceEval+"/types/page")

		require.Equal(t, http.StatusNotFound, w.Code)
		assert.Contains(t, w.Body.String(), refSpaceEval)
	})
}

func TestResolveSpaceRefAndTheGrantGate(t *testing.T) {
	t.Run("resolution runs BEFORE the gate, so a granted space's short reference passes", func(t *testing.T) {
		// given: the gate compares against full ids, so an unresolved short
		// reference would be refused as a non-granted space
		grant := &util.ApiGrant{Spaces: []string{refSpaceEval}, Perms: util.GrantPermsRead}
		engine := newSpaceRefEngine(t, grant, refSpaceEval, refSpaceTracker)

		// when
		w := serveSpaceRef(t, engine, "/v2/spaces/hxwz2i/objects")

		// then
		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, refSpaceEval, w.Body.String())
	})

	t.Run("a short reference is not a way to reach a space the key does not hold", func(t *testing.T) {
		// given: both spaces are live, only one is granted
		grant := &util.ApiGrant{Spaces: []string{refSpaceEval}, Perms: util.GrantPermsRead}
		engine := newSpaceRefEngine(t, grant, refSpaceEval, refSpaceTracker)

		// when: the NON-granted space's tail
		w := serveSpaceRef(t, engine, "/v2/spaces/bugaxa/objects")

		// then: refused, and the refusal quotes the caller's value — it does
		// not name the space the tail belongs to
		require.Equal(t, http.StatusForbidden, w.Code)
		assert.Contains(t, w.Body.String(), "bugaxa")
		assert.NotContains(t, w.Body.String(), refSpaceTracker)
	})

	t.Run("the full id of a non-granted space is refused exactly as before", func(t *testing.T) {
		grant := &util.ApiGrant{Spaces: []string{refSpaceEval}, Perms: util.GrantPermsRead}
		engine := newSpaceRefEngine(t, grant, refSpaceEval, refSpaceTracker)

		w := serveSpaceRef(t, engine, "/v2/spaces/"+refSpaceTracker+"/objects")

		require.Equal(t, http.StatusForbidden, w.Code)
	})

	t.Run("the tail of an invisible space resolves nowhere", func(t *testing.T) {
		// given: the tracker space is NOT registered at all — resolution's
		// candidate set is the caller's visible spaces, so its tail is just
		// an unknown string
		engine := newSpaceRefEngine(t, nil, refSpaceEval)

		w := serveSpaceRef(t, engine, "/v2/spaces/bugaxa/objects")

		require.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "bugaxa", w.Body.String())
	})
}
