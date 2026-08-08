package v2service

// keys.go — the space's live type/property key surface (ADDRESSING.md §7.5,
// §7.5a): one bounded details query per kind primes entries carrying the
// stored key, the stored api slug (apiObjectKey), name and format.
//
// "Live" is the §7.5-requirement-2 corpse policy: archived and deleted
// objects are excluded by the store's injected defaults, and *uninstalled* —
// the UI-delete flag, which nothing else in the query layer filters (the
// §2.3-6 defect: a UI-deleted property still resolved by key, still listed
// in GET /properties, and blocked a same-key create) — is excluded here
// explicitly. A corpse must neither list, nor resolve as an address, nor
// hold its slug against a same-key create.

import (
	"sort"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// propertyEntry is one live relation's identity row.
type propertyEntry struct {
	Id     string
	Key    string // stored relation key (bundled word, legacy readable, or BSON)
	Slug   string // stored apiObjectKey; empty for pre-slug keys
	Name   string
	Format model.RelationFormat
}

// typeEntry is one live type object's identity row.
type typeEntry struct {
	Id   string
	Key  string // internal type key (uniqueKey's internal part)
	Slug string
	Name string
}

// livePropertyFilters are the corpse-policy filters for relation queries:
// layout, plus the explicit isUninstalled exclusion (isArchived/isDeleted
// ride the store's injected defaults).
func livePropertyFilters() []database.FilterRequest {
	return []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_relation)),
		},
		{
			RelationKey: bundle.RelationKeyIsUninstalled,
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       domain.Bool(true),
		},
	}
}

// liveTypeFilters is livePropertyFilters for type objects.
func liveTypeFilters() []database.FilterRequest {
	return []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_objectType)),
		},
		{
			RelationKey: bundle.RelationKeyIsUninstalled,
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       domain.Bool(true),
		},
	}
}

// liveProperties lists the space's live relations — one bounded query, the
// per-request resolver shape ADDRESSING §7.5a-2 prescribes (tens to low
// hundreds of rows; never one query per reference).
func (s *V2Service) liveProperties(spaceId string) []propertyEntry {
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{Filters: livePropertyFilters()})
	if err != nil {
		return nil
	}
	entries := make([]propertyEntry, 0, len(records))
	for _, record := range records {
		key := record.Details.GetString(bundle.RelationKeyRelationKey)
		if key == "" {
			continue
		}
		entries = append(entries, propertyEntry{
			Id:     record.Details.GetString(bundle.RelationKeyId),
			Key:    key,
			Slug:   record.Details.GetString(bundle.RelationKeyApiObjectKey),
			Name:   record.Details.GetString(bundle.RelationKeyName),
			Format: model.RelationFormat(record.Details.GetInt64(bundle.RelationKeyRelationFormat)),
		})
	}
	return entries
}

// liveTypes lists the space's live type objects.
func (s *V2Service) liveTypes(spaceId string) []typeEntry {
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{Filters: liveTypeFilters()})
	if err != nil {
		return nil
	}
	entries := make([]typeEntry, 0, len(records))
	for _, record := range records {
		key, err := domain.GetTypeKeyFromRawUniqueKey(record.Details.GetString(bundle.RelationKeyUniqueKey))
		if err != nil {
			continue
		}
		entries = append(entries, typeEntry{
			Id:   record.Details.GetString(bundle.RelationKeyId),
			Key:  string(key),
			Slug: record.Details.GetString(bundle.RelationKeyApiObjectKey),
			Name: record.Details.GetString(bundle.RelationKeyName),
		})
	}
	return entries
}

// livePropertyByKey resolves an exact stored relation key against the live
// set — the corpse-aware replacement for GetRelationByKey on API addressing
// paths.
func (s *V2Service) livePropertyByKey(spaceId, key string) (propertyEntry, bool) {
	for _, entry := range s.liveProperties(spaceId) {
		if entry.Key == key {
			return entry, true
		}
	}
	return propertyEntry{}, false
}

// liveTypeByKey resolves an exact internal type key against the live set.
func (s *V2Service) liveTypeByKey(spaceId, key string) (typeEntry, bool) {
	for _, entry := range s.liveTypes(spaceId) {
		if entry.Key == key {
			return entry, true
		}
	}
	return typeEntry{}, false
}

// slugHolder names the existing holder of a proposed api key — the material
// for the loud refusal the union collision check owes the caller.
type slugHolder struct {
	Kind string // "bundled property", "property", "bundled type", "type"
	Key  string // the holder's public key (bundled slug or stored key/slug)
	Name string
}

// propertySlugConflict runs the §7.5a-6 union collision check for a property
// mint: the proposed slug is tested against bundled keys, bundled-derived
// slugs, live stored keys and live stored slugs. Corpses hold nothing — the
// §8-OQ2 vacate lean, which is what makes delete-then-recreate mint cleanly
// ((a)'s headline win). The check ships WITH the mint it guards (§7.6-3).
func (s *V2Service) propertySlugConflict(spaceId, slug string) (slugHolder, bool) {
	if key, ok := bundle.RelationKeyByApiSlug(slug); ok {
		rel := bundle.MustGetRelation(key)
		return slugHolder{Kind: "bundled property", Key: slug, Name: rel.Name}, true
	}
	if bundle.HasRelation(domain.RelationKey(slug)) {
		rel := bundle.MustGetRelation(domain.RelationKey(slug))
		return slugHolder{Kind: "bundled property", Key: slug, Name: rel.Name}, true
	}
	for _, entry := range s.liveProperties(spaceId) {
		if entry.Key == slug || entry.Slug == slug {
			return slugHolder{Kind: "property", Key: entry.publicKey(), Name: entry.Name}, true
		}
	}
	return slugHolder{}, false
}

// typeSlugConflict is propertySlugConflict for the type namespace.
func (s *V2Service) typeSlugConflict(spaceId, slug string) (slugHolder, bool) {
	if key, ok := bundle.TypeKeyByApiSlug(slug); ok {
		t := bundle.MustGetType(key)
		return slugHolder{Kind: "bundled type", Key: slug, Name: t.Name}, true
	}
	if bundle.HasObjectTypeByKey(domain.TypeKey(slug)) {
		t := bundle.MustGetType(domain.TypeKey(slug))
		return slugHolder{Kind: "bundled type", Key: slug, Name: t.Name}, true
	}
	for _, entry := range s.liveTypes(spaceId) {
		if entry.Key == slug || entry.Slug == slug {
			return slugHolder{Kind: "type", Key: entry.publicKey(), Name: entry.Name}, true
		}
	}
	return slugHolder{}, false
}

// publicKey is the address an entry answers to on the API surface: the
// stored slug when the stored key is an opaque BSON, the stored key
// otherwise. (The full §7.5a slugs-always respelling of bundled keys is the
// deferred sweep; until it lands, readable stored keys stay the wire
// spelling and only BSON keys hide behind their slug.)
func (e propertyEntry) publicKey() string {
	if e.Slug != "" && isBsonLikeKey(e.Key) {
		return e.Slug
	}
	return e.Key
}

func (e typeEntry) publicKey() string {
	if e.Slug != "" && isBsonLikeKey(e.Key) {
		return e.Slug
	}
	return e.Key
}

// isBsonLikeKey reports whether a stored key is a minted BSON ObjectId hex —
// 24 hex chars with at least one digit (the v1 heuristic, core/api/util).
func isBsonLikeKey(key string) bool {
	if len(key) != 24 {
		return false
	}
	digit := false
	for _, r := range key {
		switch {
		case r >= '0' && r <= '9':
			digit = true
		case r >= 'a' && r <= 'f':
		default:
			return false
		}
	}
	return digit
}

// propertyDefinition adapts an entry to the anyblockjson definition shape.
func (e propertyEntry) propertyDefinition() anyblockjson.PropertyDefinition {
	return anyblockjson.PropertyDefinition{
		Key:    domain.RelationKey(e.Key),
		Name:   e.Name,
		Format: e.Format,
	}
}

// sortedDistinct returns the sorted distinct non-empty values.
func sortedDistinct(values []string) []string {
	seen := make(map[string]bool, len(values))
	out := make([]string, 0, len(values))
	for _, v := range values {
		if v != "" && !seen[v] {
			seen[v] = true
			out = append(out, v)
		}
	}
	sort.Strings(out)
	return out
}
