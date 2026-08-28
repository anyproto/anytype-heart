package anyblockjson

// bundledname.go — the wire spellings of BUNDLED property and type keys:
// their display names, from the tables that ship with every reader.
//
// The format spells every property and type key by the entity's display
// name, NFC-normalized and otherwise verbatim — one uniform rule for
// bundled and space-minted keys alike. For bundled keys the name comes from
// `pkg/lib/bundle`'s relations.json/types.json, so `createdDate` spells
// "Creation date" and the Page type spells "Page", offline, with no store.
// This file is the bundled half of that rule, both directions, and it
// replaces two mechanisms at once:
//
//   - the derived api-slug table (`bundle.ApiSlug`, i.e. strcase.ToSnake):
//     the slug survives on the API surface, whose key convention is a
//     separate decision, but it is no longer a document spelling — 0 of 194
//     bundled relation names are byte-equal to their derived slug, and the
//     name is the surface users and models actually see;
//   - the v0.38 alias table (alias.go, deleted): its sixteen respellings
//     existed because the stored key said "relation" where the format says
//     "property", and the display names never did — the relation TYPE is
//     named "Property" in the bundle, `relationFormat` is named "Format" —
//     so the names carry the rename with no table behind it. The sixteen
//     alias spellings themselves (`featured_properties`, …) are cut rather
//     than kept as an accept-only layer: the format is pre-freeze, no
//     back-compat is owed, and every existing bundle re-exports under the
//     new spelling either way.
//
// Legacy derived-slug spellings keep resolving WITHOUT a compatibility
// table, through the fold layer: ToSnake only inserts `_` and lowercases,
// and the fold strips `_`, `-` and case, so fold(ToSnake(key)) == fold(key)
// by construction and every pre-change spelling (`created_date`) lands in
// its stored key's fold class. The fold below answers for both the key's
// class and the name's class, so `creation_date` — the guess shaped like
// the name — forgives too.
//
// **A name that does not uniquely invert is not a spelling.** Nine hidden
// transient relations share the name "Underlying file id"; all nine sit in
// the stripped set (transientProperties), so no document ever spells them —
// but the rule does not lean on that: a key whose name the reverse table
// cannot bind spells its stored key verbatim, which is always its own
// address, so an ambiguous bundled name can never be emitted at all. The
// guard tests in bundledname_test.go keep the wire-reachable population
// clean so this fallback only ever covers invisible machinery.

import (
	"sort"
	"strings"
	"unicode"

	"golang.org/x/text/unicode/norm"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// FoldKeyTerm is the format's forgiving fold over key terms — the class two
// spellings must share for the near-miss layer to bridge them. Wider than
// the api surface's bundle.FoldApiKey on purpose: a display name separates
// its words with SPACES where a stored key uses case and a derived slug
// uses `_`, so a fold that kept spaces would put "Due Date 2" and
// `due_date_2` in different classes and the legacy-continuity proof above
// would not hold. NFC first (a hand-edited document can arrive decomposed),
// then lowercase, then drop `_`, `-`, every whitespace rune, and the
// default-ignorable code points (variation selectors, ZWJ, word joiners —
// two production names carry an invisible variation selector, and an
// invisible near-miss is the least visible near-miss there is).
//
// Exact match always wins before the fold is consulted, and two keys
// folding together is an ambiguity the caller must refuse to resolve by
// guess — the same contract bundle.FoldApiKey states.
//
// NFC runs TWICE, and the second pass is not belt and braces. Dropping a
// separator can put two runes next to each other that were not neighbours
// before, and a composable pair only composes when it is adjacent:
// "A_" + a combining acute folded to `a` + U+0301, while the precomposed
// "Á" folded to U+00E1 — two spellings a reader would call the same word,
// in different fold classes, and the fold was not even idempotent on its
// own output. Normalizing after the map puts every result in one form.
func FoldKeyTerm(s string) string {
	return norm.NFC.String(strings.Map(func(r rune) rune {
		switch {
		case r == '_' || r == '-':
			return -1
		case unicode.IsSpace(r):
			return -1
		case unicode.Is(unicode.Variation_Selector, r) ||
			unicode.Is(unicode.Other_Default_Ignorable_Code_Point, r) ||
			unicode.Is(unicode.Cf, r):
			return -1
		}
		return r
	}, strings.ToLower(norm.NFC.String(s))))
}

// The bundled name tables, built once. Forward holds only names the reverse
// uniquely binds; reverse is exact NFC name → stored key; fold holds every
// entry's key class and name class, multi-valued where classes collide.
var (
	bundledPropertyNameByKey  map[string]string
	bundledPropertyKeyByName  map[string]string
	bundledPropertyKeysByFold map[string][]string
	bundledTypeNameByKey      map[string]string
	bundledTypeKeyByName      map[string]string
	bundledTypeKeysByFold     map[string][]string
)

func init() {
	relKeys := make([]string, 0, 256)
	for _, k := range bundle.ListRelationsKeys() {
		relKeys = append(relKeys, string(k))
	}
	sort.Strings(relKeys)
	relName := func(key string) string {
		rel, err := bundle.PickRelation(domain.RelationKey(key))
		if err != nil {
			return ""
		}
		return rel.Name
	}
	// A DENIED key's fold class must answer nothing. The exact name stays in
	// the reverse table — "Format" in a `properties` map is refused WITH the
	// envelope repair named, and a reference slot naming the key spells it —
	// but the fold is the forgiving layer, and forgiveness toward a key
	// import refuses would re-break what the lift settled: `format` and
	// `include_time` are legal CUSTOM property names on any document (the
	// phantom-member warning is the guard there), and a fold that pulled
	// them onto the lifted stored keys would turn that warning into a
	// refusal.
	bundledPropertyNameByKey, bundledPropertyKeyByName, bundledPropertyKeysByFold =
		buildNameTables(relKeys, relName, func(key string) bool {
			_, denied := deniedPropertyKey(key)
			return denied
		})

	typeKeys := make([]string, 0, 64)
	for _, k := range bundle.ListTypesKeys() {
		typeKeys = append(typeKeys, string(k))
	}
	sort.Strings(typeKeys)
	typeName := func(key string) string {
		t, err := bundle.GetType(domain.TypeKey(key))
		if err != nil {
			return ""
		}
		return t.Name
	}
	bundledTypeNameByKey, bundledTypeKeyByName, bundledTypeKeysByFold =
		buildNameTables(typeKeys, typeName, func(string) bool { return false })
}

// buildNameTables derives one namespace's three tables from the shipped
// bundle. Keys arrive SORTED so that where the input is ambiguous the
// outcome is still deterministic — though ambiguity never picks a winner:
//
//   - a name two keys share binds NEITHER in the reverse table and spells
//     neither in the forward one (both fall back to their stored keys);
//   - a name that byte-equals a DIFFERENT entry's stored key is refused the
//     same way — the stored key resolves verbatim-first at every reader, so
//     a spelling equal to it could never invert to anyone else;
//   - a name that is not a writable key (empty, over the bound, control
//     characters) has no wire form and the key spells itself.
//
// The fold table is built over every entry regardless: an ambiguous fold
// class simply holds several candidates, which the forgiving layer already
// treats as "refuse, never guess".
func buildNameTables(keys []string, nameOf func(string) string, foldExcluded func(string) bool) (
	nameByKey, keyByName map[string]string, foldTable map[string][]string) {
	stored := make(map[string]bool, len(keys))
	for _, k := range keys {
		stored[k] = true
	}
	claim := map[string][]string{}
	for _, k := range keys {
		name := norm.NFC.String(nameOf(k))
		if name == "" || name == k || !isWritablePropertyKey(name) {
			continue
		}
		if stored[name] {
			continue // another entry's stored key outranks any name, verbatim-first
		}
		claim[name] = append(claim[name], k)
	}
	nameByKey = make(map[string]string, len(claim))
	keyByName = make(map[string]string, len(claim))
	for name, holders := range claim {
		if len(holders) != 1 {
			continue // shared name: nobody spells it, nobody answers to it
		}
		nameByKey[holders[0]] = name
		keyByName[name] = holders[0]
	}
	foldTable = map[string][]string{}
	addFoldClass := func(class, key string) {
		for _, existing := range foldTable[class] {
			if existing == key {
				return
			}
		}
		foldTable[class] = append(foldTable[class], key)
	}
	for _, k := range keys {
		if foldExcluded(k) {
			continue
		}
		addFoldClass(FoldKeyTerm(k), k)
		if name := norm.NFC.String(nameOf(k)); name != "" {
			addFoldClass(FoldKeyTerm(name), k)
		}
	}
	return nameByKey, keyByName, foldTable
}

// bundledPropertySpelling is the wire spelling of a BUNDLED relation key:
// its display name where the name uniquely inverts, the stored key itself
// otherwise (always its own address). The caller has already established
// the key is bundled — a non-bundled key must never reach the bundled
// table (dictionaryKeySpelling's bson-id rule).
func bundledPropertySpelling(key string) string {
	if name, ok := bundledPropertyNameByKey[key]; ok {
		return name
	}
	return key
}

// BundledPropertyKeyByName inverts bundledPropertySpelling EXACTLY: an NFC
// display name names its key, and nothing else answers. It is the exact
// layer alone, deliberately: a stored key resolves verbatim at chain step 2
// without this table, and near-misses (the legacy derived slugs included)
// belong to the fold layer, which must see EVERY candidate — a space's own
// fold claimants included — before it may answer. Exported because
// storeresolver's exact candidate layer runs this same table, and folding
// inside it would let a bundled near-miss win while a space-minted twin in
// the same fold class went unseen.
func BundledPropertyKeyByName(spelling string) (string, bool) {
	key, ok := bundledPropertyKeyByName[spelling]
	return key, ok
}

func bundledPropertyKeyBySpelling(spelling string) (string, bool) {
	return BundledPropertyKeyByName(spelling)
}

// bundledTypeSpelling / bundledTypeKeyBySpelling are the type namespace's
// halves of the same two rules.
func bundledTypeSpelling(key string) string {
	if name, ok := bundledTypeNameByKey[key]; ok {
		return name
	}
	return key
}

func bundledTypeKeyBySpelling(spelling string) (string, bool) {
	return BundledTypeKeyByName(spelling)
}

// BundledTypeKeyByName is BundledPropertyKeyByName on the type namespace.
func BundledTypeKeyByName(spelling string) (string, bool) {
	key, ok := bundledTypeKeyByName[spelling]
	return key, ok
}

// BundledPropertyKeysByFold is the bundled arm of the fold layer (§3 chain
// step 4 — the near-miss forgiveness): every bundled relation key whose
// stored key OR display name folds to the input's class. Zero matches: not
// bundled; one: the forgiveness; two or more: an ambiguity the caller must
// refuse rather than resolve. Exported because storeresolver's chain runs
// the same bundled arm the package-only reader does, and the two must not
// disagree about which fold class a key answers to.
func BundledPropertyKeysByFold(term string) []string {
	return bundledPropertyKeysByFold[FoldKeyTerm(term)]
}

// BundledTypeKeysByFold is BundledPropertyKeysByFold on the type namespace.
func BundledTypeKeysByFold(term string) []string {
	return bundledTypeKeysByFold[FoldKeyTerm(term)]
}

// BundledPropertyNameExtendedBy reports the bundled relation display name
// that `term` extends with trailing text, when one does — the copy-boundary
// hazard's bundled half: a writer gluing an annotation onto a name it was
// copying ("Creation date (text)") produces a term no table answers, and
// the one useful fact about it is which live name it started as. The
// LONGEST extended name wins so the report names the fullest match;
// equal-length ties break lexicographically for determinism.
func BundledPropertyNameExtendedBy(term string) (string, bool) {
	return nameExtendedBy(term, bundledPropertyKeyByName)
}

// BundledTypeNameExtendedBy is the type namespace's half.
func BundledTypeNameExtendedBy(term string) (string, bool) {
	return nameExtendedBy(term, bundledTypeKeyByName)
}

func nameExtendedBy(term string, names map[string]string) (string, bool) {
	var best string
	for name := range names {
		if !KeyTermExtendsName(term, name) {
			continue
		}
		if len(name) > len(best) || (len(name) == len(best) && name < best) {
			best = name
		}
	}
	return best, best != ""
}

// KeyTermExtendsName reports whether term is name plus trailing text that
// begins at a word boundary. The boundary check is what separates a glued
// annotation ("Tag (text)") from a longer name that happens to share a
// prefix ("Tagline" is not "Tag" with something glued on): the first rune
// past the name must not be a letter or digit. Exported because the
// space-backed vocabulary applies the same rule to its own live names, and
// the two diagnostics must not disagree about what counts as glue.
func KeyTermExtendsName(term, name string) bool {
	if name == "" || len(term) <= len(name) || !strings.HasPrefix(term, name) {
		return false
	}
	for _, r := range term[len(name):] {
		return !unicode.IsLetter(r) && !unicode.IsDigit(r)
	}
	return false
}

// DisambiguatedKeySpelling is the shared rung (b) of the collision ladder —
// the spelling a claimant takes when its plain name is unusable (contested
// inside one document, or equal to another live stored key) and its own
// stored key is not a readable substitute. It answers "" in exactly the
// cases where rung (a) or (c) applies instead:
//
//   - a stored key that is NOT a minted 24-hex bson id is readable, and the
//     honest disambiguation is the key itself, verbatim (rung a);
//   - a suffixed form that would not be a writable key — a name already at
//     the length bound has no room — is refused rather than truncated, and
//     the stored key is written regardless (rung c).
//
// Otherwise the answer is `<name> (<tail6>)`, tail6 = the stored key's last
// six hex: deterministic, immutable while the key lives, and visibly
// synthetic. Exported because the exporter's per-document term ledger and
// storeresolver's space vocabulary both run this ladder, and a claimant
// must take the same spelling whichever seam degrades it.
func DisambiguatedKeySpelling(name, key string) string {
	if name == "" || !isBsonShapedKey(key) {
		return ""
	}
	suffixed := name + " (" + key[len(key)-6:] + ")"
	if !isWritablePropertyKey(suffixed) {
		return ""
	}
	return suffixed
}

// isBsonShapedKey reports the one stored-key shape the ladder calls
// unreadable: the 24-char lowercase-hex bson id every editor-minted
// relation and type carries. Everything else — a bundled camelCase key, an
// API-minted readable key — is its own honest spelling.
func isBsonShapedKey(key string) bool {
	return isHexLower(key, 24)
}
