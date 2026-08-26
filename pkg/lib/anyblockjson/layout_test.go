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
			doc := `{"version": 1, "kind": "object_type", "id": "t1", "internal_key": "k",
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
	doc := `{"version": 1, "kind": "object_type", "id": "t1", "internal_key": "k",
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
	doc := `{"version": 1, "kind": "object_type", "id": "t1", "internal_key": "k",
		"type_settings": {"layout": "profile"}}`
	_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	data, err := Marshal(model.SmartBlockType_STType, snap, testOptions())
	require.NoError(t, err)
	assert.Contains(t, string(data), `"layout": "profile"`)
}

// a typo must not reach the snapshot as a bare string.
//
// The SCHEMA answers this now, not the semantic pass: `type_settings.layout`
// used to be `{"type": ["string","number"]}`, which meant a generator reading
// the published schema could emit any string it liked and only learn at the
// codec that the vocabulary is closed. The schema states the vocabulary, so
// the refusal arrives with the whole list — which the semantic message it
// replaced never carried.
func TestValidate_UnknownLayoutRejected(t *testing.T) {
	doc := `{"version": 1, "kind": "object_type", "id": "t1", "internal_key": "k",
		"type_settings": {"layout": "Profile"}}`
	_, _, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/type_settings/layout")
	assert.Contains(t, err.Error(), "'basic'", "the refusal names the vocabulary")
	assert.Contains(t, err.Error(), "'profile'", "including the name that was nearly right")
}

// Which stored keys are written by NAME is a per-key verdict (§3), and this
// pins each one together with the vocabulary it draws from — a key added to
// namedEnumProperties must state its concept here, and a key deliberately
// left as a number must stay listed as such. (This replaces the old
// isLayoutKey membership pin: the layout keys were the only named ones until
// the mechanism became a per-key table.)
func TestNamedEnumProperties_PerKeyVerdict(t *testing.T) {
	want := map[string]string{ // stored key → the concept its refusals name
		"recommendedLayout": "layout",
		"layout":            "layout",
		"resolvedLayout":    "layout",
		// the object's own page alignment: user-settable (readonly false),
		// stored as a model.BlockAlign — the enum the format already names
		// twice, on a block's align and a view column's align
		"layoutAlign": "align",
		// the object's provenance, named on the format's own §2a precedent:
		// "on ordinary objects origin is real provenance and stays" — the
		// class of createdDate and creator, not of syncStatus. importType
		// rides with it (objectorigin.go writes them as a pair).
		"origin":     "origin",
		"importType": "import type",
		// what an image was uploaded FOR, on file objects. Named on the
		// measured standard the bare-integer keys beside it were left on:
		// 4,079 occurrences against widgetLayout's 13 and
		// headerRelationsLayout's 0. Its automatically_added member is in
		// lockstep with is_hidden_discovery (4,053 of 4,053), which is the
		// key a client actually filters on — so this one is named for the
		// READER rather than for any behaviour that depends on it.
		"imageKind": "image kind",
	}
	assert.Equal(t, len(want), len(namedEnumProperties),
		"every named key owes a verdict here — a new one must say which vocabulary it draws from")
	for key, what := range want {
		vocab, named := namedEnumProperty(key)
		require.True(t, named, "%s must be written by name", key)
		assert.Equal(t, what, vocab.what, "%s draws from the wrong vocabulary", key)
	}
	// the layout-ish bundled keys that stay numbers, each for a stated
	// reason: layoutWidth is a fraction, not an enum; widgetLayout and
	// headerRelationsLayout hold enums nothing measurable writes (13 and 0
	// occurrences across 28,604 real exported documents)
	for _, key := range []string{"layoutWidth", "widgetLayout", "headerRelationsLayout"} {
		_, named := namedEnumProperty(key)
		assert.False(t, named, "%s is deliberately not named", key)
	}
}
