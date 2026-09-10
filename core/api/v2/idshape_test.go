package apiv2

// idshape_test.go pins the route half of `?ids=` (APIV2.md §8.36): the
// middleware validates the parameter once for the whole group, records a
// full request on the context so the space surfaces serve full ids, and runs
// in front of resolveSpaceRef so a refusal's candidates are spelled the way
// the request asked for.

import (
	"encoding/json"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

// refSpaceCousin differs from refSpaceTwinA at the SIXTH character from the
// end of the CID half, so the two have distinct short forms (`q3oake` /
// `r3oake`) while sharing the five-character tail `3oake`.
const refSpaceCousin = "bafyreiay4rdeleruyuy6x575hvhtifedmjq4g3ojpffvpbgmuackr3oake.28y6mgnwgodt7"

func TestEnsureIdsShapeMiddleware(t *testing.T) {
	t.Run("?ids=full serves the full space id; the default serves the short reference", func(t *testing.T) {
		// given
		engine := newSpaceRefEngine(t, nil, refSpaceEval, refSpaceTracker)

		// when: the SAME route, the same resolved space, two shapes
		full := serveSpaceRef(t, engine, "/v2/spaces/"+refSpaceEval+"?ids=full")
		short := serveSpaceRef(t, engine, "/v2/spaces/"+refSpaceEval)

		// then
		require.Equal(t, http.StatusOK, full.Code)
		require.Equal(t, http.StatusOK, short.Code)
		assert.Contains(t, full.Body.String(), `"id":"`+refSpaceEval+`"`)
		assert.Contains(t, short.Body.String(), `"id":"hxwz2i"`)
	})

	t.Run("accepting is unchanged: a short reference addresses it, ?ids=full spells it back", func(t *testing.T) {
		// given: this is the loop a caller needs to get a persistable id out
		// of a reference it was served earlier
		engine := newSpaceRefEngine(t, nil, refSpaceEval, refSpaceTracker)

		// when
		w := serveSpaceRef(t, engine, "/v2/spaces/hxwz2i?ids=full")

		// then
		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"id":"`+refSpaceEval+`"`)
	})

	t.Run("?ids=compact is the default, spelled out", func(t *testing.T) {
		engine := newSpaceRefEngine(t, nil, refSpaceEval, refSpaceTracker)

		w := serveSpaceRef(t, engine, "/v2/spaces/hxwz2i?ids=compact")

		require.Equal(t, http.StatusOK, w.Code)
		assert.Contains(t, w.Body.String(), `"id":"hxwz2i"`)
	})

	t.Run("an unknown ids value is refused 400 before the handler runs", func(t *testing.T) {
		// given
		engine := newSpaceRefEngine(t, nil, refSpaceEval)

		// when
		w := serveSpaceRef(t, engine, "/v2/spaces/hxwz2i/objects?ids=export")

		// then: the C6 shape, addressed at the parameter and naming the two
		// values — the same refusal the object read's own validation gives
		require.Equal(t, http.StatusBadRequest, w.Code)
		var body v2model.Error
		require.NoError(t, json.Unmarshal(w.Body.Bytes(), &body))
		assert.Equal(t, v2model.CodeValidationFailed, body.Code)
		require.Len(t, body.Issues, 1)
		assert.Equal(t, "ids", body.Issues[0].Path)
		assert.Contains(t, body.Issues[0].Hint, "compact, full")
	})

	t.Run("the shape is read BEFORE resolution, so a refusal's candidates obey it", func(t *testing.T) {
		// given: two spaces that both answer to `3oake` and both HAVE short
		// forms of their own — the only ambiguity in which the candidate
		// spelling can differ at all
		engine := newSpaceRefEngine(t, nil, refSpaceTwinA, refSpaceCousin)

		// when
		short := serveSpaceRef(t, engine, "/v2/spaces/3oake/objects")
		full := serveSpaceRef(t, engine, "/v2/spaces/3oake/objects?ids=full")

		// then
		require.Equal(t, http.StatusBadRequest, short.Code)
		require.Equal(t, http.StatusBadRequest, full.Code)
		assert.Contains(t, short.Body.String(), "q3oake")
		assert.NotContains(t, short.Body.String(), refSpaceTwinA)
		assert.Contains(t, full.Body.String(), refSpaceTwinA)
		assert.Contains(t, full.Body.String(), refSpaceCousin)
	})
}
