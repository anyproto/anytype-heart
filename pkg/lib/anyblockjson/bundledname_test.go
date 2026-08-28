package anyblockjson

// bundledname_test.go — bundled keys spell their display names
// (bundledname.go), the stored keys stay their own verbatim addresses, the
// legacy derived slugs keep resolving through the fold, and the shipped name
// table stays clean enough to carry the whole scheme: the CI guards at the
// bottom are the condition under which a bundled entry may be added or
// renamed.

import (
	"strings"
	"testing"
	"unicode"

	"github.com/gogo/protobuf/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/text/unicode/norm"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// The headline of the uniform rule: a bundled key writes its display name —
// spaces and all — with NO legend entry, because the name table ships with
// every reader. `audioGenre` spelling "Audio genre" is also the rename that
// made this possible: its old name "Genre" collided with the genre
// relation's, and two wire spellings reading "Genre" could not both invert.
//
// How this can fail: fall back to the derived slug and `audio_genre` comes
// back; make recordPropertyKey ask a table that does not bind names and
// every bundled key starts paying a legend line for a spelling every reader
// already knows.
func TestBundledNames_KeysSpellTheirDisplayNames(t *testing.T) {
	// given
	snap := &model.SmartBlockSnapshotBase{
		Blocks: []*model.Block{{Id: "o1",
			Content: &model.BlockContentOfSmartblock{Smartblock: &model.BlockContentSmartblock{}}}},
		Details: fields(map[string]*types.Value{
			"id":         str("o1"),
			"audioGenre": str("ambient"),
		}),
	}

	// when
	data, err := Marshal(model.SmartBlockType_Page, snap, testOptions())
	require.NoError(t, err)

	// then
	assert.Contains(t, string(data), `"Audio genre"`)
	assert.NotContains(t, string(data), `"audio_genre"`,
		"the derived slug must not survive anywhere in the document")
	assert.NotContains(t, string(data), `"property_internal_keys"`,
		"a bundled name is a shipped-table fact and owes no legend entry")
	require.NoError(t, Validate(data), "I1: Marshal never emits what its own Validate rejects")

	_, back, err := Unmarshal(data, testOptions())
	require.NoError(t, err)
	assert.Equal(t, str("ambient"), back.Details.Fields["audioGenre"],
		"the name inverts onto the stored key")
}

// The stored key itself still resolves VERBATIM — §3 chain step 2 is
// untouched by the name layer — and the pre-change derived slug keeps
// resolving through the FOLD, with no compatibility table: ToSnake only
// inserts `_` and lowercases, and the fold strips `_`, `-` and case, so
// every old slug sits in its stored key's fold class.
//
// How this can fail: drop the fold step from BundledKeyVocabulary and every
// document written before the re-spell mints phantom keys in a package-only
// reader; let the name table answer near-misses and an ambiguous class
// resolves by luck.
func TestBundledNames_StoredKeyVerbatimAndLegacySlugThroughTheFold(t *testing.T) {
	t.Run("the stored key is its own address", func(t *testing.T) {
		doc := `{"version":1,"id":"o1","properties":{"audioGenre":"jazz"}}`
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, str("jazz"), snap.Details.Fields["audioGenre"])
	})

	t.Run("the legacy derived slug folds onto its key", func(t *testing.T) {
		doc := `{"version":1,"id":"o1","properties":{"audio_genre":"jazz"}}`
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, str("jazz"), snap.Details.Fields["audioGenre"],
			"fold(ToSnake(key)) == fold(key): the continuity proof, exercised")
		assert.Nil(t, snap.Details.Fields["audio_genre"])
	})

	t.Run("the name-shaped guess folds too", func(t *testing.T) {
		// the §6.3 consolation: "Creation date" is the canonical spelling,
		// and `creation_date` — the guess shaped like the name — lands in
		// the same fold class
		doc := `{"version":1,"id":"o1","properties":{"creation_date":1700000000}}`
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		require.NotNil(t, snap.Details.Fields["createdDate"])
	})

	t.Run("the v0.38 alias spellings come back through the fold", func(t *testing.T) {
		// the alias TABLE is gone, and its spellings resolve anyway: the
		// bundled name says "Property option color", and the fold strips
		// case and `_`, so `property_option_color` lands in that name's
		// class. Nothing had to be kept for back-compat — renaming the
		// eleven bundled names that still said "relation" is what restored
		// them, and it restored the derived-slug form of each new name at
		// the same time.
		doc := `{"version":1,"id":"o1","properties":{"property_option_color":"ice"}}`
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, str("ice"), snap.Details.Fields["relationOptionColor"])
		assert.Nil(t, snap.Details.Fields["property_option_color"])
	})

	t.Run("a spelling no bundled name folds onto passes through verbatim", func(t *testing.T) {
		// pre-freeze, no back-compat: a term whose fold class is nobody's
		// is its own address — chain step 4
		doc := `{"version":1,"id":"o1","properties":{"property_wine_region":"ice"}}`
		_, snap, err := Unmarshal([]byte(doc), Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, str("ice"), snap.Details.Fields["property_wine_region"])
	})
}

// The type namespace carries the same rules: a bundled type spells its
// display name with no legend entry — a property document says
// `"type": "Property"`, because the relation TYPE is named "Property" in
// the bundle and the name carries the v0.38 rename with no table behind it
// — and the stored key still names itself verbatim on the way in.
func TestBundledNames_TypeKeysSpellTheirDisplayNames(t *testing.T) {
	t.Run("export spells the name, no legend owed", func(t *testing.T) {
		data, err := Marshal(model.SmartBlockType_Page, typedSnapshot("ot-relation"), Options{})
		require.NoError(t, err)

		doc := decodeEnvelope(t, data)
		assert.Equal(t, "Property", doc.Type)
		assert.Empty(t, doc.TypeKeys, "a bundled name is a shipped-table fact and owes no legend entry")

		_, snap, err := Unmarshal(data, Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, []string{"ot-relation"}, snap.ObjectTypes,
			"a package-only reader inverts the name through the shipped table")
	})

	t.Run("the stored type key still names itself verbatim", func(t *testing.T) {
		_, snap, err := Unmarshal([]byte(`{"version":1,"id":"o1","type":"relation"}`),
			Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, []string{"ot-relation"}, snap.ObjectTypes,
			"an exact stored key wins before any table (§3 chain step 2)")
	})

	t.Run("the legacy type slug folds onto its key", func(t *testing.T) {
		_, snap, err := Unmarshal([]byte(`{"version":1,"id":"o1","type":"object_type"}`),
			Options{GenerateId: seqIds("g")})
		require.NoError(t, err)
		assert.Equal(t, []string{"ot-objectType"}, snap.ObjectTypes)
	})

	t.Run("the Space pair reads exactly, both ways", func(t *testing.T) {
		// the other commit-one rename: `space` is "Space settings" and
		// spaceView keeps "Space" — two exact names, two exact answers
		key, ok := (BundledKeyVocabulary{}).TypeKey("Space settings")
		require.True(t, ok)
		assert.Equal(t, "space", key)
		key, ok = (BundledKeyVocabulary{}).TypeKey("Space")
		require.True(t, ok)
		assert.Equal(t, "spaceView", key)
	})
}

// The fold layer refuses an ambiguous class rather than resolving it. The
// one KNOWN ambiguous class in the shipped tables is the Space pair's:
// spaceView's NAME "Space" folds onto the stored key `space`, so a
// near-miss like "SPACE" answers to two keys and the forgiveness declines —
// measured, both keys appear as type-key spellings in 0 of 28,560 corpus
// documents, so the degraded forgiveness costs nothing real. The EXACT
// spellings stay unambiguous either way (the test above).
func TestBundledNames_AnAmbiguousFoldClassIsRefused(t *testing.T) {
	assert.Len(t, BundledTypeKeysByFold("SPACE"), 2,
		"the class holds both keys — the fixture is real")
	_, snap, err := Unmarshal([]byte(`{"version":1,"id":"o1","type":"SPACE"}`),
		Options{GenerateId: seqIds("g")})
	require.NoError(t, err)
	assert.Equal(t, []string{"ot-SPACE"}, snap.ObjectTypes,
		"an ambiguous near-miss degrades to the verbatim term, never to a guess")

	// the nine "Underlying file id" transients are the property namespace's
	// ambiguous class: shared name, so none of them spells it and the name
	// answers to nobody exactly
	_, ok := (BundledKeyVocabulary{}).PropertyKey("Underlying file id")
	assert.False(t, ok, "a shared name binds nothing")
	assert.Equal(t, "fileId", (BundledKeyVocabulary{}).PropertySlug("fileId"),
		"a key whose name cannot invert spells its stored key, always its own address")
}

// The CI rule the uniform spelling stands on (§3): over the WIRE-REACHABLE
// bundled population, names are unique, invertible, writable and clean. A
// new bundled entry that breaks any line here needs a different name — the
// way audioGenre took "Audio genre" — not a looser table.
//
// Wire-reachable means: not one of the stripped internal keys
// (InternalPropertyKeys — those never appear in a document's key slots
// under their own spelling). The nine "Underlying file id" transients are
// the tolerated remainder: they share one name, they are all stripped, and
// the table refuses to spell or bind the shared name at all, so the
// tolerance can never leak into a document.
func TestBundledNames_TheWireReachableTableStaysClean(t *testing.T) {
	stripped := InternalPropertyKeys()

	t.Run("properties", func(t *testing.T) {
		byName := map[string][]string{}
		keys := bundledRelationKeys()
		require.NotEmpty(t, keys)
		for _, key := range keys {
			rel, err := bundle.PickRelation(domain.RelationKey(key))
			require.NoError(t, err)
			name := norm.NFC.String(rel.Name)

			if !stripped[key] {
				require.NotEqualf(t, "", name, "wire-reachable %q has no name to spell", key)
				byName[name] = append(byName[name], key)
				got, ok := BundledPropertyKeyByName(name)
				require.Truef(t, ok, "the name %q of %q must invert", name, key)
				assert.Equalf(t, key, got, "the name %q must invert to its own key", name)
			}
			if name == "" {
				continue
			}
			assert.Truef(t, isWritablePropertyKey(name),
				"bundled name %q (of %q) is not a writable key", name, key)
			assert.Equalf(t, name, strings.TrimSpace(name),
				"bundled name %q (of %q) carries edge whitespace", name, key)
			for _, r := range name {
				assert.Falsef(t, unicode.Is(unicode.Variation_Selector, r) ||
					unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) ||
					unicode.Is(unicode.Cf, r),
					"bundled name %q (of %q) carries an invisible code point", name, key)
			}
			for _, other := range keys {
				assert.Falsef(t, other != key && name == other,
					"the name %q of %q byte-equals the stored key %q — verbatim-first would answer first",
					name, key, other)
			}
		}
		for name, owners := range byName {
			assert.Lenf(t, owners, 1,
				"wire-reachable bundled properties %v share the name %q", owners, name)
		}
		// and no wire-reachable name sits in another wire-reachable entry's
		// fold class: the forgiveness layer must never be pre-degraded for
		// the population documents actually spell
		byFold := map[string][]string{}
		for _, key := range keys {
			if stripped[key] {
				continue
			}
			rel, _ := bundle.PickRelation(domain.RelationKey(key))
			for _, class := range []string{FoldKeyTerm(key), FoldKeyTerm(rel.Name)} {
				owners := byFold[class]
				if len(owners) == 0 || owners[len(owners)-1] != key {
					byFold[class] = append(owners, key)
				}
			}
		}
		for class, owners := range byFold {
			assert.Lenf(t, owners, 1, "wire-reachable bundled properties %v share the fold class %q",
				owners, class)
		}
	})

	t.Run("types", func(t *testing.T) {
		byName := map[string][]string{}
		for _, tk := range bundle.ListTypesKeys() {
			typ, err := bundle.GetType(tk)
			require.NoError(t, err)
			name := norm.NFC.String(typ.Name)
			require.NotEqualf(t, "", name, "bundled type %q has no name to spell", tk)
			byName[name] = append(byName[name], string(tk))
			got, ok := BundledTypeKeyByName(name)
			require.Truef(t, ok, "the name %q of type %q must invert", name, tk)
			assert.Equal(t, string(tk), got)
			assert.True(t, isWritablePropertyKey(name))
			assert.Equal(t, name, strings.TrimSpace(name))
		}
		for name, owners := range byName {
			assert.Lenf(t, owners, 1, "bundled types %v share the name %q", owners, name)
		}
		// the fold classes hold exactly ONE tolerated collision — the Space
		// pair (pinned above); anything beyond it is a new defect
		byFold := map[string]map[string]bool{}
		for _, tk := range bundle.ListTypesKeys() {
			typ, _ := bundle.GetType(tk)
			for _, class := range []string{FoldKeyTerm(string(tk)), FoldKeyTerm(typ.Name)} {
				if byFold[class] == nil {
					byFold[class] = map[string]bool{}
				}
				byFold[class][string(tk)] = true
			}
		}
		for class, owners := range byFold {
			if class == "space" {
				assert.Len(t, owners, 2, "the tolerated Space pair")
				continue
			}
			assert.Lenf(t, owners, 1, "bundled types %v share the fold class %q", owners, class)
		}
	})
}

// Every bundled spelling — names with spaces included — is a writable key
// the whole codec carries: as a `properties` member name, a legend spelling
// and an envelope type term. The old guard asserted bundled slugs were bare
// FILTER-GRAMMAR identifiers; raw names deliberately are not (the
// identifier grammar binds only the compact filter string, which is the API
// request surface's), so the writable-key rule is the right bound now.
func TestBundledNames_EverySpellingIsAWritableKey(t *testing.T) {
	for _, key := range bundledRelationKeys() {
		spelling := (BundledKeyVocabulary{}).PropertySlug(key)
		assert.Truef(t, isWritablePropertyKey(spelling),
			"bundled key %q spells %q, which is not a writable key", key, spelling)
	}
	for _, tk := range bundle.ListTypesKeys() {
		spelling := (BundledKeyVocabulary{}).TypeSlug(string(tk))
		assert.Truef(t, isWritablePropertyKey(spelling),
			"bundled type %q spells %q, which is not a writable key", tk, spelling)
	}
}

// The fold is IDEMPOTENT, and it has to be: a caller that folds a term it
// already folded — the near-miss layer indexing its own keys, a test
// comparing two classes — must land in the same class, or two spellings a
// reader calls one word sit in two.
//
// The way it failed is worth keeping: dropping a separator makes two runes
// neighbours that were not, and a composable pair only composes when it is
// adjacent. NFC ran BEFORE the strip, so `A_` + a combining acute folded to
// a decomposed `á` while the precomposed `Á` folded to U+00E1.
func TestFoldKeyTerm_NormalizesAfterTheStrip(t *testing.T) {
	const (
		combining   = "A_́" // "A", "_", COMBINING ACUTE ACCENT
		precomposed = "Á"   // "Á"
	)
	t.Run("a separator between a letter and its accent does not split the class", func(t *testing.T) {
		assert.Equal(t, FoldKeyTerm(precomposed), FoldKeyTerm(combining),
			"the accent composes onto the letter the strip made it adjacent to")
	})

	t.Run("folding a folded term changes nothing", func(t *testing.T) {
		for _, s := range []string{combining, precomposed, "Due Date", "due_date", "Дата выполнения", "作業内容", "C++", "☕"} {
			once := FoldKeyTerm(s)
			assert.Equal(t, once, FoldKeyTerm(once), "fold is idempotent on %q", s)
		}
	})
}
