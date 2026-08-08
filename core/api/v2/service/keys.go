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
	// Hidden entries stay addressable by their exact stored key but do NOT
	// participate in the slug namespace (resolution, fold, collision,
	// serving): a hidden holder is invisible and undeletable to the caller,
	// so letting it block or ambiguate a visible holder's slug would make
	// that slug permanently unusable through no visible cause.
	Hidden bool
}

// typeEntry is one live type object's identity row.
type typeEntry struct {
	Id     string
	Key    string // internal type key (uniqueKey's internal part)
	Slug   string
	Name   string
	Hidden bool
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
// hundreds of rows; never one query per reference). A store error is
// returned, never swallowed: the collision check and the resolution chain
// are load-bearing, and an empty-looking namespace on a store hiccup would
// wave every collision through (fail closed, not open).
func (s *V2Service) liveProperties(spaceId string) ([]propertyEntry, error) {
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{Filters: livePropertyFilters()})
	if err != nil {
		return nil, fmt.Errorf("query live properties of space %s: %w", spaceId, err)
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
			Hidden: record.Details.GetBool(bundle.RelationKeyIsHidden),
		})
	}
	return entries, nil
}

// liveTypes lists the space's live type objects (error contract as above).
func (s *V2Service) liveTypes(spaceId string) ([]typeEntry, error) {
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{Filters: liveTypeFilters()})
	if err != nil {
		return nil, fmt.Errorf("query live types of space %s: %w", spaceId, err)
	}
	entries := make([]typeEntry, 0, len(records))
	for _, record := range records {
		key, err := domain.GetTypeKeyFromRawUniqueKey(record.Details.GetString(bundle.RelationKeyUniqueKey))
		if err != nil {
			continue
		}
		entries = append(entries, typeEntry{
			Id:     record.Details.GetString(bundle.RelationKeyId),
			Key:    string(key),
			Slug:   record.Details.GetString(bundle.RelationKeyApiObjectKey),
			Name:   record.Details.GetString(bundle.RelationKeyName),
			Hidden: record.Details.GetBool(bundle.RelationKeyIsHidden),
		})
	}
	return entries, nil
}

// resolvePropertyInput implements the §7.5a-5 resolution chain for one
// inbound property term over a primed live set (entries are mandatory —
// the caller loads them and owns the load error, so a store hiccup fails
// closed, never open): (1) exact live stored key; (2) the space's live
// slug namespace; (3) the bundled vocabulary — exact key or derived slug
// (`due_date` names bundled dueDate); (4) the forgiving fold layer
// (§7.5a-3: lowercase, `_`/`-` stripped — `dueDate` for `due_date`).
// Hidden entries answer to their exact stored key only (step 1) and are
// invisible to the slug and fold steps — see propertyEntry.Hidden.
// Ambiguity at any step returns the candidate descriptions and never a
// guess (the git rule); a full miss returns ok=false and the caller's R9
// machinery owns the refusal. A resolved bundled term that is not installed
// in the space has Id == "" — address routes require an installed entry,
// existence checks do not.
func (s *V2Service) resolvePropertyInput(input string, entries []propertyEntry) (propertyEntry, bool, []string) {
	// 1: exact stored key (hidden included — the stored key is always an
	// address)
	for _, entry := range entries {
		if entry.Key == input {
			return entry, true, nil
		}
	}
	// 2: exact live slug — two visible holders is the loud ambiguity
	var slugMatches []propertyEntry
	for _, entry := range entries {
		if !entry.Hidden && entry.Slug != "" && entry.Slug == input {
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
		if entry.Hidden {
			continue
		}
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
func (s *V2Service) resolveTypeInput(input string, entries []typeEntry) (typeEntry, bool, []string) {
	for _, entry := range entries {
		if entry.Key == input {
			return entry, true, nil
		}
	}
	var slugMatches []typeEntry
	for _, entry := range entries {
		if !entry.Hidden && entry.Slug != "" && entry.Slug == input {
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
		if entry.Hidden {
			continue
		}
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

// describePropertyEntries renders ambiguity candidates ACTIONABLY: the
// stored key is the one address that always resolves (twin slugs print
// identically, so the slug alone steers the caller back into the same
// 400 — the review's unactionable-floor finding).
func describePropertyEntries(entries []propertyEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, fmt.Sprintf("%q (key %s, id %s)", entry.Name, entry.Key, entry.Id))
	}
	return out
}

func describeTypeEntries(entries []typeEntry) []string {
	out := make([]string, 0, len(entries))
	for _, entry := range entries {
		out = append(out, fmt.Sprintf("%q (key %s, id %s)", entry.Name, entry.Key, entry.Id))
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
// is a 400, anything not installed live in the space is the keyed 404, and
// a store failure propagates (fail closed).
func (s *V2Service) requireLiveProperty(spaceId, input string) (propertyEntry, error) {
	entries, err := s.liveProperties(spaceId)
	if err != nil {
		return propertyEntry{}, err
	}
	entry, ok, ambiguous := s.resolvePropertyInput(input, entries)
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
	entries, err := s.liveTypes(spaceId)
	if err != nil {
		return typeEntry{}, err
	}
	entry, ok, ambiguous := s.resolveTypeInput(input, entries)
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

// propertySlugConflict runs the §7.5a-6 union collision check for a
// property mint by asking the resolution chain itself: a slug is free iff
// NOTHING resolves for it — live stored keys, live slugs, bundled keys,
// bundled-derived slugs AND the fold layer (a `moodlevel` minted beside
// `mood_level` would make the folded spelling permanently ambiguous for
// every caller, so an occupied fold class refuses too). Corpses and hidden
// holders vacate the namespace (§8-OQ2 / propertyEntry.Hidden). The check
// ships WITH the mint it guards (§7.6-3).
func (s *V2Service) propertySlugConflict(slug string, entries []propertyEntry) (slugHolder, bool) {
	entry, ok, ambiguous := s.resolvePropertyInput(slug, entries)
	if len(ambiguous) > 0 {
		return slugHolder{Kind: "properties", Key: slug, Name: strings.Join(ambiguous, " and ")}, true
	}
	if !ok {
		return slugHolder{}, false
	}
	if entry.Id == "" {
		return slugHolder{Kind: "bundled property", Key: bundle.ApiSlug(entry.Key), Name: entry.Name}, true
	}
	return slugHolder{Kind: "property", Key: entry.Key, Name: entry.Name}, true
}

// typeSlugConflict is propertySlugConflict for the type namespace.
func (s *V2Service) typeSlugConflict(slug string, entries []typeEntry) (slugHolder, bool) {
	entry, ok, ambiguous := s.resolveTypeInput(slug, entries)
	if len(ambiguous) > 0 {
		return slugHolder{Kind: "types", Key: slug, Name: strings.Join(ambiguous, " and ")}, true
	}
	if !ok {
		return slugHolder{}, false
	}
	if entry.Id == "" {
		return slugHolder{Kind: "bundled type", Key: bundle.ApiSlug(entry.Key), Name: entry.Name}, true
	}
	return slugHolder{Kind: "type", Key: entry.Key, Name: entry.Name}, true
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

// sanitizeApiSlug constrains a DERIVED slug (from a display name or a
// document key — inputs no pattern ever checked) to the advertised key
// grammar ^[a-zA-Z0-9_]+$ and maxV2KeyLength: every disallowed rune
// becomes `_`, runs collapse, edges trim. Without this, "50% done", "C++"
// or "☕" (unidecode: "?") became identity-bearing apiObjectKey values the
// create returned as keys that no /properties/{key} route could accept.
// Empty result = no derivable slug; the caller falls back to the minted
// BSON as the only address.
func sanitizeApiSlug(raw string) string {
	var b strings.Builder
	lastUnderscore := false
	for _, r := range raw {
		valid := r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9')
		if !valid {
			r = '_'
		}
		if r == '_' {
			if lastUnderscore {
				continue
			}
			lastUnderscore = true
		} else {
			lastUnderscore = false
		}
		b.WriteRune(r)
	}
	out := strings.Trim(b.String(), "_")
	if len(out) > maxV2KeyLength {
		out = strings.Trim(out[:maxV2KeyLength], "_")
	}
	return out
}

// servedKeySets primes the two maps the served-spelling rule needs from one
// live set: every live stored key, and the VISIBLE holder count per slug
// (hidden holders don't participate in the slug namespace — a hidden twin
// must not downgrade the visible row's spelling).
func servedPropertyKeySets(entries []propertyEntry) (keys map[string]bool, slugCount map[string]int) {
	keys = make(map[string]bool, len(entries))
	slugCount = map[string]int{}
	for _, entry := range entries {
		keys[entry.Key] = true
		if !entry.Hidden && entry.Slug != "" {
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
		if !entry.Hidden && entry.Slug != "" {
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
			if typeEntries, err = s.liveTypes(spaceId); err != nil {
				return nil, err
			}
		}
		entry, ok, ambiguous := s.resolveTypeInput(term, typeEntries)
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
			propEntries, err := s.liveProperties(spaceId)
			if err != nil {
				return nil, err
			}
			renames := map[string]string{}
			for _, key := range sortedKeys(props) {
				entry, ok, ambiguous := s.resolvePropertyInput(key, propEntries)
				if len(ambiguous) > 0 {
					return nil, ambiguousKeyError("property key", key, "/properties/"+key, ambiguous)
				}
				if ok && entry.Key != key {
					renames[key] = entry.Key
				}
			}
			if len(renames) > 0 {
				rewritten := make(map[string]json.RawMessage, len(props))
				// deterministic order, so a duplicate-spelling refusal
				// names the same path on every run
				for _, key := range sortedKeys(props) {
					canonical := key
					if to, ok := renames[key]; ok {
						canonical = to
					}
					if _, dup := rewritten[canonical]; dup {
						return nil, v2model.ValidationFailed("duplicate property key",
							v2model.Issue{Path: "/properties/" + key,
								Message: fmt.Sprintf("%q and another spelling both address property %q — keep one", key, canonical)})
					}
					rewritten[canonical] = props[key]
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
