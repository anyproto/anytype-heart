package anyblockjson

// keyslotbounds_test.go — the writable-key rule (§3: non-empty,
// control-character-free, inside the 128-character bound) at every PROPERTY
// key slot outside /properties, through all three doors.
//
// $defs/propertyDefinition carried the bound and the pattern all along; the
// sibling slots — a dataview's properties[], a view's
// group_by/cover_property/end_property, columns, sorts, filters, a link
// block's properties[], the property block — carried only minLength, so a
// million-character key and raw NUL/CR/ESC bytes validated clean, imported
// clean, and persisted into RelationLink.Key. Export emitted such stored
// keys verbatim into the same slots, so the schema half and the export half
// land in one change: a schema-only bound made Marshal emit what its own
// Validate rejects (§11, I1) — that was tried and reverted.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// hostileKeys are stored-key strings no spelling can carry. Built from runes
// so no source literal has to hold a raw control byte.
func hostileKeys() map[string]string {
	return map[string]string{
		"an over-long key":        strings.Repeat("k", maxPropertyKeyLen+1),
		"a control-character key": "bad" + string(rune(0x07)) + "key",
		"a NUL-and-ESC key":       "bad" + string(rune(0x00)) + "key" + string(rune(0x1b)),
		// DEL slips past the schema's pattern (its lower-plane character
		// class stops at 0x1f, matching $defs/propertyDefinition) — the
		// restated writable-key rule is what has to catch it, exactly as it
		// does for /properties and the definition slots
		"a DEL-carrying key": "bad" + string(rune(0x7f)) + "key",
	}
}

// The document door: every property-key slot refuses an over-long and a
// control-character spelling, path-addressed, and Unmarshal agrees.
func TestValidate_EveryKeySlotBoundsTheSpelling(t *testing.T) {
	for _, tc := range keySlotCases() {
		switch tc.slot {
		case "envelope type", "template_for", "type_properties object_types", "relation object_types":
			continue // type-key slots: any non-empty stored key round-trips (§3)
		case "properties member", "type_properties key":
			continue // bounded before this change; the seven sibling slots are the point
		}
		for name, hostile := range hostileKeys() {
			t.Run(tc.slot+" refuses "+name, func(t *testing.T) {
				// given — the good fixture with its key swapped for the
				// hostile one, JSON-escaped the way a document would carry it
				enc, err := json.Marshal(hostile)
				require.NoError(t, err)
				doc := strings.ReplaceAll(tc.named, `"prio"`, string(enc))
				doc = strings.ReplaceAll(doc, `"tag"`, string(enc))
				require.NotEqual(t, tc.named, doc,
					"the fixture must actually carry the hostile key")
				require.NoError(t, Validate([]byte(tc.named)),
					"the control must be a document this format accepts:\n%s", tc.named)

				// when
				err = Validate([]byte(doc))

				// then
				require.Error(t, err, "accepted %s at %s", name, tc.slot)
				assert.Contains(t, issuePaths(t, err), tc.path,
					"the refusal has to name the slot it is about (§12): %v", err)

				// and Unmarshal agrees with Validate (§12)
				_, _, err = Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
				assert.Error(t, err, "Unmarshal accepted what Validate refuses")
			})
		}
	}
}

// The resolution door: a vocabulary resolving a legal spelling onto an
// unwritable stored key used to land it in RelationLink.Key at the block
// slots, where /properties has refused it all along.
func TestUnmarshal_KeySlotsRefuseAnUnwritableResolution(t *testing.T) {
	// given
	doc := `{"version":2,"id":"o1","blocks":[{"id":"b1","type":"dataview",` +
		`"views":[{"id":"v1","type":"table","sorts":[{"property":"prio","direction":"asc"}]}]}]}`
	require.NoError(t, Validate([]byte(doc)),
		"the document itself is fine; the fault is the reader's vocabulary")

	// when
	_, snap, err := Unmarshal([]byte(doc),
		Options{GenerateId: seqIds("g"), Keys: unwritableAnswerVocabulary{}})

	// then
	require.Error(t, err, "the unwritable key landed in the model; export can only drop it later")
	assert.Nil(t, snap, "a refused document hands back no object")
	assert.Contains(t, err.Error(), "carries a control character")
}

// unwritableAnswerVocabulary resolves the spelling "prio" onto a stored key
// carrying a control byte — a shape nothing in KeyVocabulary's preconditions
// forbids.
type unwritableAnswerVocabulary struct{ BundledKeyVocabulary }

func (unwritableAnswerVocabulary) PropertyKey(slug string) (string, bool) {
	if slug == "prio" {
		return "bad" + string(rune(0x00)) + "key", true
	}
	return BundledKeyVocabulary{}.PropertyKey(slug)
}

// The export door: a stored key no spelling can carry is dropped from every
// reference slot with a warning, the way the empty key already is, instead
// of being emitted verbatim into a document Marshal's own Validate rejects.
func TestExport_UnwritableKeysAreDroppedFromEveryReferenceSlot(t *testing.T) {
	// given — every slot holds one good key (the control: the view must
	// survive, so this cannot pass by dropping everything) and one hostile
	hostile := "bad" + string(rune(0x00)) + "key" + string(rune(0x1b))
	overlong := strings.Repeat("k", maxPropertyKeyLen+1)
	dv := &model.BlockContentDataview{
		RelationLinks: []*model.RelationLink{
			{Key: "tag", Format: model.RelationFormat_tag},
			{Key: hostile, Format: model.RelationFormat_longtext},
			{Key: overlong, Format: model.RelationFormat_longtext},
		},
		Views: []*model.BlockContentDataviewView{{
			Id: "v1", Name: "All", Type: model.BlockContentDataviewView_Kanban,
			GroupRelationKey: hostile,
			CoverRelationKey: overlong,
			EndRelationKey:   hostile,
			Sorts: []*model.BlockContentDataviewSort{
				{Id: "s1", RelationKey: "tag"},
				{Id: "s2", RelationKey: hostile},
			},
			Filters: []*model.BlockContentDataviewFilter{
				{Id: "f1", RelationKey: "tag",
					Condition: model.BlockContentDataviewFilter_Equal, Value: str("x")},
				{Id: "f2", RelationKey: overlong,
					Condition: model.BlockContentDataviewFilter_Equal, Value: str("y")},
			},
			Relations: []*model.BlockContentDataviewRelation{
				{Key: "tag"},
				{Key: hostile},
			},
		}},
	}
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "o1", ChildrenIds: []string{"dv1", "l1", "rel1"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "dv1", Content: &model.BlockContentOfDataview{Dataview: dv}},
			{Id: "l1", Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
				TargetBlockId: "t1", Relations: []string{"tag", hostile}}}},
			{Id: "rel1", Content: &model.BlockContentOfRelation{
				Relation: &model.BlockContentRelation{Key: hostile}}},
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
	assert.NotContains(t, string(data), overlong,
		"the over-long key must not be emitted anywhere")

	// the named slots survive; the unwritable ones are gone and said so
	view := backView(t, back)
	require.Len(t, view.Sorts, 1, "emitted:\n%s", data)
	assert.Equal(t, "tag", view.Sorts[0].RelationKey)
	require.Len(t, view.Filters, 1)
	assert.Equal(t, "tag", view.Filters[0].RelationKey)
	require.Len(t, view.Relations, 1)
	assert.Equal(t, "tag", view.Relations[0].Key)
	assert.Empty(t, view.GroupRelationKey)
	assert.Empty(t, view.CoverRelationKey)
	assert.Empty(t, view.EndRelationKey)
	for _, b := range back.Blocks {
		switch c := b.Content.(type) {
		case *model.BlockContentOfDataview:
			for _, rl := range c.Dataview.RelationLinks {
				assert.True(t, isWritablePropertyKey(rl.Key),
					"an unwritable key persisted into RelationLink.Key: %q", rl.Key)
			}
		case *model.BlockContentOfLink:
			assert.Equal(t, []string{"tag"}, c.Link.Relations)
		case *model.BlockContentOfRelation:
			t.Errorf("a property block naming an unwritable key survived: %q", c.Relation.Key)
		}
	}
	assert.NotEmpty(t, warned, "every dropped slot owes a warning")
}

// The bundle index's widget `properties[]` is the same key-slot family in a
// different grammar: the schema bounds it the same way, and the widget-object
// lift refuses an unwritable key the way it refuses an out-of-range limit —
// the document then travels whole, where the link block's own slot rule
// applies.
func TestIndex_WidgetPropertiesAreBoundedTheSameWay(t *testing.T) {
	for name, hostile := range hostileKeys() {
		if strings.Contains(name, "DEL") {
			continue // the index grammar has no restatement pass; the schema's bound is its rule
		}
		t.Run("the index refuses "+name, func(t *testing.T) {
			// given
			enc, err := json.Marshal(hostile)
			require.NoError(t, err)
			doc := `{"version":2,"widgets":[{"target":"page-home","properties":[` + string(enc) + `]}]}`
			control := `{"version":2,"widgets":[{"target":"page-home","properties":["Due date"]}]}`
			_, err = UnmarshalIndex([]byte(control))
			require.NoError(t, err, "the control must be an index this format accepts")

			// when
			_, err = UnmarshalIndex([]byte(doc))

			// then
			require.Error(t, err, "accepted %s at the index widget properties slot", name)
			assert.Contains(t, issuePaths(t, err), "/widgets/0/properties/0",
				"the refusal has to name the slot it is about (§12): %v", err)
		})
	}
}
