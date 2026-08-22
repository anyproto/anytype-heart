package anyblockjson

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

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
