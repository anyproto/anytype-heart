package anyblockjson

// Regression tests for the confirmed findings of the review pass.

import (
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// Finding 1: a same-param merge that extends an accepted range must re-run
// overlap resolution, or a same-type different-param overlap survives and
// the second export resolves it differently (byte-stability break).
func TestInline_MergeExtensionReresolvesOverlaps(t *testing.T) {
	text := strings.Repeat("abcde", 6) // 30 chars
	marks := []*model.BlockContentTextMark{
		mark(mLink, 0, 5, "http://p1"),
		mark(mLink, 3, 30, "http://p2"),
		mark(mLink, 4, 20, "http://p1"),
	}
	md1 := renderInline(text, marks)
	text1, marks1, err := parseInline(md1)
	require.NoError(t, err)
	md2 := renderInline(text1, marks1)
	require.Equal(t, md1, md2, "must be byte-stable")
	// the resolved decomposition: p1 wins [0,20), p2 truncated to [20,30)
	assert.Equal(t, "[abcdeabcdeabcdeabcde](http://p1)[abcdeabcde](http://p2)", md1)
}

// Finding 5: a bare link destination starting with '<' must not be misread
// as the angle-wrapped form on re-parse.
func TestInline_DestLeadingAngle(t *testing.T) {
	marks := []*model.BlockContentTextMark{mark(mLink, 0, 2, "<x>y")}
	md := renderInline("ab", marks)
	text, parsed, err := parseInline(md)
	require.NoError(t, err)
	assert.Equal(t, "ab", text)
	require.Len(t, parsed, 1)
	assert.Equal(t, "<x>y", parsed[0].Param)
	assert.Equal(t, md, renderInline(text, parsed))
}

// Finding 9: brackets/backticks inside tag attribute values must be
// entity-encoded, or the link-label scan derails when the tag sits inside a
// link label.
func TestInline_BracketInAttrInsideLabel(t *testing.T) {
	marks := []*model.BlockContentTextMark{
		mark(mLink, 0, 2, "http://u"),
		mark(mColor, 0, 2, "a]b"),
	}
	md := renderInline("hi", marks)
	text, parsed, err := parseInline(md)
	require.NoError(t, err)
	assert.Equal(t, "hi", text)
	require.Len(t, parsed, 2)
	assert.Equal(t, "http://u", parsed[0].Param)
	assert.Equal(t, "a]b", parsed[1].Param)
	assert.Equal(t, md, renderInline(text, parsed))
}

// Finding 2: verbatim property passthrough (structs, nulls inside lists)
// must produce schema-valid canonical output and round-trip.
func TestExport_VerbatimPropertyShapes(t *testing.T) {
	structVal := &types.Value{Kind: &types.Value_StructValue{StructValue: fields(map[string]*types.Value{
		"a": num(1),
	})}}
	listWithNull := &types.Value{Kind: &types.Value_ListValue{ListValue: &types.ListValue{Values: []*types.Value{
		str("x"), {Kind: &types.Value_NullValue{}},
	}}}}
	snap := &model.SmartBlockSnapshotBase{
		Details: fields(map[string]*types.Value{
			"id":          str("obj1"),
			"customWeird": structVal,
			"customList":  listWithNull,
		}),
	}
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
	require.NoError(t, err)
	require.NoError(t, Validate(data), "canonical export must pass its own schema")
	_, snap2, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	second, err := Marshal(model.SmartBlockType_Page, snap2, Options{})
	require.NoError(t, err)
	assert.Equal(t, string(data), string(second))
}

// Finding 3: a sort with an empty property key is dropped instead of
// emitting a document that fails the schema's required "property"; an empty
// filter group is a no-op and is dropped too.
func TestExport_EmptyKeySortSkipped(t *testing.T) {
	snap := &model.SmartBlockSnapshotBase{
		Details: fields(map[string]*types.Value{"id": str("obj1")}),
		Blocks: []*model.Block{
			{Id: "obj1", ChildrenIds: []string{"dv"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "dv", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{{
					Id:    "v1",
					Sorts: []*model.BlockContentDataviewSort{{RelationKey: ""}},
					Filters: []*model.BlockContentDataviewFilter{
						{Operator: model.BlockContentDataviewFilter_And},
					},
				}},
			}}},
		},
	}
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	assert.NotContains(t, string(data), `"sorts"`)
	assert.NotContains(t, string(data), `"filters"`)
}

// Finding 4: nil inner content messages are proto-equivalent to empty ones
// and must not panic Marshal.
func TestExport_NilInnerContent(t *testing.T) {
	contents := []model.IsBlockContent{
		&model.BlockContentOfText{},
		&model.BlockContentOfFile{},
		&model.BlockContentOfBookmark{},
		&model.BlockContentOfLink{},
		&model.BlockContentOfDiv{},
		&model.BlockContentOfLayout{},
		&model.BlockContentOfLatex{},
		&model.BlockContentOfRelation{},
		&model.BlockContentOfDataview{},
		&model.BlockContentOfWidget{},
		&model.BlockContentOfIcon{},
		&model.BlockContentOfTableRow{},
	}
	for _, c := range contents {
		snap := &model.SmartBlockSnapshotBase{
			Details: fields(map[string]*types.Value{"id": str("obj1")}),
			Blocks: []*model.Block{
				{Id: "obj1", ChildrenIds: []string{"b1"},
					Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
				{Id: "b1", Content: c},
			},
		}
		require.NotPanics(t, func() {
			_, _ = Marshal(model.SmartBlockType_Page, snap, Options{})
		}, "content %T", c)
	}
}

// Finding 6, revisited under raw names: a Page whose type key is `template`
// once needed an explicit kind, because the type SPELLED itself `template` —
// the byte shape that meant a template before `kind` existed. The type spells
// its display name "Template" now, which is not that byte shape, so the kind
// is derivable again and stays omitted — and the round trip must still come
// home a Page. The emission guard survives for the one spelling that still
// needs it: a writer whose vocabulary yields the raw term `template` (a
// package-only export of a custom type stored-keyed so, or a vocabulary that
// answers the key itself), which the second arm pins.
func TestExport_PageWithTemplateTypeKeepsKind(t *testing.T) {
	snap := &model.SmartBlockSnapshotBase{
		Details:     fields(map[string]*types.Value{"id": str("obj1")}),
		ObjectTypes: []string{"ot-template"},
	}

	t.Run("the display-name spelling derives its kind", func(t *testing.T) {
		data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
		require.NoError(t, err)
		assert.Contains(t, string(data), `"type": "Template"`)
		assert.NotContains(t, string(data), `"kind"`, "Page is derivable — the term is not the legacy byte shape")
		require.NoError(t, Validate(data), "I1")
		sbType, _, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, model.SmartBlockType_Page, sbType)
	})

	t.Run("the raw term `template` still forces the kind", func(t *testing.T) {
		data, err := Marshal(model.SmartBlockType_Page, snap, Options{Keys: verbatimKeys{}})
		require.NoError(t, err)
		assert.Contains(t, string(data), `"type": "template"`)
		assert.Contains(t, string(data), `"kind": "page"`,
			"a kindless type:template document is the shape that used to mean a template (export.go's emission rule)")
		require.NoError(t, Validate(data), "I1")
		sbType, _, err := Unmarshal(data, Options{GenerateId: seqIds("g"), Keys: verbatimKeys{}})
		require.NoError(t, err)
		assert.Equal(t, model.SmartBlockType_Page, sbType)
	})
}

// verbatimKeys is the minimum conforming vocabulary: every key spells
// itself.
type verbatimKeys struct{}

func (verbatimKeys) PropertySlug(key string) string      { return key }
func (verbatimKeys) PropertyKey(s string) (string, bool) { return s, false }
func (verbatimKeys) TypeSlug(key string) string          { return key }
func (verbatimKeys) TypeKey(s string) (string, bool)     { return s, false }

// Finding 7: a stray properties.id / properties.type in the document must not
// clobber the envelope-lifted details. It used to be dropped in silence, which
// left an author wondering why the id they wrote had no effect; since the
// pre-freeze pass on property-key admission (Tier 1 #5) it is refused by name,
// and the envelope stays the only place either one is set.
func TestImport_PropertiesIdDoesNotLeak(t *testing.T) {
	doc := `{"version": 2, "id": "realid", "properties": {"id": "fakeid", "type": "faketype", "name": "N"}}`
	err := Validate([]byte(doc))
	require.Error(t, err)
	assert.Contains(t, err.Error(), `/properties/id: "id" belongs in the envelope`)
	assert.Contains(t, err.Error(), `/properties/type: "type" belongs in the envelope`)

	_, _, err = Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.Error(t, err)

	// and the envelope keeps working on its own
	_, snap, err := Unmarshal([]byte(`{"version": 2, "id": "realid", "properties": {"name": "N"}}`),
		Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, "realid", snap.Details.Fields["id"].GetStringValue())
	assert.Nil(t, snap.Details.Fields["type"])
	assert.Equal(t, "N", snap.Details.Fields["name"].GetStringValue())
}

// Finding 8: tables nested inside table cells join the id-uniqueness domain
// and get their inline text checked.
// A table inside a table cell is rejected at the schema level: cells use the
// non-recursive cellBlock definition — the guarantee that keeps the whole
// schema free of block recursion (§12). Cell arrays get the same treatment,
// and their inline text still reaches the markup checks.
func TestValidate_NestedTableInCell(t *testing.T) {
	nestedTable := `{"version": 2, "blocks": [
		{"type": "table", "columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": [
			{"type": "table", "columns": [{"id": "c2"}], "rows": []}
		]}]}
	]}`
	err := Validate([]byte(nestedTable))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/blocks/0/rows/0/cells/0")

	dupIdInCellArray := `{"version": 2, "blocks": [
		{"id": "x1", "type": "paragraph"},
		{"type": "table", "columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": [[
			{"type": "toggle", "text": "cell"},
			{"indent": 1, "id": "x1", "type": "paragraph", "text": "dup"}
		]]}]}
	]}`
	err = Validate([]byte(dupIdInCellArray))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "duplicate id")

	badInline := `{"version": 2, "blocks": [
		{"type": "table", "columns": [{"id": "c1"}], "rows": [{"id": "r1", "cells": [[
			{"type": "toggle", "text": "cell"},
			{"indent": 1, "type": "paragraph", "text": "<u>unclosed"}
		]]}]}
	]}`
	err = Validate([]byte(badInline))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "inline markup")
}

// Finding 11: a non-list "objects" value in the internal store stays in
// store instead of being dropped.
func TestExport_NonListObjectsStaysInStore(t *testing.T) {
	snap := &model.SmartBlockSnapshotBase{
		Details:     fields(map[string]*types.Value{"id": str("obj1")}),
		Collections: fields(map[string]*types.Value{"objects": str("notalist")}),
	}
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
	require.NoError(t, err)
	s := string(data)
	assert.NotContains(t, s, `"items"`)
	assert.Contains(t, s, `"objects": "notalist"`)
	_, snap2, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, "notalist", snap2.Collections.Fields["objects"].GetStringValue())
}

// Finding 12: integer-valued float versions are accepted (JSON Schema
// numeric equality).
func TestValidate_FloatVersion(t *testing.T) {
	require.NoError(t, Validate([]byte(`{"version": 2.0}`)))
	require.Error(t, Validate([]byte(`{"version": 2.5}`)))
}

// Out-of-range enum values are omitted (or error for the type discriminator)
// instead of emitting schema-invalid empty strings.
func TestExport_OutOfRangeEnums(t *testing.T) {
	snap := &model.SmartBlockSnapshotBase{
		Details: fields(map[string]*types.Value{"id": str("obj1")}),
		Blocks: []*model.Block{
			{Id: "obj1", ChildrenIds: []string{"dv"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "dv", Content: &model.BlockContentOfDataview{Dataview: &model.BlockContentDataview{
				Views: []*model.BlockContentDataviewView{{
					Id:   "v1",
					Type: model.BlockContentDataviewViewType(99),
					Filters: []*model.BlockContentDataviewFilter{{
						RelationKey: "k",
						Condition:   model.BlockContentDataviewFilterCondition(99),
						QuickOption: model.BlockContentDataviewFilterQuickOption(99),
					}},
				}},
			}}},
		},
	}
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{})
	require.NoError(t, err)
	require.NoError(t, Validate(data), "out-of-range enums must not produce invalid output")

	// unknown text styles are an export error, not silent mangling
	snap2 := &model.SmartBlockSnapshotBase{
		Details: fields(map[string]*types.Value{"id": str("obj1")}),
		Blocks: []*model.Block{
			{Id: "obj1", ChildrenIds: []string{"t"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "t", Content: &model.BlockContentOfText{Text: &model.BlockContentText{
				Style: model.BlockContentTextStyle(99), Text: "x",
			}}},
		},
	}
	_, err = Marshal(model.SmartBlockType_Page, snap2, Options{})
	require.Error(t, err)
}
