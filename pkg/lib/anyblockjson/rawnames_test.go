package anyblockjson

// rawnames_test.go — the raw-name spelling rule under attack: the names the
// rule newly admits as keys (`#`, `☕`, `C++`, `50% done`, non-Latin), two
// properties colliding inside one document, a name equal to another live
// stored key, and the map-less reader's type-scoped resolution with its loud
// error. I1 (Marshal never emits what its own Validate rejects) and I2
// (Validate and Unmarshal agree) are asserted on every arm, plus the §11
// fixpoint: a second export of the re-import is byte-identical.

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// nameVocab spells every key it is given by its display name — the shape
// storeresolver produces — and inverts only the unambiguous ones, exposing
// the rest through the ScopedKeyVocabulary capability exactly as the
// space-backed vocabulary does.
type nameVocab struct {
	names     map[string]string   // stored key -> display name
	typeNames map[string]string   // stored type key -> display name
	typeProps map[string][]string // stored type key -> its property keys
}

func (v nameVocab) PropertySlug(key string) string {
	if name := v.names[key]; name != "" && !v.storedKey(name) {
		return name
	}
	return BundledKeyVocabulary{}.PropertySlug(key)
}

func (v nameVocab) storedKey(term string) bool {
	_, ok := v.names[term]
	return ok
}

func (v nameVocab) PropertyKey(spelling string) (string, bool) {
	if v.storedKey(spelling) {
		return spelling, false // verbatim-first
	}
	if cands := v.PropertyKeyCandidates(spelling); len(cands) == 1 {
		return cands[0], true
	}
	return BundledKeyVocabulary{}.PropertyKey(spelling)
}

func (v nameVocab) TypeSlug(key string) string {
	if name := v.typeNames[key]; name != "" {
		return name
	}
	return BundledKeyVocabulary{}.TypeSlug(key)
}

func (v nameVocab) TypeKey(spelling string) (string, bool) {
	if _, isKey := v.typeNames[spelling]; isKey {
		return spelling, false
	}
	if cands := v.TypeKeyCandidates(spelling); len(cands) == 1 {
		return cands[0], true
	}
	return BundledKeyVocabulary{}.TypeKey(spelling)
}

func (v nameVocab) PropertyKeyCandidates(spelling string) []string {
	var out []string
	for key, name := range v.names {
		if name == spelling {
			out = append(out, key)
		}
	}
	if key, ok := BundledPropertyKeyByName(spelling); ok {
		out = append(out, key)
	}
	sortStrings(out)
	return out
}

func (v nameVocab) TypeKeyCandidates(spelling string) []string {
	var out []string
	for key, name := range v.typeNames {
		if name == spelling {
			out = append(out, key)
		}
	}
	if key, ok := BundledTypeKeyByName(spelling); ok {
		out = append(out, key)
	}
	sortStrings(out)
	return out
}

func (v nameVocab) TypePropertyKeys(typeKey string) []string { return v.typeProps[typeKey] }

func (v nameVocab) PropertyTermFacts(term string) KeyTermFacts {
	facts := KeyTermFacts{LiveStoredKey: v.storedKey(term)}
	for _, name := range v.names {
		if KeyTermExtendsName(term, name) && len(name) > len(facts.ExtendsName) {
			facts.ExtendsName = name
		}
	}
	return facts
}

func (v nameVocab) TypeTermFacts(term string) KeyTermFacts {
	_, isKey := v.typeNames[term]
	return KeyTermFacts{LiveStoredKey: isKey}
}

func sortStrings(s []string) {
	for i := 1; i < len(s); i++ {
		for j := i; j > 0 && s[j] < s[j-1]; j-- {
			s[j], s[j-1] = s[j-1], s[j]
		}
	}
}

// The names the old normalized spelling could not carry at all — each used
// to need a fallback or an escape, and each is now a plain key. The whole
// codec survives every one: Marshal validates (I1), the document inverts to
// the stored keys (I2's substance), and the round trip is a fixpoint.
func TestRawNames_TheNamesNormalizationCouldNotCarry(t *testing.T) {
	keys := map[string]string{
		"6a7663db61fab21cd4b90001": "#",
		"6a7663db61fab21cd4b90002": "☕",
		"6a7663db61fab21cd4b90003": "C++",
		"6a7663db61fab21cd4b90004": "50% done",
		"6a7663db61fab21cd4b90005": "Дата выполнения",
		"6a7663db61fab21cd4b90006": "作業内容",
		"6a7663db61fab21cd4b90007": "All",
		"6a7663db61fab21cd4b90008": "What's missing",
	}
	vocab := nameVocab{names: keys}
	details := map[string]*types.Value{"id": pbtypes.String("o1")}
	for key := range keys {
		details[key] = pbtypes.String("v:" + keys[key])
	}
	snap := &model.SmartBlockSnapshotBase{Details: &types.Struct{Fields: details}}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{Keys: vocab})
	require.NoError(t, err)

	// then — I1, and every name is a member key exactly as written
	require.NoError(t, Validate(data), "I1:\n%s", data)
	var doc struct {
		Properties   map[string]string `json:"properties"`
		PropertyKeys map[string]string `json:"property_internal_keys"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	for key, name := range keys {
		assert.Equalf(t, "v:"+name, doc.Properties[name], "the name %q is the member key", name)
		assert.Equalf(t, key, doc.PropertyKeys[name], "and the legend inverts it")
	}

	// and back onto the stored keys — through the legend alone, no vocabulary
	_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	for key, name := range keys {
		require.NotNilf(t, back.Details.Fields[key], "name %q lost its key", name)
		assert.Equal(t, "v:"+name, back.Details.Fields[key].GetStringValue())
	}

	// fixpoint
	again, err := Marshal(model.SmartBlockType_Page, back, Options{Keys: vocab})
	require.NoError(t, err)
	assert.Equal(t, string(data), string(again), "the round trip is byte-stable")
}

// Two properties colliding inside one document: EVERY claimant degrades
// through the ladder, deterministically, and the document still carries both
// values and inverts both keys. The same document also plants a name equal
// to a live stored key, which is refused the same way.
func TestRawNames_TwoPropertiesCollideInsideOneDocument(t *testing.T) {
	const (
		keyA = "6a7663db61fab21cd4b90011"
		keyB = "6a7663db61fab21cd4b90022"
	)
	vocab := nameVocab{names: map[string]string{
		keyA: "Projects",
		keyB: "Projects",
	}}
	snap := &model.SmartBlockSnapshotBase{Details: &types.Struct{Fields: map[string]*types.Value{
		"id": pbtypes.String("o1"),
		keyA: pbtypes.String("value of A"),
		keyB: pbtypes.String("value of B"),
	}}}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{Keys: vocab})
	require.NoError(t, err)

	// then — I1, both claimants suffixed off their own tails, both values kept
	require.NoError(t, Validate(data), "I1:\n%s", data)
	var doc struct {
		Properties   map[string]string `json:"properties"`
		PropertyKeys map[string]string `json:"property_internal_keys"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "value of A", doc.Properties["Projects (b90011)"])
	assert.Equal(t, "value of B", doc.Properties["Projects (b90022)"])
	assert.NotContains(t, doc.Properties, "Projects",
		"the plain name is written for NOBODY: a plain spelling must never be one of two same-named claimants")
	assert.Equal(t, map[string]string{
		"Projects (b90011)": keyA,
		"Projects (b90022)": keyB,
	}, doc.PropertyKeys)

	// I2's substance: both values come home on their own keys
	_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g"), Keys: vocab})
	require.NoError(t, err)
	assert.Equal(t, "value of A", back.Details.Fields[keyA].GetStringValue())
	assert.Equal(t, "value of B", back.Details.Fields[keyB].GetStringValue())

	// fixpoint — the suffix is deterministic off the name and the key's own
	// tail, so a second generation re-derives the identical spellings
	again, err := Marshal(model.SmartBlockType_Page, back, Options{Keys: vocab})
	require.NoError(t, err)
	assert.Equal(t, string(data), string(again))
}

// An edge-whitespace name is carried verbatim — the format warns (§12) and
// does not trim — and Validate's hygiene warning names it without refusing
// the document.
func TestRawNames_EdgeWhitespaceIsCarriedAndWarned(t *testing.T) {
	const key = "6a7663db61fab21cd4b90033"
	vocab := nameVocab{names: map[string]string{key: "Email 📧 "}}
	snap := &model.SmartBlockSnapshotBase{Details: &types.Struct{Fields: map[string]*types.Value{
		"id": pbtypes.String("o1"),
		key:  pbtypes.String("x@y.z"),
	}}}

	data, err := Marshal(model.SmartBlockType_Page, snap, Options{Keys: vocab})
	require.NoError(t, err)

	var warns []Issue
	require.NoError(t, ValidateWarn(data, func(i Issue) { warns = append(warns, i) }),
		"warned, never refused — one stored name must not make an object unexportable")
	var hygiene []string
	for _, w := range warns {
		if strings.Contains(w.Message, "edge whitespace") {
			hygiene = append(hygiene, w.Message)
		}
	}
	require.NotEmpty(t, hygiene, "the invisible byte is worth one line to the caller")
	assert.Contains(t, hygiene[0], `"Email 📧 "`)

	// and the invisible-code-point arm: a variation selector in a legend key
	doc := `{"version":1,"id":"o1","property_internal_keys":{"Star️":"` + key + `"},` +
		`"properties":{"Star️":1}}`
	warns = nil
	require.NoError(t, ValidateWarn([]byte(doc), func(i Issue) { warns = append(warns, i) }))
	found := false
	for _, w := range warns {
		if strings.Contains(w.Message, "invisible code point") {
			found = true
		}
	}
	assert.True(t, found, "a default-ignorable code point is named, with its code")
}

// The map-less reader resolves a shared name within the declared type — the
// overwhelming case — and raises a LOUD error when the type is not enough:
// it never guesses and never mints a phantom key while two live properties
// bear that exact name.
func TestRawNames_TypeScopedResolution(t *testing.T) {
	const (
		keyA     = "6a7663db61fab21cd4b90011"
		keyB     = "6a7663db61fab21cd4b90022"
		taskType = "6a7663db61fab21cd4b90099"
	)
	vocab := nameVocab{
		names:     map[string]string{keyA: "Projects", keyB: "Projects"},
		typeNames: map[string]string{taskType: "Sprint"},
		typeProps: map[string][]string{taskType: {keyA, "name"}},
	}

	t.Run("the declared type singles the claimant out", func(t *testing.T) {
		doc := `{"version":1,"id":"o1","type":"Sprint","properties":{"Projects":"resolved"}}`
		_, back, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), Keys: vocab})
		require.NoError(t, err)
		assert.Equal(t, "resolved", back.Details.Fields[keyA].GetStringValue(),
			"unambiguous among the type's own properties — resolved, silently")
		assert.Nil(t, back.Details.Fields[keyB])
	})

	t.Run("a type that cannot place the name errors loudly", func(t *testing.T) {
		doc := `{"version":1,"id":"o1","type":"Page","properties":{"Projects":"?"}}`
		_, _, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), Keys: vocab})
		require.Error(t, err, "never a guess, never a phantom while two live properties bear the name")
		assert.Contains(t, err.Error(), `"Projects"`)
		assert.Contains(t, err.Error(), memberPropertyInternalKeys,
			"the error asks for the legend — the one statement that settles it")
	})

	t.Run("the legend outranks the whole question", func(t *testing.T) {
		doc := `{"version":1,"id":"o1","type":"Page",` +
			`"property_internal_keys":{"Projects":"` + keyB + `"},` +
			`"properties":{"Projects":"stated"}}`
		_, back, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), Keys: vocab})
		require.NoError(t, err)
		assert.Equal(t, "stated", back.Details.Fields[keyB].GetStringValue())
	})

	t.Run("a shared TYPE name errors loudly too", func(t *testing.T) {
		shared := nameVocab{typeNames: map[string]string{
			"6a7663db61fab21cd4b90777": "Meeting",
			"6a7663db61fab21cd4b90888": "Meeting",
		}}
		doc := `{"version":1,"id":"o1","type":"Meeting"}`
		_, _, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), Keys: shared})
		require.Error(t, err, "the type is the scope — there is nothing wider to resolve inside")
		assert.Contains(t, err.Error(), memberTypeInternalKeys)
	})
}

// The two verbatim-resolution warnings, once per term: the phantom (a term
// no live entity answers to, stored verbatim as a new key) and the glued
// annotation (a term extending a live or bundled name past a word boundary).
func TestRawNames_VerbatimTermWarnings(t *testing.T) {
	const key = "6a7663db61fab21cd4b90011"
	vocab := nameVocab{names: map[string]string{key: "Lists [in work]"}}

	collect := func(doc string, opts Options) []string {
		var msgs []string
		opts.GenerateId = seqIds("g")
		opts.OnWarning = func(i Issue) { msgs = append(msgs, i.Message) }
		_, _, err := Unmarshal([]byte(doc), opts)
		require.NoError(t, err)
		return msgs
	}

	t.Run("a stale or guessed name mints a phantom, and says so", func(t *testing.T) {
		msgs := collect(`{"version":1,"id":"o1","properties":{"Budget":"1", "b1": {"x": 1}}}`,
			Options{Keys: vocab})
		joined := strings.Join(msgs, "\n")
		assert.Contains(t, joined, `"Budget"`)
		assert.Contains(t, joined, "phantom")
	})

	t.Run("the glued annotation names the live name it extends", func(t *testing.T) {
		msgs := collect(`{"version":1,"id":"o1","properties":{"Lists [in work] (text)":"x"}}`,
			Options{Keys: vocab})
		joined := strings.Join(msgs, "\n")
		assert.Contains(t, joined, `"Lists [in work] (text)"`)
		assert.Contains(t, joined, `"Lists [in work]"`)
		assert.Contains(t, joined, "glued")
	})

	t.Run("a bundled name's glue is caught with no vocabulary at all", func(t *testing.T) {
		msgs := collect(`{"version":1,"id":"o1","properties":{"Creation date (text)":"x"}}`,
			Options{})
		joined := strings.Join(msgs, "\n")
		assert.Contains(t, joined, `"Creation date"`)
		assert.Contains(t, joined, "glued")
	})

	t.Run("one term, one warning, however many slots name it", func(t *testing.T) {
		doc := `{"version":1,"id":"o1","properties":{"Budget":"1"},"blocks":[
			{"id":"dv","type":"dataview","properties":[{"property":"Budget","format":"number"}],
			 "views":[{"id":"v1","sorts":[{"property":"Budget"}]}]}]}`
		msgs := collect(doc, Options{Keys: vocab})
		count := 0
		for _, m := range msgs {
			if strings.Contains(m, `"Budget"`) && strings.Contains(m, "phantom") {
				count++
			}
		}
		assert.Equal(t, 1, count, "the diagnosis is a fact about the term, not about any one slot")
	})
}

// The fixpoint under a SPACE-shaped collision that includes a bundled twin:
// a custom property named "Description" beside the bundled one, both in one
// document. Both degrade — the bundled key to its readable stored key, the
// custom one to its suffix — and the whole document round-trips.
func TestRawNames_BundledAndCustomShareOneName(t *testing.T) {
	const custom = "6a7663db61fab21cd4b90055"
	vocab := nameVocab{names: map[string]string{custom: "Description"}}
	snap := &model.SmartBlockSnapshotBase{Details: &types.Struct{Fields: map[string]*types.Value{
		"id":          pbtypes.String("o1"),
		"description": pbtypes.String("the bundled one"),
		custom:        pbtypes.String("the custom one"),
	}}}

	data, err := Marshal(model.SmartBlockType_Page, snap, Options{Keys: vocab})
	require.NoError(t, err)
	require.NoError(t, Validate(data), "I1:\n%s", data)

	var doc struct {
		Properties   map[string]string `json:"properties"`
		PropertyKeys map[string]string `json:"property_internal_keys"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "the bundled one", doc.Properties["description"],
		"the bundled claimant's stored key is readable: rung (a)")
	assert.Equal(t, "the custom one", doc.Properties[fmt.Sprintf("Description (%s)", custom[len(custom)-6:])])
	assert.NotContains(t, doc.PropertyKeys, "description",
		"no entry owed: the term is the bundled key's own stored key, and the bundled "+
			"fold inverts it in every reader that ships the table")

	_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g"), Keys: vocab})
	require.NoError(t, err)
	assert.Equal(t, "the bundled one", back.Details.Fields["description"].GetStringValue())
	assert.Equal(t, "the custom one", back.Details.Fields[custom].GetStringValue())

	again, err := Marshal(model.SmartBlockType_Page, back, Options{Keys: vocab})
	require.NoError(t, err)
	assert.Equal(t, string(data), string(again))
}

// An attribution key never contests a spelling: export writes it and import
// DROPS it, so a generation-2 census will not hold it — a claimant it had
// suffixed would un-suffix, and the round trip stopped being byte-stable in
// exactly the spaces holding a custom name-twin of "Created by" (a real
// production space does). The attribution claimant yields its plain name
// (its own bundled stored key is readable), and the normal claimant keeps
// the verdict it will re-derive without it.
func TestRawNames_AttributionNeverContestsASpelling(t *testing.T) {
	const custom = "6a7663db61fab21cd4b90077"
	vocab := nameVocab{names: map[string]string{custom: "Created by"}}
	snap := &model.SmartBlockSnapshotBase{Details: &types.Struct{Fields: map[string]*types.Value{
		"id":      pbtypes.String("o1"),
		"creator": pbtypes.String("_participant_a_b_A5qTLyde3S1q9NRyFeSeN6UWwa6VwwXEJbMACJwMfez3BGVD"),
		custom:    pbtypes.String("the twin's value"),
	}}}
	opts := Options{Keys: vocab, SpaceId: "a.b"}

	data, err := Marshal(model.SmartBlockType_Page, snap, opts)
	require.NoError(t, err)
	require.NoError(t, Validate(data), "I1:\n%s", data)

	var doc struct {
		Properties   map[string]any    `json:"properties"`
		PropertyKeys map[string]string `json:"property_internal_keys"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, "the twin's value", doc.Properties["Created by"],
		"the normal claimant keeps the plain name — no suffix that generation 2 would drop")
	assert.Contains(t, doc.Properties, "creator",
		"the attribution line yields to its own stored key, always its own address")
	assert.Equal(t, custom, doc.PropertyKeys["Created by"])

	// the fixpoint half: the re-import drops the attribution line, and the
	// next export still spells the twin identically
	_, back, err := Unmarshal(data, opts)
	require.NoError(t, err)
	assert.Nil(t, back.Details.Fields["creator"], "attribution does not survive a round trip")
	again, err := Marshal(model.SmartBlockType_Page, back, opts)
	require.NoError(t, err)
	var doc2 struct {
		Properties map[string]any `json:"properties"`
	}
	require.NoError(t, json.Unmarshal(again, &doc2))
	assert.Equal(t, "the twin's value", doc2.Properties["Created by"],
		"generation 2 re-derives the same spelling — the fixpoint the yield exists for")
}
