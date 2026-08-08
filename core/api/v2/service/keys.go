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
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
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

// resolvePropertyInput implements the §7.5a-5 resolution chain for one
// inbound property term: (1) exact live stored key; (2) the space's live
// slug namespace; (3) the bundled vocabulary — exact key or derived slug
// (`due_date` names bundled dueDate); (4) the forgiving fold layer
// (§7.5a-3: lowercase, `_`/`-` stripped — `dueDate` for `due_date`).
// Ambiguity at any step returns the candidate descriptions and never a
// guess (the git rule); a full miss returns ok=false and the caller's R9
// machinery owns the refusal. A resolved bundled term that is not installed
// in the space has Id == "" — address routes require an installed entry,
// existence checks do not.
func (s *V2Service) resolvePropertyInput(spaceId, input string, entries []propertyEntry) (propertyEntry, bool, []string) {
	if entries == nil {
		entries = s.liveProperties(spaceId)
	}
	// 1: exact stored key
	for _, entry := range entries {
		if entry.Key == input {
			return entry, true, nil
		}
	}
	// 2: exact live slug — two live holders is the loud ambiguity
	var slugMatches []propertyEntry
	for _, entry := range entries {
		if entry.Slug != "" && entry.Slug == input {
			slugMatches = append(slugMatches, entry)
		}
	}
	if len(slugMatches) == 1 {
		return slugMatches[0], true, nil
	}
	if len(slugMatches) > 1 {
		return propertyEntry{}, false, describePropertyEntries(slugMatches)
	}
	// 3: the bundled vocabulary (exact key, then the derived table)
	if rel, err := bundle.PickRelation(domain.RelationKey(input)); err == nil {
		return propertyEntry{Key: input, Name: rel.Name, Format: rel.Format}, true, nil
	}
	if key, ok := bundle.RelationKeyByApiSlug(input); ok {
		for _, entry := range entries {
			if entry.Key == string(key) {
				return entry, true, nil
			}
		}
		rel := bundle.MustGetRelation(key)
		return propertyEntry{Key: string(key), Name: rel.Name, Format: rel.Format}, true, nil
	}
	// 4: the fold layer — exact has failed everywhere, so a single folded
	// candidate is the intended forgiveness and several are a loud 400
	fold := bundle.FoldApiKey(input)
	var candidates []propertyEntry
	seen := map[string]bool{}
	for _, entry := range entries {
		if bundle.FoldApiKey(entry.Key) == fold || (entry.Slug != "" && bundle.FoldApiKey(entry.Slug) == fold) {
			if !seen[entry.Key] {
				seen[entry.Key] = true
				candidates = append(candidates, entry)
			}
		}
	}
	for _, key := range bundle.RelationKeysByApiFold(input) {
		if seen[string(key)] {
			continue
		}
		seen[string(key)] = true
		rel := bundle.MustGetRelation(key)
		candidates = append(candidates, propertyEntry{Key: string(key), Name: rel.Name, Format: rel.Format})
	}
	if len(candidates) == 1 {
		return candidates[0], true, nil
	}
	if len(candidates) > 1 {
		return propertyEntry{}, false, describePropertyEntries(candidates)
	}
	return propertyEntry{}, false, nil
}

// resolveTypeInput is resolvePropertyInput for the type namespace.
func (s *V2Service) resolveTypeInput(spaceId, input string, entries []typeEntry) (typeEntry, bool, []string) {
	if entries == nil {
		entries = s.liveTypes(spaceId)
	}
	for _, entry := range entries {
		if entry.Key == input {
			return entry, true, nil
		}
	}
	var slugMatches []typeEntry
	for _, entry := range entries {
		if entry.Slug != "" && entry.Slug == input {
			slugMatches = append(slugMatches, entry)
		}
	}
	if len(slugMatches) == 1 {
		return slugMatches[0], true, nil
	}
	if len(slugMatches) > 1 {
		return typeEntry{}, false, describeTypeEntries(slugMatches)
	}
	if t, err := bundle.GetType(domain.TypeKey(input)); err == nil {
		return typeEntry{Key: input, Name: t.Name}, true, nil
	}
	if key, ok := bundle.TypeKeyByApiSlug(input); ok {
		for _, entry := range entries {
			if entry.Key == string(key) {
				return entry, true, nil
			}
		}
		t := bundle.MustGetType(key)
		return typeEntry{Key: string(key), Name: t.Name}, true, nil
	}
	fold := bundle.FoldApiKey(input)
	var candidates []typeEntry
	seen := map[string]bool{}
	for _, entry := range entries {
		if bundle.FoldApiKey(entry.Key) == fold || (entry.Slug != "" && bundle.FoldApiKey(entry.Slug) == fold) {
			if !seen[entry.Key] {
				seen[entry.Key] = true
				candidates = append(candidates, entry)
			}
		}
	}
	for _, key := range bundle.TypeKeysByApiFold(input) {
		if seen[string(key)] {
			continue
		}
		seen[string(key)] = true
		t := bundle.MustGetType(key)
		candidates = append(candidates, typeEntry{Key: string(key), Name: t.Name})
	}
	if len(candidates) == 1 {
		return candidates[0], true, nil
	}
	if len(candidates) > 1 {
		return typeEntry{}, false, describeTypeEntries(candidates)
	}
	return typeEntry{}, false, nil
}

func describePropertyEntries(entries []propertyEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, fmt.Sprintf("%q (%s)", entry.Name, entry.publicKey()))
	}
	return out
}

func describeTypeEntries(entries []typeEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, fmt.Sprintf("%q (%s)", entry.Name, entry.publicKey()))
	}
	return out
}

// ambiguousKeyError is the loud 400 the chain owes an input that several
// holders answer to — candidates listed, never a guess (C6/§7.5a-3).
func ambiguousKeyError(what, input, path string, candidates []string) error {
	return v2model.AmbiguousInput(
		fmt.Sprintf("%s %q is ambiguous", what, input),
		v2model.Issue{Path: path,
			Message: fmt.Sprintf("%q matches %s", input, strings.Join(candidates, " and ")),
			Hint:    "address the intended one by its exact key"})
}

// requireLiveProperty resolves a property-addressing route param: ambiguity
// is a 400, anything not installed live in the space is the keyed 404.
func (s *V2Service) requireLiveProperty(spaceId, input string) (propertyEntry, error) {
	entry, ok, ambiguous := s.resolvePropertyInput(spaceId, input, nil)
	if len(ambiguous) > 0 {
		return propertyEntry{}, ambiguousKeyError("property key", input, "/key", ambiguous)
	}
	if !ok || entry.Id == "" {
		return propertyEntry{}, s.propertyNotFoundError(spaceId, input)
	}
	return entry, nil
}

// requireLiveType is requireLiveProperty for type routes.
func (s *V2Service) requireLiveType(spaceId, input, path string) (typeEntry, error) {
	entry, ok, ambiguous := s.resolveTypeInput(spaceId, input, nil)
	if len(ambiguous) > 0 {
		return typeEntry{}, ambiguousKeyError("type key", input, path, ambiguous)
	}
	if !ok || entry.Id == "" {
		return typeEntry{}, s.typeNotFoundError(spaceId, input)
	}
	return entry, nil
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

// servedKeySets primes the two maps the served-spelling rule needs from one
// live set: every live stored key, and the live holder count per slug.
func servedPropertyKeySets(entries []propertyEntry) (keys map[string]bool, slugCount map[string]int) {
	keys = make(map[string]bool, len(entries))
	slugCount = map[string]int{}
	for _, entry := range entries {
		keys[entry.Key] = true
		if entry.Slug != "" {
			slugCount[entry.Slug]++
		}
	}
	return keys, slugCount
}

func servedTypeKeySets(entries []typeEntry) (keys map[string]bool, slugCount map[string]int) {
	keys = make(map[string]bool, len(entries))
	slugCount = map[string]int{}
	for _, entry := range entries {
		keys[entry.Key] = true
		if entry.Slug != "" {
			slugCount[entry.Slug]++
		}
	}
	return keys, slugCount
}

// servedKey is the wire spelling of one live entry's key: the stored slug
// iff the stored key is an opaque BSON AND the slug round-trips to this
// entry through the §7.5a-5 chain — i.e. no live stored key equals it
// (chain step 1 would win) and it has exactly one live holder (twins are
// ambiguous). An address the API serves must resolve to the row it labels;
// anything else keeps the honest BSON spelling. (The full §7.5a respelling
// of READABLE keys — dueDate → due_date on the wire — is the deferred
// sweep; readable stored keys keep their spelling until it lands.)
func servedKey(storedKey, slug string, keyTaken map[string]bool, slugCount map[string]int) string {
	if slug == "" || !isBsonLikeKey(storedKey) {
		return storedKey
	}
	if keyTaken[slug] || slugCount[slug] != 1 {
		return storedKey
	}
	return slug
}

// canonicalizeDocumentKeys rewrites an inbound document's addressing terms
// to their canonical stored spellings BEFORE validation and import: the
// envelope's type/templateFor (slug → internal type key — the import path
// derives `ot-<key>` URLs from them) and the properties-map keys (slug →
// stored relation key — they become detail keys verbatim). Terms already
// canonical, or resolving to nothing (the R9 validation owns that refusal),
// pass through verbatim so errors keep the caller's spelling. Ambiguity is
// a path-addressed 400; two spellings canonicalizing onto one key is too.
func (s *V2Service) canonicalizeDocumentKeys(spaceId string, body []byte) ([]byte, error) {
	fields, err := parseEnvelope(body)
	if err != nil {
		return body, nil // not an object — the document validator owns this
	}
	changed := false

	var typeEntries []typeEntry
	for _, field := range []string{"type", "templateFor"} {
		raw, ok := fields[field]
		if !ok {
			continue
		}
		var term string
		if err := json.Unmarshal(raw, &term); err != nil || term == "" {
			continue
		}
		if typeEntries == nil {
			typeEntries = s.liveTypes(spaceId)
		}
		entry, ok, ambiguous := s.resolveTypeInput(spaceId, term, typeEntries)
		if len(ambiguous) > 0 {
			return nil, ambiguousKeyError("type key", term, "/"+field, ambiguous)
		}
		if ok && entry.Key != term {
			if fields[field], err = rawJSON(entry.Key); err != nil {
				return nil, err
			}
			changed = true
		}
	}

	if raw, ok := fields["properties"]; ok {
		var props map[string]json.RawMessage
		if err := json.Unmarshal(raw, &props); err == nil && len(props) > 0 {
			propEntries := s.liveProperties(spaceId)
			renames := map[string]string{}
			for _, key := range sortedKeys(props) {
				entry, ok, ambiguous := s.resolvePropertyInput(spaceId, key, propEntries)
				if len(ambiguous) > 0 {
					return nil, ambiguousKeyError("property key", key, "/properties/"+key, ambiguous)
				}
				if ok && entry.Key != key {
					renames[key] = entry.Key
				}
			}
			if len(renames) > 0 {
				rewritten := make(map[string]json.RawMessage, len(props))
				for key, value := range props {
					canonical := key
					if to, ok := renames[key]; ok {
						canonical = to
					}
					if _, dup := rewritten[canonical]; dup {
						return nil, v2model.ValidationFailed("duplicate property key",
							v2model.Issue{Path: "/properties/" + key,
								Message: fmt.Sprintf("%q and another spelling both address property %q — keep one", key, canonical)})
					}
					rewritten[canonical] = value
				}
				if fields["properties"], err = rawJSON(rewritten); err != nil {
					return nil, err
				}
				changed = true
			}
		}
	}

	if !changed {
		return body, nil
	}
	return encodeEnvelope(fields)
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
