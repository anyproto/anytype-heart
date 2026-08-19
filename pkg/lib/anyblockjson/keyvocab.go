package anyblockjson

// keyvocab.go — the wire vocabulary for type and property keys.
//
// APIV2_ADDRESSING.md §7.5a: the API and the format speak ONE key vocabulary, the
// snake_case api slug, everywhere a type or property is named — envelope
// `type`/`templateFor`, `properties` map keys, `typeProperties[].key`,
// dataview `properties[].key`/`groupBy`/`coverProperty`/`endProperty`/sort
// and filter `property`/column `property`, the `property` block's `key`, and
// a link block's `properties`. `dueDate` is `due_date` on the wire; bundled,
// API-created and UI-created keys are indistinguishable to a reader. This
// overturns SPEC §3's "camelCase stored keys" rule, deliberately (§7.3).
//
// **The reverse is a TABLE, both directions — never a case transform.** That
// is proven, not cautionary: `mediaArtistURL` → `media_artist_url` →
// ToLowerCamel yields `mediaArtistUrl`, and `_score` does not round-trip
// either. TestKeyVocabulary_ReverseIsATableNotACaseTransform pins both, so a
// later "simplification" to a case function fails loudly.
//
// The DEFAULT vocabulary is the bundled derived table in `pkg/lib/bundle`,
// which ships with every reader — so a document written by a full node still
// resolves its bundled keys in a package-only reader, offline, with no store
// (§7.5a-5 chain step 3). A node-backed caller supplies a wider vocabulary
// that also knows the space's stored slugs (chain step 2); v2 does, via
// storeresolver.

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// KeyVocabulary translates between the STORED keys the snapshot carries and
// the SLUGS the document spells, in both directions. Implementations owe
// three things, and none of them is implied by the one before it.
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
//  3. **A live stored key outranks the vocabulary's own slug binding.** This
//     is §3 chain step 2 (verbatim-first) stated as an obligation on the
//     implementation: when a term is the stored key of a live entity,
//     `…Key(term)` must answer "not a slug" (ok == false) even when some
//     other holder carries that term as its api key, and `…Slug(key)` must
//     not emit a spelling that some live stored key answers to.
//     storeresolver has implemented this from the start and the interface
//     never said so — `keyMaps.key`'s `if m.storedKey[slug] { return "",
//     false }` is the accept half, `roundTrips`' first refusal the emit half.
//     Without it, a document naming a relation by its stored key lands on
//     whichever OTHER relation minted that string as its `apiObjectKey` — the
//     document names one entity and the reader writes another, with no error
//     — and the emit half labels a value with an address that resolves to
//     somebody else's row. Two live entities can always be told apart by
//     their stored keys; if the slug layer is allowed to outrank them,
//     nothing can.
//
// The second rule is what makes §11's round-trip guarantee true for a reader
// export never met, and a vocabulary can satisfy the first completely while
// breaking it. The legend is the reason: a document owes a
// `type_keys`/`property_keys` entry only for a spelling the READER's chain
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
// storeresolver, the vocabulary the product wires, refuses both halves —
// keyMaps.roundTrips will not SPELL a key with a slug the bundled table binds
// elsewhere, and the bundledKey check in keyMaps.key will not BIND one — so
// this is a rule for hand-written implementations, which Options.Keys accepts
// from anyone. TestKeyVocabulary_ShadowingSlugBreaksInversion pins what
// happens when it is broken.
//
// What NO rule here can prevent, and the legend therefore must: the third
// rule lets a live slug binding win once the stored key stops being live,
// which is what a UI delete does (storeresolver's corpse policy). A fully
// conforming vocabulary then binds a spelling that objects still carry as a
// stored key, so export writes the identity entry for it —
// TestKeyVocabulary_VocabularyInForceIsAReaderToo and
// TestCorpseStoredKeyStillNamesItsObjects.
type KeyVocabulary interface {
	// PropertySlug is the wire spelling of a stored relation key. Returning
	// the input unchanged is always valid ("no slug for this key").
	PropertySlug(key string) string
	// PropertyKey inverts PropertySlug. ok is false when the term is not a
	// known slug — the caller then treats it as a stored key verbatim, which
	// is the §3 verbatim-first rule (an exact stored key always wins over
	// the slug layer).
	PropertyKey(slug string) (key string, ok bool)

	TypeSlug(key string) string
	TypeKey(slug string) (key string, ok bool)
}

// BundledKeyVocabulary is the package default: the bundled derived table,
// both directions, and nothing else. Custom keys pass through unchanged —
// a package-only reader has no space to ask about stored slugs.
type BundledKeyVocabulary struct{}

func (BundledKeyVocabulary) PropertySlug(key string) string {
	if bundle.HasRelation(domain.RelationKey(key)) {
		return bundle.ApiSlug(key)
	}
	return key
}

func (BundledKeyVocabulary) PropertyKey(slug string) (string, bool) {
	if key, ok := bundle.RelationKeyByApiSlug(slug); ok {
		return string(key), true
	}
	return slug, false
}

func (BundledKeyVocabulary) TypeSlug(key string) string {
	if bundle.HasObjectTypeByKey(domain.TypeKey(key)) {
		return bundle.ApiSlug(key)
	}
	return key
}

func (BundledKeyVocabulary) TypeKey(slug string) (string, bool) {
	if key, ok := bundle.TypeKeyByApiSlug(slug); ok {
		return string(key), true
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
	key, _ := o.keys().PropertyKey(slug)
	return key
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
	key, _ := o.keys().TypeKey(slug)
	return key
}
