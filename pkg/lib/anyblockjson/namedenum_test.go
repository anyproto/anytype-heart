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
		require.NoError(t, Validate(data), "I1: Marshal never emits what its own Validate rejects")
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
		require.NoError(t, Validate(data), "I1: Marshal never emits what its own Validate rejects")
	})
}

// origin and import_type — the object's provenance, named on the format's
// own §2a precedent ("on ordinary objects origin is real provenance and
// stays"). The corpus carried them as the two largest bare-integer enums:
// origin on 15,943 documents spanning all TEN enum values, import_type on
// 8,303 — a reader saw `origin: 7` beside `resolved_layout: "dashboard"`
// with no way to learn that 7 meant anything.
func TestNamedEnum_Provenance(t *testing.T) {
	t.Run("export writes the names", func(t *testing.T) {
		snap := &model.SmartBlockSnapshotBase{
			Blocks: []*model.Block{{Id: "o1",
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
			Details: fields(map[string]*types.Value{
				"id":         str("o1"),
				"origin":     num(float64(model.ObjectOrigin_builtin)),
				"importType": num(float64(model.Import_Markdown)),
			}),
		}
		data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
		require.NoError(t, err)
		assert.Contains(t, string(data), `"origin": "builtin"`)
		assert.Contains(t, string(data), `"import_type": "markdown"`)
		require.NoError(t, Validate(data), "I1: Marshal never emits what its own Validate rejects")
	})

	t.Run("import maps the names to the stored numbers", func(t *testing.T) {
		doc := `{"version": 1, "id": "o1", "properties": {"origin": "webclipper", "import_type": "obsidian"}}`
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, float64(model.ObjectOrigin_webclipper), snap.Details.Fields["origin"].GetNumberValue())
		assert.Equal(t, float64(model.Import_Obsidian), snap.Details.Fields["importType"].GetNumberValue())
	})

	// The Notion-zero trap, pinned. This document used to be VALID: the
	// string "markdown" landed on the number-format detail, and every int
	// getter answered 0 — which for this enum is not "unset" but NOTION, a
	// false claim about where the object came from. The key is named now,
	// so a name is meaningful and a typo is an error.
	t.Run("markdown no longer reads as notion", func(t *testing.T) {
		doc := `{"version": 1, "id": "o1", "properties": {"import_type": "markdown"}}`
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, float64(model.Import_Markdown), snap.Details.Fields["importType"].GetNumberValue(),
			"the name means what it says, not the enum's zero")
	})

	t.Run("an unknown origin is refused, naming the vocabulary", func(t *testing.T) {
		doc := `{"version": 1, "id": "o1", "properties": {"origin": "clipbord"}}`
		err := Validate([]byte(doc))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/properties/origin")
		assert.Contains(t, err.Error(), "unknown origin")
		assert.Contains(t, err.Error(), "'clipboard'", "the refusal names the name that was nearly right")
		_, _, unmErr := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.Error(t, unmErr, "Unmarshal must reject what Validate rejects (§11 I2)")
	})

	t.Run("an unknown import type is refused too", func(t *testing.T) {
		err := Validate([]byte(`{"version": 1, "id": "o1", "properties": {"import_type": "md"}}`))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "unknown import type")
		assert.Contains(t, err.Error(), "'markdown'")
	})

	t.Run("raw numbers still round-trip", func(t *testing.T) {
		doc := `{"version": 1, "id": "o1", "properties": {"origin": 7, "import_type": 1}}`
		require.NoError(t, Validate([]byte(doc)), "every corpus document carries the pair this way")
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, float64(model.ObjectOrigin_builtin), snap.Details.Fields["origin"].GetNumberValue())
		assert.Equal(t, float64(model.Import_Markdown), snap.Details.Fields["importType"].GetNumberValue())
	})
}

// Both provenance vocabularies are TOTAL over their proto enums, the
// formatNames discipline: a member added to the proto without a name here
// would export as a bare integer again, which is the defect this file
// exists to keep closed.
func TestNamedEnum_VocabulariesTotalOverModelEnums(t *testing.T) {
	for raw, enumName := range model.ObjectOrigin_name {
		assert.NotEmpty(t, originNames.name(model.ObjectOrigin(raw)),
			"origin %s (%d) has no §3 name", enumName, raw)
	}
	for raw, enumName := range model.ImportType_name {
		assert.NotEmpty(t, importTypeNames.name(model.ImportType(raw)),
			"import type %s (%d) has no §3 name", enumName, raw)
	}
	for raw, enumName := range model.BlockAlign_name {
		assert.NotEmpty(t, alignNames.name(model.BlockAlign(raw)),
			"align %s (%d) has no §3 name", enumName, raw)
	}
	for raw, enumName := range model.ImageKind_name {
		assert.NotEmpty(t, imageKindNames.name(model.ImageKind(raw)),
			"image kind %s (%d) has no §3 name", enumName, raw)
	}
}

// A file object's `image_kind` says what an image was uploaded FOR. It used
// to travel as the proto's bare integer, so a reader of an export saw `3`
// beside a named `origin` and had no way to learn it meant the image was
// added by a pipeline rather than by a person — on 4,079 documents across
// the 77-space corpus, which is the measured standard the bare-integer keys
// beside it (widgetLayout at 13, headerRelationsLayout at 0) were left on.
//
// Naming it changes nothing a client depends on: the filter that hides
// auto-added images reads `isHiddenDiscovery`, which travels on its own and
// is in lockstep with this key's automatically_added member (4,053 of
// 4,053). This is a change to the READ surface.
//
// How this can fail: name it on the way out and not back in, and every
// import of an exported file object silently loses the kind; leave the enum
// ZERO out of the vocabulary and a future writer of Basic — the app skips
// storing it today — exports a bare 0 again.
func TestNamedEnum_ImageKind(t *testing.T) {
	t.Run("export writes the name", func(t *testing.T) {
		for kind, want := range map[model.ImageKind]string{
			model.ImageKind_AutomaticallyAdded: "automatically_added",
			model.ImageKind_Icon:               "icon",
			model.ImageKind_Cover:              "cover",
			model.ImageKind_Basic:              "basic",
		} {
			snap := &model.SmartBlockSnapshotBase{
				Details: fields(map[string]*types.Value{
					"id":        str("f1"),
					"imageKind": num(float64(kind)),
				}),
			}
			data, err := Marshal(model.SmartBlockType_FileObject, snap, Options{})
			require.NoError(t, err)
			assert.Contains(t, string(data), `"image_kind": "`+want+`"`,
				"the kind is spelled, not left as the proto integer")
			require.NoError(t, Validate(data), "I1: Marshal never emits what its own Validate rejects")
		}
	})

	t.Run("import maps the name to the stored number", func(t *testing.T) {
		doc := `{"version": 1, "id": "f1", "properties": {"image_kind": "automatically_added"}}`
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		v := snap.Details.Fields["imageKind"]
		require.NotNil(t, v)
		_, isNum := v.GetKind().(*types.Value_NumberValue)
		require.True(t, isNum, "must be stored as a number, not %T", v.GetKind())
		assert.Equal(t, float64(model.ImageKind_AutomaticallyAdded), v.GetNumberValue())
	})

	// closed vocabulary: a near-miss is refused by name rather than stored as
	// a stray string on a number detail, the accepted-then-zeroed failure
	// this whole file exists to prevent.
	t.Run("an unknown name is refused", func(t *testing.T) {
		doc := `{"version": 1, "id": "f1", "properties": {"image_kind": "Icon"}}`
		_, _, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.Error(t, err)
		assert.Contains(t, err.Error(), "image_kind")
		assert.Contains(t, err.Error(), "'icon'", "the refusal names the vocabulary")
	})
}
