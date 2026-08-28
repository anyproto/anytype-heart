package anyblockjson

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// The label rule (§3): a space-minted key is spelled by its display name,
// NFC-normalized, otherwise verbatim. There is no normalization step left to
// fail, so the old repair machinery — the empty-normalization fallback, the
// leading-`_` escape for digit-initial and keyword names — is deleted, not
// reimplemented, and this table pins that the inputs which used to NEED it
// are now plain spellings.
//
// How this can fail: reintroduce any transform (a case fold, a separator
// collapse, a grammar escape) and the verbatim rows below stop being
// verbatim; drop the writable-key check and a control-character name becomes
// a spelling Validate rejects (I1).
func TestPropertyLabel(t *testing.T) {
	const bson = "6a7663db61fab21cd4b9e101"

	for name, tc := range map[string]struct{ display, want, why string }{
		"a name is the label, verbatim": {
			display: "Publish Date", want: "Publish Date",
			why: "no case fold, no separator collapse"},
		"spaces and punctuation survive": {
			display: "Manual export & import", want: "Manual export & import", why: ""},
		"a one-symbol name is a legal key exactly as written": {
			display: "#", want: "#",
			why: "under a NORMALIZED spelling this normalized to nothing and needed a fallback"},
		"an emoji name likewise": {
			display: "☕", want: "☕", why: ""},
		"a digit-initial name needs no escape": {
			display: "50% done", want: "50% done",
			why: "the leading-underscore escape existed for an identifier grammar keys no longer live in"},
		"a grammar keyword is just a name": {
			display: "All", want: "All", why: ""},
		"non-Latin scripts are kept, never transliterated": {
			display: "Дата выполнения", want: "Дата выполнения", why: ""},
		"CJK likewise": {
			display: "作業内容", want: "作業内容", why: ""},
		"edge whitespace is carried, not trimmed": {
			display: "Email 📧 ", want: "Email 📧 ",
			why: "the format warns about it (§12) but a cleanup belongs at authoring time, not at the export seam"},
		"an empty name has no label but the stored key": {
			display: "", want: "", why: ""},
	} {
		t.Run(name, func(t *testing.T) {
			assert.Equal(t, tc.want, PropertyLabel(bson, tc.display), tc.why)
		})
	}

	t.Run("a name that repeats the stored key is not a label", func(t *testing.T) {
		// the verbatim key already says exactly that, and the legend rule
		// would owe an identity entry either way
		assert.Equal(t, "", PropertyLabel("website", "website"))
	})

	// the schema's propertyNames bound (§3). Truncating would invent a
	// spelling nobody chose and could collide two long names onto one
	// label; the stored key is right there.
	t.Run("a name past the key bound is refused, never truncated", func(t *testing.T) {
		assert.Equal(t, "", PropertyLabel(bson, strings.Repeat("a", maxPropertyKeyLen+1)))
		assert.Len(t, PropertyLabel(bson, strings.Repeat("a", maxPropertyKeyLen)), maxPropertyKeyLen)
	})

	t.Run("a control character has no written form", func(t *testing.T) {
		assert.Equal(t, "", PropertyLabel(bson, "a\nb"))
	})

	// §2 refuses `id` and `type` as property SPELLINGS before any
	// resolution, so minting one produces a label export throws away with a
	// warning. The type namespace has no such reservation — its home
	// surface is a value, not a member name.
	t.Run("the two reserved property spellings are never minted", func(t *testing.T) {
		assert.Equal(t, "", PropertyLabel(bson, "id"))
		assert.Equal(t, "", PropertyLabel(bson, "type"))
		assert.Equal(t, "id", TypeLabel(bson, "id"))
		assert.Equal(t, "type", TypeLabel(bson, "type"))
	})

	// "Type" with a capital T is NOT the refused spelling: the refusal is
	// byte-exact (§2 refuses the member names `id` and `type`), and raw
	// naming means the capitalized name no longer lowercases into the
	// refused form on its way to the wire.
	t.Run("the refusal is byte-exact, not case-folded", func(t *testing.T) {
		assert.Equal(t, "Type", PropertyLabel(bson, "Type"))
		assert.Equal(t, "Id", PropertyLabel(bson, "Id"))
	})

	// Pitfall stated in §3: two visually identical names can be different
	// byte sequences, and a reader matches a spelling byte-for-byte. Export
	// is safe by construction (it writes the same string as label and as
	// legend key), but the space's stored name may itself be decomposed, and
	// two exports of one property must not differ by normalization form.
	t.Run("NFD and NFC forms of one name yield one label", func(t *testing.T) {
		nfc := "Ünïcødé"
		nfd := norm.NFD.String(nfc)
		assert.NotEqual(t, nfc, nfd, "the fixture has to actually differ in bytes")
		assert.Equal(t, PropertyLabel(bson, nfc), PropertyLabel(bson, nfd))
		assert.Equal(t, nfc, PropertyLabel(bson, nfd))
	})
}

// labelVocab is a vocabulary that answers with a §3 LABEL — the shape
// storeresolver produces for a space-minted key: the display name, raw.
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
// rule keeps the name verbatim in any script, so `Тоггл` labels a property
// `Тоггл` and the format writes that as a JSON member name, as a legend
// spelling, and as an envelope type term.
//
// This is I1 (§11 — "Marshal never emits what its own Validate rejects") for
// the string shape the format itself now MINTS. It is not implied by the
// label rule being correct: the published schema states `propertyNames` as a
// pattern, and had it been the api key's `^[a-zA-Z0-9_]+$` — which is what
// every other slug-shaped surface in this codebase carries — every non-Latin
// label would have validated as an error against the document that had just
// been written.
func TestNonASCIILabelSurvivesTheWholeCodec(t *testing.T) {
	// given
	const key = "6a7663db61fab21cd4b9e101"
	const typeKey = "6a7663db61fab21cd4b9e103"
	vocab := labelVocab{key: key, label: "Тоггл", typeKey: typeKey, typeLabel: "日本語のプロパティ"}
	snapshot := &model.SmartBlockSnapshotBase{
		Details: &types.Struct{Fields: map[string]*types.Value{
			// an explicit id, or the byte-stability check below trips over
			// the documented id exception (§11) rather than over a label
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
		PropertyKeys map[string]string `json:"property_internal_keys"`
		TypeKeys     map[string]string `json:"type_internal_keys"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "on", doc.Properties["Тоггл"])
	assert.Equal(t, "日本語のプロパティ", doc.Type)
	assert.Equal(t, key, doc.PropertyKeys["Тоггл"])
	assert.Equal(t, typeKey, doc.TypeKeys["日本語のプロパティ"])

	// and its own validation accepts it (I1) — with NO vocabulary, which is
	// the reader the schema speaks for
	require.NoError(t, Validate(data))

	// and it reads back onto the stored keys, through the legend alone
	_, back, err := Unmarshal(data, Options{})
	require.NoError(t, err)
	assert.Equal(t, "on", back.Details.Fields[key].GetStringValue())
	assert.Equal(t, []string{"ot-" + typeKey}, back.ObjectTypes)

	// and the round trip is byte-stable (§11)
	again, err := Marshal(model.SmartBlockType_Page, back, Options{Keys: vocab})
	require.NoError(t, err)
	assert.Equal(t, string(data), string(again))
}
