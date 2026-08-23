package anyblockjson

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"

	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/filterstring"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// The label rule leaves bundled keys alone — their spelling is the derived
// api slug, in every space and offline (§3). That is only safe if the api
// slug is already a key this format can write everywhere, and nothing in
// `pkg/lib/bundle` knows about the §6.2.1 grammar: `ApiSlug` is
// `strcase.ToSnake` and no more, so a bundled key added later as `50Percent`
// or `all` would produce a slug no filter string can name — silently, since
// the label rule would pass it straight through.
//
// It holds today for all 223 (194 relations, 29 types), and this asserts it
// rather than assuming it, at the moment such a key would be added.
func TestBundledSlugsAreKeysTheFilterGrammarAccepts(t *testing.T) {
	var keys []string
	for _, u := range bundle.ListRelationsUrls() {
		keys = append(keys, strings.TrimPrefix(u, addr.BundledRelationURLPrefix))
	}
	for _, k := range bundle.ListTypesKeys() {
		keys = append(keys, string(k))
	}
	require.NotEmpty(t, keys)
	for _, key := range keys {
		slug := bundle.ApiSlug(key)
		_, err := filterstring.Parse(slug+` = "x"`, filterstring.Options{})
		require.NoErrorf(t, err, "bundled key %q spells %q, which is not a key this format can write", key, slug)
	}
}

// The label rule (§3): what a document spells for a key the bundled table
// does not speak for. Every case here is a decision with a plausible
// alternative, and the alternative is named in the case's own comment — a
// table that only pinned the happy path would pass with the transliterating
// normalizer this change replaced.
func TestNormalizeKeyLabel(t *testing.T) {
	for _, tc := range []struct {
		in   string
		want string
		why  string
	}{
		{"Publish Date", "publish_date", "a name separates its own words, and this one does"},
		{"Original creation date", "original_creation_date", ""},
		// The input to this rule is a DISPLAY NAME. camelCase is a KEY
		// phenomenon — `dueDate`, `iconEmoji` are stored keys — and a person
		// types "Due Date". So splitting humps buys nothing on the real
		// input domain and costs everything on the acronym-and-digit cases
		// beside it: ApiSlug's snake-caser is what turns "P2P Sync" into
		// `p_2_p_sync`, "Platform SDKs" into `platform_sd_ks` and "GitHub"
		// into `git_hub`. Convergence with ApiSlug was the old reason to
		// keep it; that reason died when the mangling was measured against
		// 38,061 production documents.
		//
		// A name that IS camelCase is an import artifact — a stored key
		// copied into the name field — and it is the whole price: two such
		// relations exist in the corpus.
		{"iconEmoji", "iconemoji", "a camelCase NAME is a key someone pasted into a name field: the price of the line above"},
		{"mediaArtistURL", "mediaartisturl", "same, and no acronym rule can tell SDKs from XMLParser anyway"},
		{"P2P Sync", "p2p_sync", "a letter and a digit are one word; `p_2_p_sync` is what splitting them cost"},
		{"E2E Encryption", "e2e_encryption", ""},
		{"GitHub Stars", "github_stars", "`git_hub_stars` is the api slug stored for this exact relation"},
		{"Platform SDKs", "platform_sdks", "`platform_sd_ks` likewise"},
		{"Objectives S3Y24", "objectives_s3y24", ""},
		// a leading `_` run is CONTENT: `_` is identStart, so there is
		// nothing to repair. 20 production relations from two integrations
		// namespace themselves this way.
		{"__amemory_salience", "__amemory_salience", "a namespace prefix survives; trimming it would merge it with ordinary keys"},
		{"_leading", "_leading", ""},
		{"trailing_", "trailing", "a trailing run is still a gap between a word and nothing"},
		{"a__b", "a_b", "an interior run still collapses"},
		{"___", "", "underscores alone name nothing"},
		{"  spaced  name ", "spaced_name", "separator runs collapse and edges trim"},
		{"Cost & type", "cost_type", ""},
		{"What's missing", "what_s_missing", ""},

		// the whole point of the change: non-Latin scripts are ALREADY legal
		// in the §6.2.1 grammar, so they are kept, not transliterated.
		// ApiSlugFromName answers `toggl`, `tieng_viet` and
		// `ri_ben_yu_nopuropatei` here — unguessable and unreadable at once.
		{"Тоггл", "тоггл", "no transliteration"},
		{"日本語のプロパティ", "日本語のプロパティ", "no transliteration"},
		{"tiếng Việt", "tiếng_việt", "no transliteration"},
		{"العربية", "العربية", "no transliteration"},

		// emoji, punctuation and symbols are separators, not letters
		{"Priority 📌", "priority", ""},
		{"Email 📧 ", "email", ""},
		{"C#", "c", ""},
		{"@home", "home", ""},
		{"tag/tag", "tag_tag", ""},

		// nothing left to name
		{"#", "", "the four production relations that store `#` as their api slug"},
		{"🎉", "", ""},
		{"", "", ""},

		// the two ways this construction can fail the grammar, and the one
		// escape for both. Dropping the leading digits instead would turn
		// `50% done` into `done`, which is a DIFFERENT (bundled) property.
		{"50% done", "_50_done", "identStart is a letter or `_`, never a digit"},
		{"1221312425", "_1221312425", ""},
		{"All", "_all", "`all` is a reserved word of the filter grammar"},
		{"NOT", "_not", ""},
		{"in", "_in", ""},

		// A combining mark modifies the letter before it, so it is neither a
		// separator (which would cut the word at every virama, `क_ष_त_र_य`)
		// nor droppable (which would strip the VOWELS: this script writes them
		// as marks, and मिल/मूल/मल/मैल would all become मल). It is an
		// identifier part — UAX #31 ID_Continue — and the word labels itself.
		{"क्षत्रिय", "क्षत्रिय", "marks are kept: they carry the word"},
		{"İstanbul", "istanbul", "lowercasing İ leaves a combining dot behind"},
	} {
		t.Run(tc.in, func(t *testing.T) {
			assert.Equal(t, tc.want, normalizeKeyLabel(tc.in), tc.why)
		})
	}

	// Pitfall stated in §3: two visually identical names can be different
	// byte sequences, and a reader matches a label byte-for-byte. Export is
	// safe by construction (it writes the same string as label and as legend
	// key), but a hand-edited document in the other form must still land on
	// the same label. This fails the moment the NFC pass is dropped.
	t.Run("NFD and NFC forms of one name normalize to one label", func(t *testing.T) {
		nfc := "Ünïcødé"
		nfd := norm.NFD.String(nfc)
		assert.NotEqual(t, nfc, nfd, "the fixture has to actually differ in bytes")
		assert.Equal(t, normalizeKeyLabel(nfc), normalizeKeyLabel(nfd))
		assert.Equal(t, "ünïcødé", normalizeKeyLabel(nfd))
	})

}

// The ladder itself — which of the two stored facts the label comes from.
// Each case fails loudly if the branches are reordered: swapping slug and
// name would spell `manual_property` as `manual_property_renamed`, and
// dropping the `slug == key` arm would spell a bson-slugged relation
// `_6a7663db61fab21cd4b9e101`, one character longer than the key it already
// had.
func TestPropertyLabel(t *testing.T) {
	const bson = "6a7663db61fab21cd4b9e101"

	t.Run("a conforming stored slug is the label, verbatim", func(t *testing.T) {
		assert.Equal(t, "manual_property", PropertyLabel(bson, "manual_property", "Manual property renamed"))
	})

	t.Run("a stored slug the grammar refuses is normalized", func(t *testing.T) {
		assert.Equal(t, "_50_done", PropertyLabel(bson, "50_done", "50% done"))
	})

	t.Run("no usable slug falls to the display name", func(t *testing.T) {
		assert.Equal(t, "publish_date", PropertyLabel(bson, "", "Publish Date"))
	})

	t.Run("a slug that merely repeats the stored key is not a slug", func(t *testing.T) {
		// production rows store the bson id as their own apiObjectKey; the
		// name is the only thing left that can name them
		assert.Equal(t, "recipe", PropertyLabel(bson, bson, "Recipe"))
	})

	t.Run("nothing derivable leaves the stored key as its own label", func(t *testing.T) {
		assert.Equal(t, "", PropertyLabel(bson, "#", "#"))
		assert.Equal(t, "", PropertyLabel(bson, "", ""))
	})

	t.Run("a label equal to the stored key is not a label", func(t *testing.T) {
		// nothing is gained and the legend rule would owe an identity entry
		// either way
		assert.Equal(t, "", PropertyLabel("website", "", "Website"))
	})

	// the schema's propertyNames bound (§3), which the label rule enforces
	// rather than the exporter's after-the-fact warning. Truncating would
	// invent a spelling nobody chose and could collide two long names onto
	// one label; the stored key is right there.
	t.Run("a label past the key bound is refused, never truncated", func(t *testing.T) {
		assert.Equal(t, "", PropertyLabel(bson, "", strings.Repeat("a", maxPropertyKeyLen+1)))
		assert.Equal(t, "", PropertyLabel(bson, strings.Repeat("a", maxPropertyKeyLen+1), ""))
		assert.Len(t, PropertyLabel(bson, "", strings.Repeat("a", maxPropertyKeyLen)), maxPropertyKeyLen)
	})

	// §2 refuses `id` and `type` as property SPELLINGS before any
	// resolution, so minting one produces a label export throws away with a
	// warning. The type namespace has no such reservation — its home
	// surface is a value, not a member name.
	t.Run("the two reserved property spellings are never minted", func(t *testing.T) {
		assert.Equal(t, "", PropertyLabel(bson, "", "id"))
		assert.Equal(t, "", PropertyLabel(bson, "", "Type"))
		assert.Equal(t, "id", TypeLabel(bson, "", "id"))
		assert.Equal(t, "type", TypeLabel(bson, "", "Type"))
	})
}

// labelVocab is a vocabulary that answers with a §3 LABEL rather than with an
// api slug — the shape storeresolver now produces for a space-minted key.
type labelVocab struct{ key, label, typeKey, typeLabel string }

func (v labelVocab) PropertySlug(key string) string {
	if key == v.key {
		return v.label
	}
	return BundledKeyVocabulary{}.PropertySlug(key)
}

func (v labelVocab) PropertyKey(slug string) (string, bool) {
	if slug == v.label {
		return v.key, true
	}
	return BundledKeyVocabulary{}.PropertyKey(slug)
}

func (v labelVocab) TypeSlug(key string) string {
	if key == v.typeKey {
		return v.typeLabel
	}
	return BundledKeyVocabulary{}.TypeSlug(key)
}

func (v labelVocab) TypeKey(slug string) (string, bool) {
	if slug == v.typeLabel {
		return v.typeKey, true
	}
	return BundledKeyVocabulary{}.TypeKey(slug)
}

// A label is not ASCII, and the whole codec has to survive that: the label
// rule keeps any script the §6.2.1 grammar admits, so `Тоггл` labels a
// property `тоггл` and the format writes that as a JSON member name, as a
// legend spelling, and as an envelope type term.
//
// This is I1 (§11 — "Marshal never emits what its own Validate rejects") for
// the one string shape the format itself now MINTS. It is not implied by the
// label rule being correct: the published schema states `propertyNames` as a
// pattern, and had it been the api key's `^[a-zA-Z0-9_]+$` — which is what
// every other slug-shaped surface in this codebase carries — every non-Latin
// label would have validated as an error against the document that had just
// been written.
func TestNonASCIILabelSurvivesTheWholeCodec(t *testing.T) {
	// given
	const key = "6a7663db61fab21cd4b9e101"
	const typeKey = "6a7663db61fab21cd4b9e103"
	vocab := labelVocab{key: key, label: "тоггл", typeKey: typeKey, typeLabel: "日本語のプロパティ"}
	snapshot := &model.SmartBlockSnapshotBase{
		Details: &types.Struct{Fields: map[string]*types.Value{
			// an explicit id, or the byte-stability check below trips over
			// the documented id exception (§11.2) rather than over a label
			"id":   pbtypes.String("o1"),
			"name": pbtypes.String("A page"),
			key:    pbtypes.String("on"),
		}},
		ObjectTypes: []string{"ot-" + typeKey},
	}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snapshot, Options{Keys: vocab})
	require.NoError(t, err)

	// then: the document spells the labels, and carries the legends that
	// invert them
	var doc struct {
		Type         string            `json:"type"`
		Properties   map[string]any    `json:"properties"`
		PropertyKeys map[string]string `json:"property_keys"`
		TypeKeys     map[string]string `json:"type_keys"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "on", doc.Properties["тоггл"])
	assert.Equal(t, "日本語のプロパティ", doc.Type)
	assert.Equal(t, key, doc.PropertyKeys["тоггл"])
	assert.Equal(t, typeKey, doc.TypeKeys["日本語のプロパティ"])

	// and its own validation accepts it (I1) — with NO vocabulary, which is
	// the reader the schema speaks for
	require.NoError(t, Validate(data))

	// and it reads back onto the stored keys, through the legend alone
	_, back, err := Unmarshal(data, Options{})
	require.NoError(t, err)
	assert.Equal(t, "on", back.Details.Fields[key].GetStringValue())
	assert.Equal(t, []string{"ot-" + typeKey}, back.ObjectTypes)

	// and the round trip is byte-stable (§11.2)
	again, err := Marshal(model.SmartBlockType_Page, back, Options{Keys: vocab})
	require.NoError(t, err)
	assert.Equal(t, string(data), string(again))
}

// The slug says WHICH WORD; within one word, the name says how to spell it.
//
// api slugs are minted through a snake-caser that splits acronyms and digit
// runs, so where slug and name disagree about breaks only, the slug is
// reliably the mangled half. They are one fold class either way
// (bundle.FoldApiKey drops `_`), so re-spelling can neither lose the address
// nor create a collision that did not already exist.
//
// How this can fail: drop the fold-class arm and the mangled spellings come
// back; widen it past the fold class and a deliberately namespaced slug is
// replaced by the bare name it was namespaced to disambiguate.
func TestPropertyLabel_TheNameSpellsTheSlugsOwnWord(t *testing.T) {
	const key = "69bbfc78877a91b1d12d1a89"
	for name, tc := range map[string]struct{ slug, display, want string }{
		"a mangled slug is re-spelled by its name": {
			slug: "p_2_p_sync", display: "P2P Sync", want: "p2p_sync",
		},
		"acronym mangling likewise": {
			slug: "platform_sd_ks", display: "Platform SDKs", want: "platform_sdks",
		},
		"a slug naming a DIFFERENT word keeps its spelling": {
			// the space namespaced this on purpose; "Rating" alone would
			// collide with every other rating in the space
			slug: "restaurant_rating", display: "Rating", want: "restaurant_rating",
		},
		"an abbreviating slug keeps its spelling": {
			slug: "workspace_id", display: "Space", want: "workspace_id",
		},
		"a namespace prefix survives on both sides": {
			slug: "__amemory_salience", display: "__amemory_salience", want: "__amemory_salience",
		},
		"no slug falls to the name, as before": {
			slug: "", display: "Website", want: "website",
		},
		"no slug and no name has no label but the stored key": {
			slug: "", display: "", want: "",
		},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, PropertyLabel(key, tc.slug, tc.display))
		})
	}
}
