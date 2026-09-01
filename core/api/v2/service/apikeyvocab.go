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
	svc     *Service
	spaceId string
	inner   anyblockjson.KeyVocabulary
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

	typeSlugByKey   map[string]string
	typeKeyBySlug   map[string]string
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

func (v *apiKeyVocab) PropertyKeyCandidates(spelling string) []string {
	var out []string
	if v.ensure() && !v.propKeyTaken[spelling] {
		if key, ok := v.propKeyBySlug[spelling]; ok {
			out = append(out, key)
		} else if key, ok := bundle.RelationKeyByApiSlug(spelling); ok && v.PropertySlug(string(key)) == spelling {
			out = append(out, string(key))
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
