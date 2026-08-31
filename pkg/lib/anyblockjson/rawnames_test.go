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

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

// nameVocab is the stand-in for a SPACE-backed vocabulary in this package's
// own tests, and it is held to the shipped one's contract rather than to a
// convenient approximation of it. storeresolver is the implementation it
// mirrors, method for method:
//
//   - the emit side runs grant's degradation ladder, so a name that is some
//     other live entity's stored key degrades to `<name> (<tail6>)` here too,
//     instead of silently falling back to the plain stored key;
//   - the candidate lists are SETS, sorted, keyed by the GRANTED LABEL — a
//     claimant that degraded is listed under the spelling it actually writes;
//   - an exact live stored key outranks every table (verbatim-first);
//   - an ambiguous candidate set is REFUSED. Nothing behind it is consulted,
//     the fold layer included.
//
// The last two are what a double gets wrong for free. Falling through to
// BundledKeyVocabulary after an ambiguous set — which this double used to do
// — means every test that thought it was pinning "the importer refuses to
// guess" was really pinning "the double guessed the bundled twin", and no
// assertion in the file could tell the difference. Duplicate-blind candidate
// lists are the same shape of blindness one layer down: the importer reads a
// candidate list as a COUNT, so a double that cannot produce a wrong count
// cannot test what the importer does with one (keycandidates_test.go supplies
// a deliberately duplicating vocabulary for exactly that).
type nameVocab struct {
	names     map[string]string   // stored key -> display name
	typeNames map[string]string   // stored type key -> display name
	typeProps map[string][]string // stored type key -> its property keys
}

func (v nameVocab) storedKey(term string) bool {
	_, ok := v.names[term]
	return ok
}

func (v nameVocab) storedTypeKey(term string) bool {
	_, ok := v.typeNames[term]
	return ok
}

// grantedLabel is keyMaps.grant's ladder, the half a double can drop without
// any assertion noticing: a name that is some OTHER live entity's stored key
// could never resolve back to its owner, because an exact stored key outranks
// every table at every reader. Its holder therefore degrades to
// `<name> (<tail6>)`, and to no label at all when even that string is taken or
// cannot be built — in which case the stored key is the spelling.
func grantedLabel(name, key string, storedKey func(string) bool) string {
	if name == "" || !storedKey(name) {
		return name
	}
	label := DisambiguatedKeySpelling(name, key)
	if label == "" || storedKey(label) {
		return ""
	}
	return label
}

// propertyLabel / typeLabel are labelByKey: the spelling one live, visible,
// non-bundled entity is granted. A bundled key takes the code table's word in
// every space, so the space row never speaks for it.
func (v nameVocab) propertyLabel(key string) string {
	if bundle.HasRelation(domain.RelationKey(key)) {
		return ""
	}
	return grantedLabel(PropertyLabel(key, v.names[key]), key, v.storedKey)
}

func (v nameVocab) typeLabel(key string) string {
	if bundle.HasObjectTypeByKey(domain.TypeKey(key)) {
		return ""
	}
	return grantedLabel(TypeLabel(key, v.typeNames[key]), key, v.storedTypeKey)
}

func (v nameVocab) PropertySlug(key string) string {
	if bundle.HasRelation(domain.RelationKey(key)) {
		// the bundled table is the authority in every space and offline —
		// unless a live stored key owns the very string, which outranks it
		if spelling := (BundledKeyVocabulary{}).PropertySlug(key); spelling != key && !v.storedKey(spelling) {
			return spelling
		}
		return key
	}
	if label := v.propertyLabel(key); label != "" {
		return label
	}
	return key
}

func (v nameVocab) TypeSlug(key string) string {
	if bundle.HasObjectTypeByKey(domain.TypeKey(key)) {
		if spelling := (BundledKeyVocabulary{}).TypeSlug(key); spelling != key && !v.storedTypeKey(spelling) {
			return spelling
		}
		return key
	}
	if label := v.typeLabel(key); label != "" {
		return label
	}
	return key
}

func (v nameVocab) PropertyKey(spelling string) (string, bool) {
	if v.storedKey(spelling) {
		return spelling, false // verbatim-first: an exact stored key wins
	}
	switch cands := v.PropertyKeyCandidates(spelling); len(cands) {
	case 1:
		return cands[0], true
	case 0:
		return BundledKeyVocabulary{}.PropertyKey(spelling) // the forgiving fold
	default:
		// several live claimants: refused outright, and the fold layer is not
		// consulted either. A caller with type context may resolve this; a
		// caller without one degrades to the verbatim term, never to a guess
		return spelling, false
	}
}

func (v nameVocab) TypeKey(spelling string) (string, bool) {
	if v.storedTypeKey(spelling) {
		return spelling, false
	}
	switch cands := v.TypeKeyCandidates(spelling); len(cands) {
	case 1:
		return cands[0], true
	case 0:
		return BundledKeyVocabulary{}.TypeKey(spelling)
	default:
		return spelling, false
	}
}

// PropertyKeyCandidates / TypeKeyCandidates are keysByLabel plus the bundled
// table's binding: every live claimant of the exact spelling, as a sorted SET.
// They are keyed by the granted LABEL, not by the raw name — a claimant that
// degraded through the ladder answers to the spelling it actually writes, and
// no longer to the one it lost.
func (v nameVocab) PropertyKeyCandidates(spelling string) []string {
	out := newTestKeySet()
	for key := range v.names {
		if v.propertyLabel(key) == spelling {
			out.add(key)
		}
	}
	if key, ok := BundledPropertyKeyByName(spelling); ok {
		out.add(key)
	}
	return out.sorted()
}

func (v nameVocab) TypeKeyCandidates(spelling string) []string {
	out := newTestKeySet()
	for key := range v.typeNames {
		if v.typeLabel(key) == spelling {
			out.add(key)
		}
	}
	if key, ok := BundledTypeKeyByName(spelling); ok {
		out.add(key)
	}
	return out.sorted()
}

// TypePropertyKeys is a set too: the importer intersects it with the candidate
// list and counts what survives, so a property the type named twice would stop
// the type from singling out its own property.
func (v nameVocab) TypePropertyKeys(typeKey string) []string {
	out := newTestKeySet()
	for _, key := range v.typeProps[typeKey] {
		out.add(key)
	}
	return out.keys
}

func (v nameVocab) PropertyTermFacts(term string) KeyTermFacts {
	facts := KeyTermFacts{LiveStoredKey: v.storedKey(term)}
	facts.ExtendsName = extendedLiveName(term, v.names, PropertyLabel)
	return facts
}

func (v nameVocab) TypeTermFacts(term string) KeyTermFacts {
	return KeyTermFacts{
		LiveStoredKey: v.storedTypeKey(term),
		ExtendsName:   extendedLiveName(term, v.typeNames, TypeLabel),
	}
}

// extendedLiveName is extendsLiveName: the live name the term extends past a
// word boundary, longest first, ties broken lexicographically so the answer
// does not depend on Go's map order.
func extendedLiveName(term string, names map[string]string, label func(key, name string) string) string {
	var best string
	for key, name := range names {
		name = label(key, name)
		if name == "" || !KeyTermExtendsName(term, name) {
			continue
		}
		if len(name) > len(best) || (len(name) == len(best) && name < best) {
			best = name
		}
	}
	return best
}

// testKeySet accumulates stored keys in first-seen order, dropping repeats.
type testKeySet struct {
	keys []string
	seen map[string]bool
}

func newTestKeySet() *testKeySet {
	return &testKeySet{seen: map[string]bool{}}
}

func (s *testKeySet) add(key string) {
	if key == "" || s.seen[key] {
		return
	}
	s.seen[key] = true
	s.keys = append(s.keys, key)
}

func (s *testKeySet) sorted() []string {
	sortStrings(s.keys)
	return s.keys
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

// A name that IS some other live property's stored key. Verbatim-first
// outranks every table at every reader, so that spelling can never resolve to
// the entity merely NAMED it — and the holder degrades through grant's ladder
// rather than writing a spelling that lands on somebody else's row. The bson
// key is unreadable, so the rung taken is the disambiguated name, not the raw
// key; the entity that OWNS the string keeps its own plain name, because
// nothing contests it.
//
// This is the arm this file's header has always claimed and the test double
// could not previously produce: the double skipped the ladder's middle rung
// and answered with the raw 24-hex stored key, which is a spelling the shipped
// vocabulary never writes for a bson id.
func TestRawNames_ANameThatIsAnotherLivePropertysStoredKey(t *testing.T) {
	// given — one relation whose STORED KEY is the string "Projects", and
	// another whose display NAME is "Projects"
	const holder = "6a7663db61fab21cd4b90044"
	vocab := nameVocab{names: map[string]string{
		"Projects": "Task list",
		holder:     "Projects",
	}}
	snap := &model.SmartBlockSnapshotBase{Details: &types.Struct{Fields: map[string]*types.Value{
		"id":       pbtypes.String("o1"),
		"Projects": pbtypes.String("value of the key holder"),
		holder:     pbtypes.String("value of the named one"),
	}}}
	want := map[string]string{
		"Task list":         "value of the key holder",
		"Projects (b90044)": "value of the named one",
	}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, Options{Keys: vocab})
	require.NoError(t, err)

	// then — I1, and neither claimant wrote the contested string
	require.NoError(t, Validate(data), "I1:\n%s", data)
	var doc struct {
		Properties   map[string]string `json:"properties"`
		PropertyKeys map[string]string `json:"property_internal_keys"`
	}
	require.NoError(t, json.Unmarshal(data, &doc))
	assert.Equal(t, want, doc.Properties)
	assert.NotContains(t, doc.Properties, "Projects",
		"the string is one entity's address and the other's lost name: it is written for neither")
	assert.Equal(t, holder, doc.PropertyKeys["Projects (b90044)"],
		"the degraded spelling is nobody's chain, so the legend states it")

	// and back onto both stored keys
	_, back, err := Unmarshal(data, Options{GenerateId: seqIds("g"), Keys: vocab})
	require.NoError(t, err)
	assert.Equal(t, "value of the key holder", back.Details.Fields["Projects"].GetStringValue())
	assert.Equal(t, "value of the named one", back.Details.Fields[holder].GetStringValue())

	// fixpoint — the tail6 suffix is derived, so generation 2 re-derives it
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
	doc := `{"version":2,"id":"o1","property_internal_keys":{"Star️":"` + key + `"},` +
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
		doc := `{"version":2,"id":"o1","type":"Sprint","properties":{"Projects":"resolved"}}`
		_, back, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), Keys: vocab})
		require.NoError(t, err)
		assert.Equal(t, "resolved", back.Details.Fields[keyA].GetStringValue(),
			"unambiguous among the type's own properties — resolved, silently")
		assert.Nil(t, back.Details.Fields[keyB])
	})

	t.Run("a type that cannot place the name errors loudly", func(t *testing.T) {
		doc := `{"version":2,"id":"o1","type":"Page","properties":{"Projects":"?"}}`
		_, _, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g"), Keys: vocab})
		require.Error(t, err, "never a guess, never a phantom while two live properties bear the name")
		assert.Contains(t, err.Error(), `"Projects"`)
		assert.Contains(t, err.Error(), memberPropertyInternalKeys,
			"the error asks for the legend — the one statement that settles it")
	})

	t.Run("the legend outranks the whole question", func(t *testing.T) {
		doc := `{"version":2,"id":"o1","type":"Page",` +
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
		doc := `{"version":2,"id":"o1","type":"Meeting"}`
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
		msgs := collect(`{"version":2,"id":"o1","properties":{"Budget":"1", "b1": {"x": 1}}}`,
			Options{Keys: vocab})
		joined := strings.Join(msgs, "\n")
		assert.Contains(t, joined, `"Budget"`)
		assert.Contains(t, joined, "phantom")
	})

	t.Run("the glued annotation names the live name it extends", func(t *testing.T) {
		msgs := collect(`{"version":2,"id":"o1","properties":{"Lists [in work] (text)":"x"}}`,
			Options{Keys: vocab})
		joined := strings.Join(msgs, "\n")
		assert.Contains(t, joined, `"Lists [in work] (text)"`)
		assert.Contains(t, joined, `"Lists [in work]"`)
		assert.Contains(t, joined, "glued")
	})

	t.Run("a bundled name's glue is caught with no vocabulary at all", func(t *testing.T) {
		msgs := collect(`{"version":2,"id":"o1","properties":{"Creation date (text)":"x"}}`,
			Options{})
		joined := strings.Join(msgs, "\n")
		assert.Contains(t, joined, `"Creation date"`)
		assert.Contains(t, joined, "glued")
	})

	t.Run("one term, one warning, however many slots name it", func(t *testing.T) {
		doc := `{"version":2,"id":"o1","properties":{"Budget":"1"},"blocks":[
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
