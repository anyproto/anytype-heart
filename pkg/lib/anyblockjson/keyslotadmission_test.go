package anyblockjson

// keyslotadmission_test.go — a key slot has to name something, at every slot
// and through every door.
//
// Three slots enforced it and thirteen did not. `/properties`,
// `type_properties[].key` and `type_properties[].object_types[]` refused an
// empty key; the property block, a link block's `properties`, a dataview's
// `properties[].property` (spelled `key` then), `group_by`, `cover_property`, `end_property`,
// `columns[].property`, `sorts[].property`, `filters[].property`, the
// envelope `type` and `template_for` all took one — from a plain document, no
// vocabulary needed — and then LOST the slot on the way back out, in silence.
// A column and a sort vanish; a property block and a link's shown-property
// list come back nameless; a filter re-exports as a node that filters on
// nothing; and `"type": ""` costs the object its TYPE.
//
// Export had two matching leaks: a filter and a property block whose stored
// key was empty were written as nameless nodes, where the sort and the column
// beside them have always been dropped.

import (
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// keySlotCase is one slot, spelled twice: once with a real key, once with the
// empty one. The `named` half is the control — without it a fixture that is
// malformed for some unrelated reason refuses for the wrong reason and the
// test says nothing.
type keySlotCase struct {
	slot  string
	named string
	empty string
	path  string
}

func keySlotCases() []keySlotCase {
	dv := func(inner string) string {
		return `{"version":2,"id":"o1","blocks":[{"id":"b1","type":"dataview",` + inner + `}]}`
	}
	view := func(inner string) string {
		return dv(`"views":[{"id":"v1","type":"table",` + inner + `}]`)
	}
	return []keySlotCase{
		{"envelope type", `{"version":2,"id":"o1","type":"page"}`,
			`{"version":2,"id":"o1","type":""}`, "/type"},
		{"template_for",
			`{"version":2,"kind":"template","id":"o1","type":"template","template_for":"page"}`,
			`{"version":2,"kind":"template","id":"o1","type":"template","template_for":""}`,
			"/template_for"},
		{"type_properties key",
			`{"version":2,"kind":"object_type","id":"o1","type":"object_type","type_settings":{"property_definitions": [{"property":"prio","format":"text"}]}}`,
			`{"version":2,"kind":"object_type","id":"o1","type":"object_type","type_settings":{"property_definitions": [{"property":"","format":"text"}]}}`,
			"/type_settings/property_definitions/0/property"},
		{"type_properties object_types",
			`{"version":2,"kind":"object_type","id":"o1","type":"object_type","type_settings":{"property_definitions": [{"property":"who","format":"objects","object_types":["page"]}]}}`,
			`{"version":2,"kind":"object_type","id":"o1","type":"object_type","type_settings":{"property_definitions": [{"property":"who","format":"objects","object_types":[""]}]}}`,
			"/type_settings/property_definitions/0/object_types/0"},
		{"relation object_types",
			`{"version":2,"kind":"property","id":"o1","internal_key":"who","property_settings":{"format":"objects","object_types":["page"]}}`,
			`{"version":2,"kind":"property","id":"o1","internal_key":"who","property_settings":{"format":"objects","object_types":[""]}}`,
			"/property_settings/object_types/0"},
		{"properties member", `{"version":2,"id":"o1","properties":{"prio":"x"}}`,
			`{"version":2,"id":"o1","properties":{"":"x"}}`, "/properties/"},
		{"property block key",
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"property","property":"prio"}]}`,
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"property","property":""}]}`,
			"/blocks/0/property"},
		{"link block properties",
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"link","object_id":"t1","properties":["prio"]}]}`,
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"link","object_id":"t1","properties":[""]}]}`,
			"/blocks/0/properties/0"},
		{"dataview properties key",
			dv(`"properties":[{"property":"prio","format":"text"}],"views":[{"id":"v1","type":"table"}]`),
			dv(`"properties":[{"property":"","format":"text"}],"views":[{"id":"v1","type":"table"}]`),
			"/blocks/0/properties/0/property"},
		{"view group_by",
			dv(`"views":[{"id":"v1","type":"kanban","group_by":"tag"}]`),
			dv(`"views":[{"id":"v1","type":"kanban","group_by":""}]`),
			"/blocks/0/views/0/group_by"},
		{"view cover_property",
			dv(`"views":[{"id":"v1","type":"gallery","cover_property":"prio"}]`),
			dv(`"views":[{"id":"v1","type":"gallery","cover_property":""}]`),
			"/blocks/0/views/0/cover_property"},
		{"view end_property",
			dv(`"views":[{"id":"v1","type":"calendar","end_property":"prio"}]`),
			dv(`"views":[{"id":"v1","type":"calendar","end_property":""}]`),
			"/blocks/0/views/0/end_property"},
		{"view column property", view(`"columns":[{"property":"prio"}]`),
			view(`"columns":[{"property":""}]`), "/blocks/0/views/0/columns/0/property"},
		{"sort property", view(`"sorts":[{"property":"prio","direction":"asc"}]`),
			view(`"sorts":[{"property":"","direction":"asc"}]`), "/blocks/0/views/0/sorts/0/property"},
		{"filter property", view(`"filters":[{"property":"prio","condition":"equal","value":"x"}]`),
			view(`"filters":[{"property":"","condition":"equal","value":"x"}]`),
			"/blocks/0/views/0/filters/0/property"},
		{"nested filter property",
			view(`"filters":[{"operator":"or","filters":[{"property":"prio","condition":"equal","value":"x"}]}]`),
			view(`"filters":[{"operator":"or","filters":[{"property":"","condition":"equal","value":"x"}]}]`),
			"/blocks/0/views/0/filters/0/filters/0/property"},
	}
}

// The document door: every key slot refuses the empty spelling, and the same
// document with a real spelling is accepted — so the refusal is about the
// key, not about the fixture.
func TestValidate_EveryKeySlotRefusesTheEmptySpelling(t *testing.T) {
	for _, tc := range keySlotCases() {
		t.Run(tc.slot, func(t *testing.T) {
			require.NoError(t, Validate([]byte(tc.named)),
				"the control must be a document this format accepts:\n%s", tc.named)

			err := Validate([]byte(tc.empty))
			require.Error(t, err, "accepted an empty key at %s:\n%s", tc.slot, tc.empty)
			assert.Contains(t, issuePaths(t, err), tc.path,
				"the refusal has to name the slot it is about (§12): %v", err)

			// and Unmarshal agrees with Validate (§12): what one refuses the
			// other refuses, which is the half that used to fail — the
			// document imported clean and the slot was simply gone
			_, _, err = Unmarshal([]byte(tc.empty), Options{GenerateId: seqIds("g")})
			assert.Error(t, err, "Unmarshal accepted what Validate refuses")
		})
	}
}

// A filter with no `property` member at all — the shape export used to write
// for a view whose relation was deleted. It carried the same meaning as the
// empty spelling and had the same silence.
func TestValidate_AFilterHasToNameItsProperty(t *testing.T) {
	doc := `{"version":2,"id":"o1","blocks":[{"id":"b1","type":"dataview","views":[` +
		`{"id":"v1","type":"table","filters":[{"condition":"equal","value":"x"}]}]}]}`

	err := Validate([]byte(doc))
	require.Error(t, err, "a filter that filters on nothing:\n%s", doc)
	var ve *ValidationError
	require.ErrorAs(t, err, &ve)
	assert.Equal(t, "/blocks/0/views/0/filters/0", ve.Issues[0].Path,
		"the FIRST issue is the one an agent acts on (§12): %v", err)
	assert.Contains(t, ve.Issues[0].Message, "a filter has to name the property it filters on")

	_, _, err = Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	assert.Error(t, err)
}

// The resolution door, which needs no document fault at all: a vocabulary
// answering ("", true) — accepted from anyone (§3) — used to land the empty
// key in the model at nine of these slots.
func TestUnmarshal_EveryKeySlotRefusesAnEmptyResolution(t *testing.T) {
	// `want` is the substring the refusal has to carry. The four slots that
	// already refused name the SLOT (their pointer is exact); the ten this
	// change reaches name the SPELLING, because the fault is the reader's
	// vocabulary and their pointer can only be the coarse `/blocks`. The four
	// are here to state the uniformity, not because they are load-bearing —
	// reverting the change leaves them green and fails the other ten.
	for _, tc := range []struct{ slot, doc, want string }{
		{"envelope type", `{"version":2,"id":"o1","type":"prio"}`,
			"/type: resolved type key is empty"},
		{"template_for",
			`{"version":2,"kind":"template","id":"o1","type":"template","template_for":"prio"}`,
			"/template_for: resolved type key is empty"},
		{"type_properties key",
			`{"version":2,"kind":"object_type","id":"o1","type":"object_type","type_settings":{"property_definitions": [{"property":"prio","format":"text"}]}}`,
			"/type_settings/property_definitions/0/property: resolved property key is empty"},
		{"type_properties object_types",
			`{"version":2,"kind":"object_type","id":"o1","type":"object_type","type_settings":{"property_definitions": [{"property":"who","format":"objects","object_types":["prio"]}]}}`,
			"/type_settings/property_definitions/0/object_types/0: resolved type key is empty"},
		{"relation object_types",
			`{"version":2,"kind":"property","id":"o1","internal_key":"who","property_settings":{"format":"objects","object_types":["prio"]}}`,
			"/property_settings/object_types/0: resolved type key is empty"},
		{"properties member", `{"version":2,"id":"o1","properties":{"prio":"x"}}`,
			"/properties/prio: resolved property key is empty"},
		{"property block key",
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"property","property":"prio"}]}`,
			"the property block `property` spelling \"prio\""},
		{"link block properties",
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"link","object_id":"t1","properties":["prio"]}]}`,
			"the link block `properties` spelling \"prio\""},
		{"dataview properties key",
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"dataview","properties":[{"property":"prio","format":"text"}],"views":[{"id":"v1","type":"table"}]}]}`,
			"the dataview `properties` spelling \"prio\""},
		{"view group_by",
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"dataview","views":[{"id":"v1","type":"kanban","group_by":"prio"}]}]}`,
			"the view `group_by` spelling \"prio\""},
		{"view cover_property",
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"dataview","views":[{"id":"v1","type":"gallery","cover_property":"prio"}]}]}`,
			"the view `cover_property` spelling \"prio\""},
		{"view end_property",
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"dataview","views":[{"id":"v1","type":"calendar","end_property":"prio"}]}]}`,
			"the view `end_property` spelling \"prio\""},
		{"view column property",
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"dataview","views":[{"id":"v1","type":"table","columns":[{"property":"prio"}]}]}]}`,
			"the view column `property` spelling \"prio\""},
		{"sort property",
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"dataview","views":[{"id":"v1","type":"table","sorts":[{"property":"prio","direction":"asc"}]}]}]}`,
			"the sort `property` spelling \"prio\""},
		{"filter property",
			`{"version":2,"id":"o1","blocks":[{"id":"b1","type":"dataview","views":[{"id":"v1","type":"table","filters":[{"property":"prio","condition":"equal","value":"x"}]}]}]}`,
			"the filter `property` spelling \"prio\""},
	} {
		t.Run(tc.slot, func(t *testing.T) {
			// the document itself is fine — Validate, which takes no
			// vocabulary, accepts it. The fault is entirely in the reader.
			require.NoError(t, Validate([]byte(tc.doc)), "%s", tc.doc)

			_, snap, err := Unmarshal([]byte(tc.doc),
				Options{GenerateId: seqIds("g"), Keys: emptyAnswerVocabulary{}})
			require.Error(t, err,
				"the empty key landed in the model at %s; the slot is lost on the way back out", tc.slot)
			assert.Nil(t, snap, "a refused document hands back no object")
			assert.Contains(t, err.Error(), "a key slot has to name something")
			assert.Contains(t, err.Error(), tc.want,
				"the refusal has to say which slot, or which spelling, it is about")
		})
	}
}

// emptyAnswerVocabulary answers "this spelling is a slug for the relation ”"
// — the shape a buggy or half-built resolver produces, and one nothing in
// KeyVocabulary's preconditions forbids.
type emptyAnswerVocabulary struct{ BundledKeyVocabulary }

func (emptyAnswerVocabulary) PropertyKey(slug string) (string, bool) {
	if slug == "prio" {
		return "", true
	}
	return BundledKeyVocabulary{}.PropertyKey(slug)
}

func (emptyAnswerVocabulary) TypeKey(slug string) (string, bool) {
	if slug == "prio" {
		return "", true
	}
	return BundledKeyVocabulary{}.TypeKey(slug)
}

// The export door. A stored relation key can be empty — that is real data —
// and export wrote a filter and a property block that named nothing, which
// the rule above now refuses. Both are dropped, with a warning, exactly as
// the sort and the column beside them already were.
func TestExport_ANamelessFilterAndPropertyBlockAreDropped(t *testing.T) {
	// given — one dataview view holding a named filter and a nameless one,
	// plus a nameless property block. The named filter is the control: the
	// view must survive, so this cannot pass by dropping everything.
	dv := &model.BlockContentDataview{
		RelationLinks: []*model.RelationLink{{Key: "tag", Format: model.RelationFormat_tag}},
		Views: []*model.BlockContentDataviewView{{
			Id: "v1", Name: "All", Type: model.BlockContentDataviewView_Table,
			Filters: []*model.BlockContentDataviewFilter{
				{Id: "f1", RelationKey: "tag",
					Condition: model.BlockContentDataviewFilter_Equal, Value: str("x")},
				{Id: "f2", RelationKey: "",
					Condition: model.BlockContentDataviewFilter_Equal, Value: str("y")},
			},
		}},
	}
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "o1", ChildrenIds: []string{"dv1", "rel1", "rel2"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "dv1", Content: &model.BlockContentOfDataview{Dataview: dv}},
			{Id: "rel1", Content: &model.BlockContentOfRelation{
				Relation: &model.BlockContentRelation{Key: "tag"}}},
			{Id: "rel2", Content: &model.BlockContentOfRelation{
				Relation: &model.BlockContentRelation{Key: ""}}},
		},
		Details: fields(map[string]*types.Value{"id": str("o1"), "name": str("Board")}),
	}
	var warned []Issue

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap,
		Options{OnWarning: func(i Issue) { warned = append(warned, i) }})

	// then — I1: what Marshal writes, its own Validate and Unmarshal accept
	require.NoError(t, err)
	require.NoError(t, Validate(data), "emitted:\n%s", data)
	_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err, "emitted:\n%s", data)

	// the named ones survive, the nameless ones are gone and said so
	view := backView(t, back)
	require.Len(t, view.Filters, 1, "emitted:\n%s", data)
	assert.Equal(t, "tag", view.Filters[0].RelationKey)
	var blockKeys []string
	for _, b := range back.Blocks {
		if c, ok := b.Content.(*model.BlockContentOfRelation); ok {
			blockKeys = append(blockKeys, c.Relation.Key)
		}
	}
	assert.Equal(t, []string{"tag"}, blockKeys)
	joined := warningsAt(warned, "")
	assert.Contains(t, joined, "a filter names no property and is dropped")
	assert.Contains(t, joined, "a property block names no property and is dropped")
}

// issuePaths lists the pointers a ValidationError names.
func issuePaths(t *testing.T, err error) []string {
	t.Helper()
	var ve *ValidationError
	require.ErrorAs(t, err, &ve)
	out := make([]string, 0, len(ve.Issues))
	for _, i := range ve.Issues {
		out = append(out, i.Path)
	}
	return out
}
