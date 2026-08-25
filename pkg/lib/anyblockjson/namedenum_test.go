package anyblockjson

// namedenum_test.go — the name-over-number properties beyond the layout keys
// (§3). Each stored key in namedEnumProperties writes its enum's NAME, reads
// the name back to the stored number, refuses an unknown name as an ERROR,
// and passes a raw number through unchanged in both directions.
//
// The error half is the point. Before these keys were named, the name was
// accepted-then-zeroed: `{"layout_align": "center"}` VALIDATED, Unmarshal
// stored the STRING on a number-format detail, and every consumer reading it
// with an int getter silently saw 0 — a warning existed but Validate
// discards warnings, so no caller ever learned. These tests replace that
// behaviour deliberately: same scenario, new rule, pre-freeze and
// corpus-checked — zero of the 26,803 real stored values across the three
// newly named keys is a string, so the promoted error refuses nothing any
// real export carries.

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestNamedEnum_LayoutAlign(t *testing.T) {
	t.Run("export writes the name", func(t *testing.T) {
		snap := &model.SmartBlockSnapshotBase{
			Blocks: []*model.Block{{Id: "o1",
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
			Details: fields(map[string]*types.Value{
				"id":          str("o1"),
				"layoutAlign": num(float64(model.Block_AlignCenter)),
			}),
		}
		data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
		require.NoError(t, err)
		assert.Contains(t, string(data), `"layout_align": "center"`,
			"the same four names blocks and view columns spell — one concept, one spelling (§15 #14)")
		require.NoError(t, Validate(data), "§11 I1")
	})

	t.Run("import maps the name to the stored number", func(t *testing.T) {
		doc := `{"version": 1, "id": "o1", "properties": {"layout_align": "center"}}`
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		v := snap.Details.Fields["layoutAlign"]
		require.NotNil(t, v)
		_, isNum := v.GetKind().(*types.Value_NumberValue)
		require.True(t, isNum, "must be stored as a number, not %T", v.GetKind())
		assert.Equal(t, float64(model.Block_AlignCenter), v.GetNumberValue())
	})

	// This scenario used to be VALID: the string landed on the number-format
	// detail and every int getter answered 0 (left). A warning existed, but
	// Validate discards warnings — a consumer calling Validate saw a clean
	// document and a silently mis-set object. The key is named now, so an
	// unknown name is an ERROR that states the vocabulary.
	t.Run("an unknown name is refused, naming the vocabulary", func(t *testing.T) {
		doc := `{"version": 1, "id": "o1", "properties": {"layout_align": "centre"}}`
		err := Validate([]byte(doc))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/properties/layout_align")
		assert.Contains(t, err.Error(), "unknown align")
		assert.Contains(t, err.Error(), "'center'", "the refusal names the name that was nearly right")
		_, _, unmErr := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.Error(t, unmErr, "Unmarshal must reject what Validate rejects (§11 I2)")
	})

	t.Run("a raw number still round-trips", func(t *testing.T) {
		doc := `{"version": 1, "id": "o1", "properties": {"layout_align": 2}}`
		require.NoError(t, Validate([]byte(doc)))
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, float64(model.Block_AlignRight), snap.Details.Fields["layoutAlign"].GetNumberValue())
	})

	t.Run("a number outside the vocabulary exports as the number", func(t *testing.T) {
		snap := &model.SmartBlockSnapshotBase{
			Blocks: []*model.Block{{Id: "o1",
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
			Details: fields(map[string]*types.Value{
				"id":          str("o1"),
				"layoutAlign": num(99),
			}),
		}
		data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
		require.NoError(t, err)
		assert.Contains(t, string(data), `"layout_align": 99`,
			"a stored value outside the vocabulary round-trips as its number rather than being lost")
		require.NoError(t, Validate(data), "§11 I1")
	})
}
