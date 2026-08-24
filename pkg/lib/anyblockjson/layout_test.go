package anyblockjson

// Layout is stored as a number but named in the format (§3). Before this, a
// document following the spec ("layout": "profile") imported the *string*
// onto a number-format property: every consumer reads it with an int64
// getter, so the type silently fell back to basic (== 0). Since v0.32 the
// recommended layout travels as `type_settings.layout` (§2a); the same rule
// rides along.

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func TestImport_LayoutNameToNumber(t *testing.T) {
	for _, tc := range []struct {
		name string
		want model.ObjectTypeLayout
	}{
		{"basic", model.ObjectType_basic},
		{"profile", model.ObjectType_profile},
		{"todo", model.ObjectType_todo},
		{"note", model.ObjectType_note},
		{"set", model.ObjectType_set},
		{"collection", model.ObjectType_collection},
	} {
		t.Run(tc.name, func(t *testing.T) {
			doc := `{"version": 1, "kind": "object_type", "id": "t1", "key": "k",
				"type_settings": {"layout": "` + tc.name + `"}}`
			_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
			require.NoError(t, err)

			v := snap.Details.Fields["recommendedLayout"]
			require.NotNil(t, v)
			_, isNum := v.GetKind().(*types.Value_NumberValue)
			require.True(t, isNum, "must be stored as a number, not %T", v.GetKind())
			assert.Equal(t, float64(tc.want), v.GetNumberValue())
		})
	}
}

// legacy documents that wrote the raw enum still import unchanged
func TestImport_LayoutNumberStillAccepted(t *testing.T) {
	doc := `{"version": 1, "kind": "object_type", "id": "t1", "key": "k",
		"type_settings": {"layout": 1}}`
	_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, float64(model.ObjectType_profile),
		snap.Details.Fields["recommendedLayout"].GetNumberValue())
}

func TestExport_LayoutNumberToName(t *testing.T) {
	snapshot := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "t1", Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
		},
		Details: fields(map[string]*types.Value{
			"id":                str("t1"),
			"recommendedLayout": num(float64(model.ObjectType_profile)),
			"resolvedLayout":    num(float64(model.ObjectType_todo)),
		}),
		Key: "k",
	}
	data, err := Marshal(model.SmartBlockType_STType, snapshot, testOptions())
	require.NoError(t, err)
	assert.Contains(t, string(data), `"layout": "profile"`,
		"the recommended layout is the group's layout member (§2a)")
	assert.NotContains(t, string(data), `"resolved_layout"`,
		"a type document does not carry its own display provenance (§2a)")
}

func TestRoundtrip_LayoutSurvives(t *testing.T) {
	doc := `{"version": 1, "kind": "object_type", "id": "t1", "key": "k",
		"type_settings": {"layout": "profile"}}`
	_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	data, err := Marshal(model.SmartBlockType_STType, snap, testOptions())
	require.NoError(t, err)
	assert.Contains(t, string(data), `"layout": "profile"`)
}

// a typo must not reach the snapshot as a bare string
func TestValidate_UnknownLayoutRejected(t *testing.T) {
	doc := `{"version": 1, "kind": "object_type", "id": "t1", "key": "k",
		"type_settings": {"layout": "Profile"}}`
	_, _, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown layout")
}

// the other layout-ish bundled keys hold different enums and must be untouched
func TestLayout_OnlyObjectLayoutKeysAreNamed(t *testing.T) {
	for _, key := range []string{"layoutAlign", "layoutWidth", "widgetLayout", "headerRelationsLayout"} {
		assert.False(t, isLayoutKey(key), "%s is not an ObjectTypeLayout", key)
	}
	for _, key := range []string{"recommendedLayout", "layout", "resolvedLayout"} {
		assert.True(t, isLayoutKey(key), "%s is an ObjectTypeLayout", key)
	}
}
