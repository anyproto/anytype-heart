package anyblockjson

// relationformat_test.go — the §2d relation-definition envelope fields:
// `format`, `include_time`, `object_types` on kind:relation documents, the
// refusal of their flat spellings in `properties`, and the presence-mirror
// round trip.

import (
	"encoding/json"
	"math"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func nullValue() *types.Value {
	return &types.Value{Kind: &types.Value_NullValue{}}
}

// relationSnapshot is a minimal kind:relation snapshot — the details a real
// relation object carries, minus the install noise this test does not need.
func relationSnapshot(details map[string]*types.Value) *model.SmartBlockSnapshotBase {
	if details == nil {
		details = map[string]*types.Value{}
	}
	details["id"] = str("relObjectId")
	if _, has := details["name"]; !has {
		details["name"] = str("Budget")
	}
	return &model.SmartBlockSnapshotBase{
		Key:     "budget",
		Details: fields(details),
		Blocks: []*model.Block{{
			Id:      "relObjectId",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}},
	}
}

// typeIdVocabulary is a TypeResolver-capable property resolver: the shape
// storeresolver has, reduced to the id↔key translation the §2d slot needs.
type typeIdVocabulary struct {
	testPropertyResolver
	keyById map[string]string
	idByKey map[string]string
}

func (r *typeIdVocabulary) TypeKeyById(id string) (string, bool) {
	key, ok := r.keyById[id]
	return key, ok
}

func (r *typeIdVocabulary) TypeIdByKey(key string) (string, bool) {
	id, ok := r.idByKey[key]
	return id, ok
}

func newTypeIdVocabulary() *typeIdVocabulary {
	return &typeIdVocabulary{
		testPropertyResolver: *newTestPropertyResolver(),
		keyById:              map[string]string{"typeid-page": "page", "typeid-wine": "wine"},
		idByKey:              map[string]string{"page": "typeid-page", "wine": "typeid-wine"},
	}
}

// The three stored details travel on the envelope, never in `properties`.
//
// How this can fail: drop relationLiftedDetailKeys from envelopeLiftedKeys
// and the flat spellings reappear in properties; drop the
// buildRelationEnvelope call from buildDoc and the envelope fields vanish.
func TestRelationEnvelope_LiftsTheThreeDetails(t *testing.T) {
	// given
	snap := relationSnapshot(map[string]*types.Value{
		"relationFormat":            num(2),
		"relationFormatIncludeTime": boolValue(false),
		"relationFormatObjectTypes": strList("typeid-page"),
	})

	// when
	data, err := Marshal(model.SmartBlockType_STRelation, snap, testOptions())
	require.NoError(t, err)

	// then
	assert.Contains(t, string(data), `"format": "number"`)
	assert.Contains(t, string(data), `"include_time": false`)
	assert.Contains(t, string(data), `"object_types"`)
	for _, spelling := range []string{`"relation_format"`, `"relation_format_include_time"`,
		`"relation_format_object_types"`} {
		assert.NotContains(t, string(data), spelling,
			"the flat spelling must not survive anywhere — properties refuses it (§2d)")
	}
	require.NoError(t, Validate(data), "Marshal never emits what Validate rejects (§11 I1)")
}

// `format` is required on a relation document, so every relation export
// writes one — including a stored 0 (longtext, a real format) and a snapshot
// with no relationFormat detail at all, both of which write "text".
//
// How this can fail: emit `format` with setNonEmpty, or skip it when the
// detail is absent, and the exported document is one Validate refuses (I1).
func TestRelationEnvelope_FormatIsAlwaysWritten(t *testing.T) {
	for name, details := range map[string]map[string]*types.Value{
		"stored zero":   {"relationFormat": num(0)},
		"absent detail": {},
	} {
		t.Run(name, func(t *testing.T) {
			// when
			data, err := Marshal(model.SmartBlockType_STRelation, relationSnapshot(details), testOptions())
			require.NoError(t, err)

			// then
			assert.Contains(t, string(data), `"format": "text"`)
			require.NoError(t, Validate(data), "§11 I1")
		})
	}
}

// formatNames is total over model.RelationFormat — the property that makes
// the required §2d `format` safe for every stored enum value. shorttext is
// the one deliberate hole; formatName folds it into "text".
//
// How this can fail: add a value to the model enum without naming it here
// (the way "map" was missing before v0.31, on 72 production documents), or
// remove a name from formatNames.
func TestFormatNames_TotalOverModelEnum(t *testing.T) {
	for raw, enumName := range model.RelationFormat_name {
		f := model.RelationFormat(raw)
		assert.NotEmpty(t, formatName(f),
			"stored format %s (%d) has no §3 name: a relation object carrying it cannot be exported (§2d)",
			enumName, raw)
	}
}

// Format "map" (102) is real data — 72 production relation documents, every
// one the bundled templatePlaceholders relation — and round-trips by name.
//
// How this can fail: remove RelationFormat_map from formatNames and export
// errors; remove "map" from the schema enum and the exported document fails
// its own validation.
func TestRelationEnvelope_MapFormatRoundTrips(t *testing.T) {
	// given
	snap := relationSnapshot(map[string]*types.Value{"relationFormat": num(102)})

	// when
	data, err := Marshal(model.SmartBlockType_STRelation, snap, testOptions())
	require.NoError(t, err)
	require.NoError(t, Validate(data), "§11 I1")
	sbType, got, err := Unmarshal(data, testOptions())

	// then
	require.NoError(t, err)
	assert.Contains(t, string(data), `"format": "map"`)
	assert.Equal(t, model.SmartBlockType_STRelation, sbType)
	assert.Equal(t, float64(102), got.Details.Fields["relationFormat"].GetNumberValue())
}

// A stored format the vocabulary cannot name fails the export by name,
// rather than being rewritten to "text": `format` is required, so there is
// nothing to omit, and a false format claim imports as a permanent silent
// format rewrite — the disease the lift exists to kill.
//
// How this can fail: make relationFormatName fall back to "text" for an
// unnameable value and the error disappears.
func TestRelationEnvelope_UnnameableFormatFailsExport(t *testing.T) {
	for name, v := range map[string]*types.Value{
		"outside the enum": num(47),
		"not a number":     str("weird"),
	} {
		t.Run(name, func(t *testing.T) {
			// when
			_, err := Marshal(model.SmartBlockType_STRelation,
				relationSnapshot(map[string]*types.Value{"relationFormat": v}), testOptions())

			// then
			require.Error(t, err)
			assert.Contains(t, err.Error(), "cannot state what it defines",
				"the failure has to say the document could not be written, not merely that a value was odd")
		})
	}
}

// A stored relationFormat that is NaN, infinite, or outside int32 fails the
// export BY THE GUARD, before the float ever reaches int32: what int32(n)
// yields for such values is implementation-dependent (Go spec, Conversions),
// and measured here int32(NaN) is 0 — so without the guard a NaN format
// exports as `"format": "text"`, a false claim about what the property is
// that imports as a permanent silent rewrite to longtext, the exact disease
// the §2d lift exists to kill. The other inputs happen to land outside the
// enum on this architecture and would still error by name, which is why
// every subtest pins the GUARD's own message rather than merely
// require.Error: the pin must hold on every architecture, never on the
// accident of what the conversion produces. Corrupt-data-only territory —
// all 10,617 production relation documents carry an in-enum integer — but
// corrupt data is exactly what an export must refuse to launder into a
// well-formed lie.
//
// How this can fail: drop the IsNaN/IsInf/int32-range guard from
// relationFormatName. The NaN subtest then exports `"format": "text"`
// cleanly, and the rest degrade to the no-name error — the wrong statement
// about a value the reading never legitimately produced.
func TestRelationEnvelope_NonFiniteFormatFailsExportByTheGuard(t *testing.T) {
	for name, raw := range map[string]float64{
		"NaN":          math.NaN(),
		"+Inf":         math.Inf(1),
		"-Inf":         math.Inf(-1),
		"beyond int32": 1e10,
		"negative":     -1,
	} {
		t.Run(name, func(t *testing.T) {
			// when
			_, err := Marshal(model.SmartBlockType_STRelation,
				relationSnapshot(map[string]*types.Value{"relationFormat": num(raw)}), testOptions())

			// then
			require.Error(t, err, "a value the enum cannot hold must fail export, never become text")
			assert.Contains(t, err.Error(), "outside the format enum",
				"the guard, not the int32 conversion's accident, must be what refuses this value")
		})
	}
}

// The flat spellings are refused in `properties`, by Validate AND by
// Unmarshal (§12 I2), with the envelope repair named. This is also the whole
// of the legacy story (§10): a pre-v0.31 document spells `relation_format`
// here and is refused loudly instead of read.
//
// How this can fail: drop the relationLiftedDetailKeys arm from
// deniedPropertyKey and both doors accept the raw number again — a phantom
// property in Validate's case, a silent second spelling in Unmarshal's.
func TestRelationEnvelope_RefusedInProperties(t *testing.T) {
	for spelling, value := range map[string]string{
		"relation_format":              "100",
		"relation_format_include_time": "true",
		"relation_format_object_types": `["page"]`,
	} {
		t.Run(spelling, func(t *testing.T) {
			doc := `{"version":1,"kind":"relation","id":"o1","key":"budget","format":"number",` +
				`"properties":{"` + spelling + `":` + value + `}}`

			err := Validate([]byte(doc))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "/properties/"+spelling)
			assert.Contains(t, err.Error(), "§2d", "the refusal names the repair")

			_, _, err = Unmarshal([]byte(doc), testOptions())
			require.Error(t, err, "Unmarshal must refuse what Validate refuses (§12 I2)")
		})
	}
}

// Envelope presence mirrors stored presence EXACTLY — false, `[]` and null
// all travel, absence stays absence — so the same details go in and out and
// the snapshot comparator needs no new rule (§2d, §11).
//
// How this can fail: emit include_time or object_types with setNonEmpty
// (false and [] vanish), drop the null arms (80 production relations hold a
// null includeTime), or make import invent a detail the document does not
// carry.
func TestRelationEnvelope_PresenceMirrorsTheStore(t *testing.T) {
	for name, tc := range map[string]struct {
		details  map[string]*types.Value
		wire     []string // substrings the document must carry
		notOnDoc []string // members the document must NOT carry
	}{
		"present and false/empty": {
			details: map[string]*types.Value{
				"relationFormat":            num(6),
				"relationFormatIncludeTime": boolValue(false),
				"relationFormatObjectTypes": strList(),
			},
			wire: []string{`"include_time": false`, `"object_types": []`},
		},
		"present and null": {
			details: map[string]*types.Value{
				"relationFormat":            num(4),
				"relationFormatIncludeTime": nullValue(),
			},
			wire:     []string{`"include_time": null`},
			notOnDoc: []string{`"object_types"`},
		},
		// object_types stored as a NULL is its own arm on both sides, and
		// it is the one that can break §11 I1: export writes
		// `"object_types": null`, so the schema's type union has to admit
		// null or Marshal emits what Validate rejects. Nothing reached this
		// path before — the include_time null case above exercises a
		// different arm.
		"object_types present and null": {
			details: map[string]*types.Value{
				"relationFormat":            num(100),
				"relationFormatObjectTypes": nullValue(),
			},
			wire:     []string{`"object_types": null`},
			notOnDoc: []string{`"include_time"`},
		},
		"absent": {
			details:  map[string]*types.Value{"relationFormat": num(2)},
			notOnDoc: []string{`"include_time"`, `"object_types"`},
		},
	} {
		t.Run(name, func(t *testing.T) {
			// given
			snap := relationSnapshot(tc.details)
			want := map[string]*types.Value{}
			for k, v := range snap.Details.Fields {
				if relationLiftedDetailKeys()[k] {
					want[k] = v
				}
			}

			// when
			data, err := Marshal(model.SmartBlockType_STRelation, snap, testOptions())
			require.NoError(t, err)
			require.NoError(t, Validate(data), "§11 I1")
			_, got, err := Unmarshal(data, testOptions())
			require.NoError(t, err)

			// then
			for _, w := range tc.wire {
				assert.Contains(t, string(data), w)
			}
			for _, w := range tc.notOnDoc {
				assert.NotContains(t, string(data), w,
					"an absent stored key writes no envelope member")
			}
			for k, v := range want {
				assert.Equal(t, v, got.Details.Fields[k], "detail %q changed on the way round", k)
			}
			for k := range relationLiftedDetailKeys() {
				if _, wanted := want[k]; !wanted {
					assert.Nil(t, got.Details.Fields[k], "the round trip invented detail %q", k)
				}
			}
		})
	}
}

// With the TypeResolver capability wired, the stored target-type ids are
// spelled as type keys on the wire and come back as the same ids — the id↔key
// translation is an inverse, so the snapshot round-trips byte-exactly. An
// entry the resolver cannot answer passes through verbatim, its own address.
//
// How this can fail: drop the TypeKeyById arm from relationTargetKeys and
// the wire carries raw ids under a resolver; drop the TypeIdByKey arm from
// applyRelationEnvelope and the round trip stores keys where ids were.
func TestRelationEnvelope_TargetTypesTranslateThroughTheResolver(t *testing.T) {
	// given
	opts := testOptions()
	opts.ResolveProperties = newTypeIdVocabulary()
	snap := relationSnapshot(map[string]*types.Value{
		"relationFormat":            num(100),
		"relationFormatObjectTypes": strList("typeid-page", "bafyreidangling"),
	})

	// when
	data, err := Marshal(model.SmartBlockType_STRelation, snap, opts)
	require.NoError(t, err)
	_, got, err := Unmarshal(data, opts)
	require.NoError(t, err)

	// then
	assert.Equal(t, []string{"page", "bafyreidangling"}, docObjectTypes(t, data),
		"a resolvable id spells its type key; an unresolvable one passes through verbatim (§3)")
	assert.Equal(t, strList("typeid-page", "bafyreidangling"),
		got.Details.Fields["relationFormatObjectTypes"],
		"the translation must invert: ids in, ids out")
}

// Without the capability, entries pass through verbatim in both directions —
// the offline round trip is byte-exact, and nothing is invented or dropped.
//
// How this can fail: make relationTargetKeys drop entries it cannot
// translate (the §2a dangling-id policy, wrong here: the stored value IS the
// meaning) and the ids vanish from the wire.
func TestRelationEnvelope_TargetTypesPassThroughWithoutResolver(t *testing.T) {
	// given
	snap := relationSnapshot(map[string]*types.Value{
		"relationFormat":            num(100),
		"relationFormatObjectTypes": strList("bafyreitypeone", "wine"),
	})

	// when
	data, err := Marshal(model.SmartBlockType_STRelation, snap, testOptions())
	require.NoError(t, err)
	_, got, err := Unmarshal(data, testOptions())
	require.NoError(t, err)

	// then
	assert.Equal(t, []string{"bafyreitypeone", "wine"}, docObjectTypes(t, data))
	assert.Equal(t, strList("bafyreitypeone", "wine"),
		got.Details.Fields["relationFormatObjectTypes"])
}

// blankTypeResolver answers every translation with ("", true) — the shape a
// resolver bug really produces when its map carries an entry whose value was
// never filled (storeresolver builds keyById from a bounded space query, and
// a type row with an empty stored key lands exactly this). The same defect
// class already bit the property seam once: a vocabulary resolving a slug to
// "" put details[""] in the store (TestImport_SeamRefusesAnUnwritableResolvedKey).
type blankTypeResolver struct{ *testPropertyResolver }

func (blankTypeResolver) TypeKeyById(id string) (string, bool)  { return "", true }
func (blankTypeResolver) TypeIdByKey(key string) (string, bool) { return "", true }

// The TypeResolver contract for an EMPTY answer: ok-with-"" is NO answer, in
// both directions. The empty string has no written form (§3) and is no
// store address either, so trusting the ok flag alone would let the
// translation DESTROY the value it was asked to translate — export would put
// "" in a type-key slot where the stored id was, import would store "" where
// the key was — when the §2d rule is that an entry the resolver cannot
// answer passes through verbatim, its own address, for the wiring to
// reconcile. Pass-through, never a refusal: the stored value IS the meaning,
// and a backup format that loses it to a resolver bug is disqualifying.
//
// How this can fail: drop `key != ""` from relationTargetKeys' TypeKeyById
// arm and the wire carries "" where the stored id was; drop `resolved != ""`
// from applyRelationEnvelope's TypeIdByKey arm and the store receives ""
// where the key was. Each drop is silent on every other test — the working
// resolvers never answer ok-with-empty.
func TestRelationEnvelope_AResolverAnsweringEmptyIsNoAnswer(t *testing.T) {
	// given
	opts := testOptions()
	opts.ResolveProperties = blankTypeResolver{newTestPropertyResolver()}
	snap := relationSnapshot(map[string]*types.Value{
		"relationFormat":            num(100),
		"relationFormatObjectTypes": strList("bafyreitypeone"),
	})

	// when
	data, err := Marshal(model.SmartBlockType_STRelation, snap, opts)
	require.NoError(t, err)
	_, got, err := Unmarshal(data, opts)
	require.NoError(t, err)

	// then
	assert.Equal(t, []string{"bafyreitypeone"}, docObjectTypes(t, data),
		"export: ok-with-empty is no translation — the stored id is its own address (§3)")
	assert.Equal(t, strList("bafyreitypeone"),
		got.Details.Fields["relationFormatObjectTypes"],
		"import: ok-with-empty is no translation — the key passes through verbatim")
}

// A reference slot that legitimately NAMES a lifted key — a dataview column
// on the Property type, a type_properties entry — keeps the §3 slug: the
// deny rule protects the legend, and a bundled-bound slug needs no legend
// entry, so the rule never sees it. 64 production spaces carry exactly this
// document.
//
// How this can fail: restore writableSlug's blanket deny refusal and the
// column spells "relationFormat" camelCase-verbatim with a warning.
func TestRelationEnvelope_ReferenceSlotsKeepTheBundledSlug(t *testing.T) {
	// given a set over relation objects, showing the format column — the
	// Property type's own view
	snap := dataviewSnapshot(&model.RelationLink{
		Key: "relationFormat", Format: model.RelationFormat_number,
	})

	// when
	var warns []Issue
	opts := testOptions()
	opts.OnWarning = func(i Issue) { warns = append(warns, i) }
	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)

	// then
	assert.Contains(t, string(data), `"key": "relation_format"`,
		"naming the relation is not writing its value — the reference keeps its slug")
	assert.NotContains(t, string(data), `"relationFormat"`,
		"the verbatim fallback is for keys whose slug would need a legend entry")
	assert.NotContains(t, string(data), `"property_keys"`,
		"a bundled binding needs no legend entry — that is what makes the slug safe")
	assert.Empty(t, warns)
	require.NoError(t, Validate(data), "§11 I1")
}

// The denied-key exemption is TWO questions, and this pins the first: the
// bundled table must BIND the slug to this very key. Slugs come from
// apiObjectKey — user-editable — so a space really can spell `relationFormat`
// with a slug of its own, and that slug inverts through the vocabulary in
// force while the bundled table has never heard of it. Such a slug OWES a
// legend entry (recordPropertyKey's rule: a spelling the bundled table does
// not bind always does), and the entry's value would be the denied key, which
// the §3 deny rule refuses — so the document would spell `fmt_col` with
// nothing anywhere saying what it means, and every reader would resolve it
// verbatim (chain step 4) as a custom property named fmt_col: a silent
// repoint of the reference. The stored key is the one spelling that needs no
// entry, so it is the one written. The safe population the exemption serves
// (64 production spaces showing the Property type's format column) is
// bundled-bound by construction, so backing THIS slug off costs it nothing.
//
// How this can fail: drop the bundledBinds half from writableSlug's
// denied-key exemption. termInverts alone accepts this slug — the writer's
// own space does invert it — and the column spells "fmt_col" with no legend
// entry possible for it.
func TestRelationEnvelope_ADeniedKeySlugTheBundledTableDoesNotBindBacksOff(t *testing.T) {
	// given the Property type's format column, under a vocabulary that
	// spells the lifted key with a space-minted slug of its own
	snap := dataviewSnapshot(&model.RelationLink{
		Key: "relationFormat", Format: model.RelationFormat_number,
	})
	var warns []Issue
	opts := testOptions()
	opts.Keys = typedSpaceVocabulary{propSlugOf: map[string]string{"relationFormat": "fmt_col"}}
	opts.OnWarning = func(i Issue) { warns = append(warns, i) }

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)

	// then
	assert.Contains(t, string(data), `"key": "relationFormat"`,
		"a slug that would owe an unwritable legend entry backs off to the stored key (§3)")
	assert.NotContains(t, string(data), "fmt_col")
	assert.NotContains(t, string(data), `"property_keys"`,
		"the denied key can never be a legend value — that is why the slug had to go")
	require.NotEmpty(t, warns, "the backed-off spelling is reported")
	assert.Contains(t, warns[0].Message, "cannot be a legend value")
	require.NoError(t, Validate(data), "§11 I1")
}

// …and this pins the second question: the vocabulary IN FORCE must invert
// the slug. The bundled table binds `relation_format` to relationFormat, but
// the writer's own space is a reader too — its vocabulary answers FIRST on
// import, ahead of the bundled table — and here a custom relation has
// claimed the api key `relation_format` for itself (the freed-spelling
// hazard recordPropertyKey documents: measured on the property namespace,
// the same shadowing silently lands dueDate's value on the custom relation
// that wanted the spelling). Writing the slug would re-point the column to
// the shadowing relation the moment the document is read back where it was
// written, and no legend entry can correct it, because the entry's value
// would be the denied key. Only the verbatim stored key — always its own
// address (§3) — survives that reader.
//
// How this can fail: drop the termInverts half from writableSlug's
// denied-key exemption. bundledBinds alone accepts the slug, the column
// spells "relation_format", and the writer's own vocabulary binds it to the
// shadowing custom relation — a repoint with no error anywhere.
func TestRelationEnvelope_ADeniedKeySlugShadowedByTheVocabularyBacksOff(t *testing.T) {
	// given the same format column, under a vocabulary where a custom
	// relation holds the bundled slug of the lifted key
	snap := dataviewSnapshot(&model.RelationLink{
		Key: "relationFormat", Format: model.RelationFormat_number,
	})
	var warns []Issue
	opts := testOptions()
	opts.Keys = typedSpaceVocabulary{
		propSlugOf: map[string]string{"64af1efbc52a6a5ed6e9dabc": "relation_format"}}
	opts.OnWarning = func(i Issue) { warns = append(warns, i) }

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)

	// then
	assert.Contains(t, string(data), `"key": "relationFormat"`,
		"the writer's own space would re-point the slug, so the stored key is the honest spelling")
	assert.NotContains(t, string(data), "relation_format")
	require.NotEmpty(t, warns, "the backed-off spelling is reported")
	assert.Contains(t, warns[0].Message, "cannot be a legend value")
	require.NoError(t, Validate(data), "§11 I1")
}

// The target keys are in the TYPE-KEY CENSUS: verbatim-first (§3) makes each
// its own address, so no other key's slug may take one as a spelling — the
// same duty §2a's targets have carried since typeProperties shipped.
//
// How this can fail: drop the relationTargetKeys loop from
// seedTypeTermLedger. A vocabulary that slugs the document's own TYPE onto a
// target key's spelling then wins the term, the envelope `type` and a target
// entry both spell "wine", and the legend binds the spelling to the wrong
// key — a type substitution with no error anywhere.
func TestRelationEnvelope_TargetKeysAreInTheTypeCensus(t *testing.T) {
	// given a vocabulary spelling the relation's own type key as `wine`,
	// while the relation targets the stored type key `wine`
	snap := relationSnapshot(map[string]*types.Value{
		"relationFormat":            num(100),
		"relationFormatObjectTypes": strList("wine"),
	})
	snap.ObjectTypes = []string{"ot-custom123"}
	opts := testOptions()
	opts.Keys = typedSpaceVocabulary{typeSlugOf: map[string]string{"custom123": "wine"}}

	// when
	data, err := Marshal(model.SmartBlockType_STRelation, snap, opts)
	require.NoError(t, err)

	// then: the stored key `wine` kept its spelling, so the vocabulary's
	// binding backed off to the verbatim key
	var doc struct {
		Type        string   `json:"type"`
		ObjectTypes []string `json:"object_types"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "custom123", doc.Type,
		"the census reserved %q for the target, so the type spells its stored key", "wine")
	assert.Equal(t, []string{"wine"}, doc.ObjectTypes)
}

// docObjectTypes reads the envelope object_types out of a rendered document.
func docObjectTypes(t *testing.T, data []byte) []string {
	t.Helper()
	var doc struct {
		ObjectTypes []string `json:"object_types"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	return doc.ObjectTypes
}

// A custom stored type key in `object_types` owes the type_keys legend its
// identity entry, exactly as the same key would in
// type_properties[].object_types — the §2d slot is a type-key slot, not a
// free string.
//
// How this can fail: write the entries raw instead of through typeSlugs and
// the legend entry disappears — a reader whose vocabulary binds the spelling
// elsewhere then resolves the target to a different type.
func TestRelationEnvelope_TargetTypesOweTheTypeLegend(t *testing.T) {
	// given a custom stored key whose spelling the bundled table cannot invert
	snap := relationSnapshot(map[string]*types.Value{
		"relationFormat":            num(100),
		"relationFormatObjectTypes": strList("wine"),
	})

	// when
	data, err := Marshal(model.SmartBlockType_STRelation, snap, testOptions())
	require.NoError(t, err)

	// then
	var doc struct {
		TypeKeys map[string]string `json:"type_keys"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "wine", doc.TypeKeys["wine"],
		"a verbatim custom type key owes the identity entry (§3)")
}

// `include_time` against a non-date format and a non-empty `object_types`
// against a non-objects/files format WARN and stay valid: the stored details
// are not authored, so a refusal would make Marshal emit what Validate
// rejects (I1). Only a MEANINGFUL value warns — a false or empty one against
// the wrong format is most of the corpus (8,375 present-and-false
// include_time alone) and says nothing an author could act on.
//
// How this can fail: drop the relationEnvelopeIssues call from
// semanticIssues and the warnings vanish; warn on any presence and the
// no-warning cases light up.
func TestRelationEnvelope_WrongFormatWarnsButCarries(t *testing.T) {
	for name, tc := range map[string]struct {
		doc      string
		wantWarn string // "" = no warning expected
	}{
		"include_time true on number": {
			doc:      `{"version":1,"kind":"relation","id":"o1","key":"b","format":"number","include_time":true}`,
			wantWarn: "/include_time",
		},
		"object_types non-empty on number": {
			doc:      `{"version":1,"kind":"relation","id":"o1","key":"b","format":"number","object_types":["page"]}`,
			wantWarn: "/object_types",
		},
		"include_time false on number": {
			doc: `{"version":1,"kind":"relation","id":"o1","key":"b","format":"number","include_time":false}`,
		},
		"object_types empty on number": {
			doc: `{"version":1,"kind":"relation","id":"o1","key":"b","format":"number","object_types":[]}`,
		},
		"include_time true on date": {
			doc: `{"version":1,"kind":"relation","id":"o1","key":"b","format":"date","include_time":true}`,
		},
		"object_types non-empty on objects": {
			doc: `{"version":1,"kind":"relation","id":"o1","key":"b","format":"objects","object_types":["page"]}`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			// when
			var warns []Issue
			err := ValidateWarn([]byte(tc.doc), func(i Issue) { warns = append(warns, i) })

			// then
			require.NoError(t, err, "a warning-grade fault must not refuse the document (§2d)")
			if tc.wantWarn == "" {
				assert.Empty(t, warns)
				return
			}
			require.Len(t, warns, 1)
			assert.Equal(t, tc.wantWarn, warns[0].Path)
			assert.Contains(t, warns[0].Message, "only meaningful on")
		})
	}
}

// A `properties` member spelling one of the three FIELD names on a relation
// document warns: it is a custom property named "format", not the relation's
// format — the phantom-property shape the 9-of-9 eval failures wrote, which
// with the envelope field ALSO present would otherwise validate in silence.
// A warning and not a refusal, because the spelling is a legitimate custom
// key and a relation object carrying one must stay exportable (I1).
//
// How this can fail: drop the properties-member loop from
// relationEnvelopeIssues and the phantom shape validates with no notice;
// make it a refusal and the page case (where the member is an ordinary
// property) starts failing I1 for spaces that really have one. The members
// run one at a time, because relationPhantomIssues lists the three
// literally and a member dropped from that list keeps the other two
// warning — the 9-of-9 eval failures happened to write `format`, but the
// bug they demonstrate (a §2d field name is an ordinary spelling inside
// `properties`) is a property of the container, so all three members walk
// into it the same way.
func TestRelationEnvelope_PhantomFieldNameInPropertiesWarns(t *testing.T) {
	for member, value := range map[string]string{
		"format":       `"number"`,
		"include_time": `true`,
		"object_types": `["vinyl"]`,
	} {
		t.Run(member, func(t *testing.T) {
			// given the envelope format AND the phantom twin in properties
			doc := `{"version":1,"kind":"relation","id":"o1","key":"b","format":"number",` +
				`"properties":{"name":"Budget","` + member + `":` + value + `}}`

			// when
			var warns []Issue
			err := ValidateWarn([]byte(doc), func(i Issue) { warns = append(warns, i) })

			// then
			require.NoError(t, err, "a custom property named %s is legal — the warning is the guard", member)
			require.Len(t, warns, 1)
			assert.Equal(t, "/properties/"+member, warns[0].Path)
			assert.Contains(t, warns[0].Message, "CUSTOM property")

			// and on a PAGE the same member is an ordinary property: no warning
			warns = nil
			page := `{"version":1,"id":"o1","properties":{"` + member + `":` + value + `}}`
			require.NoError(t, ValidateWarn([]byte(page), func(i Issue) { warns = append(warns, i) }))
			assert.Empty(t, warns)
		})
	}
}

// The three fields are legal only on kind:relation, and `format` is required
// there — the schema conditional, with the messages naming the rule rather
// than the mechanism.
//
// How this can fail: remove the allOf conditional from object.schema.json
// and every case below validates clean; remove the schemaIssueMessage arm
// and the off-relation refusals degrade to a bare "not allowed".
func TestRelationEnvelope_FieldsAreGatedByKind(t *testing.T) {
	for name, tc := range map[string]struct{ doc, want string }{
		"format on a page": {
			doc:  `{"version":1,"id":"o1","format":"number"}`,
			want: `/format: property "format" is only valid on relation documents`,
		},
		"include_time on a template": {
			doc:  `{"version":1,"kind":"template","id":"o1","type":"template","include_time":true}`,
			want: `/include_time: property "include_time" is only valid on relation documents`,
		},
		"object_types on a type": {
			doc:  `{"version":1,"kind":"object_type","id":"o1","object_types":["page"]}`,
			want: `/object_types: property "object_types" is only valid on relation documents`,
		},
		"missing format on a relation": {
			doc:  `{"version":1,"kind":"relation","id":"o1","key":"b"}`,
			want: "missing property 'format': a relation document states the format",
		},
		"legacy relation_format beside a missing format": {
			doc:  `{"version":1,"kind":"relation","id":"o1","key":"b","properties":{"relation_format":100}}`,
			want: "the pre-v0.31 form",
		},
	} {
		t.Run(name, func(t *testing.T) {
			err := Validate([]byte(tc.doc))
			require.Error(t, err)
			assert.Contains(t, err.Error(), tc.want)

			_, _, err = Unmarshal([]byte(tc.doc), testOptions())
			assert.Error(t, err, "Unmarshal must refuse what Validate refuses (§12 I2)")
		})
	}
}

// A non-relation snapshot carrying any of the three details drops them with
// a warning: the refusal in `properties` is unconditional, so export may not
// write the flat spelling anywhere (I1), and no §2d field exists off a
// relation document to carry the value. Never observed in production — 0 of
// 27,444 non-relation documents carry any of the three — which is exactly
// why the warning is the whole guard: when the shape does appear, the drop
// is the only trace the value ever existed, so it must be per KEY, not per
// document. Each key runs alone, because the reporting loop in
// buildRelationEnvelope lists the three literally and a key dropped from
// that list keeps the other two warning.
//
// How this can fail: make envelopeLiftedKeys include the three keys only on
// relation documents and the page export writes the flat spelling into
// properties — a document its own Validate refuses; or drop one key from
// buildRelationEnvelope's off-relation reporting loop and that key's value
// vanishes in silence while the other two still warn.
func TestRelationEnvelope_NonRelationKindDropsTheDetails(t *testing.T) {
	for name, tc := range map[string]struct {
		storedKey string
		value     *types.Value
		flat      string
		field     string
	}{
		"relationFormat": {
			storedKey: "relationFormat", value: num(2),
			flat: `"relation_format"`, field: `"format"`,
		},
		"relationFormatIncludeTime": {
			storedKey: "relationFormatIncludeTime", value: boolValue(true),
			flat: `"relation_format_include_time"`, field: `"include_time"`,
		},
		"relationFormatObjectTypes": {
			storedKey: "relationFormatObjectTypes", value: strList("typeid-page"),
			flat: `"relation_format_object_types"`, field: `"object_types"`,
		},
	} {
		t.Run(name, func(t *testing.T) {
			// given
			snap := trimSnapshot(map[string]*types.Value{
				tc.storedKey: tc.value,
				"name":       str("A page, oddly stamped"),
			})

			// when
			var warns []Issue
			opts := testOptions()
			opts.OnWarning = func(i Issue) { warns = append(warns, i) }
			data, err := Marshal(model.SmartBlockType_Page, snap, opts)
			require.NoError(t, err)

			// then
			assert.NotContains(t, string(data), tc.flat)
			assert.NotContains(t, string(data), tc.field,
				"a page has no §2d field to lift into")
			require.NoError(t, Validate(data), "§11 I1")
			require.Len(t, warns, 1, "the warning is the only trace of the dropped value")
			assert.Contains(t, warns[0].Message, "not a relation document")
			assert.Contains(t, warns[0].Message, tc.storedKey,
				"the warning must name which detail was dropped")
		})
	}
}

// "text" resolves per key on the way back in, exactly as a type_properties
// entry's format does (§3): the relation's own envelope `key` disambiguates,
// so a bundled short-text relation keeps its stored format across a round
// trip even though the document never spells shorttext.
//
// How this can fail: map the envelope format name blindly through
// formatNames.value in applyRelationEnvelope — "text" then lands longtext on
// every relation, and the bundled `name` relation comes back reformatted.
func TestRelationEnvelope_TextFoldResolvesPerKey(t *testing.T) {
	// given the bundled `name` relation, stored shorttext
	snap := relationSnapshot(map[string]*types.Value{
		"relationFormat": num(float64(model.RelationFormat_shorttext)),
	})
	snap.Key = "name"

	// when
	data, err := Marshal(model.SmartBlockType_STRelation, snap, testOptions())
	require.NoError(t, err)
	_, got, err := Unmarshal(data, testOptions())
	require.NoError(t, err)

	// then
	assert.Contains(t, string(data), `"format": "text"`,
		"shorttext has no name of its own (§3)")
	assert.Equal(t, float64(model.RelationFormat_shorttext),
		got.Details.Fields["relationFormat"].GetNumberValue(),
		"the bundled key resolves the fold — shorttext survives the trip")
}

// Export ∘ Import is byte-stable over a relation document (§11 guarantee 2).
//
// How this can fail: any asymmetry between buildRelationEnvelope and
// applyRelationEnvelope — a field written that import drops, or one import
// rewrites into a different value — shows up as a byte diff on the second
// export.
func TestRelationEnvelope_ExportImportIsByteStable(t *testing.T) {
	// given
	snap := relationSnapshot(map[string]*types.Value{
		"relationFormat":            num(100),
		"relationFormatIncludeTime": boolValue(false),
		"relationFormatObjectTypes": strList("bafyreitypeone"),
	})

	// when
	first, err := Marshal(model.SmartBlockType_STRelation, snap, testOptions())
	require.NoError(t, err)
	sbType, got, err := Unmarshal(first, testOptions())
	require.NoError(t, err)
	second, err := Marshal(sbType, got, testOptions())
	require.NoError(t, err)

	// then
	assert.Equal(t, string(first), string(second))
}

// The published schema states the format vocabulary once —
// $defs/propertyFormat — and every slot that speaks it ($2a's typeProperty,
// §6.2's dataviewProperty, the §2d envelope) references that one list, which
// must equal formatNames exactly: the schema is what an external validator
// runs, and a name in one place but not the other is a document one side
// writes and the other refuses.
//
// How this can fail: add a format name to formatNames without the schema (or
// vice versa), or point one of the three slots at a private enum again.
func TestPropertyFormatEnum_MatchesFormatNames(t *testing.T) {
	// given the schema's own statement
	schemaNames := propertyFormatEnum()
	require.NotEmpty(t, schemaNames, "the schema must publish $defs/propertyFormat")

	want := map[string]bool{}
	for _, name := range formatNames.toName {
		want[name] = true
	}
	got := map[string]bool{}
	for _, name := range schemaNames {
		got[name] = true
	}
	assert.Equal(t, want, got)

	// and the three slots all reference the shared list
	var schema struct {
		Properties map[string]json.RawMessage `json:"properties"`
		Defs       map[string]struct {
			Properties map[string]json.RawMessage `json:"properties"`
		} `json:"$defs"`
	}
	require.NoError(t, json.Unmarshal(SchemaJSON(), &schema))
	for slot, raw := range map[string]json.RawMessage{
		"root format":             schema.Properties["format"],
		"typeProperty.format":     schema.Defs["typeProperty"].Properties["format"],
		"dataviewProperty.format": schema.Defs["dataviewProperty"].Properties["format"],
	} {
		assert.True(t, strings.Contains(string(raw), `"#/$defs/propertyFormat"`),
			"%s must reference the shared vocabulary, not restate it", slot)
	}
}

// A kind nothing emits is exactly the kind nobody thought to guard. Export
// writes neither `bundled_relation` nor `sub_object` — 0 of 38,061 corpus
// documents — but the schema's `kind` enum offers both beside `relation`
// with nothing marking them non-authorable, and a small model picked one:
// `{"kind":"bundled_relation", …, "properties":{"format":"number"}}`
// validated clean, with no warning, and imported as a phantom property with
// no relationFormat at all. That is verbatim the §2d bug, one kind over.
//
// Two kinds are deliberately NOT in the set. `relation_option` because an
// option document is a value, not a property definition, so `format` there
// is an ordinary custom key. `sub_object` because it is deprecated: a kind
// being retired must not pick up a new obligation in a format about to
// freeze, and 0 of 38,061 corpus documents carry it either way.
//
// How this can fail: narrow isRelationKind back to STRelation alone and the
// two side doors reopen; widen it to relation_option and the last case
// starts refusing a legitimate document.
func TestRelationEnvelope_TheSideDoorKindsAreGuardedToo(t *testing.T) {
	for kind, wantValid := range map[string]bool{
		"relation":         false,
		"bundled_relation": false,
		// `sub_object` is DEPRECATED and deliberately outside the set: a kind
		// on its way out must not acquire a new obligation in a format about
		// to freeze. It therefore accepts this shape in silence, like any
		// non-relation kind — that is a decision, not an oversight, and
		// re-widening it would be re-adopting a kind we are dropping.
		"sub_object":      true,
		"relation_option": true,
	} {
		t.Run(kind, func(t *testing.T) {
			// given the shape 9 of 9 small-model attempts wrote
			doc := []byte(`{"version":1,"kind":"` + kind + `","key":"eh",` +
				`"properties":{"name":"Estimated Hours","format":"number"}}`)

			// when
			err := Validate(doc)

			// then
			if wantValid {
				assert.NoError(t, err, "an option document is a value, not a property definition")
				return
			}
			require.Error(t, err, "a relation document must state its own format (§2d)")
			assert.Contains(t, err.Error(), "missing property 'format'")
		})
	}
}

// Told only that a member is MISSING, an author has no reason to connect
// that to the member they did write. The warning that would say so lives in
// the semantic pass, and a schema failure never reaches it — so the verdict
// that does run has to name the wrong container itself.
//
// How this can fail: drop the phantom clause from relationFormatSlotIssue
// and the commonest authoring mistake in the corpus of small-model attempts
// gets a message that never mentions the line it is about.
func TestRelationEnvelope_TheMissingFormatVerdictNamesTheWrongContainer(t *testing.T) {
	t.Run("format written into properties", func(t *testing.T) {
		// given
		doc := []byte(`{"version":1,"kind":"relation","key":"eh",` +
			`"properties":{"name":"Estimated Hours","format":"number"}}`)

		// when
		err := Validate(doc)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), `spells "format" inside properties`)
		assert.Contains(t, err.Error(), "move that member out to the envelope")
	})

	t.Run("the pre-v0.31 spelling still gets its own hint", func(t *testing.T) {
		// given
		doc := []byte(`{"version":1,"kind":"relation","key":"eh",` +
			`"properties":{"name":"Estimated Hours","relation_format":2}}`)

		// when
		err := Validate(doc)

		// then
		require.Error(t, err)
		assert.Contains(t, err.Error(), "pre-v0.31")
	})
}

// The phantom warning is the SEMANTIC half of the guard, and the semantic
// pass is the only place isRelationKind is load-bearing: for a missing
// `format` the schema's own kind conditional already refuses, so a test
// there passes whether or not the Go gate agrees. This document is valid —
// it has its envelope format — so nothing but isRelationKind decides
// whether the phantom member is reported.
//
// How this can fail: narrow isRelationKind back to STRelation alone and the
// two side-door kinds go quiet again.
func TestRelationEnvelope_ThePhantomWarningReachesTheSideDoorKinds(t *testing.T) {
	for _, kind := range []string{"relation", "bundled_relation"} {
		t.Run(kind, func(t *testing.T) {
			// given a VALID relation document that also carries the member
			doc := []byte(`{"version":1,"kind":"` + kind + `","key":"eh","format":"number",` +
				`"properties":{"name":"Estimated Hours","format":"number"}}`)

			// when
			var warned []Issue
			err := ValidateWarn(doc, func(i Issue) { warned = append(warned, i) })

			// then
			require.NoError(t, err, "the envelope format is present, so this document stands")
			require.Len(t, warned, 1, "the phantom member must be reported on every kind that IS a relation")
			assert.Equal(t, "/properties/format", warned[0].Path)
		})
	}
}

// A relation-shaped snapshot on a legacy kind must round-trip like any
// other. The schema requires `format` on all three relation kinds, so the
// export gate has to lift for all three or Marshal emits a document its own
// Validate rejects — which is precisely what happened when the document
// side was widened alone.
//
// Zero of these kinds come out of a live store, which is why the narrow gate
// went unnoticed. cmd/anyblockrecover reads arbitrary pb backups, where a
// relation on the legacy `sub_object` kind is the thing a recovery is for.
//
// How this can fail: narrow isRelationDoc back to STRelation and the last
// two kinds emit no format, fail their own Validate, and lose the detail.
func TestRelationEnvelope_EveryRelationKindRoundTripsLossless(t *testing.T) {
	for _, sb := range []model.SmartBlockType{
		model.SmartBlockType_STRelation,
		model.SmartBlockType_BundledRelation,
	} {
		t.Run(sb.String(), func(t *testing.T) {
			// given
			snap := relationSnapshot(map[string]*types.Value{
				"relationFormat":            num(4),
				"relationFormatIncludeTime": boolValue(true),
			})

			// when
			data, err := Marshal(sb, snap, Options{})
			require.NoError(t, err)
			require.NoError(t, Validate(data), "Marshal never emits what Validate rejects (§11 I1)")
			_, back, err := Unmarshal(data, Options{})
			require.NoError(t, err)

			// then
			assert.Contains(t, string(data), `"format": "date"`)
			assert.Equal(t, num(4).String(),
				back.GetDetails().GetFields()["relationFormat"].String(),
				"the definition survives on every kind that IS a relation")
		})
	}
}
