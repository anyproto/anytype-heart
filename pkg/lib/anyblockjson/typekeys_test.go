package anyblockjson

// The type namespace gets the same verbatim-first treatment as the property
// namespace (§3): a term that names a stored type key IS that key, the
// bundled slug table applies only to terms that are not stored keys, and the
// document carries its own inverse — the `type_keys` envelope legend —
// wherever the shipped table would give a package-only reader the wrong
// answer. Before the legend existed, a node-backed vocabulary slugging a
// custom type `69bbfc…` to `task` exported `"type": "task"` with nothing to
// invert it, and a package-only reader bound it to the bundled Task type — a
// different type, silently. Same for `type_properties[].object_types`.

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// customTypeKey is a space-minted (bson) type key of the shape real spaces
// produce for user-created types.
const customTypeKey = "69bbfc78877a91b1d12d1a7c"

// typedSpaceVocabulary is a node-backed vocabulary for BOTH namespaces: it
// knows the space's stored slugs for properties and for types, which the
// bundled table cannot.
type typedSpaceVocabulary struct {
	propSlugOf map[string]string
	typeSlugOf map[string]string
}

func (v typedSpaceVocabulary) PropertySlug(key string) string {
	if slug, ok := v.propSlugOf[key]; ok {
		return slug
	}
	return BundledKeyVocabulary{}.PropertySlug(key)
}

func (v typedSpaceVocabulary) PropertyKey(slug string) (string, bool) {
	for key, s := range v.propSlugOf {
		if s == slug {
			return key, true
		}
	}
	return BundledKeyVocabulary{}.PropertyKey(slug)
}

func (v typedSpaceVocabulary) TypeSlug(key string) string {
	if slug, ok := v.typeSlugOf[key]; ok {
		return slug
	}
	return BundledKeyVocabulary{}.TypeSlug(key)
}

func (v typedSpaceVocabulary) TypeKey(slug string) (string, bool) {
	for key, s := range v.typeSlugOf {
		if s == slug {
			return key, true
		}
	}
	return BundledKeyVocabulary{}.TypeKey(slug)
}

func typedSnapshot(objectTypes ...string) *model.SmartBlockSnapshotBase {
	return &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{Id: "o1",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		Details:     fields(map[string]*types.Value{"id": str("o1")}),
		ObjectTypes: objectTypes,
	}
}

type envelopeTypeDoc struct {
	Kind         string            `json:"kind"`
	Type         string            `json:"type"`
	TemplateFor  string            `json:"template_for"`
	PropertyKeys map[string]string `json:"property_keys"`
	TypeKeys     map[string]string `json:"type_keys"`
	Properties   map[string]any    `json:"properties"`
	TypeProps    []TypeProperty    `json:"type_properties"`
}

func decodeEnvelope(t *testing.T, data []byte) envelopeTypeDoc {
	t.Helper()
	var doc envelopeTypeDoc
	require.NoError(t, json.Unmarshal(data, &doc))
	return doc
}

// censusPropResolver serves two recommended-list entries, one of which
// buildTypeProperties drops: its definition carries no key, which real type
// objects hold whenever a vocabulary once resolved a spelling onto the empty
// key. Both name target types, and only the surviving one's target reaches
// the document.
type censusPropResolver struct{}

func (censusPropResolver) PropertyById(id string) (PropertyDefinition, bool) {
	switch id {
	case "k1":
		return PropertyDefinition{Key: "", Format: model.RelationFormat_object,
			ObjectTypes: []string{"cust"}}, true
	case "k2":
		return PropertyDefinition{Key: "owner", Format: model.RelationFormat_object,
			ObjectTypes: []string{"custom1"}}, true
	}
	return PropertyDefinition{}, false
}

func (censusPropResolver) PropertyId(def PropertyDefinition) (string, bool) {
	if def.Key == "owner" {
		return "k2", true
	}
	return "", false
}

// TestExport_IsAFixpointWhenTheCensusShrinks: exporting an object, importing
// it and exporting it again must produce the same document (§9 — "provided
// ids are preserved so re-exports diff cleanly" is worth nothing if the terms
// move instead).
//
// The census is what threatened it. It reserved every stored type key the
// SNAPSHOT named, while the document spells only the keys §2 models — one
// type, plus a template's target — and only the type properties export
// actually writes. Every key in the gap was reserved for a term no reader
// ever sees, and it backed a real slug off: generation 1 wrote the stored key
// verbatim, generation 2 — one round trip later, with the extra keys gone
// from the snapshot — wrote the slug and a legend line to invert it. Same
// object, two documents.
//
// Both halves of the gap are here. Neither needs a hostile vocabulary: the
// vocabulary is an ordinary space-backed one, and the shapes are an object
// with a second type the format does not model and a type object holding a
// keyless entry in a recommended list.
func TestExport_IsAFixpointWhenTheCensusShrinks(t *testing.T) {
	regenerate := func(t *testing.T, sbType model.SmartBlockType,
		snap *model.SmartBlockSnapshotBase, opts Options) (string, string) {
		t.Helper()
		gen1, err := Marshal(sbType, snap, opts)
		require.NoError(t, err)
		read := opts
		read.GenerateId = seqIds("g")
		_, back, err := Unmarshal(gen1, read)
		require.NoError(t, err)
		gen2, err := Marshal(sbType, back, opts)
		require.NoError(t, err)
		return string(gen1), string(gen2)
	}
	vocab := typedSpaceVocabulary{typeSlugOf: map[string]string{"custom1": "cust"}}

	t.Run("an object type the envelope does not model", func(t *testing.T) {
		// given: `cust` is the first type's slug AND the second type's stored
		// key, and the second type is past the only position §2 models
		snap := typedSnapshot("ot-custom1", "ot-cust")

		// when
		gen1, gen2 := regenerate(t, model.SmartBlockType_Page, snap, Options{Keys: vocab})

		// then
		assert.Equal(t, gen1, gen2, "the second generation must repeat the first")
		assert.Contains(t, gen1, `"type": "cust"`,
			"the truncated entry is not in the document, so its key reserves nothing")
		assert.Contains(t, gen1, `"cust": "custom1"`, "and the spelling owes its legend line")
	})

	t.Run("a type property no slot writes", func(t *testing.T) {
		// given: the keyless entry names `cust` and is dropped; the entry that
		// survives targets `custom1`, whose slug is `cust`
		snap := typedSnapshot("ot-page")
		snap.Details.Fields["recommendedFeaturedRelations"] = strList("k1", "k2")

		// when
		gen1, gen2 := regenerate(t, model.SmartBlockType_STType, snap,
			Options{Keys: vocab, ResolveProperties: censusPropResolver{}})

		// then
		assert.Equal(t, gen1, gen2, "the second generation must repeat the first")
		assert.Contains(t, gen1, `"cust"`, "the dropped definition's target reserves nothing")
	})
}

// The legend carries exactly what the bundled table cannot invert — the
// mirror of TestExport_PropertyKeysLegendCarriesWhatTheTableCannot for the
// type namespace.
func TestExport_TypeKeysLegendCarriesWhatTheTableCannot(t *testing.T) {
	vocab := typedSpaceVocabulary{typeSlugOf: map[string]string{customTypeKey: "task"}}
	snap := typedSnapshot("ot-" + customTypeKey)

	data, err := Marshal(model.SmartBlockType_Page, snap, Options{Keys: vocab})
	require.NoError(t, err)

	doc := decodeEnvelope(t, data)
	assert.Equal(t, "task", doc.Type, "the custom type key is spelled as its slug")
	assert.Equal(t, map[string]string{"task": customTypeKey}, doc.TypeKeys,
		"the slug shadows the bundled task key, so the document owes the entry that inverts it")
	assert.Empty(t, doc.PropertyKeys, "the type legend is not the property legend")
}

// A bundled type spelled as its derived slug needs no entry: the table ships
// with every reader.
func TestExport_TypeKeysLegendOmitsWhatTheTableInverts(t *testing.T) {
	for _, key := range []string{"page", "objectType"} {
		data, err := Marshal(model.SmartBlockType_Page, typedSnapshot("ot-"+key), Options{})
		require.NoError(t, err)
		doc := decodeEnvelope(t, data)
		assert.Empty(t, doc.TypeKeys, "bundled key %q owes no legend entry", key)
	}
}

// The identity entry (§3, type namespace): a stored type key written verbatim
// whose spelling the bundled table binds to a DIFFERENT key. `object_type`
// the custom stored key beside bundled `objectType` is exactly the §3 shadow
// shape — without the entry, a package-only reader resolves the spelling
// through the table and lands on the bundled twin.
func TestExport_TypeKeysIdentityEntryForAShadowStoredKey(t *testing.T) {
	data, err := Marshal(model.SmartBlockType_Page, typedSnapshot("ot-object_type"), Options{})
	require.NoError(t, err)

	doc := decodeEnvelope(t, data)
	assert.Equal(t, "object_type", doc.Type)
	assert.Equal(t, map[string]string{"object_type": "object_type"}, doc.TypeKeys,
		"the document's only way to say the term is a stored key, not the bundled table's objectType")

	_, snap, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, []string{"ot-object_type"}, snap.ObjectTypes,
		"a package-only reader lands on the stored key, not the bundled twin")
}

// The point of the legend: a reader with no space gets the stored type key
// back, and the legend outranks the reader's own vocabulary.
func TestImport_TypeKeysLegendInvertsWithoutTheSpace(t *testing.T) {
	doc := `{"version": 1, "type_keys": {"task": "` + customTypeKey + `"}, "type": "task"}`

	t.Run("package-only reader", func(t *testing.T) {
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, []string{"ot-" + customTypeKey}, snap.ObjectTypes)
	})

	t.Run("legend outranks the reader's vocabulary", func(t *testing.T) {
		vocab := typedSpaceVocabulary{typeSlugOf: map[string]string{"readerLocalKey": "task"}}
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), Keys: vocab})
		require.NoError(t, err)
		assert.Equal(t, []string{"ot-" + customTypeKey}, snap.ObjectTypes,
			"the legend is the document's own statement; a vocabulary belongs to the reader")
	})
}

// The confirmed defect, end to end: a node-backed writer slugs a custom type,
// and the archive's consumer is a package-only reader. Before the legend the
// reader bound the slug to the bundled Task type — a different type, silently.
func TestRoundTrip_TypeSlugIsInvertibleInAPackageOnlyReader(t *testing.T) {
	vocab := typedSpaceVocabulary{typeSlugOf: map[string]string{customTypeKey: "task"}}
	data, err := Marshal(model.SmartBlockType_Page, typedSnapshot("ot-"+customTypeKey), Options{Keys: vocab})
	require.NoError(t, err)

	_, snap, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, []string{"ot-" + customTypeKey}, snap.ObjectTypes,
		"the document alone must invert its own spellings (§3)")
}

// type_properties[].object_types is a type-key slot like the envelope type,
// and it owes (and reads) the same legend.
func TestTypeKeysLegendCoversObjectTypes(t *testing.T) {
	t.Run("export records the entry", func(t *testing.T) {
		vocab := typedSpaceVocabulary{typeSlugOf: map[string]string{customTypeKey: "task"}}
		snap := &model.SmartBlockSnapshotBase{
			Blocks: []*model.Block{{Id: "t1",
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
			Details: fields(map[string]*types.Value{
				"id":                   str("t1"),
				"recommendedRelations": strList("rel-owner"),
			}),
			ObjectTypes: []string{"ot-objectType"},
			Key:         "k",
		}
		resolver := &staticPropertyResolver{def: PropertyDefinition{
			Key: "owner", Name: "Owner", Format: model.RelationFormat_object,
			ObjectTypes: []string{customTypeKey},
		}}

		data, err := Marshal(model.SmartBlockType_STType, snap,
			Options{Keys: vocab, ResolveProperties: resolver})
		require.NoError(t, err)

		doc := decodeEnvelope(t, data)
		require.Len(t, doc.TypeProps, 1)
		assert.Equal(t, []string{"task"}, doc.TypeProps[0].ObjectTypes)
		assert.Equal(t, map[string]string{"task": customTypeKey}, doc.TypeKeys,
			"a slot that writes the slug without recording the entry inverts only by luck")
	})

	t.Run("import reads the legend first", func(t *testing.T) {
		doc := `{"version": 1, "kind": "object_type", "id": "t1", "key": "k",
			"type_keys": {"task": "` + customTypeKey + `"},
			"type_properties": [{"key": "owner", "name": "Owner", "format": "objects",
			 "object_types": ["task", "participant"]}]}`
		r := &recordingPropertyResolver{}

		_, _, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), ResolveProperties: r})
		require.NoError(t, err)
		require.Len(t, r.defs, 1)
		assert.Equal(t, []string{customTypeKey, "participant"}, r.defs[0].ObjectTypes,
			"the legend inverts task; participant stays the bundled key")
	})
}

// Decision: one ledger and one legend PER NAMESPACE. A property slug and a
// type slug may coincide without conflict (§3: `objectType` the layout value
// coexists with `object_type` the type key, and that is intended) — sharing
// one ledger would make one namespace's claim back the other namespace off
// its own slug.
func TestExportImport_PropertyAndTypeNamespacesShareATerm(t *testing.T) {
	const customPropKey = "6a32d4856761631534b22f85"
	vocab := typedSpaceVocabulary{
		propSlugOf: map[string]string{customPropKey: "task"},
		typeSlugOf: map[string]string{customTypeKey: "task"},
	}
	snap := typedSnapshot("ot-" + customTypeKey)
	snap.Details.Fields[customPropKey] = str("both namespaces spell task")

	data, err := Marshal(model.SmartBlockType_Page, snap, Options{Keys: vocab})
	require.NoError(t, err)

	doc := decodeEnvelope(t, data)
	assert.Equal(t, "task", doc.Type)
	assert.Contains(t, doc.Properties, "task")
	assert.Equal(t, map[string]string{"task": customPropKey}, doc.PropertyKeys)
	assert.Equal(t, map[string]string{"task": customTypeKey}, doc.TypeKeys)

	_, snap2, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, []string{"ot-" + customTypeKey}, snap2.ObjectTypes)
	assert.Equal(t, "both namespaces spell task",
		snap2.Details.Fields[customPropKey].GetStringValue())
}

// One term, one key — document-wide, per namespace: a stored type key named
// anywhere in the document always keeps its own term, so no other key's slug
// may take it, and a contested slug goes to its first claimant.
func TestExport_TypeTermLedgerBacksACollidingSlugOff(t *testing.T) {
	// the envelope names customTypeKey, whose vocabulary slug is `wiki_person`
	// — but the document ALSO names the stored key `wiki_person` in
	// object_types, so the slug is taken (verbatim-first) and the envelope
	// falls back to the stored key, which is always its own address.
	vocab := typedSpaceVocabulary{typeSlugOf: map[string]string{customTypeKey: "wiki_person"}}
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{Id: "t1",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		Details: fields(map[string]*types.Value{
			"id":                   str("t1"),
			"recommendedRelations": strList("rel-owner"),
		}),
		ObjectTypes: []string{"ot-" + customTypeKey},
		Key:         "k",
	}
	resolver := &staticPropertyResolver{def: PropertyDefinition{
		Key: "owner", Name: "Owner", Format: model.RelationFormat_object,
		ObjectTypes: []string{"wiki_person"},
	}}

	data, err := Marshal(model.SmartBlockType_STType, snap,
		Options{Keys: vocab, ResolveProperties: resolver})
	require.NoError(t, err)

	doc := decodeEnvelope(t, data)
	assert.Equal(t, customTypeKey, doc.Type,
		"the slug is not emitted — its spelling belongs to the stored key wiki_person")
	require.Len(t, doc.TypeProps, 1)
	assert.Equal(t, []string{"wiki_person"}, doc.TypeProps[0].ObjectTypes)
	assert.Equal(t, map[string]string{"wiki_person": "wiki_person"}, doc.TypeKeys,
		"the bundled table is silent on both keys, but THIS vocabulary binds the "+
			"spelling `wiki_person` to customTypeKey — so the stored key written verbatim "+
			"owes the identity entry, or its own space reads the target type back as the "+
			"type that took its spelling")

	// a package-only reader, which has no vocabulary at all
	_, snap2, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, []string{"ot-" + customTypeKey}, snap2.ObjectTypes)

	// and the writer's OWN reader, which is the one the entry is for
	r := &recordingPropertyResolver{}
	_, snap3, err := Unmarshal(data, Options{GenerateId: seqIds("h"), Keys: vocab, ResolveProperties: r})
	require.NoError(t, err)
	assert.Equal(t, []string{"ot-" + customTypeKey}, snap3.ObjectTypes)
	require.Len(t, r.defs, 1)
	assert.Equal(t, []string{"wiki_person"}, r.defs[0].ObjectTypes,
		"without the entry this reads back as customTypeKey — the target type re-pointed, silently")
}

// `template` is an envelope-semantic spelling: export keys template_for
// emission off it, validation gates template_for on it, and import derives
// the smartblock kind from it. A vocabulary may not move it — in either
// direction — or a template's target type is silently dropped.
func TestExport_TemplateSpellingIsReserved(t *testing.T) {
	t.Run("the template key keeps its spelling", func(t *testing.T) {
		vocab := typedSpaceVocabulary{typeSlugOf: map[string]string{
			"template":    "tmpl",
			customTypeKey: "task",
		}}
		var warned []Issue
		snap := typedSnapshot("ot-template", "ot-"+customTypeKey)

		data, err := Marshal(model.SmartBlockType_Template, snap,
			Options{Keys: vocab, OnWarning: func(i Issue) { warned = append(warned, i) }})
		require.NoError(t, err)

		doc := decodeEnvelope(t, data)
		assert.Equal(t, "template", doc.Type, "the vocabulary's tmpl is refused")
		assert.Equal(t, "task", doc.TemplateFor, "the target type survives")
		assert.Equal(t, map[string]string{"task": customTypeKey}, doc.TypeKeys)
		assert.NotEmpty(t, warned, "the refused vocabulary answer is reported")

		_, snap2, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, []string{"ot-template", "ot-" + customTypeKey}, snap2.ObjectTypes)
	})

	t.Run("no other key may take the spelling", func(t *testing.T) {
		vocab := typedSpaceVocabulary{typeSlugOf: map[string]string{customTypeKey: "template"}}
		var warned []Issue

		data, err := Marshal(model.SmartBlockType_Page, typedSnapshot("ot-"+customTypeKey),
			Options{Keys: vocab, OnWarning: func(i Issue) { warned = append(warned, i) }})
		require.NoError(t, err)

		doc := decodeEnvelope(t, data)
		assert.Equal(t, customTypeKey, doc.Type, "the stored key is its own address")
		assert.Empty(t, doc.TypeKeys)
		assert.NotEmpty(t, warned)
	})
}

// blankTypeVocab resolves a type spelling to the empty string — the type
// namespace's twin of blankKeyVocab. Unrefused, the empty key became the
// ObjectTypes entry "ot-", and the re-export then dropped the type with no
// error anywhere.
type blankTypeVocab struct{ BundledKeyVocabulary }

func (blankTypeVocab) TypeKey(slug string) (string, bool) {
	if slug == "blanktype" {
		return "", true
	}
	return BundledKeyVocabulary{}.TypeKey(slug)
}

func TestImport_SeamRefusesAnEmptyResolvedTypeKey(t *testing.T) {
	opts := func() Options { return Options{GenerateId: seqIds("g"), Keys: blankTypeVocab{}} }

	t.Run("envelope type", func(t *testing.T) {
		doc := `{"version": 1, "type": "blanktype"}`
		require.NoError(t, Validate([]byte(doc)),
			"the document's own chain resolves blanktype verbatim — Validate cannot see the vocabulary")
		_, _, err := Unmarshal([]byte(doc), opts())
		require.Error(t, err)
		var ve *ValidationError
		require.ErrorAs(t, err, &ve, "the refusal is path-addressed")
		assert.Contains(t, err.Error(), "/type")
	})

	t.Run("template_for", func(t *testing.T) {
		doc := `{"version": 1, "type": "template", "template_for": "blanktype"}`
		require.NoError(t, Validate([]byte(doc)))
		_, _, err := Unmarshal([]byte(doc), opts())
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/template_for")
	})

	t.Run("object_types", func(t *testing.T) {
		doc := `{"version": 1, "kind": "object_type", "id": "t1", "key": "k",
			"type_properties": [{"key": "owner", "format": "objects",
			 "object_types": ["page", "blanktype"]}]}`
		require.NoError(t, Validate([]byte(doc)))
		o := opts()
		o.ResolveProperties = &recordingPropertyResolver{}
		_, _, err := Unmarshal([]byte(doc), o)
		require.Error(t, err)
		assert.Contains(t, err.Error(), "/type_properties/0/object_types/1")
	})
}

// The template_for gate and the kind derivation run on the STORED key the
// type spelling resolves to through the document's own chain — not on the
// raw spelling. A legend that rebinds the spelling `template` says the
// document's type is NOT the template type, and a legend that binds another
// spelling ONTO the template key says it is.
func TestTemplateGateRunsOnTheResolvedTypeKey(t *testing.T) {
	t.Run("a rebound template spelling is not a template", func(t *testing.T) {
		doc := `{"version": 1, "type_keys": {"template": "custom1"},
			"type": "template", "template_for": "page"}`
		err := Validate([]byte(doc))
		require.Error(t, err, "template_for on a document whose type resolves to custom1")
		assert.Contains(t, err.Error(), "/template_for")
		_, _, unmErr := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.Error(t, unmErr, "Unmarshal agrees (I2)")
	})

	t.Run("a spelling bound onto the template key is one", func(t *testing.T) {
		doc := `{"version": 1, "type_keys": {"tpl": "template"},
			"type": "tpl", "template_for": "page"}`
		require.NoError(t, Validate([]byte(doc)))
		sbType, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, model.SmartBlockType_Template, sbType)
		assert.Equal(t, []string{"ot-template", "ot-page"}, snap.ObjectTypes)
	})
}

// A type legend value is a stored key and obeys the writable-key rule, like a
// property legend value (§3).
func TestValidate_TypeKeysLegendShape(t *testing.T) {
	for name, doc := range map[string]string{
		"empty value":     `{"version": 1, "type_keys": {"t": ""}}`,
		"control value":   `{"version": 1, "type_keys": {"t": "a` + "\\n" + `b"}}`,
		"over-long value": `{"version": 1, "type_keys": {"t": "` + strings.Repeat("k", 129) + `"}}`,
		"empty spelling":  `{"version": 1, "type_keys": {"": "task"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			err := Validate([]byte(doc))
			require.Error(t, err)
			assert.Contains(t, err.Error(), "/type_keys")
		})
	}
	assert.NoError(t, Validate([]byte(`{"version": 1, "type_keys": {"task": "`+customTypeKey+`"}}`)))
}

// A snapshot's ObjectTypes is untrusted data, and real stores hold entries
// with no type key in them — a bare "ot-", which older builds of this very
// package wrote back whenever a vocabulary resolved a spelling to "". Such an
// entry has no spelling, so setNonEmpty omits the slot it lands in; written
// positionally, it was therefore CONTAGIOUS. An empty `type` slot makes
// `template_for` inexpressible (export keys it off the spelled term), so
// ["ot-", "ot-task"] came back as no types at all — the good sibling gone
// with the bad one, and OnWarning never called, while the import seam refuses
// exactly this shape loudly and path-addressed.
func TestExport_AKeylessObjectTypeIsDroppedAndDoesNotTakeItsSiblings(t *testing.T) {
	marshal := func(t *testing.T, sbType model.SmartBlockType, ots ...string) (envelopeTypeDoc, []Issue, []string) {
		t.Helper()
		var warned []Issue
		data, err := Marshal(sbType, typedSnapshot(ots...),
			Options{OnWarning: func(i Issue) { warned = append(warned, i) }})
		require.NoError(t, err)
		_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		return decodeEnvelope(t, data), warned, back.ObjectTypes
	}

	t.Run("a hole in front: the target type moves up into the type slot", func(t *testing.T) {
		doc, warned, back := marshal(t, model.SmartBlockType_Template, "ot-", "ot-task")

		assert.Equal(t, "task", doc.Type, "the good sibling survives its bad neighbour")
		assert.Equal(t, "template", doc.Kind,
			"the type term is no longer `template`, so the kind must be spelled out")
		assert.Equal(t, []string{"ot-task"}, back)
		require.Len(t, warned, 1, "a dropped type owes a diagnostic, as a dropped property key does")
		assert.Equal(t, "/type", warned[0].Path)
		assert.Contains(t, warned[0].Message, "no type key")
	})

	t.Run("a hole behind: the type survives and the drop is still reported", func(t *testing.T) {
		doc, warned, back := marshal(t, model.SmartBlockType_Template, "ot-template", "ot-")

		assert.Equal(t, "template", doc.Type)
		assert.Empty(t, doc.TemplateFor, "there is no second type to name")
		assert.Equal(t, []string{"ot-template"}, back)
		require.Len(t, warned, 1)
		assert.Equal(t, "/type", warned[0].Path)
	})

	t.Run("a hole between: template_for takes the next real entry", func(t *testing.T) {
		doc, warned, back := marshal(t, model.SmartBlockType_Template, "ot-template", "ot-", "ot-"+customTypeKey)

		assert.Equal(t, "template", doc.Type)
		assert.Equal(t, customTypeKey, doc.TemplateFor)
		assert.Equal(t, []string{"ot-template", "ot-" + customTypeKey}, back)
		require.Len(t, warned, 1)
	})

	t.Run("nothing but holes is nothing, quietly reported", func(t *testing.T) {
		doc, warned, back := marshal(t, model.SmartBlockType_Page, "ot-", "")

		assert.Empty(t, doc.Type)
		assert.Empty(t, back)
		assert.Len(t, warned, 2, "one per dropped entry")
	})

	t.Run("a whole list needs no warning", func(t *testing.T) {
		_, warned, back := marshal(t, model.SmartBlockType_Template, "ot-template", "ot-task")

		assert.Equal(t, []string{"ot-template", "ot-task"}, back)
		assert.Empty(t, warned)
	})

	// The OTHER drop, and the one that was silent: an entry with a perfectly
	// good key that the envelope has no position for (§2 models one type,
	// plus the target type on a template). §3 says every drop is reported
	// through OnWarning, and this one was not — a user's second type left the
	// archive with nothing said, in a document whose own shape gives the
	// caller no way to notice.
	t.Run("a keyed entry past the modelled positions is reported too", func(t *testing.T) {
		doc, warned, back := marshal(t, model.SmartBlockType_Page, "ot-page", "ot-task")

		assert.Equal(t, "page", doc.Type)
		assert.Equal(t, []string{"ot-page"}, back, "the second type is not in the document")
		require.Len(t, warned, 1)
		assert.Equal(t, "/type", warned[0].Path)
		assert.Contains(t, warned[0].Message, `object type 1 ("ot-task")`,
			"the message names the entry that was lost, at the position it stood in")
		assert.Contains(t, warned[0].Message, "the envelope carries one type",
			"and why: there is no position for it, not that something went wrong")
	})

	t.Run("a template reports only what is past its two positions", func(t *testing.T) {
		_, warned, back := marshal(t, model.SmartBlockType_Template, "ot-template", "ot-task", "ot-page")

		assert.Equal(t, []string{"ot-template", "ot-task"}, back)
		require.Len(t, warned, 1, "the target type has a slot; the third entry does not")
		assert.Contains(t, warned[0].Message, `object type 2 ("ot-page")`)
	})

	// the index is the one the SNAPSHOT holds, not the one the survivor took
	// after closing ranks: a caller matching the warning against its own
	// ObjectTypes must land on the entry that was dropped
	t.Run("the reported position survives a keyless entry in front", func(t *testing.T) {
		_, warned, back := marshal(t, model.SmartBlockType_Page, "ot-", "ot-page", "ot-task")

		assert.Equal(t, []string{"ot-page"}, back)
		require.Len(t, warned, 2)
		assert.Contains(t, warned[0].Message, `object type 0 ("ot-")`)
		assert.Contains(t, warned[1].Message, `object type 2 ("ot-task")`)
	})
}

// The type legend must name only types the document actually mentions.
// envelopeTypeTerms slugged every ObjectTypes entry, and typeSlug is the term
// ledger's CLAIM step — it records the legend entry the spelling owes — so a
// document carried a `type_keys` line for a type no slot names, publishing a
// space's slug→key mapping for nothing. buildProperties cannot do this,
// because it filters before it slugs.
func TestExport_TypeLegendNamesOnlyTypesTheDocumentMentions(t *testing.T) {
	vocab := typedSpaceVocabulary{typeSlugOf: map[string]string{customTypeKey: "task"}}

	t.Run("a second type no slot writes leaves no legend line", func(t *testing.T) {
		data, err := Marshal(model.SmartBlockType_Page,
			typedSnapshot("ot-page", "ot-"+customTypeKey), Options{Keys: vocab})
		require.NoError(t, err)

		doc := decodeEnvelope(t, data)
		assert.Equal(t, "page", doc.Type)
		assert.Empty(t, doc.TypeKeys,
			"the document never spells `task`, so it owes no entry inverting it")
		assert.NotContains(t, string(data), customTypeKey,
			"and the space's stored key does not appear anywhere")
	})

	// the control: when the second slot IS written, the line is owed and
	// written — without this the test above would pass on a legend that never
	// works at all
	t.Run("a second type template_for writes keeps its legend line", func(t *testing.T) {
		data, err := Marshal(model.SmartBlockType_Template,
			typedSnapshot("ot-template", "ot-"+customTypeKey), Options{Keys: vocab})
		require.NoError(t, err)

		doc := decodeEnvelope(t, data)
		assert.Equal(t, "task", doc.TemplateFor)
		assert.Equal(t, map[string]string{"task": customTypeKey}, doc.TypeKeys)
	})
}

// templateMovingVocab is the hand-written third-party vocabulary the
// `template` reservation exists for: it answers with a different stored key
// for the reserved spelling, and binds another spelling onto the template key.
// Neither shipped vocabulary can produce these — storeresolver's keyMaps.key
// refuses any slug the bundled table binds elsewhere — but Options.Keys is a
// public interface, so nothing but this guard stops one.
type templateMovingVocab struct{ BundledKeyVocabulary }

func (templateMovingVocab) TypeKey(slug string) (string, bool) {
	switch slug {
	case "template":
		return customTypeKey, true
	case "tpl":
		return "template", true
	}
	return BundledKeyVocabulary{}.TypeKey(slug)
}

// The import half of TestExport_TemplateSpellingIsReserved. The kind
// derivation and the /template_for gate run through the document's own chain
// alone (docTypeKey — legend, bundled table, verbatim; Validate has no
// vocabulary either, §12), while the stored key came from the vocabulary. So
// the two resolutions of one field could disagree: a Template smartblock whose
// ObjectTypeKeys do not contain `template`, which every downstream template
// check misses — they all test lo.Contains(ObjectTypeKeys, TypeKeyTemplate) —
// and whose re-export drops the target type outright, since template_for is
// keyed off the spelled term.
func TestImport_TemplateSpellingIsReservedAgainstTheVocabulary(t *testing.T) {
	opts := func() Options { return Options{GenerateId: seqIds("g"), Keys: templateMovingVocab{}} }

	t.Run("the vocabulary may not move the template key", func(t *testing.T) {
		doc := `{"version": 1, "type": "template", "template_for": "task"}`
		var warned []Issue
		o := opts()
		o.OnWarning = func(i Issue) { warned = append(warned, i) }

		sbType, snap, err := Unmarshal([]byte(doc), o)

		require.NoError(t, err)
		assert.Equal(t, model.SmartBlockType_Template, sbType)
		assert.Equal(t, []string{"ot-template", "ot-task"}, snap.ObjectTypes,
			"a Template whose ObjectTypeKeys lack `template` is invisible to every template check")
		assert.NotEmpty(t, warned, "the refused vocabulary answer is reported, as on export")

		// and the snapshot re-exports whole: template_for is keyed off the
		// spelled term, so a moved spelling used to drop the target type
		out, err := Marshal(sbType, snap, Options{})
		require.NoError(t, err)
		assert.Equal(t, "task", decodeEnvelope(t, out).TemplateFor)
	})

	// §13: a warning's path addresses the fault. The guard fires in three
	// slots and named `/type` in all three, so a caller following the pointer
	// landed on a field that was fine — or, from a type document, on one the
	// document does not have at all.
	t.Run("the warning addresses the slot it fired in", func(t *testing.T) {
		for name, tc := range map[string]struct {
			doc   string
			paths []string
		}{
			"the envelope type": {
				`{"version": 1, "type": "tpl"}`, []string{"/type"}},
			"a template's target type": {
				// `template` is moved by this vocabulary too, so the envelope
				// slot refuses it as well — one warning per slot, each
				// naming its own
				`{"version": 1, "type": "template", "template_for": "tpl"}`,
				[]string{"/type", "/template_for"}},
			"a type property's object_types entry": {
				`{"version": 1, "kind": "object_type", "id": "t1", "key": "k",
				  "type_properties": [{"key": "owner", "format": "objects",
				   "object_types": ["page", "tpl"]}]}`,
				[]string{"/type_properties/0/object_types/1"}},
		} {
			t.Run(name, func(t *testing.T) {
				var warned []Issue
				o := opts()
				o.OnWarning = func(i Issue) { warned = append(warned, i) }

				_, _, err := Unmarshal([]byte(tc.doc), o)

				require.NoError(t, err)
				var paths []string
				for _, w := range warned {
					assert.Contains(t, w.Message, "template semantics")
					paths = append(paths, w.Path)
				}
				assert.Equal(t, tc.paths, paths)
			})
		}
	})

	t.Run("no other spelling may take the template key", func(t *testing.T) {
		doc := `{"version": 1, "type": "tpl"}`

		sbType, snap, err := Unmarshal([]byte(doc), opts())

		require.NoError(t, err)
		assert.Equal(t, model.SmartBlockType_Page, sbType,
			"the document's own chain says `tpl` is not the template type")
		assert.Equal(t, []string{"ot-tpl"}, snap.ObjectTypes,
			"so the stored key must not be the template key either")
	})

	// The reservation binds the VOCABULARY, not the document. A legend moves
	// docTypeKey with it, so both halves still agree — and that case is
	// already specified (TestTemplateGateRunsOnTheResolvedTypeKey).
	t.Run("the document's own legend still moves the spelling", func(t *testing.T) {
		doc := `{"version": 1, "type_keys": {"tpl": "template"},
			"type": "tpl", "template_for": "page"}`

		sbType, snap, err := Unmarshal([]byte(doc), opts())

		require.NoError(t, err)
		assert.Equal(t, model.SmartBlockType_Template, sbType)
		assert.Equal(t, []string{"ot-template", "ot-page"}, snap.ObjectTypes)
	})
}

// The one property-namespace rule the type namespace deliberately does NOT
// carry: `/properties` refuses two spellings that bind one stored key, because
// two members collapse into one details field and one of the two values is
// lost with nothing to say which. Two type slots collapse into nothing —
// ObjectTypes is an ordered list, and a repeated entry displaces no value — so
// the document is accepted. This is a decision, not an oversight: it is pinned
// here so that adding the refusal has to argue with §3 first.
func TestTypeNamespaceHasNoDuplicateBindingRefusal(t *testing.T) {
	doc := `{"version": 1, "type": "a", "template_for": "b",
		"type_keys": {"a": "template", "b": "template"}}`

	require.NoError(t, Validate([]byte(doc)))
	sbType, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})

	require.NoError(t, err)
	assert.Equal(t, model.SmartBlockType_Template, sbType)
	assert.Equal(t, []string{"ot-template", "ot-template"}, snap.ObjectTypes,
		"a repeated entry is a repeated entry; nothing was displaced")
}
