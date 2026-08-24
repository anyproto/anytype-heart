package anyblockjson

// relationformat_test.go — the §2d relation-definition envelope fields:
// `format`, `include_time`, `object_types` on kind:relation documents, the
// refusal of their flat spellings in `properties`, and the presence-mirror
// round trip.

import (
	"encoding/json"
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
// property) starts failing I1 for spaces that really have one.
func TestRelationEnvelope_PhantomFieldNameInPropertiesWarns(t *testing.T) {
	// given the envelope field AND its phantom twin in properties
	doc := `{"version":1,"kind":"relation","id":"o1","key":"b","format":"number",` +
		`"properties":{"name":"Budget","format":"number"}}`

	// when
	var warns []Issue
	err := ValidateWarn([]byte(doc), func(i Issue) { warns = append(warns, i) })

	// then
	require.NoError(t, err, "a custom property named format is legal — the warning is the guard")
	require.Len(t, warns, 1)
	assert.Equal(t, "/properties/format", warns[0].Path)
	assert.Contains(t, warns[0].Message, "CUSTOM property")

	// and on a PAGE the same member is an ordinary property: no warning
	warns = nil
	page := `{"version":1,"id":"o1","properties":{"format":"vinyl"}}`
	require.NoError(t, ValidateWarn([]byte(page), func(i Issue) { warns = append(warns, i) }))
	assert.Empty(t, warns)
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
// 27,444 non-relation documents carry any of the three.
//
// How this can fail: make envelopeLiftedKeys include the three keys only on
// relation documents and the page export writes `relation_format` into
// properties — a document its own Validate refuses.
func TestRelationEnvelope_NonRelationKindDropsTheDetails(t *testing.T) {
	// given
	snap := trimSnapshot(map[string]*types.Value{
		"relationFormat": num(2),
		"name":           str("A page, oddly stamped"),
	})

	// when
	var warns []Issue
	opts := testOptions()
	opts.OnWarning = func(i Issue) { warns = append(warns, i) }
	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)

	// then
	assert.NotContains(t, string(data), `"relation_format"`)
	assert.NotContains(t, string(data), `"format"`,
		"a page has no §2d field to lift into")
	require.NoError(t, Validate(data), "§11 I1")
	require.Len(t, warns, 1)
	assert.Contains(t, warns[0].Message, "not a relation document")
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
// `relation_option` is deliberately NOT in the set: an option document is a
// value, not a property definition, so `format` there is an ordinary custom
// key.
//
// How this can fail: narrow isRelationKind back to STRelation alone and the
// two side doors reopen; widen it to relation_option and the last case
// starts refusing a legitimate document.
func TestRelationEnvelope_TheSideDoorKindsAreGuardedToo(t *testing.T) {
	for kind, wantValid := range map[string]bool{
		"relation":         false,
		"bundled_relation": false,
		"sub_object":       false,
		"relation_option":  true,
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
	for _, kind := range []string{"relation", "bundled_relation", "sub_object"} {
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
		model.SmartBlockType_SubObject,
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
