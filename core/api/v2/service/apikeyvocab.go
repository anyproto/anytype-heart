package v2service

// apikeyvocab.go implements the API's document vocabulary: every AnyBlock
// body v2 serves spells property and type keys as the stable
// minted api slug, the same spelling the listings serve, from the same
// authority (servedKey/servedTypeKeyOf — keys.go). The format itself uses
// raw display names; this vocabulary is the API's
// half of that split: names are what users rename, slugs are what
// integrations remember, and the codec's Options.Keys seam is the ONE place
// a spelling is ever decided — no marshaled document is ever re-parsed to
// re-spell it.
//
// Emit is a TABLE lookup, never a derivation: the slug was minted once at
// creation and frozen (D4), and 4% of production option slugs are not
// derivable from their current name — a derived emit would mis-address
// exactly those. Accept is the format's whole forgiving chain WIDENED by
// the same table, slug first: the post-switch storeresolver no longer reads
// apiObjectKey, so a body carrying a non-fold-derivable slug ("discovery"
// for a property now named "Awareness") would not invert through the
// resolver alone, and the inversion obligation the KeyVocabulary interface
// states would break for precisely the keys the slug exists to keep stable.
//
// The wrapper embeds the space's storeresolver as the inner vocabulary, so
// everything the format accepts — display names, old snake spellings,
// stored keys, the fold — still resolves; obligations (inversion, live
// stored key outranks any table, no shadowing) are pinned by
// apikeyvocab_test.go.

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
)

// apiKeyVocab is per-request, like the storeresolver it embeds: lazily
// loaded on first use, no locking (a request is one goroutine).
type apiKeyVocab struct {
	// removedSlug marks the served slugs that belong to REMOVED properties;
	// corpseKeyBySlug inverts those this vocabulary has emitted (rememberCorpse)
	removedSlug     map[string]bool
	corpseKeyBySlug map[string]string
	svc             *Service
	spaceId         string
	inner           anyblockjson.KeyVocabulary
	// scoped is inner's ScopedKeyVocabulary half when it has one (the
	// storeresolver always does); the importer's richer resolution walks
	// candidates instead of PropertyKey, so the slug table must surface
	// there too or the accept widening would silently not apply to bodies.
	scoped anyblockjson.ScopedKeyVocabulary

	loaded bool
	// degraded: the live-entry load failed. Emit then serves the stored key
	// verbatim — always its own address, the same honest degradation a
	// pre-backfill entity gets — and accept delegates to the inner chain.
	// Nothing is guessed on a store hiccup (fail closed, keys.go's rule).
	degraded bool

	propSlugByKey   map[string]string
	propKeyBySlug   map[string]string
	propKeyTaken    map[string]bool
	propSlugHolders map[string][]string

	typeSlugByKey map[string]string
	typeKeyBySlug map[string]string
	// removedTypeKeys is every removed type this vocabulary knows of,
	// slug-bearing or not, for the read marker (TypeRemoved).
	removedTypeKeys map[string]bool
	typeKeyTaken    map[string]bool
	typeSlugHolders map[string][]string
}

// apiKeys wraps a space resolver's vocabulary with the slug table. inner is
// the vocabulary the codec would otherwise use (the storeresolver bound to
// the SAME space).
func (s *Service) apiKeys(spaceId string, inner anyblockjson.KeyVocabulary) *apiKeyVocab {
	v := &apiKeyVocab{svc: s, spaceId: spaceId, inner: inner}
	v.scoped, _ = inner.(anyblockjson.ScopedKeyVocabulary)
	return v
}

// apiRefSpelling settles how a served document names a TYPE, which it must
// do twice, because a type is two things at once.
//
// As a KIND it is named by a key that means the same thing in every space —
// the envelope `type`, `template_for`, every `object_types` — and this
// surface spells those with the api slug, which is the word its own /types
// routes accept. As an OBJECT it is named by an id that exists in one space
// — `set_of`, `default_type_id`, a mention or link target, a type document's
// own envelope id — and this surface spells those with the store id, which
// is the string GET /objects/{id} resolves.
//
// The format's derived id, `type-<stored_key>`, is neither: for a custom
// type the remainder is the bson stored key, so `type-68f1a9c…` names a type
// no /types route can address and no /objects route can open. Leaving it on
// put THREE spellings of one type in a single envelope — `"type": "bug"`
// beside `"template_for": "type-68f1a9c…"` beside a `set_of` wearing the
// prefix again.
//
// It rides EXPORT only, so a body arriving with `type-<key>` still resolves:
// declining to write a spelling is not declining to read one. Callers that
// unmarshal need nothing from here.
//
// Every v2 surface whose bytes reach a caller goes through this — the object
// read, list and search rows, the views fragment, and the applier's own
// marshal (whose after-documents are COMPARED against what the read served,
// so a disagreement here would make a view op diff against a shape no read
// ever emitted). The file exporter deliberately does not: portable documents
// want the derived id, which is why this cannot live in storeresolver.
func apiRefSpelling(opts anyblockjson.Options) anyblockjson.Options {
	opts.NoDerivedTypeIds = true
	return opts
}

// corpseKeyBySlug inverts the removed-property slugs THIS vocabulary has
// emitted (PropertySlug), and nothing else: what a vocabulary served, it
// understands back — so a document it rendered with a corpse slug re-imports
// onto the stored key (the object channel's in-document edit) — while a
// request that never saw the slug served (a type definition, a create)
// resolves it like any unknown name and mints anew (the namespace vacated,
// §8-OQ2).
func (v *apiKeyVocab) rememberCorpse(slug, key string) {
	if v.corpseKeyBySlug == nil {
		v.corpseKeyBySlug = map[string]string{}
	}
	v.corpseKeyBySlug[slug] = key
}

func (v *apiKeyVocab) ensure() bool {
	if v.loaded {
		return !v.degraded
	}
	v.loaded = true
	props, err := v.svc.liveProperties(v.spaceId)
	if err != nil {
		v.degraded = true
		return false
	}
	types, err := v.svc.liveTypes(v.spaceId)
	if err != nil {
		v.degraded = true
		return false
	}

	v.propKeyTaken, v.propSlugHolders = servedPropertyKeySets(props)
	v.propSlugByKey = make(map[string]string, len(props))
	v.propKeyBySlug = map[string]string{}
	for _, e := range props {
		served := servedKey(e.Key, e.Slug, v.propKeyTaken, v.propSlugHolders)
		v.propSlugByKey[e.Key] = served
		if served != e.Key {
			claimTerm(v.propKeyBySlug, v.propSlugByKey, served, e.Key)
		}
	}

	// a REMOVED property keeps its slug on the EMIT side: the value an
	// object still holds, the type list that still names it and the view
	// column that still shows it spell the slug the caller was taught
	// (round-two eval F1 — a caller shown the bson key took it for garbage
	// and unset the value on every object). Not on the accept side: the
	// reverse table stays unaware, so a definition or a create naming that
	// slug mints a NEW property (the namespace vacated, §8-OQ2 — deleting
	// and re-creating a field must not resurrect the corpse), and a write
	// to it on an object is refused as removed (removedCustomProperty). The
	// one place the slug is understood back is the in-document escape
	// (stateops checkKey), which reads the stored key off the document
	// itself. A slug a live entry already answers to is left alone, so the
	// live one keeps its spelling and the corpse reads under its stored key.
	if removed, rerr := v.svc.removedProperties(v.spaceId); rerr == nil {
		// two corpses answering to one slug (delete, re-create, delete
		// again) would both emit it and the codec would suffix one of them
		// into a spelling nothing here inverts — so neither gets the slug,
		// exactly as claimTerm demotes live twins
		corpseBySlug := map[string]string{}
		for _, e := range removed {
			if _, live := v.propSlugByKey[e.Key]; live {
				continue
			}
			served := servedKey(e.Key, e.Slug, v.propKeyTaken, v.propSlugHolders)
			if served == e.Key {
				continue
			}
			if _, taken := v.propKeyBySlug[served]; taken {
				continue
			}
			if prev, twin := corpseBySlug[served]; twin {
				delete(v.propSlugByKey, prev)
				delete(v.removedSlug, served)
				continue
			}
			corpseBySlug[served] = e.Key
			v.propSlugByKey[e.Key] = served
			if v.removedSlug == nil {
				v.removedSlug = map[string]bool{}
			}
			v.removedSlug[served] = true
		}
	}

	v.typeKeyTaken, v.typeSlugHolders = servedTypeKeySets(types)
	v.typeSlugByKey = make(map[string]string, len(types))
	v.typeKeyBySlug = map[string]string{}
	for _, e := range types {
		served := servedTypeKeyOf(e.Key, e.Slug, v.typeKeyTaken, v.typeSlugHolders)
		v.typeSlugByKey[e.Key] = served
		if served != e.Key {
			claimTerm(v.typeKeyBySlug, v.typeSlugByKey, served, e.Key)
		}
	}
	// a REMOVED type keeps its slug on the EMIT side, exactly as a removed
	// property does (round-four eval R4-1: deleting a type rewrote every
	// surviving object's `type` from the slug to a 24-hex stored key that
	// nothing in the API resolved). Emit only: the reverse table stays
	// unaware, so a create naming the slug is refused as removed
	// (removedTypeBySpelling) and a re-created type may take the slug —
	// then the corpse reads under its stored key, as the live one owns it.
	if removed, rerr := v.svc.removedTypes(v.spaceId); rerr == nil {
		corpseBySlug := map[string]string{}
		for _, e := range removed {
			if _, live := v.typeSlugByKey[e.Key]; live {
				continue
			}
			if v.removedTypeKeys == nil {
				v.removedTypeKeys = map[string]bool{}
			}
			v.removedTypeKeys[e.Key] = true
			served := servedTypeKeyOf(e.Key, e.Slug, v.typeKeyTaken, v.typeSlugHolders)
			if served == e.Key {
				continue
			}
			if _, taken := v.typeKeyBySlug[served]; taken {
				continue
			}
			if prev, twin := corpseBySlug[served]; twin {
				delete(v.typeSlugByKey, prev)
				continue
			}
			corpseBySlug[served] = e.Key
			v.typeSlugByKey[e.Key] = served
		}
	}
	return true
}

// claimTerm binds term → key in the reverse table, demoting BOTH holders to
// their stored keys when two entries land on one term. servedKeyOf's guards
// make that unreachable for visible entries; hidden entries do not
// participate in its slugHolders namespace, so two hidden twins sharing a
// stored slug can still collide here — and an emit that cannot be inverted
// uniquely must not be emitted at all (the inversion obligation).
func claimTerm(keyByTerm, termByKey map[string]string, term, key string) {
	if prev, taken := keyByTerm[term]; taken && prev != key {
		termByKey[prev] = prev
		termByKey[key] = key
		delete(keyByTerm, term)
		return
	}
	keyByTerm[term] = key
	termByKey[key] = term
}

//
// ---- emit: the slug, always ----
//

func (v *apiKeyVocab) PropertySlug(key string) string {
	if key == "" || !v.ensure() {
		return key
	}
	if served, ok := v.propSlugByKey[key]; ok {
		if served != key && v.removedSlug[served] {
			v.rememberCorpse(served, key)
		}
		return served
	}
	// not live in this space: a bundled key spells as its derived slug
	// under the same three round-trip guards the listings apply; anything
	// else is its own address.
	return servedKey(key, "", v.propKeyTaken, v.propSlugHolders)
}

func (v *apiKeyVocab) TypeSlug(key string) string {
	if key == "" || !v.ensure() {
		return key
	}
	if served, ok := v.typeSlugByKey[key]; ok {
		return served
	}
	return servedTypeKeyOf(key, "", v.typeKeyTaken, v.typeSlugHolders)
}

// TypeRemoved reports whether a stored type key this vocabulary has seen
// belongs to a REMOVED type — the read marker's question (R4-1: an object
// whose type is gone must not look like an ordinary object).
func (v *apiKeyVocab) TypeRemoved(key string) bool {
	if key == "" || !v.ensure() || v.typeKeyTaken[key] {
		return false
	}
	return v.removedTypeKeys[key]
}

//
// ---- accept: slug table first, then the format's whole chain ----
//

func (v *apiKeyVocab) PropertyKey(slug string) (string, bool) {
	if !v.ensure() {
		return v.inner.PropertyKey(slug)
	}
	if v.propKeyTaken[slug] {
		// a live stored key outranks every table (§3 verbatim-first, stated
		// as the interface's obligation 3). The convention on "not a
		// spelling" is (input, false) — Options.propertyKey drops ok and
		// uses the string, so an empty here becomes an empty resolved key.
		return slug, false
	}
	if key, ok := v.propKeyBySlug[slug]; ok {
		return key, true
	}
	if key, ok := bundle.RelationKeyByApiSlug(slug); ok && v.PropertySlug(string(key)) == slug {
		// the served-spelling check is the shadow guard: a bundled slug a
		// space entry squats on was demoted at emit and must not resolve
		// to the bundled key on accept either.
		return string(key), true
	}
	if key, ok := v.corpseKeyBySlug[slug]; ok {
		return key, true // emitted by this vocabulary, so understood back
	}
	return v.inner.PropertyKey(slug)
}

func (v *apiKeyVocab) TypeKey(slug string) (string, bool) {
	if !v.ensure() {
		return v.inner.TypeKey(slug)
	}
	if v.typeKeyTaken[slug] {
		return slug, false // verbatim-first; (input, false), see PropertyKey
	}
	if key, ok := v.typeKeyBySlug[slug]; ok {
		return key, true
	}
	if key, ok := bundle.TypeKeyByApiSlug(slug); ok && v.TypeSlug(string(key)) == slug {
		return string(key), true
	}
	return v.inner.TypeKey(slug)
}

//
// ---- ScopedKeyVocabulary: the importer's richer walk sees the table ----
//

// emittedCorpse reports the stored key this vocabulary served spelling for,
// when spelling is a removed property's slug it emitted (rememberCorpse).
func (v *apiKeyVocab) emittedCorpse(spelling string) (string, bool) {
	key, ok := v.corpseKeyBySlug[spelling]
	return key, ok
}

func (v *apiKeyVocab) PropertyKeyCandidates(spelling string) []string {
	var out []string
	if v.ensure() && !v.propKeyTaken[spelling] {
		if key, ok := v.propKeyBySlug[spelling]; ok {
			out = append(out, key)
		} else if key, ok := bundle.RelationKeyByApiSlug(spelling); ok && v.PropertySlug(string(key)) == spelling {
			out = append(out, string(key))
		} else if key, ok := v.corpseKeyBySlug[spelling]; ok {
			// a spelling this vocabulary emitted for a removed property is
			// that property, before any live property's display name can
			// claim it through the scoped chain
			return []string{key}
		}
	}
	if v.scoped != nil {
		out = append(out, v.scoped.PropertyKeyCandidates(spelling)...)
	}
	return sortedDistinct(out)
}

func (v *apiKeyVocab) TypeKeyCandidates(spelling string) []string {
	var out []string
	if v.ensure() && !v.typeKeyTaken[spelling] {
		if key, ok := v.typeKeyBySlug[spelling]; ok {
			out = append(out, key)
		} else if key, ok := bundle.TypeKeyByApiSlug(spelling); ok && v.TypeSlug(string(key)) == spelling {
			out = append(out, string(key))
		}
	}
	if v.scoped != nil {
		out = append(out, v.scoped.TypeKeyCandidates(spelling)...)
	}
	return sortedDistinct(out)
}

func (v *apiKeyVocab) TypePropertyKeys(typeKey string) []string {
	if v.scoped == nil {
		return nil
	}
	return v.scoped.TypePropertyKeys(typeKey)
}

func (v *apiKeyVocab) PropertyTermFacts(term string) anyblockjson.KeyTermFacts {
	if bundle.HasRelation(domain.RelationKey(term)) {
		// v2's chain resolves an exact bundled stored key in every space,
		// installed or not (resolvePropertyInput step 3) — so `coverType`
		// in a filter is a real address, never the phantom-key warning the
		// importer raises for a term nobody's vocabulary knows.
		return anyblockjson.KeyTermFacts{LiveStoredKey: true}
	}
	if v.scoped == nil {
		return anyblockjson.KeyTermFacts{}
	}
	return v.scoped.PropertyTermFacts(term)
}

func (v *apiKeyVocab) TypeTermFacts(term string) anyblockjson.KeyTermFacts {
	if bundle.HasObjectTypeByKey(domain.TypeKey(term)) {
		return anyblockjson.KeyTermFacts{LiveStoredKey: true}
	}
	if v.scoped == nil {
		return anyblockjson.KeyTermFacts{}
	}
	return v.scoped.TypeTermFacts(term)
}

var _ anyblockjson.KeyVocabulary = (*apiKeyVocab)(nil)
var _ anyblockjson.ScopedKeyVocabulary = (*apiKeyVocab)(nil)
