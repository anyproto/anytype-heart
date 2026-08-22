package anyblockjson

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The real shape, 135 characters of it, on the account that produced the
// corpus these numbers come from.
const testParticipantId = "_participant_bafyreid62d5e6hny6mv6zass2zg73nxyhjzhjasx7imvzxvqz6rcnjqcgq_30afw2fe3tvff_AASdKiEGfcyhxX3ufr4auHRviACUXxkF68uZwtSb2AnyRoMA"

// `creator` and `lastModifiedBy` are DERIVED: recovered from the object
// tree root's own signature on every rebuild, and discarded by every write
// path. A document naming one is telling a reader who wrote the object, not
// setting anything — so import drops the key instead of refusing it, in every
// spelling, and whatever shape the value arrives in.
//
// Before this, the two behaved differently for no reason anyone could state:
// `creator` was accepted (it sat in propertiesKeptOnExport, so the deny rule
// never saw it) and landed a detail that the next rebuild overwrote, while
// `lastModifiedBy` — the same relation definition, one word apart in the
// bundle — was refused outright, making 71 documents in a 36,966-object
// corpus unimportable for a line neither side could act on.
//
// How these can fail: drop the keys from derivedAttributionProperties and
// every case flips to an error (the deny rule reaches them the moment they
// are not exempt, since export strips both); wire the exemption so that it
// only skips the ERROR and not the write, and the NotContains assertions
// fail with the value on details.
func TestAttributionProperties_DroppedNotRefused(t *testing.T) {
	for name, tc := range map[string]struct {
		doc string
		key string
	}{
		"creator, as a member name (what export writes)": {
			doc: `{"version": 1, "properties": {"creator": "Roman"}}`,
			key: "creator",
		},
		"creator, as the id array older exports wrote": {
			doc: fmt.Sprintf(`{"version": 1, "properties": {"creator": [%q]}}`, testParticipantId),
			key: "creator",
		},
		"last_modified_by, the spelling that used to be refused": {
			doc: `{"version": 1, "properties": {"last_modified_by": "Roman"}}`,
			key: "lastModifiedBy",
		},
		"last_modified_by, as an id array": {
			doc: fmt.Sprintf(`{"version": 1, "properties": {"last_modified_by": [%q]}}`, testParticipantId),
			key: "lastModifiedBy",
		},
		"the stored spelling drops too": {
			doc: `{"version": 1, "properties": {"lastModifiedBy": "Roman"}}`,
			key: "lastModifiedBy",
		},
	} {
		t.Run(name, func(t *testing.T) {
			// given the document above

			// when
			validateErr := Validate([]byte(tc.doc))
			_, snap, err := Unmarshal([]byte(tc.doc), Options{})

			// then
			require.NoError(t, validateErr, "an attribution line makes a document stale, not wrong")
			require.NoError(t, err, "Validate and Unmarshal agree (§11 I2)")
			assert.NotContains(t, snap.GetDetails().GetFields(), tc.key,
				"and it must not reach the snapshot: the value is derived, not input")
		})
	}
}

// The control that keeps this from spreading. `assignee` points at a
// participant too, and looks identical in a document — but it is
// `source: details, maxCount: 0`, chosen by a person, and the id is the whole
// of its meaning. Dropping it, or spelling it as a name, would be data loss.
//
// How this can fail: add assignee (or author, or any `source: details`
// property) to derivedAttributionProperties and the value stops arriving.
func TestAttributionProperties_UserChosenParticipantsAreUntouched(t *testing.T) {
	for _, key := range []string{"assignee", "author", "stakeholders"} {
		t.Run(key, func(t *testing.T) {
			// given
			doc := fmt.Sprintf(`{"version": 1, "properties": {%q: [%q]}}`, key, testParticipantId)

			// when
			_, snap, err := Unmarshal([]byte(doc), Options{})

			// then
			require.NoError(t, err)
			require.Contains(t, snap.GetDetails().GetFields(), key)
			assert.Equal(t, []string{testParticipantId},
				valueStringList(snap.GetDetails().GetFields()[key]),
				"a user-chosen participant reference keeps its full id")
		})
	}
}

// The legend resolves a spelling onto a stored key before admission runs
// (§3), so it is the way a document could smuggle a key past a rule keyed on
// the spelling. It must reach the same verdict here: dropped, not landed and
// not refused.
func TestAttributionProperties_LegendCannotLandThem(t *testing.T) {
	for name, tc := range map[string]struct{ doc, key string }{
		"creator":        {`{"version": 1, "property_keys": {"who": "creator"}, "properties": {"who": "Roman"}}`, "creator"},
		"lastModifiedBy": {`{"version": 1, "property_keys": {"who": "lastModifiedBy"}, "properties": {"who": "Roman"}}`, "lastModifiedBy"},
	} {
		t.Run(name, func(t *testing.T) {
			// when
			validateErr := Validate([]byte(tc.doc))
			_, snap, err := Unmarshal([]byte(tc.doc), Options{})

			// then
			require.NoError(t, validateErr)
			require.NoError(t, err)
			assert.NotContains(t, snap.GetDetails().GetFields(), tc.key)
			assert.NotContains(t, snap.GetDetails().GetFields(), "who")
		})
	}
}

//
// ---- export: the member's name, or nothing ----
//

// nameResolver answers with a fixed table, and records what it was asked.
type nameResolver struct {
	names map[string]string
	asked []string
}

func (r *nameResolver) ParticipantName(id string) (string, bool) {
	r.asked = append(r.asked, id)
	name, ok := r.names[id]
	return name, ok && name != ""
}

// attributionSnapshot is an ordinary object with both attribution details set,
// stored the way real objects store them.
func attributionSnapshot(details map[string]*types.Value) *model.SmartBlockSnapshotBase {
	full := map[string]*types.Value{
		"id":   str("obj1"),
		"name": str("Notes"),
	}
	for k, v := range details {
		full[k] = v
	}
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{
			Id:      "obj1",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}},
		Details: fields(full),
	}
}

// exportedProperties runs a whole-document export and hands back the
// `properties` object as plain JSON, plus the raw bytes — the public entry
// point, so nothing here can pass by re-implementing the rule.
func exportedProperties(t *testing.T, snap *model.SmartBlockSnapshotBase, opts Options) (map[string]any, string) {
	t.Helper()
	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	require.NoError(t, Validate(data), "Marshal never emits what Validate rejects (§11 I1)")
	var doc struct {
		Properties map[string]any `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	return doc.Properties, string(data)
}

// A participant id is 135 characters and says nothing to a reader; the member's
// name is the whole of what the line is for. `creator` is on 100% of a
// 36,966-object corpus, so this is the single most repeated string the format
// used to write.
//
// How these can fail: emit the stored value instead of the name and the first
// case finds an array (and the id in the bytes); write the name inside a list
// and the plain-string assertion fails — `maxCount: 1` makes the wrapper
// definitionally wrong, and 0 of 36,966 corpus values were multi-valued.
func TestAttribution_ExportWritesTheMemberName(t *testing.T) {
	resolver := &nameResolver{names: map[string]string{testParticipantId: "Roman"}}
	opts := testOptions()
	opts.ResolveParticipants = resolver

	t.Run("creator is a plain string holding the name", func(t *testing.T) {
		// given
		snap := attributionSnapshot(map[string]*types.Value{"creator": strList(testParticipantId)})

		// when
		props, raw := exportedProperties(t, snap, opts)

		// then
		assert.Equal(t, "Roman", props["creator"])
		assert.NotContains(t, raw, testParticipantId, "the id must not survive anywhere in the document")
	})

	t.Run("a scalar-stored value reads the same", func(t *testing.T) {
		// given real data stores this key both ways
		snap := attributionSnapshot(map[string]*types.Value{"creator": str(testParticipantId)})

		// when
		props, _ := exportedProperties(t, snap, opts)

		// then
		assert.Equal(t, "Roman", props["creator"])
	})

	t.Run("lastModifiedBy is spelled last_modified_by and named too", func(t *testing.T) {
		// given
		snap := attributionSnapshot(map[string]*types.Value{
			"creator":        strList(testParticipantId),
			"lastModifiedBy": strList(testParticipantId),
		})

		// when
		props, raw := exportedProperties(t, snap, opts)

		// then
		assert.Equal(t, "Roman", props["last_modified_by"])
		assert.NotContains(t, props, "lastModifiedBy", "the document spells slugs (§3)")
		assert.NotContains(t, raw, testParticipantId)
	})
}

// The rule the spec is emphatic about: no resolver, or no name, and the
// property is ABSENT — never the raw id, never an empty string. A format whose
// `creator` is sometimes a name and sometimes a 135-character address is worse
// to read than one that consistently carries neither.
//
// How these can fail: any fallback to the id or to "" makes the NotContains
// assertions fail, and so does emitting an explicit null.
func TestAttribution_OmittedWhenThereIsNoName(t *testing.T) {
	for name, opts := range map[string]Options{
		"no participant resolver at all": testOptions(),
		"a resolver that cannot name this member": func() Options {
			o := testOptions()
			o.ResolveParticipants = &nameResolver{names: map[string]string{}}
			return o
		}(),
		"a member whose profile name is empty": func() Options {
			o := testOptions()
			o.ResolveParticipants = &nameResolver{names: map[string]string{testParticipantId: ""}}
			return o
		}(),
	} {
		t.Run(name, func(t *testing.T) {
			// given
			snap := attributionSnapshot(map[string]*types.Value{
				"creator":        strList(testParticipantId),
				"lastModifiedBy": strList(testParticipantId),
			})

			// when
			props, raw := exportedProperties(t, snap, opts)

			// then
			assert.NotContains(t, props, "creator")
			assert.NotContains(t, props, "last_modified_by")
			assert.NotContains(t, raw, testParticipantId)
			assert.Contains(t, props, "name", "the rest of the object is untouched")
		})
	}
}

// The control, again, on the export side: `assignee` is `source: details`,
// user-chosen, and its id is the whole of its meaning. It keeps the full id
// and the array shape even with a resolver that could name the member.
func TestAttribution_ExportLeavesUserChosenParticipantsAlone(t *testing.T) {
	// given
	opts := testOptions()
	opts.ResolveParticipants = &nameResolver{names: map[string]string{testParticipantId: "Roman"}}
	snap := attributionSnapshot(map[string]*types.Value{
		"creator":  strList(testParticipantId),
		"assignee": strList(testParticipantId),
	})

	// when
	props, _ := exportedProperties(t, snap, opts)

	// then
	assert.Equal(t, "Roman", props["creator"])
	assert.Equal(t, []any{testParticipantId}, props["assignee"],
		"a user-chosen participant reference keeps its id and its list shape")
}

// The term census reserves what the document SPELLS (§9a). `creator` is on the
// stripped list now, so the census stopped seeing it through the ordinary
// details walk — and a custom relation whose api slug is `creator` would then
// claim the spelling, putting two properties on one JSON member and losing one.
//
// How this can fail: drop the attribution arm of seedTermLedger and the custom
// key takes `creator`, so this finds "Roman" under a key that means the custom
// relation, or one of the two values gone.
func TestAttribution_CensusReservesTheSpellingItWrites(t *testing.T) {
	// given a space that slugs a custom relation onto `creator`
	opts := testOptions()
	opts.ResolveParticipants = &nameResolver{names: map[string]string{testParticipantId: "Roman"}}
	// the custom key sorts BEFORE `creator`, so it reaches the term ledger
	// first and claims the spelling unless the census has reserved it
	opts.Keys = slugVocabulary{"aCustomKey": "creator"}
	snap := attributionSnapshot(map[string]*types.Value{
		"creator":    strList(testParticipantId),
		"aCustomKey": str("mine"),
	})

	// when
	props, _ := exportedProperties(t, snap, opts)

	// then
	assert.Equal(t, "Roman", props["creator"], "the attribution key keeps its own spelling")
	assert.Equal(t, "mine", props["aCustomKey"],
		"the custom key falls back to its stored key, which is always its own address (§3)")
}

// slugVocabulary spells the stored keys it is given and passes everything else
// through, which is the minimum a KeyVocabulary owes (§3).
type slugVocabulary map[string]string

func (v slugVocabulary) PropertySlug(key string) string {
	if slug, ok := v[key]; ok {
		return slug
	}
	return BundledKeyVocabulary{}.PropertySlug(key)
}

func (v slugVocabulary) PropertyKey(slug string) (string, bool) {
	for key, s := range v {
		if s == slug {
			return key, true
		}
	}
	return BundledKeyVocabulary{}.PropertyKey(slug)
}

func (v slugVocabulary) TypeSlug(key string) string { return BundledKeyVocabulary{}.TypeSlug(key) }
func (v slugVocabulary) TypeKey(slug string) (string, bool) {
	return BundledKeyVocabulary{}.TypeKey(slug)
}

// The documented cost of the change, pinned so it cannot become a surprise:
// the attribution line does NOT survive a round trip, so an object carrying a
// creator is the one case where Export(S) ≠ Export(Import(Export(S))) for
// something that is not an id (§11).
//
// It is not recoverable and was never data: the value is derived from the tree
// root's signature, and an imported object gets the importing account's own.
// What this pins is that the loss happens ONCE — the second export equals the
// third — so a re-export still diffs cleanly against itself.
func TestAttribution_DoesNotSurviveARoundTrip(t *testing.T) {
	// given
	opts := testOptions()
	opts.ResolveParticipants = &nameResolver{names: map[string]string{testParticipantId: "Roman"}}
	snap := attributionSnapshot(map[string]*types.Value{"creator": strList(testParticipantId)})

	// when
	first, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	_, imported, err := Unmarshal(first, opts)
	require.NoError(t, err)
	second, err := Marshal(model.SmartBlockType_Page, imported, opts)
	require.NoError(t, err)
	_, reimported, err := Unmarshal(second, opts)
	require.NoError(t, err)
	third, err := Marshal(model.SmartBlockType_Page, reimported, opts)
	require.NoError(t, err)

	// then
	assert.Contains(t, string(first), `"creator": "Roman"`)
	assert.NotContains(t, string(second), `"creator"`, "import drops it, so the next export has nothing to name")
	assert.Equal(t, string(second), string(third), "and everything after the first export is byte-stable")
}
