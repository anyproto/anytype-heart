package storeresolver

// keyvocab.go — the space-backed key vocabulary (ADDRESSING.md §7.5a).
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
	bundledKey func(slug string) (string, bool)
}

func newKeyMaps(bundledKey func(string) (string, bool)) *keyMaps {
	return &keyMaps{
		slugByKey:  map[string]string{},
		keyBySlug:  map[string]string{},
		storedKey:  map[string]bool{},
		bundledKey: bundledKey,
	}
}

// add records one live entity. A slug with two holders is dropped from BOTH
// directions: an ambiguous address must never resolve by store order (the git
// rule), and dropping it degrades to the stored key, which always works.
// Clearing only the reverse map left the first holder still EXPORTING a slug
// that import then refused to invert — a document naming an address the
// server itself rejects.
func (m *keyMaps) add(key, slug string) {
	if key == "" {
		return
	}
	m.storedKey[key] = true
	if slug == "" || slug == key {
		return
	}
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
	return k, ok && k != ""
}

func (r *Resolvers) relationKeyMaps() *keyMaps {
	if r.relVocab == nil {
		r.relVocab = r.loadKeyMaps(model.ObjectType_relation, func(d *domain.Details) string {
			return d.GetString(bundle.RelationKeyRelationKey)
		}, func(slug string) (string, bool) {
			key, ok := bundle.RelationKeyByApiSlug(slug)
			return string(key), ok
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
		})
	}
	return r.typeVocab
}

// loadKeyMaps runs the one bounded listing. A store error yields an EMPTY
// vocabulary, not a partial one: the caller then falls back to the bundled
// table, which is the offline-safe answer — never a stale or half-built map,
// which would resolve a write against the wrong property (the exact
// silent-failure class §7.5a-2 forbids a cache from ever producing).
func (r *Resolvers) loadKeyMaps(layout model.ObjectTypeLayout, keyOf func(*domain.Details) string, bundledKey func(string) (string, bool)) *keyMaps {
	maps := newKeyMaps(bundledKey)
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
		maps.add(keyOf(record.Details), record.Details.GetString(bundle.RelationKeyApiObjectKey))
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
	if key, ok := r.relationKeyMaps().key(slug); ok {
		return key, true
	}
	if r.relationKeyMaps().storedKey[slug] {
		return slug, false // chain step 1 — do not consult the bundled table
	}
	return anyblockjson.BundledKeyVocabulary{}.PropertyKey(slug)
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
	if key, ok := r.typeKeyMaps().key(slug); ok {
		return key, true
	}
	if r.typeKeyMaps().storedKey[slug] {
		return slug, false
	}
	return anyblockjson.BundledKeyVocabulary{}.TypeKey(slug)
}
