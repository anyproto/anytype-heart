package anyblockjson

// typesettings_test.go — the §2a type_settings group: the five settings
// lifted from `properties`, the kind-scoped refusal of their flat spellings,
// the install-provenance drops, and the migration story off the pre-v0.32
// root `type_properties`.

import (
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// settingsTypeSnapshot is a type snapshot carrying every lifted setting in
// its stored shape, plus the install provenance the document must not carry.
func settingsTypeSnapshot() *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Key: "use_case",
		Details: fields(map[string]*types.Value{
			"id":                str("t1"),
			"name":              str("Use Case"),
			"recommendedLayout": num(float64(model.ObjectType_basic)),
			"apiObjectKey":      str("use_case"),
			"pluralName":        str("Use Cases"),
			"defaultTemplateId": strList("bafyreitemplate"),
			"defaultViewType":   num(float64(model.BlockContentDataviewView_Table)),
			// the provenance block (§2a): each admitted to the drop
			// individually, see typeProvenanceKeys
			"layout":          num(float64(model.ObjectType_objectType)),
			"resolvedLayout":  num(float64(model.ObjectType_objectType)),
			"smartblockTypes": strList("16"),
			"sourceObject":    strList("_otuse_case"),
			"origin":          num(7),
			"addedDate":       num(0),
			"revision":        num(3),
			"setOf":           strList("bafyreinothing"),
		}),
		Blocks: []*model.Block{{
			Id:      "t1",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}},
		}},
	}
}

// The five stored settings travel in the group, in their §3 spellings, and
// never in `properties` — decision 5's example, rendered.
//
// How this can fail: drop typeSettingsLiftedDetailKeys from
// envelopeLiftedKeys (the flat spellings reappear in properties, and the
// document fails its own validation there), or drop a member from
// buildTypeSettings (the fact vanishes).
func TestTypeSettings_LiftsTheFiveSettings(t *testing.T) {
	// when
	data, err := Marshal(model.SmartBlockType_STType, settingsTypeSnapshot(), testOptions())
	require.NoError(t, err)

	// then
	for _, want := range []string{
		`"layout": "basic"`,
		`"api_key": "use_case"`,
		`"plural_name": "Use Cases"`,
		`"default_template": "bafyreitemplate"`,
		`"default_view": "table"`,
	} {
		assert.Contains(t, string(data), want)
	}
	var doc struct {
		Properties map[string]any `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	for _, flat := range []string{"recommended_layout", "api_object_key", "plural_name",
		"default_template_id", "default_view_type"} {
		_, has := doc.Properties[flat]
		assert.Falsef(t, has, "%s must not survive in properties — the group carries it (§2a)", flat)
	}
	require.NoError(t, Validate(data), "Marshal never emits what Validate rejects (§11 I1)")
}

// A type document does not carry its own install provenance: the eight
// admitted keys are omitted on export and dropped on import, silently, the
// transientProperties policy scoped by kind — and on every OTHER kind the
// same keys are ordinary properties and survive.
//
// How this can fail: remove a key from typeProvenanceKeys (it reappears in
// the type document), or scope the drop wider than isTypeSmartBlock (the
// page case loses its origin).
func TestTypeSettings_ProvenanceIsDroppedOnTypeDocumentsOnly(t *testing.T) {
	t.Run("omitted on export of a type document", func(t *testing.T) {
		data, err := Marshal(model.SmartBlockType_STType, settingsTypeSnapshot(), testOptions())
		require.NoError(t, err)
		var doc struct {
			Properties map[string]any `json:"properties"`
		}
		require.NoError(t, json.Unmarshal(data, &doc))
		for _, slug := range []string{"layout", "resolved_layout", "smartblock_types",
			"source_object", "origin", "added_date", "set_of"} {
			_, has := doc.Properties[slug]
			assert.Falsef(t, has, "%s describes the install, not the type (§2a)", slug)
		}
	})

	t.Run("dropped on import of a type document", func(t *testing.T) {
		doc := `{"version":1,"kind":"object_type","id":"t1","key":"k",
			"properties":{"name":"T","origin":7,"set_of":["bafyreinothing"],"revision":3}}`
		_, snap, err := Unmarshal([]byte(doc), testOptions())
		require.NoError(t, err, "a document carrying install provenance is stale, not wrong")
		for _, key := range []string{"origin", "setOf"} {
			assert.Nilf(t, snap.Details.Fields[key], "%s is dropped on a type document", key)
		}
	})

	t.Run("kept everywhere else", func(t *testing.T) {
		snap := trimSnapshot(map[string]*types.Value{
			"name":   str("A page"),
			"origin": num(1),
			"setOf":  strList("bafyreitype"),
		})
		data, err := Marshal(model.SmartBlockType_Page, snap, testOptions())
		require.NoError(t, err)
		assert.Contains(t, string(data), `"origin"`, "on a page, origin is real provenance")
		assert.Contains(t, string(data), `"set_of"`, "on a set, setOf is the collection's meaning")
	})
}

// The flat spellings of the five settings are refused in `properties` ON
// TYPE DOCUMENTS, by Validate and by Unmarshal (§12 I2), with the group
// repair named — and accepted everywhere else, because the lift is
// kind-scoped: apiObjectKey is real data on 9,725 relation documents.
//
// How this can fail: drop the isTypeSmartBlock/isTypeKind gates and the
// relation case starts refusing real documents; drop the refusal itself and
// a type document gets two legal spellings for one fact.
func TestTypeSettings_FlatSpellingsAreRefusedOnTypeDocumentsOnly(t *testing.T) {
	t.Run("refused on a type document", func(t *testing.T) {
		doc := `{"version":1,"kind":"object_type","id":"t1","key":"k",
			"properties":{"plural_name":"Tasks"}}`
		err := Validate([]byte(doc))
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/properties/plural_name")
		assert.Contains(t, err.Error(), "type_settings")

		_, _, err = Unmarshal([]byte(doc), testOptions())
		require.Error(t, err, "Unmarshal must refuse what Validate refuses (§12 I2)")
	})

	t.Run("accepted on a relation document", func(t *testing.T) {
		doc := `{"version":1,"kind":"relation","id":"r1","key":"budget",
			"relation_settings":{"format":"number"},
			"properties":{"name":"Budget","api_object_key":"budget"}}`
		require.NoError(t, Validate([]byte(doc)),
			"apiObjectKey is an ordinary property off a type document")
		_, snap, err := Unmarshal([]byte(doc), testOptions())
		require.NoError(t, err)
		assert.Equal(t, "budget", snap.Details.Fields["apiObjectKey"].GetStringValue())
	})
}

// The group is legal only on type kinds, and the older spellings get their
// migration hints: the root `type_properties` names its new home, the way
// the §2d root members name theirs.
//
// How this can fail: remove the type_settings arm from the schema's allOf
// (the page case validates clean); drop the migration hint from the
// root-member special case (the type_properties case degrades to a bare
// "not allowed").
func TestTypeSettings_GatedByKindWithMigrationHints(t *testing.T) {
	for name, tc := range map[string]struct{ doc, want string }{
		"type_settings on a page": {
			doc:  `{"version":1,"id":"o1","type_settings":{"layout":"basic"}}`,
			want: `/type_settings: property "type_settings" is only valid on type documents`,
		},
		"type_settings on a relation": {
			doc:  `{"version":1,"kind":"relation","id":"o1","key":"b","relation_settings":{"format":"number"},"type_settings":{}}`,
			want: `property "type_settings" is only valid on type documents`,
		},
		"the pre-v0.32 root type_properties": {
			doc:  `{"version":1,"kind":"object_type","id":"o1","key":"k","type_properties":[{"key":"due_date"}]}`,
			want: `property "type_properties" moved`,
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

// The settings round-trip onto the same stored keys, stored shapes included:
// the scalar default_template comes back as the one-entry LIST the store
// keeps, and the view/layout names come back as their numbers.
//
// How this can fail: write defaultTemplateId back as a scalar (the store's
// list readers see nothing), or map the names through the wrong enum.
func TestTypeSettings_RoundTripsOntoTheStoredKeys(t *testing.T) {
	// given
	snap := settingsTypeSnapshot()
	want := map[string]*types.Value{}
	for k := range typeSettingsLiftedDetailKeys() {
		want[k] = snap.Details.Fields[k]
	}

	// when
	data, err := Marshal(model.SmartBlockType_STType, snap, testOptions())
	require.NoError(t, err)
	_, got, err := Unmarshal(data, testOptions())
	require.NoError(t, err)

	// then
	for k, v := range want {
		assert.Equal(t, v, got.Details.Fields[k], "detail %q changed on the way round", k)
	}
}

// Export ∘ Import is byte-stable over a type document with every setting
// present (§11 guarantee 2).
//
// How this can fail: any asymmetry between buildTypeSettings and
// applyTypeSettings — a member written that import drops, or one import
// rewrites into a different value — shows up as a byte diff on the second
// export.
func TestTypeSettings_ExportImportIsByteStable(t *testing.T) {
	first, err := Marshal(model.SmartBlockType_STType, settingsTypeSnapshot(), testOptions())
	require.NoError(t, err)
	sbType, got, err := Unmarshal(first, testOptions())
	require.NoError(t, err)
	second, err := Marshal(sbType, got, testOptions())
	require.NoError(t, err)
	assert.Equal(t, string(first), string(second))
}

// The five members follow the §4 omit-empty canon — unlike the §2d members,
// which are the property's definition and mirror presence. A pluralName of
// "" (145 corpus docs) and a defaultTemplateId of [] (87) say nothing a
// reader could act on; DroppedEmptyTypeSetting is the comparator's half of
// exactly this rule.
//
// How this can fail: emit the members unconditionally (empty strings appear
// in the group), or narrow DroppedEmptyTypeSetting so the comparator starts
// reporting the documented omission as loss.
func TestTypeSettings_EmptySettingsAreOmitted(t *testing.T) {
	// given
	snap := settingsTypeSnapshot()
	snap.Details.Fields["pluralName"] = str("")
	snap.Details.Fields["defaultTemplateId"] = strList()

	// when
	data, err := Marshal(model.SmartBlockType_STType, snap, testOptions())
	require.NoError(t, err)

	// then
	assert.NotContains(t, string(data), `"plural_name"`)
	assert.NotContains(t, string(data), `"default_template"`)
	assert.True(t, DroppedEmptyTypeSetting(model.SmartBlockType_STType, "pluralName", str("")),
		"the comparator's predicate is the same rule")
	assert.False(t, DroppedEmptyTypeSetting(model.SmartBlockType_Page, "pluralName", str("")),
		"scoped to type documents, like the lift itself")
	assert.False(t, DroppedEmptyTypeSetting(model.SmartBlockType_STType, "pluralName", str("Tasks")),
		"a non-empty value is never this rule's business")
}

// A stored defaultTemplateId with a SECOND entry has no written form: the
// member is the one default template, only the first entry is written, and
// the drop is reported — 0 of 1,760 corpus type documents carry one, so the
// warning is the only trace when the shape does appear.
//
// How this can fail: emit the whole list (the schema's scalar member fails,
// I1), or drop the warning (the second entry vanishes in silence).
func TestTypeSettings_SecondDefaultTemplateWarns(t *testing.T) {
	// given
	snap := settingsTypeSnapshot()
	snap.Details.Fields["defaultTemplateId"] = strList("bafyreione", "bafyreitwo")
	var warns []Issue
	opts := testOptions()
	opts.OnWarning = func(i Issue) { warns = append(warns, i) }

	// when
	data, err := Marshal(model.SmartBlockType_STType, snap, opts)
	require.NoError(t, err)

	// then
	assert.Contains(t, string(data), `"default_template": "bafyreione"`)
	assert.NotContains(t, string(data), "bafyreitwo")
	require.NotEmpty(t, warns)
	found := false
	for _, w := range warns {
		if w.Path == "/type_settings/default_template" {
			found = true
			assert.Contains(t, w.Message, "only the first is written")
		}
	}
	assert.True(t, found, "the drop must be reported at the member")
	require.NoError(t, Validate(data), "§11 I1")
}

// An unknown default_view NAME is refused like an unknown layout: a typo
// would import as a raw string onto a number-format detail, which every
// consumer reads with an int getter and silently sees as table. A raw
// NUMBER outside the enum still passes — a stored value round-trips as its
// number.
//
// The SCHEMA answers this now — `default_view` $refs the same viewType
// definition a view's own `type` does, so the two cannot drift — and the
// refusal names the vocabulary, which the semantic message did not.
//
// How this can fail: loosen either slot back to a bare string and a
// schema-driven generator emits a name the codec will refuse.
func TestTypeSettings_UnknownViewNameRefusedRawNumberPasses(t *testing.T) {
	err := Validate([]byte(`{"version":1,"kind":"object_type","id":"t1","key":"k",
		"type_settings":{"default_view":"Table"}}`))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "/type_settings/default_view")
	assert.Contains(t, err.Error(), "'table'", "the refusal names the vocabulary")

	_, snap, err := Unmarshal([]byte(`{"version":1,"kind":"object_type","id":"t1","key":"k",
		"type_settings":{"default_view":42}}`), testOptions())
	require.NoError(t, err)
	assert.Equal(t, float64(42), snap.Details.Fields["defaultViewType"].GetNumberValue())
}

// The snapshot comparator accepts the documented type-document
// normalizations — the provenance drops and the empty-setting omissions —
// and still reports everything else, through the format's own predicates.
//
// How this can fail: teach export a drop without the predicate (every type
// document in a corpus sweep lights up — the 1,344-false-failures miss), or
// scope the predicate wider than the drop (a real loss goes quiet).
func TestTypeSettings_ComparatorPredicatesMatchTheDrops(t *testing.T) {
	for key := range typeProvenanceKeys {
		assert.Truef(t, DroppedTypeProvenanceKey(model.SmartBlockType_STType, key),
			"%s is dropped on type documents and the comparator must know", key)
		assert.Falsef(t, DroppedTypeProvenanceKey(model.SmartBlockType_Page, key),
			"%s is real data off a type document", key)
	}
	assert.False(t, DroppedTypeProvenanceKey(model.SmartBlockType_STType, "name"),
		"the predicate covers the admitted keys and nothing else")
}

// `revision` is NOT provenance, and the difference is not cosmetic: it is the
// guard that stops SystemObjectReviser re-applying a bundled definition over
// a user's own.
//
// systemobjectreviser short-circuits on
// `bundleRevision <= localObject.GetInt64(revisionKey)`. An absent revision
// reads 0, so the guard stops firing, and buildDiffDetails then copies the
// BUNDLED values over the local ones for every key in systemObjectFilterKeys
// — name, pluralName, recommendedLayout, isHidden, relationMaxCount.
//
// Measured on 1,599 installed bundled type documents: 40 carry a local
// `name` the reviser would overwrite (key `relation` is locally "Relation",
// bundled "Property") and 36 a local plural name. Dropping revision reverts
// those renames on restore, silently.
//
// How this can fail: put "revision" back into typeProvenanceKeys and a type
// document stops carrying the marker that protects its own name.
func TestTypeSettings_RevisionIsNotProvenance(t *testing.T) {
	// given a type document carrying a revision
	doc := []byte(`{"version":1,"kind":"object_type","key":"task",
		"properties":{"name":"Task","revision":3},
		"type_settings":{"layout":"basic"}}`)

	// when
	require.NoError(t, Validate(doc))
	_, snap, err := Unmarshal(doc, Options{})
	require.NoError(t, err)

	// then
	assert.Contains(t, snap.GetDetails().GetFields(), "revision",
		"revision guards the type's name against the bundled reviser and must survive")
	assert.NotContains(t, typeProvenanceKeys, "revision",
		"it failed the §15 #12 admission test — the verdict is recorded beside the list")
}

// A definition that names no `key` has no identity. Four of four schema-only
// runs in the small-model authoring eval wrote exactly this — a type
// document with `"type": "podcast_episode"` and no `key` — and every one
// validated, imported and round-tripped with the type coming back nameless.
//
// It is a WARNING, not a refusal: §11 I1 forbids emitting what Validate
// rejects, and a snapshot's stored key is untrusted (the hostile corpus
// builds a type whose stored key is the empty string on purpose), so export
// must stay able to write one.
//
// How this can fail: drop definitionIdentityIssue and the keyless shape goes
// silent again; widen it past type and relation documents and an ordinary
// page starts warning about a key it never owed.
func TestDefinitionIdentity_AKeylessDefinitionWarns(t *testing.T) {
	for name, tc := range map[string]struct {
		doc  string
		want bool
	}{
		"type document with no key": {
			`{"version":1,"kind":"object_type","properties":{"name":"Podcast Episode"}}`, true},
		"relation document with no key": {
			`{"version":1,"kind":"relation","relation_settings":{"format":"number"},` +
				`"properties":{"name":"Episode"}}`, true},
		"type document WITH a key": {
			`{"version":1,"kind":"object_type","key":"podcast","properties":{"name":"Podcast"}}`, false},
		"an ordinary page owes no key": {
			`{"version":1,"kind":"page","properties":{"name":"A page"}}`, false},
	} {
		t.Run(name, func(t *testing.T) {
			// when
			var warned []Issue
			require.NoError(t, ValidateWarn([]byte(tc.doc), func(i Issue) { warned = append(warned, i) }))

			// then
			var got bool
			for _, w := range warned {
				if w.Path == "/key" {
					got = true
				}
			}
			assert.Equal(t, tc.want, got, "warnings: %v", warned)
		})
	}
}

// The space's own object holds the space's SETTINGS — its name, icon,
// homepage — not the space itself. `space_settings` says that; `workspace`
// said something the product no longer calls anything.
//
// It is a wire value in the `kind` enum, so it is free today and a version
// bump after the freeze: one per space, 77 in a 77-space corpus, all
// machine-written and never authored. The Go smartblock type is untouched —
// only the spelling moves.
//
// No backward compatibility, per §10: the draft has no external consumers,
// and the retired spelling is refused rather than quietly accepted, so a
// document written against the old vocabulary fails loudly instead of
// importing as something else.
//
// How this can fail: point kindNames back at "workspace" and the export
// spelling changes under a reader that expects the new one; widen the schema
// enum to accept both and the retired spelling stops failing.
func TestKind_TheSpacesOwnObjectSpellsItsSettings(t *testing.T) {
	// given the space's own object
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{Id: "bafyreispace",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		Details: fields(map[string]*types.Value{"id": str("bafyreispace"), "name": str("My space")}),
	}

	// when
	data, err := Marshal(model.SmartBlockType_Workspace, snap, testOptions())
	require.NoError(t, err)

	// then it spells the new name, and reads back as the same smartblock type
	require.NoError(t, Validate(data), "§11 I1")
	assert.Contains(t, string(data), `"kind": "space_settings"`)
	sbType, _, err := Unmarshal(data, testOptions())
	require.NoError(t, err)
	assert.Equal(t, model.SmartBlockType_Workspace, sbType,
		"only the spelling moves; the smartblock type is unchanged")

	// and the retired spelling is refused, not silently reinterpreted
	assert.Error(t, Validate([]byte(`{"version":1,"kind":"workspace","properties":{"name":"x"}}`)),
		"§10: no backward compatibility while the format is a draft — fail loudly")
}
