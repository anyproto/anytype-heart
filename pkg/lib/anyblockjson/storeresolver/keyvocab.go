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
// gives chain step 1 its precedence.
type keyMaps struct {
	slugByKey map[string]string
	keyBySlug map[string]string
	storedKey map[string]bool
}

func newKeyMaps() *keyMaps {
	return &keyMaps{slugByKey: map[string]string{}, keyBySlug: map[string]string{}, storedKey: map[string]bool{}}
}

// add records one live entity. A slug with two holders is dropped from the
// reverse direction: an ambiguous address must never resolve by store order
// (the git rule), and dropping it degrades to the stored key, which always
// works.
func (m *keyMaps) add(key, slug string) {
	if key == "" {
		return
	}
	m.storedKey[key] = true
	if slug == "" || slug == key {
		return
	}
	if _, taken := m.keyBySlug[slug]; taken {
		m.keyBySlug[slug] = "" // twin slugs: neither wins
		return
	}
	m.keyBySlug[slug] = key
	m.slugByKey[key] = slug
}

// slug is the wire spelling of a stored key: its stored slug, unless a live
// stored key already answers to that spelling (step 1 would win, so serving
// it would not round-trip).
func (m *keyMaps) slug(key string) (string, bool) {
	s, ok := m.slugByKey[key]
	if !ok || m.storedKey[s] {
		return "", false
	}
	return s, true
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
		})
	}
	return r.typeVocab
}

// loadKeyMaps runs the one bounded listing. A store error yields an EMPTY
// vocabulary, not a partial one: the caller then falls back to the bundled
// table, which is the offline-safe answer — never a stale or half-built map,
// which would resolve a write against the wrong property (the exact
// silent-failure class §7.5a-2 forbids a cache from ever producing).
func (r *Resolvers) loadKeyMaps(layout model.ObjectTypeLayout, keyOf func(*domain.Details) string) *keyMaps {
	maps := newKeyMaps()
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
// their derived slug (the code table is their authority — §7.5a-1), the rest
// as their stored slug when they have one.
func (r *Resolvers) PropertySlug(key string) string {
	if bundle.HasRelation(domain.RelationKey(key)) {
		return bundle.ApiSlug(key)
	}
	if slug, ok := r.relationKeyMaps().slug(key); ok {
		return slug
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

func (r *Resolvers) TypeSlug(key string) string {
	if bundle.HasObjectTypeByKey(domain.TypeKey(key)) {
		return bundle.ApiSlug(key)
	}
	if slug, ok := r.typeKeyMaps().slug(key); ok {
		return slug
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
