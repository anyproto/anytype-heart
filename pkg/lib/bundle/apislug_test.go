package bundle

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
)

// The derived table is the API surface's one authority for bundled keys; its
// whole claim to authority is collision-freedom. These tests are the loud
// failure a future bundle addition hits if it ever mints a colliding slug.

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
		// the documented non-invertible cases: the
		// table must carry them because no case transform can
		key, ok := RelationKeyByApiSlug("media_artist_url")
		require.True(t, ok)
		assert.Equal(t, domain.RelationKey("mediaArtistURL"), key)
	})
}

func TestApiSlugSpellings(t *testing.T) {
	// the spellings the API surface promises: a drift in the
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

// The mint pair is what the app STORES as apiObjectKey. Measured over a
// 38,123-object account, 27 of 1,530 stored api keys sat outside the key
// grammar the api advertises; every fixture below is one of those shapes,
// taken from that account.
func TestMintApiSlugFromName(t *testing.T) {
	t.Run("punctuation a display name carries never reaches the key", func(t *testing.T) {
		assert.Equal(t, "lists_[in_work]", ApiSlugFromName("Lists [in work]"),
			"the derive half leaves brackets in place — this is the stored key today")
		assert.Equal(t, "lists_in_work", MintApiSlugFromName("Lists [in work]"))
		assert.Equal(t, "manual_export_import", MintApiSlugFromName("Manual export & import"))
		assert.Equal(t, "50_done", MintApiSlugFromName("50% done"))
	})

	t.Run("an emoji arrives as unidecode's literal [?] and is dropped", func(t *testing.T) {
		assert.Equal(t, "[?]_medium", ApiSlugFromName("➡️ Medium"),
			"unidecode romanizes an unmappable rune to the three bytes `[?]`")
		assert.Equal(t, "medium", MintApiSlugFromName("➡️ Medium"))
	})

	t.Run("a name that romanizes keeps its word", func(t *testing.T) {
		assert.Equal(t, "zadacha", MintApiSlugFromName("Задача"))
	})

	t.Run("a name with nothing to romanize mints no slug at all", func(t *testing.T) {
		// the caller's cue to leave the object addressed by its internal key
		assert.Equal(t, "", MintApiSlugFromName("➡️"))
		assert.Equal(t, "", MintApiSlugFromName("☕"))
		assert.Equal(t, "", MintApiSlugFromName("   "))
	})

	t.Run("length is bounded", func(t *testing.T) {
		got := MintApiSlugFromName(strings.Repeat("a", 300))
		assert.Len(t, got, MaxApiSlugLen)
		assert.Equal(t, strings.Repeat("a", MaxApiSlugLen), got)
	})
}

func TestMintApiSlug(t *testing.T) {
	t.Run("a supplied key keeps its spelling where the grammar allows", func(t *testing.T) {
		assert.Equal(t, "due_date", MintApiSlug("dueDate"))
		assert.Equal(t, "due_date", MintApiSlug("due date"))
		assert.Equal(t, "already_snake", MintApiSlug("already_snake"))
		assert.Equal(t, "web_3", MintApiSlug("Web3"),
			"the snake transform splits a digit run; that is the rule both surfaces already use")
	})

	t.Run("and loses what the grammar does not admit", func(t *testing.T) {
		assert.Equal(t, "my_key", MintApiSlug("my key!"))
		assert.Equal(t, "lists_in_work", MintApiSlug("Lists [in work]"))
	})

	t.Run("no transliteration on a supplied key", func(t *testing.T) {
		// the name arm romanizes because nobody is there to ask; a key was
		// CHOSEN, and answering `Задача` with `zadacha` would name the object
		// something its author never wrote. Empty says so instead.
		assert.Equal(t, "", MintApiSlug("Задача"))
		assert.Equal(t, "", MintApiSlug("!!!"))
		assert.Equal(t, "", MintApiSlug("___"))
	})

	t.Run("length is bounded", func(t *testing.T) {
		assert.Len(t, MintApiSlug(strings.Repeat("k", 300)), MaxApiSlugLen)
	})
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
