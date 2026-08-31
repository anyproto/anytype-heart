package anyblockjson

// keyvocab.go — the wire vocabulary for type and property keys.
//
// The format speaks ONE key vocabulary, the display NAME — NFC-normalized,
// otherwise verbatim — everywhere a type or property is named: envelope
// `type`/`template_for`, `properties` map keys,
// `property_definitions[].property`, dataview
// `properties[].property`/`group_by`/`cover_property`/`end_property`/sort
// and filter `property`/column `property`, the `property` block's
// `property`, and a link block's `properties`. `dueDate` is "Due date" on
// the wire; bundled, API-created and UI-created keys are indistinguishable
// to a reader, and there is no derived identifier anywhere in the format —
// the api slug stays the API surface's affair.
//
// **The reverse is a TABLE, both directions — never a string transform.**
// That is proven, not cautionary: "Creation date" says a different word
// than `createdDate`, so no derivation in either direction exists, and a
// spelling that IS the stored key (a shared bundled name has no wire form)
// must pass through rather than be "restored".
// TestKeyVocabulary_ReverseIsATableNotACaseTransform pins both.
//
// The DEFAULT vocabulary is the bundled name table (bundledname.go), which
// ships with every reader — so a document written by a full node still
// resolves its bundled keys in a package-only reader, offline, with no
// store (§3 chain step 3). A node-backed caller supplies a wider vocabulary
// that also knows the space's own names (chain step 2); v2 does, via
// storeresolver.

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// KeyVocabulary translates between the STORED keys the snapshot carries and
// the SPELLINGS the document writes — display names, under §3's rule — in
// both directions. The method names still say "slug" from the era when the
// spelling was a derived identifier; they are kept because the contract
// they name (spelling ↔ stored key) is unchanged, and every implementor
// would churn for a word. Implementations owe three things, and none of
// them is implied by the one before it.
//
//  1. **Inversion.** Whatever `…Slug` emits, `…Key` must invert, or a document
//     does not round-trip.
//
//  2. **No shadowing of the bundled table.** No answer, in either direction,
//     may bind a spelling that the bundled table binds to a DIFFERENT key:
//     `…Slug(key)` must not return a slug the table inverts to something other
//     than `key`, and `…Key(slug)` must not answer for a slug the table binds
//     elsewhere.
//
//  3. **A live stored key outranks the vocabulary's own name binding.** This
//     is §3 chain step 2 (verbatim-first) stated as an obligation on the
//     implementation: when a term is the stored key of a live entity,
//     `…Key(term)` must answer "not a spelling" (ok == false) even when some
//     other live entity is NAMED by that exact string, and `…Slug(key)` must
//     not emit a spelling that some live stored key answers to.
//     storeresolver has implemented this from the start and the interface
//     never said so — `keyMaps.key`'s `if m.storedKey[term] { return "",
//     false }` is the accept half; the emit half is `grant`'s vetting of
//     every label against the stored-key set plus `PropertySlug`'s
//     bundled-arm re-check.
//     Without it, a document naming a relation by its stored key lands on
//     whichever OTHER relation happens to be named by that string — the
//     document names one entity and the reader writes another, with no error
//     — and the emit half labels a value with an address that resolves to
//     somebody else's row. Two live entities can always be told apart by
//     their stored keys; if the name layer is allowed to outrank them,
//     nothing can.
//
// The second rule is what makes §11's round-trip guarantee true for a reader
// export never met, and a vocabulary can satisfy the first completely while
// breaking it. The legend is the reason: a document owes a
// `type_internal_keys`/`property_internal_keys` entry only for a spelling the READER's chain
// cannot invert, and export can only ask the chains it can see — the bundled
// table, which ships with every reader, and the vocabulary it is running
// under (recordTypeKey / termInverts; that second half was missing, and a
// conforming vocabulary lost a type because of it). A third reader's
// vocabulary is not one of them. So a stored key both visible chains invert —
// `task`, spelled `task` — is written with NO legend entry, and a reader whose
// vocabulary answers `TypeKey("task") == "69bbfc…"` resolves it through that
// answer instead. A template for the bundled Task type comes back as a
// template for an unrelated custom type, silently. The property namespace has
// the same shape: a bundled `description` reads back as whatever custom key
// claimed the spelling.
//
// storeresolver, the vocabulary the product wires, keeps both halves by
// construction — a name shared with the bundled table is answered as a
// CANDIDATE set (never bound to one holder), and the emit side yields to
// live stored keys — so this is a rule for hand-written implementations,
// which Options.Keys accepts from anyone.
// TestKeyVocabulary_ShadowingSlugBreaksInversion pins what happens when it
// is broken.
//
// What NO rule here can prevent, and the legend therefore must: the third
// rule lets a live name binding win once the stored key stops being live,
// which is what a UI delete does (storeresolver's corpse policy). A fully
// conforming vocabulary then binds a spelling that objects still carry as a
// stored key, so export writes the identity entry for it —
// TestKeyVocabulary_VocabularyInForceIsAReaderToo and
// TestCorpseStoredKeyStillNamesItsObjects.
//
// **What no rule above requires, and every shipped implementation does.**
// The three obligations are about correctness — a spelling that inverts,
// and inverts to the right key. What it LOOKS like is a separate question,
// and §3 answers it: a key is spelled by its display name, NFC and
// verbatim. PropertyLabel and TypeLabel are that rule, exported so a
// vocabulary can apply it to whatever its source of truth stores;
// storeresolver calls them, and an implementation that answers with a raw
// stored key is still correct, merely less readable.
type KeyVocabulary interface {
	// PropertySlug is the wire spelling of a stored relation key. Returning
	// the input unchanged is always valid ("no slug for this key").
	PropertySlug(key string) string
	// PropertyKey inverts PropertySlug. ok is false when the term is not a
	// known spelling — the caller then treats it as a stored key verbatim,
	// which is the §3 verbatim-first rule (an exact stored key always wins
	// over the name tables).
	PropertyKey(slug string) (key string, ok bool)

	TypeSlug(key string) string
	TypeKey(slug string) (key string, ok bool)
}

// ScopedKeyVocabulary is the OPTIONAL capability a space-backed vocabulary
// adds beyond KeyVocabulary, discovered by type assertion (the TypeResolver
// pattern, §2d). It exists because raw-name addressing admits questions the
// four-method interface cannot ask:
//
//   - **A shared name.** Two live properties may bear one name, and
//     PropertyKey then refuses to answer (an ambiguous address is never
//     resolved by guess). The importer, which knows the document's declared
//     type, asks for the full candidate list and resolves WITHIN THE TYPE:
//     a name unambiguous among the type's own properties — the overwhelming
//     case, measured at 1 ambiguous type in 1,753 — is resolved; a name the
//     type cannot place raises a loud error asking for the legend, never a
//     phantom key.
//   - **A term about to be stored verbatim.** The importer warns when a
//     verbatim term is not any live entity's stored key (the
//     stale-or-guessed-name phantom) and when it extends a live name with
//     trailing text (the glued-annotation hazard). Both diagnoses need the
//     space's stored-key set and name list, which only the vocabulary has.
//
// storeresolver implements it; the bundled-only default does not (bundled
// names are unique by CI guard, so neither question arises offline).
//
// **Every list this interface returns is a SET.** No key appears twice in a
// candidate list or in a type's property list. This is not tidiness: the
// importer reads these lists as COUNTS — "how many live entities answer to
// this spelling", "does the declared type single one of them out" — and a
// count is exactly what a bookkeeping slip in the implementation can falsify
// while leaving every key in the list correct. One entity listed twice reads
// as two, and the import REFUSES a document that the matching export had just
// written, with nothing in the document at fault and nothing in the list a
// reader could check the count against. A refusal is also the one outcome the
// reader cannot work around: a wrong resolution can be overridden with a
// legend entry, a refused import stops. The importer takes the distinct keys
// defensively (distinctKeys in import.go), because Options.Keys accepts an
// implementation from anyone, but an implementation still owes the set.
type ScopedKeyVocabulary interface {
	// PropertyKeyCandidates returns every live property key whose exact
	// document spelling is the term — the space's claimants plus the
	// bundled table's binding — as a sorted set, no key twice. It says
	// nothing about stored keys: verbatim-first is the caller's step, asked
	// before this one.
	PropertyKeyCandidates(spelling string) []string
	// TypeKeyCandidates is the type namespace's half, under the same set and
	// ordering contract.
	TypeKeyCandidates(spelling string) []string
	// TypePropertyKeys returns the stored property keys the type declares —
	// the disambiguating scope for a shared property name. A set, in the
	// type's own order: it is intersected with a candidate list and the
	// survivors are counted, so a property the type names through two of its
	// lists must still be counted once.
	TypePropertyKeys(typeKey string) []string
	// PropertyTermFacts / TypeTermFacts diagnose one term for the
	// verbatim-resolution warnings.
	PropertyTermFacts(term string) KeyTermFacts
	TypeTermFacts(term string) KeyTermFacts
}

// KeyTermFacts is what a space-backed vocabulary knows about one term that
// is about to resolve verbatim.
type KeyTermFacts struct {
	// LiveStoredKey: the term is a live entity's stored key — verbatim
	// resolution is then simply chain step 2, nothing to warn about.
	LiveStoredKey bool
	// ExtendsName: a live entity's display name the term extends with
	// trailing text past a word boundary ("" when none) — the eval's one
	// real raw-name failure shape, an annotation glued onto a copied name.
	ExtendsName string
}

// BundledKeyVocabulary is the package default: the bundled name table
// (bundledname.go), both directions, plus the forgiving fold on the accept
// side, and nothing else. Custom keys pass through unchanged: a
// package-only reader has no space to ask about names.
type BundledKeyVocabulary struct{}

func (BundledKeyVocabulary) PropertySlug(key string) string {
	if bundle.HasRelation(domain.RelationKey(key)) {
		return bundledPropertySpelling(key)
	}
	return key
}

func (BundledKeyVocabulary) PropertyKey(slug string) (string, bool) {
	if key, ok := bundledPropertyKeyBySpelling(slug); ok {
		return key, true
	}
	// the forgiving fold, single candidate only — the layer that keeps every
	// pre-change derived-slug spelling (`created_date`) resolving in a
	// package-only reader with no compatibility table: ToSnake only inserts
	// `_` and lowercases, so the old slug sits in its stored key's fold class
	if candidates := BundledPropertyKeysByFold(slug); len(candidates) == 1 {
		return candidates[0], true
	}
	return slug, false
}

func (BundledKeyVocabulary) TypeSlug(key string) string {
	if bundle.HasObjectTypeByKey(domain.TypeKey(key)) {
		return bundledTypeSpelling(key)
	}
	return key
}

func (BundledKeyVocabulary) TypeKey(slug string) (string, bool) {
	if key, ok := bundledTypeKeyBySpelling(slug); ok {
		return key, true
	}
	if candidates := BundledTypeKeysByFold(slug); len(candidates) == 1 {
		return candidates[0], true
	}
	return slug, false
}

// keys returns the vocabulary in force — the caller's, or the bundled table.
func (o Options) keys() KeyVocabulary {
	if o.Keys != nil {
		return o.Keys
	}
	return BundledKeyVocabulary{}
}

// propertySlug / propertyKey / typeSlug / typeKey are the raw vocabulary
// lookups; empty terms pass through untouched so no site has to special-case
// them. They are NOT the key-slot boundary: every slot goes through the
// exporter's claim step (propertySlug/typeSlug on *exporter — the term
// ledgers and the legends they owe) and the importer's legend-first read
// (propertyKey/typeKey on *importer). Options carries only the vocabulary;
// the document's own statements live with the codec halves.
func (o Options) propertySlug(key string) string {
	if key == "" {
		return key
	}
	return o.keys().PropertySlug(key)
}

func (o Options) propertyKey(slug string) string {
	if slug == "" {
		return slug
	}
	// §3: a spelling resolves under its canonical NFC form (nfcTerm) — the
	// read half of the rule PropertyLabel writes by. Idempotent, so the
	// importer door normalizing first costs nothing here.
	key, _ := o.keys().PropertyKey(nfcTerm(slug))
	return key
}

// legendPropertyKey is propertyKey with §3 chain step 1 in front of it: the
// legend Options.Legend carries, which for a fragment entry point IS the
// enclosing document's `property_internal_keys`. Same precedence as
// importer.propertyKey, and stated once rather than twice for exactly that
// reason — the two doors into a type's property list must not disagree about
// what a spelling means.
func (o Options) legendPropertyKey(slug string) string {
	if key, ok := legendLookup(o.Legend.PropertyKeys, slug); ok {
		return key
	}
	return o.propertyKey(slug)
}

// legendTypeKey is legendPropertyKey on the type namespace.
func (o Options) legendTypeKey(slug string) string {
	if key, ok := legendLookup(o.Legend.TypeKeys, slug); ok {
		return key
	}
	return o.typeKey(slug)
}

// legendLookup answers a legend for one spelling under §3's normalization
// rule: the exact bytes first (a legend may bind a non-NFC spelling — export
// writes an identity entry for a stored key it spells verbatim), then the
// canonical NFC form, against a legend whose own non-NFC entries also answer
// for their NFC form (nfcExpandLegend). Values are stored keys and pass
// byte-verbatim.
func legendLookup(m map[string]string, slug string) (string, bool) {
	if len(m) == 0 {
		return "", false
	}
	expanded := nfcExpandLegend(m)
	if key, ok := expanded[slug]; ok && key != "" {
		return key, true
	}
	if n := nfcTerm(slug); n != slug {
		if key, ok := expanded[n]; ok && key != "" {
			return key, true
		}
	}
	return "", false
}

// nfcExpandLegend returns a legend that also answers for the NFC form of any
// non-NFC-spelled member (§3: a slot may spell one name in either byte
// form). Exact entries always win — an NFC-form shadow never displaces an
// entry the legend states at that exact spelling — and two non-NFC entries
// collapsing onto one unclaimed NFC form leave it unbound: an ambiguous
// address is never resolved by guess (the twin warning names the pair). The
// common all-NFC legend comes back untouched, unallocated.
func nfcExpandLegend[V any](m map[string]V) map[string]V {
	var nonCanonical []string
	for k := range m {
		if nfcTerm(k) != k {
			nonCanonical = append(nonCanonical, k)
		}
	}
	if len(nonCanonical) == 0 {
		return m
	}
	claims := map[string]int{}
	for _, k := range nonCanonical {
		claims[nfcTerm(k)]++
	}
	out := make(map[string]V, len(m)+len(nonCanonical))
	for k, v := range m {
		out[k] = v
	}
	for _, k := range nonCanonical {
		n := nfcTerm(k)
		if _, exact := m[n]; exact || claims[n] > 1 {
			continue
		}
		out[n] = m[k]
	}
	return out
}

func (o Options) typeSlug(key string) string {
	if key == "" {
		return key
	}
	return o.keys().TypeSlug(key)
}

func (o Options) typeKey(slug string) string {
	if slug == "" {
		return slug
	}
	// §3's canonical form, as in propertyKey above
	key, _ := o.keys().TypeKey(nfcTerm(slug))
	return key
}
