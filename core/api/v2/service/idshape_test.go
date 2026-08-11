package v2service

// idshape_test.go pins the ONE definition of `?ids=` (APIV2.md §8.36): its
// legal values, the 400 an unknown one earns, and the fact that the object
// read's own validation is that same definition rather than a second copy of
// it.

import (
	"context"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
)

func TestParseIdsShape(t *testing.T) {
	t.Run("the legal values, and which one is the default", func(t *testing.T) {
		for raw, want := range map[string]bool{"": false, V2IdsCompact: false, V2IdsFull: true} {
			// when
			full, err := ParseIdsShape(raw)

			// then
			require.NoError(t, err, "ids=%q", raw)
			assert.Equal(t, want, full, "ids=%q", raw)
		}
	})

	t.Run("an unknown value is a 400 naming the allowed ones", func(t *testing.T) {
		// given: the shapes a caller plausibly guesses
		for _, raw := range []string{"export", "FULL", "true", "short"} {
			// when
			_, err := ParseIdsShape(raw)

			// then
			var apiErr *v2model.Error
			require.ErrorAs(t, err, &apiErr, "ids=%q must be refused", raw)
			assert.Equal(t, http.StatusBadRequest, apiErr.Status)
			require.Len(t, apiErr.Issues, 1)
			assert.Equal(t, "ids", apiErr.Issues[0].Path)
			assert.Contains(t, apiErr.Issues[0].Hint, "compact, full")
		}
	})

	t.Run("the object read's plan validation IS this parse, not a second copy", func(t *testing.T) {
		// given / when
		compact, err := V2ObjectQuery{Ids: V2IdsCompact}.validate()
		require.NoError(t, err)
		full, err := V2ObjectQuery{Ids: V2IdsFull}.validate()
		require.NoError(t, err)
		_, unknownErr := V2ObjectQuery{Ids: "export"}.validate()

		// then: `full` is the export shape — full block ids, no relabeling
		assert.True(t, compact.compactBlockLabels)
		assert.False(t, full.compactBlockLabels)

		var apiErr *v2model.Error
		require.ErrorAs(t, unknownErr, &apiErr)
		assert.Equal(t, http.StatusBadRequest, apiErr.Status)
		require.Len(t, apiErr.Issues, 1)
		assert.Equal(t, "ids", apiErr.Issues[0].Path)
	})
}

func TestFullIdsCtx(t *testing.T) {
	t.Run("an untouched context means the compact default", func(t *testing.T) {
		// given: what every internal caller and every test carries
		assert.False(t, fullIdsRequested(context.Background()))
	})

	t.Run("CtxWithFullIds is what the route middleware records", func(t *testing.T) {
		assert.True(t, fullIdsRequested(CtxWithFullIds(context.Background())))
	})
}
