package bundle

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

// The derived table is the §7.5a-1 authority for bundled keys; its whole
// claim to authority is collision-freedom. These tests are the loud failure
// a future bundle addition hits if it ever mints a colliding slug.

func TestApiSlugTableIsCollisionFree(t *testing.T) {
	t.Run("every bundled relation key has a distinct slug", func(t *testing.T) {
		require.NotEmpty(t, relations)
		assert.Len(t, relationKeyByApiSlug, len(relations),
			"two bundled relation keys derive the same slug — the table lost an entry")
	})

	t.Run("every bundled type key has a distinct slug", func(t *testing.T) {
		require.NotEmpty(t, types)
		assert.Len(t, typeKeyByApiSlug, len(types),
			"two bundled type keys derive the same slug — the table lost an entry")
	})

	t.Run("the fold layer is clean over the bundle", func(t *testing.T) {
		// two bundled keys folding together would make the forgiving layer
		// permanently ambiguous for both — fail here, at the bundle change
		for fold, keys := range relationKeysByFold {
			assert.Len(t, keys, 1, "relation fold %q is ambiguous: %v", fold, keys)
		}
		for fold, keys := range typeKeysByFold {
			assert.Len(t, keys, 1, "type fold %q is ambiguous: %v", fold, keys)
		}
	})
}

func TestApiSlugRoundTrip(t *testing.T) {
	t.Run("every relation slug resolves back to its key", func(t *testing.T) {
		for key := range relations {
			got, ok := RelationKeyByApiSlug(ApiSlug(key.String()))
			require.True(t, ok, "slug of %q does not resolve", key)
			assert.Equal(t, key, got)
		}
	})

	t.Run("every type slug resolves back to its key", func(t *testing.T) {
		for key := range types {
			got, ok := TypeKeyByApiSlug(ApiSlug(key.String()))
			require.True(t, ok, "slug of %q does not resolve", key)
			assert.Equal(t, key, got)
		}
	})

	t.Run("string inversion is not the reverse mechanism", func(t *testing.T) {
		// the documented non-invertible cases (ADDRESSING.md §7.5a-1): the
		// table must carry them because no case transform can
		key, ok := RelationKeyByApiSlug("media_artist_url")
		require.True(t, ok)
		assert.Equal(t, domain.RelationKey("mediaArtistURL"), key)
	})
}

func TestApiSlugSpellings(t *testing.T) {
	// the wire spellings the surface rule promises (§7.5a): a drift in the
	// snake transform respells the API — pin the load-bearing examples
	assert.Equal(t, "due_date", ApiSlug("dueDate"))
	assert.Equal(t, "icon_emoji", ApiSlug("iconEmoji"))
	assert.Equal(t, "object_type", ApiSlug("objectType"))
	assert.Equal(t, "name", ApiSlug("name"))
}

func TestFoldApiKey(t *testing.T) {
	assert.Equal(t, "duedate", FoldApiKey("due_date"))
	assert.Equal(t, "duedate", FoldApiKey("dueDate"))
	assert.Equal(t, "duedate", FoldApiKey("due-date"))
	assert.NotEqual(t, FoldApiKey("dueDate"), FoldApiKey("dueDates"))
}

func TestApiSlugFromName(t *testing.T) {
	assert.Equal(t, "manual_property", ApiSlugFromName("Manual property"))
	assert.Equal(t, "uber", ApiSlugFromName(" Über "))
}

// TestApiSlugTablesAreInjective is the guard init panics on, run over the
// REAL tables. `key -> slug` is lossy, the reverse tables are plain maps, and
// nothing anywhere checked: a bundled key added tomorrow that snakes onto an
// existing slug would make `RelationKeyByApiSlug` a per-process coin flip —
// a different address on a different restart, with no signal at all. The
// tables are clean today, so this can only ever fail on the commit that
// breaks them.
func TestApiSlugTablesAreInjective(t *testing.T) {
	relationKeys := sortedApiSlugKeys(len(relations), func(yield func(string)) {
		for key := range relations {
			yield(key.String())
		}
	})
	typeKeys := sortedApiSlugKeys(len(types), func(yield func(string)) {
		for key := range types {
			yield(key.String())
		}
	})

	assert.NoError(t, checkApiSlugInjectivity("relation", relationKeys))
	assert.NoError(t, checkApiSlugInjectivity("type", typeKeys))
	assert.Equal(t, len(relationKeys), len(relationKeyByApiSlug), "one slug per bundled relation")
	assert.Equal(t, len(typeKeys), len(typeKeyByApiSlug), "one slug per bundled type")
}

func TestApiSlugInjectivityGuardFires(t *testing.T) {
	t.Run("two keys deriving one slug", func(t *testing.T) {
		err := checkApiSlugInjectivity("relation", []string{"dueDate", "due_date"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), `both derive the api slug "due_date"`)
	})

	t.Run("two slugs folding together", func(t *testing.T) {
		err := checkApiSlugInjectivity("type", []string{"moodlevel", "moodLevel"})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "fold together")
	})

	t.Run("a clean table passes", func(t *testing.T) {
		assert.NoError(t, checkApiSlugInjectivity("relation", []string{"dueDate", "name", "_score"}))
	})
}
