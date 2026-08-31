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
			doc: `{"version": 2, "properties": {"creator": "Roman"}}`,
			key: "creator",
		},
		"creator, as the id array older exports wrote": {
			doc: fmt.Sprintf(`{"version": 2, "properties": {"creator": [%q]}}`, testParticipantId),
			key: "creator",
		},
		"last_modified_by, the spelling that used to be refused": {
			doc: `{"version": 2, "properties": {"last_modified_by": "Roman"}}`,
			key: "lastModifiedBy",
		},
		"last_modified_by, as an id array": {
			doc: fmt.Sprintf(`{"version": 2, "properties": {"last_modified_by": [%q]}}`, testParticipantId),
			key: "lastModifiedBy",
		},
		"the stored spelling drops too": {
			doc: `{"version": 2, "properties": {"lastModifiedBy": "Roman"}}`,
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
			doc := fmt.Sprintf(`{"version": 2, "properties": {%q: [%q]}}`, key, testParticipantId)

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
		"creator":        {`{"version": 2, "property_internal_keys": {"who": "creator"}, "properties": {"who": "Roman"}}`, "creator"},
		"lastModifiedBy": {`{"version": 2, "property_internal_keys": {"who": "lastModifiedBy"}, "properties": {"who": "Roman"}}`, "lastModifiedBy"},
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
// ---- export: the member's id, with the name riding as the suffix ----
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

// The two halves of testParticipantId, for spelling expectations: the space
// the composite embeds and the checksummed identity it folds to (§9).
const (
	testAttribSpaceId  = "bafyreid62d5e6hny6mv6zass2zg73nxyhjzhjasx7imvzxvqz6rcnjqcgq.30afw2fe3tvff"
	testAttribIdentity = "AASdKiEGfcyhxX3ufr4auHRviACUXxkF68uZwtSb2AnyRoMA"
)

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

// Attribution is `<identity>#<name>` (§3): the folded participant id —
// RESOLVABLE, which the v0.24 name-only spelling was not (API v2 consumers
// need an id to resolve a member, and 76 of 2,478 production participants
// share a display name) — with the member's name as the informative suffix,
// ~57 characters against the 135 the composite was. A plain string, not an
// array: the relation is `maxCount: 1` and 0 of 36,966 corpus values were
// multi-valued.
//
// How these can fail: write the name alone and the id assertions fail; write
// the raw composite and the folded-spelling assertions fail; write the value
// inside a list and the plain-string equality fails; skip the normalizer and
// "Roma Kha" arrives with its space.
func TestAttribution_ExportWritesIdentityAndName(t *testing.T) {
	resolver := &nameResolver{names: map[string]string{testParticipantId: "Roma Kha"}}
	opts := testOptions()
	opts.ResolveParticipants = resolver
	opts.SpaceId = testAttribSpaceId

	t.Run("creator is a plain string: folded id, then the name", func(t *testing.T) {
		// given
		snap := attributionSnapshot(map[string]*types.Value{"creator": strList(testParticipantId)})

		// when
		props, raw := exportedProperties(t, snap, opts)

		// then
		assert.Equal(t, testAttribIdentity+"#roma_kha", props["Created by"])
		assert.NotContains(t, raw, testParticipantId,
			"the 135-character composite folds; the identity stands in (§9)")
	})

	t.Run("a scalar-stored value reads the same", func(t *testing.T) {
		// given real data stores this key both ways
		snap := attributionSnapshot(map[string]*types.Value{"creator": str(testParticipantId)})

		// when
		props, _ := exportedProperties(t, snap, opts)

		// then
		assert.Equal(t, testAttribIdentity+"#roma_kha", props["Created by"])
	})

	t.Run("lastModifiedBy is spelled last_modified_by and shaped the same", func(t *testing.T) {
		// given
		snap := attributionSnapshot(map[string]*types.Value{
			"creator":        strList(testParticipantId),
			"lastModifiedBy": strList(testParticipantId),
		})

		// when
		props, raw := exportedProperties(t, snap, opts)

		// then
		assert.Equal(t, testAttribIdentity+"#roma_kha", props["Last modified by"])
		assert.NotContains(t, props, "lastModifiedBy", "the document spells display names (§3)")
		assert.NotContains(t, raw, testParticipantId)
	})

	t.Run("without a space id the composite survives whole, still with the name", func(t *testing.T) {
		// given the fold is off (§9) — no SpaceId, no fold, either direction
		bare := testOptions()
		bare.ResolveParticipants = resolver
		snap := attributionSnapshot(map[string]*types.Value{"creator": strList(testParticipantId)})

		// when
		props, _ := exportedProperties(t, snap, bare)

		// then
		assert.Equal(t, testParticipantId+"#roma_kha", props["Created by"])
	})
}

// No resolver, or no name, and the id is written BARE — the id is the
// resolvable half and complete without its caption. Never a dangling `#`,
// and never an omitted property: v0.24's "no name, no property" rule made
// the line unreadable to API consumers precisely when a resolver was
// missing, which is the reversal this change corrects.
//
// How these can fail: keep the old omit-on-no-name rule and every case
// finds the property missing; write "#" with an empty name after it and the
// exact-equality cases fail.
func TestAttribution_BareIdWhenThereIsNoName(t *testing.T) {
	for name, opts := range map[string]Options{
		"no participant resolver at all": func() Options {
			o := testOptions()
			o.SpaceId = testAttribSpaceId
			return o
		}(),
		"a resolver that cannot name this member": func() Options {
			o := testOptions()
			o.SpaceId = testAttribSpaceId
			o.ResolveParticipants = &nameResolver{names: map[string]string{}}
			return o
		}(),
		"a member whose profile name is empty": func() Options {
			o := testOptions()
			o.SpaceId = testAttribSpaceId
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
			props, _ := exportedProperties(t, snap, opts)

			// then
			assert.Equal(t, testAttribIdentity, props["Created by"], "the bare folded id, nothing else")
			assert.Equal(t, testAttribIdentity, props["Last modified by"])
		})
	}

	t.Run("an empty-identity composite is omitted: it addresses nobody", func(t *testing.T) {
		// given the real artifact: 9,103 of 37,429 production objects store
		// `_participant_<space>_` — the composite built from a BLANK
		// identity — in lastModifiedBy. 86 characters that resolve to no
		// member; the id-shaped analogue of a blank name.
		degenerate := "_participant_bafyreid62d5e6hny6mv6zass2zg73nxyhjzhjasx7imvzxvqz6rcnjqcgq_30afw2fe3tvff_"
		snap := attributionSnapshot(map[string]*types.Value{
			"creator":        strList(testParticipantId),
			"lastModifiedBy": strList(degenerate),
		})
		opts := testOptions()
		opts.SpaceId = testAttribSpaceId

		// when
		props, raw := exportedProperties(t, snap, opts)

		// then
		assert.Equal(t, testAttribIdentity, props["Created by"], "the control: a real id still lands")
		assert.NotContains(t, props, "Last modified by")
		assert.NotContains(t, raw, degenerate)
	})

	t.Run("an empty stored value is still omitted", func(t *testing.T) {
		// given no id at all — a bare id is a complete answer, no id is none
		snap := attributionSnapshot(nil)
		opts := testOptions()
		opts.SpaceId = testAttribSpaceId

		// when
		props, _ := exportedProperties(t, snap, opts)

		// then
		assert.NotContains(t, props, "Created by")
		assert.NotContains(t, props, "Last modified by")
	})
}

// The control, again, on the export side: `assignee` is `source: details`,
// user-chosen, and its id is the whole of its meaning. It keeps the id (the
// §9 fold applies — the folded spelling IS the id, restated) and the ARRAY
// shape, and it takes no name suffix from the participant resolver: the
// suffix on ordinary references belongs to RefNames + ResolveObjectNames,
// not to the attribution seam.
func TestAttribution_ExportLeavesUserChosenParticipantsAlone(t *testing.T) {
	// given
	opts := testOptions()
	opts.SpaceId = testAttribSpaceId
	opts.ResolveParticipants = &nameResolver{names: map[string]string{testParticipantId: "Roma Kha"}}
	snap := attributionSnapshot(map[string]*types.Value{
		"creator":  strList(testParticipantId),
		"assignee": strList(testParticipantId),
	})

	// when
	props, _ := exportedProperties(t, snap, opts)

	// then
	assert.Equal(t, testAttribIdentity+"#roma_kha", props["Created by"], "a plain string")
	assert.Equal(t, []any{testAttribIdentity}, props["Assignee"],
		"a user-chosen participant reference keeps its list shape and takes no suffix here")
}

// The term census reserves what the document SPELLS (§9a). `creator` is on the
// stripped list now, so the census stopped seeing it through the ordinary
// details walk — and a custom relation whose vocabulary spelling collides
// with it would then claim a spelling the attribution line needs, putting
// two properties on one JSON member and losing one. The colliding string
// here is the stored key `creator` itself: verbatim-first reserves it, so
// the custom claimant degrades to its own stored key.
//
// How this can fail: drop the attribution arm of seedTermLedger and the
// custom key takes `creator` while the attribution line spells it too — one
// of the two values gone.
func TestAttribution_CensusReservesTheSpellingItWrites(t *testing.T) {
	// given a space that slugs a custom relation onto `creator`
	opts := testOptions()
	opts.SpaceId = testAttribSpaceId
	opts.ResolveParticipants = &nameResolver{names: map[string]string{testParticipantId: "Roma Kha"}}
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
	assert.Equal(t, testAttribIdentity+"#roma_kha", props["Created by"],
		"the attribution key keeps its own spelling")
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
	opts.SpaceId = testAttribSpaceId
	opts.ResolveParticipants = &nameResolver{names: map[string]string{testParticipantId: "Roma Kha"}}
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
	assert.Contains(t, string(first), `"Created by": "`+testAttribIdentity+`#roma_kha"`)
	assert.NotContains(t, string(second), `"Created by"`, "import drops it, so the next export has nothing to write")
	assert.Equal(t, string(second), string(third), "and everything after the first export is byte-stable")
}
