package anyblockjson

// Verbatim-first, made real (§3): an exact stored key is always its own
// address, and the bundled slug table applies only to terms that are NOT
// stored keys. A package-only reader has no stored-key set, so the document
// itself must say which of its terms are stored keys — the legend's identity
// entries. Five reviews traced one family of silent corruptions to the two
// halves disagreeing about this rule; these tests pin the settled behavior,
// one confirmed defect each.

import (
	"encoding/json"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type flatPropsDoc struct {
	Properties   map[string]any    `json:"properties"`
	PropertyKeys map[string]string `json:"property_keys"`
	Blocks       []struct {
		Type string `json:"type"`
		Key  string `json:"key"`
	} `json:"blocks"`
}

func decodeDoc(t *testing.T, data []byte) flatPropsDoc {
	t.Helper()
	var doc flatPropsDoc
	require.NoError(t, json.Unmarshal(data, &doc))
	return doc
}

// A stored key whose spelling the bundled table binds to a DIFFERENT key
// ("due_date" beside bundled dueDate) is written verbatim — but a package-only
// reader resolves spellings legend → bundled table → verbatim, so without a
// legend entry the value silently moves onto the bundled key. Export owes the
// identity entry: the document's own statement that the spelling is a stored
// key.
func TestExport_ShadowStoredKeyGetsAnIdentityEntry(t *testing.T) {
	for _, tc := range []struct{ storedKey, bundledKey string }{
		{"due_date", "dueDate"},
		{"icon_emoji", "iconEmoji"},
	} {
		t.Run(tc.storedKey, func(t *testing.T) {
			// given
			snap := customKeySnapshot(map[string]*types.Value{tc.storedKey: str("custom-value")})
			want := map[string]string{tc.storedKey: tc.storedKey}

			// when
			data, err := Marshal(model.SmartBlockType_Page, snap, Options{})

			// then
			require.NoError(t, err)
			require.NoError(t, Validate(data))
			assert.Equal(t, want, decodeDoc(t, data).PropertyKeys,
				"the identity entry is what tells a reader with no store that the spelling is a stored key")

			// and a package-only reader binds the value to the stored key
			_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
			require.NoError(t, err)
			assert.Equal(t, "custom-value", back.Details.Fields[tc.storedKey].GetStringValue(),
				"the stored key is its own address")
			assert.NotContains(t, back.Details.Fields, tc.bundledKey,
				"nothing may land on the bundled twin")
		})
	}
}

// A bundled key BESIDE its shadow twin: {"iconEmoji": …, "icon_emoji": …}.
// The bundled key backs off to its stored spelling (the twin owns the slug),
// the twin gets its identity entry, and the whole thing round-trips — Marshal
// used to emit a document its own Unmarshal refused as a duplicate binding.
func TestRoundTrip_BundledKeyBesideItsShadowTwin(t *testing.T) {
	for _, tc := range []struct{ bundledKey, shadowKey string }{
		{"iconEmoji", "icon_emoji"},
		{"dueDate", "due_date"},
	} {
		t.Run(tc.bundledKey, func(t *testing.T) {
			// given
			snap := customKeySnapshot(map[string]*types.Value{
				tc.bundledKey: str("bundled"),
				tc.shadowKey:  str("custom"),
			})

			// when
			data, err := Marshal(model.SmartBlockType_Page, snap, Options{})

			// then — I1 and I2 in one breath: what Marshal emits, Validate
			// accepts and Unmarshal imports
			require.NoError(t, err)
			require.NoError(t, Validate(data), "emitted:\n%s", data)
			_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
			require.NoError(t, err, "emitted:\n%s", data)
			assert.Equal(t, "bundled", back.Details.Fields[tc.bundledKey].GetStringValue())
			assert.Equal(t, "custom", back.Details.Fields[tc.shadowKey].GetStringValue())
		})
	}
}

// A stored key spelled exactly like the api slug of an INTERNAL bundled key
// ("unique_key", next to stripped "uniqueKey"). Export writes it verbatim;
// without the identity entry, validation resolved the spelling through the
// bundled table, hit the deny rule, and Marshal emitted a document its own
// Validate rejected.
func TestRoundTrip_StoredKeySpelledLikeAnInternalSlug(t *testing.T) {
	for _, storedKey := range []string{"unique_key", "space_id", "old_anytype_id", "source_file_path"} {
		t.Run(storedKey, func(t *testing.T) {
			// given
			snap := customKeySnapshot(map[string]*types.Value{storedKey: str("x")})

			// when
			data, err := Marshal(model.SmartBlockType_Page, snap, Options{})

			// then
			require.NoError(t, err)
			require.NoError(t, Validate(data), "emitted:\n%s", data)
			_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
			require.NoError(t, err, "emitted:\n%s", data)
			assert.Equal(t, "x", back.Details.Fields[storedKey].GetStringValue(),
				"a custom key that shadows an internal slug is still a custom key")
		})
	}
}

// blockKeySnapshot is a page whose details and property blocks are the
// caller's to shape — the fixture for the term-ledger collisions.
func blockKeySnapshot(details map[string]*types.Value, blockKeys ...string) *model.SmartBlockSnapshotBase {
	details["id"] = str("o1")
	root := &model.Block{Id: "o1",
		Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}
	blocks := []*model.Block{root}
	for i, key := range blockKeys {
		b := &model.Block{Id: "rel" + string(rune('a'+i)), Content: &model.BlockContentOfRelation{
			Relation: &model.BlockContentRelation{Key: key}}}
		root.ChildrenIds = append(root.ChildrenIds, b.Id)
		blocks = append(blocks, b)
	}
	return &model.SmartBlockSnapshotBase{Blocks: blocks, Details: fields(details)}
}

// The ledger, arm one: a block slot's slug may not take a term that IS a
// stored key the document names. The vocabulary slugs a custom key onto
// "due_date" while the object holds bundled dueDate: recording that legend
// entry rebound /properties/due_date to the custom key — dueDate's value
// moved, and the date degraded to a raw string.
func TestExport_BlockSlugMayNotTakeAStoredKeysTerm(t *testing.T) {
	// given
	vocab := spaceVocabulary{slugOf: map[string]string{"6a32d4856761631534b22f85": "due_date"}}
	snap := blockKeySnapshot(map[string]*types.Value{
		"name":    str("x"),
		"dueDate": str("2026-07-06T08:44:05Z"),
	}, "6a32d4856761631534b22f85")

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{Keys: vocab})

	// then — the block keeps its stored key; the legend does not rebind the term
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	doc := decodeDoc(t, data)
	require.Len(t, doc.Blocks, 1)
	assert.Equal(t, "6a32d4856761631534b22f85", doc.Blocks[0].Key,
		"the slug is taken — dueDate spells due_date, and a stored key always keeps its own term")
	assert.NotContains(t, doc.PropertyKeys, "due_date",
		"no legend entry may rebind a term a stored key answers to")

	// and a package-only reader gets both relations back intact
	_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.NotNil(t, back.Details.Fields["dueDate"], "dueDate must survive the round trip")
	assert.Nil(t, back.Details.Fields["6a32d4856761631534b22f85"],
		"the block's key is not a property on this object")
	for _, b := range back.Blocks {
		if c, ok := b.Content.(*model.BlockContentOfRelation); ok {
			assert.Equal(t, "6a32d4856761631534b22f85", c.Relation.Key)
		}
	}
}

// The ledger, arm two: when buildProperties backs a slug off (the term is
// another stored key on the object), the block slot naming the same key used
// to record the slug anyway — one document, two spellings of one key, refused
// by its own importer.
func TestExport_BackedOffSlugBacksOffEverywhere(t *testing.T) {
	// given
	vocab := spaceVocabulary{slugOf: map[string]string{"6a32d4856761631534b22f85": "due_date"}}
	snap := blockKeySnapshot(map[string]*types.Value{
		"due_date":                 str("shadow"),
		"6a32d4856761631534b22f85": str("custom"),
	}, "6a32d4856761631534b22f85")

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{Keys: vocab})

	// then
	require.NoError(t, err)
	require.NoError(t, Validate(data), "emitted:\n%s", data)
	doc := decodeDoc(t, data)
	require.Len(t, doc.Blocks, 1)
	assert.Equal(t, "6a32d4856761631534b22f85", doc.Blocks[0].Key,
		"one key, one spelling, document-wide")

	_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err, "emitted:\n%s", data)
	assert.Equal(t, "shadow", back.Details.Fields["due_date"].GetStringValue())
	assert.Equal(t, "custom", back.Details.Fields["6a32d4856761631534b22f85"].GetStringValue())
}

// The ledger, arm three: two keys, one slug, in slots outside /properties.
// The last recordPropertyKey used to win the single legend entry, so after a
// round trip BOTH property blocks named the second key — two relations
// collapsed into one.
func TestExport_TwoKeysOneSlugKeepDistinctTerms(t *testing.T) {
	// given
	vocab := twinSlugVocab{a: "aaa111", b: "bbb222", slug: "priority"}
	snap := blockKeySnapshot(map[string]*types.Value{"name": str("x")}, "aaa111", "bbb222")

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{Keys: vocab})

	// then — first claimant keeps the slug, the later holder stays verbatim
	require.NoError(t, err)
	require.NoError(t, Validate(data))
	doc := decodeDoc(t, data)
	require.Len(t, doc.Blocks, 2)
	assert.Equal(t, "priority", doc.Blocks[0].Key)
	assert.Equal(t, "bbb222", doc.Blocks[1].Key)
	assert.Equal(t, map[string]string{"priority": "aaa111"}, doc.PropertyKeys)

	// and the two relations are still two relations after the round trip
	_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	var keys []string
	for _, b := range back.Blocks {
		if c, ok := b.Content.(*model.BlockContentOfRelation); ok {
			keys = append(keys, c.Relation.Key)
		}
	}
	assert.Equal(t, []string{"aaa111", "bbb222"}, keys)
}

// twinSlugVocab spells two stored keys as one slug — the shape two properties
// minted from the same name really have.
type twinSlugVocab struct{ a, b, slug string }

func (v twinSlugVocab) PropertySlug(key string) string {
	if key == v.a || key == v.b {
		return v.slug
	}
	return BundledKeyVocabulary{}.PropertySlug(key)
}

func (v twinSlugVocab) PropertyKey(slug string) (string, bool) {
	if slug == v.slug {
		return v.a, true
	}
	return BundledKeyVocabulary{}.PropertyKey(slug)
}

func (v twinSlugVocab) TypeSlug(key string) string { return BundledKeyVocabulary{}.TypeSlug(key) }
func (v twinSlugVocab) TypeKey(slug string) (string, bool) {
	return BundledKeyVocabulary{}.TypeKey(slug)
}

// A legend value is a stored key, and admission judges it like one: the deny
// rule runs on the value itself, not only on whatever /properties member
// happens to spell the entry. Unchecked, {"sneaky": "uniqueKey"} was a
// laundering primitive — admission resolved ONE hop and checked the value
// only for writability.
func TestValidate_LegendValueObeysTheDenyRule(t *testing.T) {
	for name, doc := range map[string]string{
		"resolution vector": `{"version": 1, "property_keys": {"sneaky": "uniqueKey"}}`,
		"merge selector":    `{"version": 1, "property_keys": {"p": "oldAnytypeID"}}`,
		"envelope key":      `{"version": 1, "property_keys": {"myid": "id"}}`,
		"stripped key":      `{"version": 1, "property_keys": {"s": "spaceId"}}`,
	} {
		t.Run(name, func(t *testing.T) {
			err := Validate([]byte(doc))
			require.Error(t, err, doc)
			assert.Contains(t, err.Error(), "/property_keys/")
			_, _, unmErr := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
			require.Error(t, unmErr, "I2: Unmarshal refuses what Validate refuses")
		})
	}
}

// The other half of the laundering defect, pinned as settled behavior: under
// verbatim-first a legend value of "unique_key" names the CUSTOM stored key —
// never bundled uniqueKey — and the re-export is a document validation
// accepts (it used to emit /properties/unique_key with no legend entry and
// then reject its own output).
func TestImport_LegendBindingToACustomShadowKeyIsNotLaundering(t *testing.T) {
	doc := `{"version": 1, "id": "o1", "property_keys": {"x": "unique_key"}, "properties": {"x": "ot-page"}}`
	require.NoError(t, Validate([]byte(doc)))
	sbType, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, "ot-page", snap.Details.Fields["unique_key"].GetStringValue(),
		"the value binds to the custom key the legend names")
	assert.NotContains(t, snap.Details.Fields, "uniqueKey",
		"nothing lands on the importer's resolution vector")

	out, err := Marshal(sbType, snap, Options{})
	require.NoError(t, err)
	require.NoError(t, Validate(out), "re-export:\n%s", out)
}

// deniedKeyVocab slugs an internal key — plausible, because apiObjectKey is
// strcase(transliterate(Name)) with no reserved-word check, so a property
// named after an internal one produces exactly this.
type deniedKeyVocab struct{ BundledKeyVocabulary }

func (deniedKeyVocab) PropertySlug(key string) string {
	if key == "uniqueKey" {
		return "sneaky"
	}
	return BundledKeyVocabulary{}.PropertySlug(key)
}

// Export never writes a legend whose value the deny rule refuses: a denied
// stored key never takes a slug, because the slug's legend entry would carry
// that value. The verbatim key is the one honest rendering.
func TestExport_ADeniedKeyNeverTakesASlug(t *testing.T) {
	t.Run("property block", func(t *testing.T) {
		// given
		snap := blockKeySnapshot(map[string]*types.Value{"name": str("x")}, "uniqueKey")

		// when
		data, err := Marshal(model.SmartBlockType_Page, snap, Options{Keys: deniedKeyVocab{}})

		// then
		require.NoError(t, err)
		require.NoError(t, Validate(data), "emitted:\n%s", data)
		doc := decodeDoc(t, data)
		require.Len(t, doc.Blocks, 1)
		assert.Equal(t, "uniqueKey", doc.Blocks[0].Key)
		assert.Empty(t, doc.PropertyKeys, "a denied key cannot be a legend value")
	})

	t.Run("type properties key slot", func(t *testing.T) {
		// given — the confirmed input: a PropertyResolver answering an
		// internal key for a recommended-list entry
		snap := &model.SmartBlockSnapshotBase{
			Blocks: []*model.Block{{Id: "t1",
				Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
			Details: fields(map[string]*types.Value{
				"id": str("t1"),
				"recommendedRelations": {Kind: &types.Value_ListValue{ListValue: &types.ListValue{
					Values: []*types.Value{str("p1")}}}},
			}),
		}
		resolver := stubPropertyResolver{byId: map[string]PropertyDefinition{
			"p1": {Key: "uniqueKey"},
		}}

		// when
		data, err := Marshal(model.SmartBlockType_STType, snap,
			Options{Keys: deniedKeyVocab{}, ResolveProperties: resolver})

		// then
		require.NoError(t, err)
		require.NoError(t, Validate(data), "emitted:\n%s", data)
		var doc struct {
			TypeProperties []struct {
				Key string `json:"key"`
			} `json:"type_properties"`
			PropertyKeys map[string]string `json:"property_keys"`
		}
		require.NoError(t, json.Unmarshal(data, &doc))
		require.Len(t, doc.TypeProperties, 1)
		assert.Equal(t, "uniqueKey", doc.TypeProperties[0].Key)
		assert.Empty(t, doc.PropertyKeys)
	})
}

type stubPropertyResolver struct{ byId map[string]PropertyDefinition }

func (r stubPropertyResolver) PropertyById(id string) (PropertyDefinition, bool) {
	def, ok := r.byId[id]
	return def, ok
}

func (r stubPropertyResolver) PropertyId(def PropertyDefinition) (string, bool) { return "", false }

// envelopeSlugVocab spells stored keys as the two spellings validation
// refuses BEFORE any resolution ("id"/"type" belong to the envelope, and the
// legend cannot re-purpose them) — a property named "ID" or "Type" really
// produces this slug.
type envelopeSlugVocab struct{ BundledKeyVocabulary }

func (envelopeSlugVocab) PropertySlug(key string) string {
	switch key {
	case "artist":
		return "id"
	case "myGenre":
		return "type"
	}
	return BundledKeyVocabulary{}.PropertySlug(key)
}

// writableSlug treats a spelling the deny rule refuses AS A SPELLING as
// unwritable, exactly as it does an over-long slug: the stored key is
// written instead, with a warning. Emitting the slug made Marshal produce
// {"properties": {"id": …}}, which its own Validate refuses and no legend
// can rescue.
func TestExport_ASlugRefusedAsASpellingFallsBackToTheStoredKey(t *testing.T) {
	// given
	snap := customKeySnapshot(map[string]*types.Value{"artist": str("v"), "myGenre": str("w")})
	var warned []Issue

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap,
		Options{Keys: envelopeSlugVocab{}, OnWarning: func(i Issue) { warned = append(warned, i) }})

	// then
	require.NoError(t, err)
	require.NoError(t, Validate(data), "emitted:\n%s", data)
	doc := decodeDoc(t, data)
	assert.Contains(t, doc.Properties, "artist")
	assert.Contains(t, doc.Properties, "myGenre")
	assert.NotContains(t, doc.Properties, "id")
	assert.NotContains(t, doc.Properties, "type")
	assert.NotEmpty(t, warned, "the fallback is reported, like every vocabulary answer export cannot honor")

	_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, "v", back.Details.Fields["artist"].GetStringValue())
	assert.Equal(t, "w", back.Details.Fields["myGenre"].GetStringValue())
}
