package anyblockjson

// legendadmission_test.go — the two legends admit what they write.
//
// Every other key slot in this format checks a key before it writes it;
// `property_keys` and `type_keys` did not. The only admission on the way in
// was writableSlug/writableTypeSlug, and both return EARLY when the
// vocabulary has no slug for a key — so an unwritable stored key walked
// straight into the ledger, and Marshal emitted a legend its own Validate and
// Unmarshal reject. The whole object was unexportable and nothing said so.
//
// Reaching it needs only Options.Keys, which §3 accepts from anyone: a
// vocabulary that BINDS the spelling to some other stored key (precondition 2
// broken, which is exactly why the second termInverts table exists) makes the
// identity entry owed, and the entry is one the schema refuses.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// bindingVocabulary binds two spellings to other stored keys and spells
// nothing. Binding is the whole fixture: it is what makes an identity entry
// owed, in both namespaces, without moving any spelling.
type bindingVocabulary struct {
	BundledKeyVocabulary
	bind map[string]string
}

func (v bindingVocabulary) PropertyKey(slug string) (string, bool) {
	if key, ok := v.bind[slug]; ok {
		return key, true
	}
	return BundledKeyVocabulary{}.PropertyKey(slug)
}

func (v bindingVocabulary) TypeKey(slug string) (string, bool) {
	if key, ok := v.bind[slug]; ok {
		return key, true
	}
	return BundledKeyVocabulary{}.TypeKey(slug)
}

// overLongKey is past the §3 spelling bound by twelve characters.
var overLongKey = strings.Repeat("k", 140)

// filterKeySnapshot names a stored key at a dataview FILTER slot. That slot
// matters: unlike /properties (which drops unwritable keys before it slugs
// them), a filter hands whatever the view holds straight to propertySlug.
func filterKeySnapshot(keys ...string) *model.SmartBlockSnapshotBase {
	dv := &model.BlockContentDataview{}
	view := &model.BlockContentDataviewView{Id: "v1", Name: "All"}
	for i, key := range keys {
		dv.RelationLinks = append(dv.RelationLinks,
			&model.RelationLink{Key: key, Format: model.RelationFormat_longtext})
		view.Filters = append(view.Filters, &model.BlockContentDataviewFilter{
			Id:          "f" + string(rune('1'+i)),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Format:      model.RelationFormat_longtext,
			RelationKey: key, Value: str("x"),
		})
	}
	dv.Views = []*model.BlockContentDataviewView{view}
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{
			{Id: "o1", ChildrenIds: []string{"dv1"},
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}},
			{Id: "dv1", Content: &model.BlockContentOfDataview{Dataview: dv}},
		},
		Details: fields(map[string]*types.Value{"id": str("o1"), "name": str("Board")}),
	}
}

// The property namespace. Each case pairs the hostile key with `shadowed`, a
// perfectly writable stored key the same vocabulary binds elsewhere: that one
// MUST still get its identity entry, so a change that simply stopped writing
// legend entries cannot pass this test.
func TestExport_PropertyLegendRefusesAnEntryItCannotHold(t *testing.T) {
	for _, tc := range []struct {
		name, key, wantWarn string
	}{
		{"a control character in the stored key", "a\nb", `"a\nb" carries a control character`},
		{"a stored key past the spelling bound", overLongKey, "is 140 characters; the bound is 128"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given
			snap := filterKeySnapshot(tc.key, "shadowed")
			var warnings []Issue
			opts := Options{
				Keys: bindingVocabulary{bind: map[string]string{
					tc.key: "otherKey", "shadowed": "otherKey2"}},
				OnWarning: func(i Issue) { warnings = append(warnings, i) },
			}

			// when
			data, err := Marshal(model.SmartBlockType_Page, snap, opts)

			// then — I1: Marshal never emits what its own Validate rejects
			require.NoError(t, err)
			require.NoError(t, Validate(data), "emitted:\n%s", data)
			_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
			require.NoError(t, err, "emitted:\n%s", data)

			// the entry is refused, out loud, at the legend's own path
			assert.Equal(t, map[string]string{"shadowed": "shadowed"},
				decodeDoc(t, data).PropertyKeys,
				"only the entry the legend can hold; the shadowed key still owes one")
			require.NotEmpty(t, warnings, "a refused entry is reported")
			assert.Contains(t, warningsAt(warnings, "/property_keys"), tc.wantWarn)

			// and nothing the document was carrying is lost: the term is
			// still spelled verbatim, so it survives chain step 4
			assert.Equal(t, tc.key, backFilterKey(t, back, 0))
			assert.Equal(t, "shadowed", backFilterKey(t, back, 1))
		})
	}
}

// The value half, same rule from the other side: a DENIED stored key cannot
// be a legend value (§3 deny rule, pinned at validate.go's legend pass), and
// a vocabulary that both slugs it away and binds its verbatim spelling
// elsewhere made export write exactly that entry.
func TestExport_PropertyLegendRefusesADeniedValue(t *testing.T) {
	// given
	snap := blockKeySnapshot(map[string]*types.Value{"name": str("x")}, "uniqueKey")
	var warnings []Issue
	opts := Options{
		Keys:      denyBindingVocabulary{},
		OnWarning: func(i Issue) { warnings = append(warnings, i) },
	}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, opts)

	// then
	require.NoError(t, err)
	require.NoError(t, Validate(data), "emitted:\n%s", data)
	_, _, err = Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err, "emitted:\n%s", data)
	assert.Empty(t, decodeDoc(t, data).PropertyKeys, "a denied key cannot be a legend value")
	assert.Contains(t, warningsAt(warnings, "/property_keys"),
		`"uniqueKey" is internal: export strips it`)
}

// denyBindingVocabulary slugs an internal key away AND binds its verbatim
// spelling to another stored key — the two halves that together made the
// denied value reach the legend.
type denyBindingVocabulary struct{ BundledKeyVocabulary }

func (denyBindingVocabulary) PropertySlug(key string) string {
	if key == "uniqueKey" {
		return "sneaky"
	}
	return BundledKeyVocabulary{}.PropertySlug(key)
}

func (denyBindingVocabulary) PropertyKey(slug string) (string, bool) {
	if slug == "uniqueKey" {
		return "otherKey", true
	}
	return BundledKeyVocabulary{}.PropertyKey(slug)
}

// The type namespace has the same two shapes at the envelope `type`, which
// carries no length or charset rule of its own (§3) and so hands the stored
// key to the ledger untouched.
func TestExport_TypeLegendRefusesAnEntryItCannotHold(t *testing.T) {
	for _, tc := range []struct {
		name, key, wantWarn string
	}{
		{"a control character in the stored type key", "a\nb", `"a\nb" carries a control character`},
		{"a stored type key past the spelling bound", overLongKey, "is 140 characters; the bound is 128"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			// given — one type on the object, plus a shadowed type key at a
			// type property's object_types, which must still get its entry
			snap := &model.SmartBlockSnapshotBase{
				Blocks: []*model.Block{{Id: "t1",
					Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
				Details: fields(map[string]*types.Value{
					"id": str("t1"),
					"recommendedRelations": {Kind: &types.Value_ListValue{ListValue: &types.ListValue{
						Values: []*types.Value{str("p1")}}}},
				}),
				ObjectTypes: []string{"ot-" + tc.key},
			}
			var warnings []Issue
			opts := Options{
				Keys: bindingVocabulary{bind: map[string]string{
					tc.key: "otherType", "shadowedType": "otherType2"}},
				ResolveProperties: stubPropertyResolver{byId: map[string]PropertyDefinition{
					"p1": {Key: "prio", Format: model.RelationFormat_object,
						ObjectTypes: []string{"shadowedType"}},
				}},
				OnWarning: func(i Issue) { warnings = append(warnings, i) },
			}

			// when
			data, err := Marshal(model.SmartBlockType_STType, snap, opts)

			// then
			require.NoError(t, err)
			require.NoError(t, Validate(data), "emitted:\n%s", data)
			_, _, err = Unmarshal(data, Options{GenerateId: seqIds("g")})
			require.NoError(t, err, "emitted:\n%s", data)

			var doc struct {
				Type     string            `json:"type"`
				TypeKeys map[string]string `json:"type_keys"`
			}
			require.NoError(t, json.Unmarshal(data, &doc))
			assert.Equal(t, tc.key, doc.Type, "the term is still spelled verbatim")
			assert.Equal(t, map[string]string{"shadowedType": "shadowedType"}, doc.TypeKeys,
				"only the entry the legend can hold")
			require.NotEmpty(t, warnings)
			assert.Contains(t, warningsAt(warnings, "/type_keys"), tc.wantWarn)
		})
	}
}

// warningsAt joins the messages of every warning filed at one path, so a
// Contains assertion cannot pass on a warning about something else.
func warningsAt(issues []Issue, path string) string {
	var msgs []string
	for _, i := range issues {
		if i.Path == path {
			msgs = append(msgs, i.Message)
		}
	}
	return strings.Join(msgs, "\n")
}

// backFilterKey digs the nth filter's relation key out of an imported
// snapshot.
func backFilterKey(t *testing.T, snap *model.SmartBlockSnapshotBase, n int) string {
	t.Helper()
	for _, b := range snap.Blocks {
		if c, ok := b.Content.(*model.BlockContentOfDataview); ok {
			require.Len(t, c.Dataview.Views, 1)
			require.Greater(t, len(c.Dataview.Views[0].Filters), n)
			return c.Dataview.Views[0].Filters[n].RelationKey
		}
	}
	t.Fatal("no dataview in the imported snapshot")
	return ""
}
