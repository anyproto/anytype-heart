package storeresolver

// keyvocab.go — the space-backed key vocabulary (APIV2_ADDRESSING.md §7.5a).
//
// The package default (anyblockjson.BundledKeyVocabulary) knows only the
// bundled derived table, which is all an offline reader can know. Inside a
// node the space itself is the second authority: every non-bundled type and
// property carries a stored `apiObjectKey` slug, and §7.5a says the document
// spells THAT, not the opaque BSON the store binds.
//
// Shape, exactly as §7.5a-2 prescribes: **one bounded query per kind per
// resolver instance** (i.e. per export/import operation), never one point
// query per reference — `apiObjectKey` is an ordinary hidden detail with no
// dedicated index, so a lookup per reference would each pay a scan. The
// listing that primes it is a DETAILS query rather than ListAllRelations,
// because `model.Relation` carries no apiObjectKey.
//
// Precedence follows the §7.5a-5 chain, and the ordering is load-bearing:
// an exact live STORED key always wins over the slug layer (step 1 before
// step 2), so a document naming a relation whose stored key happens to be
// spelled like someone else's slug still binds to the relation it named.
// The vocabulary is lazy: a document with no key slots pays nothing.

import (
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// keyMaps is one namespace's two directions plus the stored-key set that
// gives chain step 1 its precedence. bundledKey is that namespace's bundled
// reverse table — the third chain step, which the EMIT side has to consult
// too (see roundTrips).
type keyMaps struct {
	slugByKey  map[string]string
	keyBySlug  map[string]string
	storedKey  map[string]bool
	keysByFold map[string][]string // chain step 4 — see fold
	bundledKey func(slug string) (string, bool)
	// bundledFold is that namespace's bundled fold table (chain step 4's
	// bundled arm), as stored-key strings.
	bundledFold func(input string) []string
}

func newKeyMaps(bundledKey func(string) (string, bool)) *keyMaps {
	return &keyMaps{
		slugByKey:  map[string]string{},
		keyBySlug:  map[string]string{},
		storedKey:  map[string]bool{},
		keysByFold: map[string][]string{},
		bundledKey: bundledKey,
	}
}

// add records one live entity. A slug with two holders is dropped from BOTH
// directions: an ambiguous address must never resolve by store order (the git
// rule), and dropping it degrades to the stored key, which always works.
// Clearing only the reverse map left the first holder still EXPORTING a slug
// that import then refused to invert — a document naming an address the
// server itself rejects.
//
// A HIDDEN entity keeps its stored key (chain step 1 — the stored key is
// always an address, and roundTrips must still refuse to emit a spelling it
// owns) but does NOT enter the slug namespace, exactly as v2's request
// namespace has it (core/api/v2/service/keys.go, propertyEntry.Hidden). The
// rule has to be the same in both builders or the two disagree on one
// spelling: a hidden twin used to erase a VISIBLE holder's slug from
// keyBySlug, so a listing advertised `severity` while a POST naming it stored
// `severity` verbatim as a relation key no relation object owns; and a hidden
// squatter holding `due_date` used to win the reverse lookup outright, so the
// bundled property's own slug resolved to the squatter.
func (m *keyMaps) add(key, slug string, hidden bool) {
	if key == "" {
		return
	}
	m.storedKey[key] = true
	if hidden {
		return
	}
	m.addFold(bundle.FoldApiKey(key), key)
	if slug == "" || slug == key {
		return
	}
	m.addFold(bundle.FoldApiKey(slug), key)
	if first, taken := m.keyBySlug[slug]; taken {
		m.keyBySlug[slug] = "" // twin slugs: neither wins
		delete(m.slugByKey, first)
		return
	}
	m.keyBySlug[slug] = key
	m.slugByKey[key] = slug
}

// roundTrips reports whether emitting `slug` for `key` inverts back to `key`
// through the §7.5a-5 chain — the emit side's whole obligation. An emitted
// spelling that resolves elsewhere does not degrade a document, it MISLABELS
// it: the value belongs to one entity and the key names another.
//
// Three ways it can fail, one per chain step:
//   - a live stored key answers to the spelling (step 1 wins over any slug);
//   - another live holder claims the slug (step 2 is ambiguous → a loud 400);
//   - the bundled table resolves it to a different key (step 3 — the §7.5a-6
//     shadow, e.g. a UI property that took `due_date` while bundled `dueDate`
//     derives it).
//
// This is the same predicate the API's servedKey applies to a listing row, and
// it must stay the same: the address a document carries and the address the
// listing advertises are the same address.
func (m *keyMaps) roundTrips(slug, key string) bool {
	if slug == "" || slug == key {
		return false
	}
	if m.storedKey[slug] {
		return false
	}
	if holder, ok := m.keyBySlug[slug]; ok && holder != key {
		return false
	}
	if m.bundledKey != nil {
		if other, ok := m.bundledKey(slug); ok && other != key {
			return false
		}
	}
	return true
}

func (m *keyMaps) key(slug string) (string, bool) {
	if m.storedKey[slug] {
		return "", false // chain step 1: an exact stored key wins
	}
	k, ok := m.keyBySlug[slug]
	if !ok || k == "" {
		return "", false
	}
	// The accept side owes the same answer the emit side gives. roundTrips
	// refuses to SPELL this holder with a slug the bundled table resolves
	// elsewhere; without the same guard here, accept BINDS that spelling to
	// this holder — so a document naming the bundled key `priority` lands on
	// whichever custom relation claimed `priority` as its api key. A
	// 36 808-object sweep found 12 objects re-pointed exactly that way: the
	// index is built from every holder's stored slug, and only the emit side
	// was filtering it.
	if m.bundledKey != nil {
		if other, ok := m.bundledKey(slug); ok && other != k {
			return "", false
		}
	}
	return k, true
}

func (m *keyMaps) addFold(fold, key string) {
	for _, existing := range m.keysByFold[fold] {
		if existing == key {
			return
		}
	}
	m.keysByFold[fold] = append(m.keysByFold[fold], key)
}

// fold is chain step 4, the forgiving layer (§7.5a-3): every exact lookup has
// already failed, so a SINGLE key whose stored key or stored slug folds to the
// input's fold is the intended forgiveness, and several are an ambiguity that
// degrades to the verbatim term — never a guess.
//
// The accept half had no step 4 at all, while the v2 ROUTE layer did: a
// dataview or a properties map naming `Severity` or `sever_ity` stored the
// term verbatim as a column key, 200 OK, though both fold to the live
// property that the very same request would have found through /properties.
// Only updateView was covered (canonicalViewKey). Hidden holders do not
// participate, as at every other step.
func (m *keyMaps) fold(input string) (string, bool) {
	stored := m.keysByFold[bundle.FoldApiKey(input)]
	candidates := append(make([]string, 0, len(stored)+1), stored...)
	if m.bundledFold != nil {
		seen := make(map[string]bool, len(candidates))
		for _, c := range candidates {
			seen[c] = true
		}
		for _, key := range m.bundledFold(input) {
			if !seen[key] {
				seen[key] = true
				candidates = append(candidates, key)
			}
		}
	}
	if len(candidates) == 1 {
		return candidates[0], true
	}
	return "", false
}

func (r *Resolvers) relationKeyMaps() *keyMaps {
	if r.relVocab == nil {
		r.relVocab = r.loadKeyMaps(model.ObjectType_relation, func(d *domain.Details) string {
			return d.GetString(bundle.RelationKeyRelationKey)
		}, func(slug string) (string, bool) {
			key, ok := bundle.RelationKeyByApiSlug(slug)
			return string(key), ok
		}, func(input string) []string {
			keys := bundle.RelationKeysByApiFold(input)
			out := make([]string, len(keys))
			for i, key := range keys {
				out[i] = string(key)
			}
			return out
		})
	}
	return r.relVocab
}

func (r *Resolvers) typeKeyMaps() *keyMaps {
	if r.typeVocab == nil {
		r.typeVocab = r.loadKeyMaps(model.ObjectType_objectType, func(d *domain.Details) string {
			key, err := domain.GetTypeKeyFromRawUniqueKey(d.GetString(bundle.RelationKeyUniqueKey))
			if err != nil {
				return ""
			}
			return string(key)
		}, func(slug string) (string, bool) {
			key, ok := bundle.TypeKeyByApiSlug(slug)
			return string(key), ok
		}, func(input string) []string {
			keys := bundle.TypeKeysByApiFold(input)
			out := make([]string, len(keys))
			for i, key := range keys {
				out[i] = string(key)
			}
			return out
		})
	}
	return r.typeVocab
}

// loadKeyMaps runs the one bounded listing. A store error yields an EMPTY
// vocabulary, not a partial one: the caller then falls back to the bundled
// table, which is the offline-safe answer — never a stale or half-built map,
// which would resolve a write against the wrong property (the exact
// silent-failure class §7.5a-2 forbids a cache from ever producing).
func (r *Resolvers) loadKeyMaps(layout model.ObjectTypeLayout, keyOf func(*domain.Details) string, bundledKey func(string) (string, bool), bundledFold func(string) []string) *keyMaps {
	maps := newKeyMaps(bundledKey)
	maps.bundledFold = bundledFold
	records, err := r.index.Query(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(layout)),
		},
		{
			// the §7.5-requirement-2 corpse policy: a UI-deleted entity
			// vacates the slug namespace here as everywhere else
			RelationKey: bundle.RelationKeyIsUninstalled,
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       domain.Bool(true),
		},
	}})
	if err != nil {
		return maps
	}
	for _, record := range records {
		maps.add(keyOf(record.Details),
			record.Details.GetString(bundle.RelationKeyApiObjectKey),
			record.Details.GetBool(bundle.RelationKeyIsHidden))
	}
	return maps
}

// PropertySlug implements anyblockjson.KeyVocabulary: bundled keys spell as
// their DERIVED slug (the code table is their authority in every space and
// offline — §7.5a-1), the rest as their STORED slug. Either way the spelling
// is emitted only when it round-trips back to this very key (keyMaps.
// roundTrips): the alternative is not a lost slug but a mislabeled value.
//
// A bundled key therefore consults the space's vocabulary too, which costs the
// one bounded listing §7.5a-2 budgets — the same query the custom path already
// pays, and the price of the emit half implementing the same chain the accept
// half does. Without it, a space where a UI property squats `due_date` emitted
// the bundled property's value under a key naming the squatter.
func (r *Resolvers) PropertySlug(key string) string {
	maps := r.relationKeyMaps()
	candidate := maps.slugByKey[key]
	if bundle.HasRelation(domain.RelationKey(key)) {
		candidate = bundle.ApiSlug(key)
	}
	if maps.roundTrips(candidate, key) {
		return candidate
	}
	return key
}

func (r *Resolvers) PropertyKey(slug string) (string, bool) {
	maps := r.relationKeyMaps()
	if key, ok := maps.key(slug); ok {
		return key, true
	}
	if maps.storedKey[slug] {
		return slug, false // chain step 1 — do not consult the bundled table
	}
	if key, ok := (anyblockjson.BundledKeyVocabulary{}).PropertyKey(slug); ok {
		return key, true
	}
	if key, ok := maps.fold(slug); ok {
		return key, true
	}
	return slug, false
}

// TypeSlug is PropertySlug for the type namespace.
func (r *Resolvers) TypeSlug(key string) string {
	maps := r.typeKeyMaps()
	candidate := maps.slugByKey[key]
	if bundle.HasObjectTypeByKey(domain.TypeKey(key)) {
		candidate = bundle.ApiSlug(key)
	}
	if maps.roundTrips(candidate, key) {
		return candidate
	}
	return key
}

func (r *Resolvers) TypeKey(slug string) (string, bool) {
	maps := r.typeKeyMaps()
	if key, ok := maps.key(slug); ok {
		return key, true
	}
	if maps.storedKey[slug] {
		return slug, false
	}
	if key, ok := (anyblockjson.BundledKeyVocabulary{}).TypeKey(slug); ok {
		return key, true
	}
	if key, ok := maps.fold(slug); ok {
		return key, true
	}
	return slug, false
}
