package snapshotdiff

// Compare used to read only details and text, so the TYPE namespace was
// invisible to it: a 36 808-object production sweep could not have caught a
// type substitution, and every claim about type-key correctness rested on
// synthetic tests. These tests pin both directions of the new axis — a
// rebinding must be reported, the documented truncation must not — because a
// comparator that cries wolf on normal exports gets ignored, and one that
// misses a rebinding is why the axis was added.

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// customTypeKey is a space-minted (bson) type key — the stored key a slug
// binds to when a vocabulary is in play.
const customTypeKey = "69bbfc78877a91b1d12d1a7c"

func typed(objectTypes ...string) *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{ObjectTypes: objectTypes}
}

func TestCompareObjectTypes(t *testing.T) {
	// --- must be reported: the type list means something different now ---

	t.Run("a rebound type is reported", func(t *testing.T) {
		// given: the round trip landed on a different type entirely
		got := Compare(typed("ot-task"), typed("ot-page"), anyblockjson.Options{})

		// then
		require.Len(t, got, 1)
		assert.Equal(t, `object type [0] changed: "task" -> "page"`, got[0])
	})

	t.Run("a rebound template target is reported", func(t *testing.T) {
		// given: position 1 is modelled for a template, so a change there is
		// the template silently moving to another type
		got := Compare(typed("ot-template", "ot-task"), typed("ot-template", "ot-page"), anyblockjson.Options{})

		// then
		require.Len(t, got, 1)
		assert.Equal(t, `object type [1] changed: "task" -> "page"`, got[0])
	})

	t.Run("a lost template target is reported", func(t *testing.T) {
		got := Compare(typed("ot-template", "ot-task"), typed("ot-template"), anyblockjson.Options{})

		require.Len(t, got, 1)
		assert.Equal(t, `object type [1] lost: "task"`, got[0])
	})

	t.Run("a lost type is reported", func(t *testing.T) {
		got := Compare(typed("ot-task"), typed(), anyblockjson.Options{})

		require.Len(t, got, 1)
		assert.Equal(t, `object type [0] lost: "task"`, got[0])
	})

	t.Run("a type the round trip invented is reported", func(t *testing.T) {
		// given: export models one slot here, so a second entry did not come
		// from the original
		got := Compare(typed("ot-page"), typed("ot-page", "ot-task"), anyblockjson.Options{})

		require.Len(t, got, 1)
		assert.Equal(t, `object type [1] added: "task"`, got[0])
	})

	// --- must NOT be reported: documented normalization ---

	t.Run("truncation past the modelled positions is not drift", func(t *testing.T) {
		// given: a non-template has one modelled slot; ot-task has nowhere to
		// be written and is dropped by design
		got := Compare(typed("ot-page", "ot-task"), typed("ot-page"), anyblockjson.Options{})

		assert.Empty(t, got)
	})

	t.Run("a template's third type is truncated, not lost", func(t *testing.T) {
		got := Compare(typed("ot-template", "ot-task", "ot-page"), typed("ot-template", "ot-task"), anyblockjson.Options{})

		assert.Empty(t, got)
	})

	t.Run("a keyless entry drops and the survivors close ranks", func(t *testing.T) {
		// given: "ot-" names no type, so export drops it with a warning and
		// ot-task moves up into the `type` slot
		got := Compare(typed("ot-", "ot-task"), typed("ot-task"), anyblockjson.Options{})

		assert.Empty(t, got)
	})

	t.Run("a keyless entry between good ones drops", func(t *testing.T) {
		got := Compare(typed("ot-template", "ot-", "ot-task"), typed("ot-template", "ot-task"), anyblockjson.Options{})

		assert.Empty(t, got)
	})

	t.Run("an empty template target drops", func(t *testing.T) {
		got := Compare(typed("ot-template", "ot-"), typed("ot-template"), anyblockjson.Options{})

		assert.Empty(t, got)
	})

	t.Run("a keyless entry taking its sibling with it IS reported", func(t *testing.T) {
		// given: the collateral-damage bug envelopeTypeTerms used to have — a
		// keyless entry omitted the `type` slot, which made template_for
		// inexpressible, so the good sibling died beside its bad neighbour.
		// The comparator has to see that, or the sweep cannot catch a
		// regression of it.
		got := Compare(typed("ot-", "ot-task"), typed(), anyblockjson.Options{})

		require.Len(t, got, 1)
		assert.Equal(t, `object type [0] lost: "task"`, got[0])
	})

	t.Run("the ot- prefix is normalized on both sides", func(t *testing.T) {
		// given: legacy rows hold a bare key; import always writes it prefixed
		got := Compare(typed("task"), typed("ot-task"), anyblockjson.Options{})

		assert.Empty(t, got)
	})

	t.Run("duplicates round-trip and are not drift", func(t *testing.T) {
		got := Compare(typed("ot-template", "ot-template"), typed("ot-template", "ot-template"), anyblockjson.Options{})

		assert.Empty(t, got)
	})

	t.Run("no types on either side is not drift", func(t *testing.T) {
		got := Compare(typed(), typed(), anyblockjson.Options{})

		assert.Empty(t, got)
	})
}

// divergentVocabulary is the production defect in miniature: one reader binds
// the slug `task` to a space-minted type, another to the bundled one. That is
// the disagreement the `type_keys` legend exists to close (§3), and the only
// way a real export can come back on a different type — so a sweep that can
// see it can see the class.
type divergentVocabulary struct {
	anyblockjson.BundledKeyVocabulary
}

func (divergentVocabulary) TypeKey(slug string) (string, bool) {
	if slug == "task" {
		return customTypeKey, true
	}
	return anyblockjson.BundledKeyVocabulary{}.TypeKey(slug)
}

// The unit cases above hand-build the two sides. This one drives the real
// codec, so the comparator is pinned against what Marshal/Unmarshal actually
// do rather than against my model of them.
func TestCompareObjectTypes_ThroughTheCodec(t *testing.T) {
	t.Run("a reader that binds the slug elsewhere is caught", func(t *testing.T) {
		// given: exported by a package-only reader, read back by one whose
		// vocabulary binds `task` to a space-minted type
		orig := typed("ot-task")
		data, err := anyblockjson.Marshal(model.SmartBlockType_Page, orig, anyblockjson.Options{})
		require.NoError(t, err)
		require.NotContains(t, string(data), "type_keys",
			"the fixture only bites while the document carries no legend to invert the slug")

		// when
		_, back, err := anyblockjson.Unmarshal(data, anyblockjson.Options{Keys: divergentVocabulary{}})
		require.NoError(t, err)
		require.Equal(t, []string{"ot-" + customTypeKey}, back.ObjectTypes,
			"the fixture must actually rebind, or the assertion below is vacuous")

		// then
		got := Compare(orig, back, anyblockjson.Options{})
		require.Len(t, got, 1)
		assert.Equal(t, `object type [0] changed: "task" -> "`+customTypeKey+`"`, got[0])
	})

	t.Run("an honest round trip of every shape reports nothing", func(t *testing.T) {
		for _, c := range []struct {
			sbType model.SmartBlockType
			types  []string
		}{
			{model.SmartBlockType_Page, []string{"ot-page", "ot-task"}},
			{model.SmartBlockType_Page, []string{"ot-", "ot-task"}},
			{model.SmartBlockType_Page, []string{"ot-"}},
			{model.SmartBlockType_Page, []string{"ot-" + customTypeKey}},
			{model.SmartBlockType_Template, []string{"ot-template", "ot-task", "ot-page"}},
			{model.SmartBlockType_Template, []string{"ot-template", "ot-"}},
			{model.SmartBlockType_Template, []string{"ot-template", "ot-", "ot-task"}},
			{model.SmartBlockType_Template, []string{"ot-", "ot-template", "ot-task"}},
			{model.SmartBlockType_STType, []string{"ot-objectType"}},
		} {
			orig := typed(c.types...)
			data, err := anyblockjson.Marshal(c.sbType, orig, anyblockjson.Options{})
			require.NoError(t, err)
			_, back, err := anyblockjson.Unmarshal(data, anyblockjson.Options{})
			require.NoError(t, err)

			assert.Empty(t, Compare(orig, back, anyblockjson.Options{}), c.types)
		}
	})
}
