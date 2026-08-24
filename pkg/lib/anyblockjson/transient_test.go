package anyblockjson

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// transientProperties describe the MOMENT an object was written rather than the
// object: internalFlags carries editor state ("this object was just created,
// offer the type picker"), which a restored object is never in. Export drops
// them; import drops them too, silently, because a document carrying one is
// stale rather than wrong.
//
// These can only fail if a transient key starts reaching the snapshot or starts
// being refused: each asserts the RESULTING DETAILS, not merely that the
// document validates, so a rule that stopped firing would have to keep both
// the acceptance and the absence to pass.
func TestTransientProperties_DroppedNotRefused(t *testing.T) {
	for name, doc := range map[string]string{
		"empty, the shape 18,647 real objects carry": `{"version": 1, "properties": {"internal_flags": []}}`,
		"populated": `{"version": 1, "properties": {"internal_flags": ["editor_select_type"]}}`,
	} {
		t.Run(name, func(t *testing.T) {
			require.NoError(t, Validate([]byte(doc)),
				"a stale export must still import: transient state is dropped, not refused")

			_, snap, err := Unmarshal([]byte(doc), Options{})
			require.NoError(t, err, "Validate and Unmarshal agree (§11 I2)")
			assert.NotContains(t, snap.GetDetails().GetFields(), "internalFlags",
				"and it must not reach the snapshot")
		})
	}

	t.Run("a merge-resolution vector is still REFUSED, not dropped", func(t *testing.T) {
		// the control that keeps the exemption honest: neverWritableProperties
		// aims a document at an object it did not create, and stays an error
		require.Error(t, Validate([]byte(`{"version": 1, "properties": {"old_anytype_id": "x"}}`)))
	})

	t.Run("an ordinary property still lands", func(t *testing.T) {
		_, snap, err := Unmarshal([]byte(`{"version": 1, "properties": {"name": "keep me"}}`), Options{})
		require.NoError(t, err)
		assert.Equal(t, "keep me", snap.GetDetails().GetFields()["name"].GetStringValue())
	})
}

// Export's half: a snapshot carrying transient state must not write it.
func TestTransientProperties_NeverExported(t *testing.T) {
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{Id: "o1", Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		Details: fields(map[string]*types.Value{
			"id":            str("o1"),
			"name":          str("Real"),
			"internalFlags": {Kind: &types.Value_ListValue{ListValue: &types.ListValue{}}},
		}),
		ObjectTypes: []string{"ot-page"},
	}

	data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
	require.NoError(t, err)
	assert.NotContains(t, string(data), "internal_flags",
		"transient state describes the moment, not the object (§3)")
	assert.Contains(t, string(data), `"name"`, "and the rest of the object is untouched")
}

// Whether a transient key is a BUNDLED relation is a per-key verdict, and
// this pins each one so a change fires here instead of silently changing
// what gets eaten.
//
// The two justifications are different, and only one of them tolerates a
// bundled key. `internalFlags` IS a bundled relation and is stripped anyway,
// because what it holds is editor state — "this object was just created,
// offer the type picker" — which a restored object is never in. The
// analytics triple is stripped for the opposite reason: nothing defines
// those keys at all, so no reader can name them, give them a format, or act
// on them. If a bundled relation ever takes one of those three spellings
// they stop being nameless, the justification evaporates, and dropping them
// would delete real schema shipped with every reader.
//
// How this can fail: add a bundled relation named `data`, `isNew` or
// `layoutFormat`; remove the bundled `internalFlags`; or add a key to
// transientProperties without deciding which case it is.
func TestTransientProperties_BundledVerdictPerKey(t *testing.T) {
	want := map[string]bool{
		"internalFlags": true,  // bundled, and stripped regardless: editor state
		"data":          false, // not a relation at all — the whole justification
		"isNew":         false,
		"layoutFormat":  false,
	}
	assert.Equal(t, len(want), len(transientProperties),
		"every transient key owes a verdict here — a new one must say which case it is")
	for key, why := range transientProperties {
		t.Run(key, func(t *testing.T) {
			verdict, listed := want[key]
			require.Truef(t, listed, "%q was added to transientProperties with no bundled verdict (%s)", key, why)
			assert.Equalf(t, verdict, bundle.HasRelation(domain.RelationKey(key)),
				"%q changed sides: a nameless key that became bundled is real schema now, "+
					"and dropping it would delete it (%s)", key, why)
		})
	}
}

// The analytics triple: 35 type objects across 7 spaces carry
// `data: {"route":"SettingsSpace"}`, `isNew: true`, `layoutFormat: 0` — the
// client's analytics route context persisted onto the object instead of
// sent as an event. `data` is a MAP-shaped value that no relation defines,
// so no reader can name it, give it a format, or act on it.
//
// How this can fail: drop any of the three from transientProperties and its
// value reaches the snapshot.
func TestTransientProperties_TheAnalyticsTripleIsDropped(t *testing.T) {
	// given the exact shape those 35 objects carry
	doc := []byte(`{"version": 1, "kind": "object_type", "key": "use_case",
		"properties": {"name": "Use Case", "data": {"route": "SettingsSpace"},
		               "isNew": true, "layoutFormat": 0}}`)

	// when
	require.NoError(t, Validate(doc), "a stale export still imports")
	_, snap, err := Unmarshal(doc, Options{})
	require.NoError(t, err, "Validate and Unmarshal agree (§11 I2)")

	// then
	for _, key := range []string{"data", "isNew", "layoutFormat"} {
		assert.NotContains(t, snap.GetDetails().GetFields(), key,
			"%q describes the click that made the object, not the object", key)
	}
	assert.Contains(t, snap.GetDetails().GetFields(), "name", "the object itself survives")
}
