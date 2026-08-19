package v2service

// keys.go — the space's live type/property key surface (APIV2_ADDRESSING.md §7.5,
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
	"context"
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
func (s *Service) liveProperties(spaceId string) ([]propertyEntry, error) {
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
func (s *Service) liveTypes(spaceId string) ([]typeEntry, error) {
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
func (s *Service) resolvePropertyInput(input string, entries []propertyEntry) (propertyEntry, bool, []string) {
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
		if shadowed, ok := shadowedBundledProperty(input, slugMatches[0].Key); ok {
			return propertyEntry{}, false, append(describePropertyEntries(slugMatches), shadowed)
		}
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
func (s *Service) resolveTypeInput(input string, entries []typeEntry) (typeEntry, bool, []string) {
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
		if shadowed, ok := shadowedBundledType(input, slugMatches[0].Key); ok {
			return typeEntry{}, false, append(describeTypeEntries(slugMatches), shadowed)
		}
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

// shadowedBundledProperty reports whether a STORED slug that just matched at
// chain step 2 shadows a different key in the bundled vocabulary — the
// live-defect shape ADDRESSING §7.5a-6 names: a UI property named "Due Date"
// took `due_date`, which is bundled `dueDate`'s derived slug, and every
// `set_properties {"set": {"due_date": …}}` since has landed in that property
// instead of the bundled one. Silently.
//
// New shadows are unreachable through every write channel that stamps
// `apiObjectKey`: the heart-side mint runs the union check
// (objectcreator/apikey.go), v2's POST refuses a taken slug, and v1's rename
// channel — which never enters objectcreator and so was NOT covered by the
// mint hardening, only by a per-space cache with no row for an uninstalled
// bundled relation — now applies the bundled arm too (service/property.go's
// shadowsBundledRelationKey). A space that already holds a shadow still cannot
// be repaired without re-pointing a slug v1 has been serving as an address
// (ADDRESSING §8-OQ3 owns that decision). What CAN be fixed without touching
// stored data is the failure mode: chain step 2 no longer picks the squatter
// over the bundled property — the input is ambiguous, and ambiguity at any
// step is a loud 400 listing every holder (§7.5-req-1). Wrong-and-silent
// becomes refused-and-actionable.
//
// The check is exact, never folded: only a slug that the bundled table
// itself resolves to a DIFFERENT key shadows anything. A bundled relation
// carrying its own derived slug (dueDate/due_date) is not a shadow, and a
// stored KEY spelled like a bundled slug still wins at step 1 — that is the
// chain's documented precedence (§8.23's stored-key-shadow case), not this.
func shadowedBundledProperty(input, matchedKey string) (string, bool) {
	key, ok := bundle.RelationKeyByApiSlug(input)
	if !ok || string(key) == matchedKey {
		return "", false
	}
	rel := bundle.MustGetRelation(key)
	return fmt.Sprintf("the bundled %q (key %s)", rel.Name, key), true
}

// shadowedBundledType is shadowedBundledProperty for the type namespace.
func shadowedBundledType(input, matchedKey string) (string, bool) {
	key, ok := bundle.TypeKeyByApiSlug(input)
	if !ok || string(key) == matchedKey {
		return "", false
	}
	return fmt.Sprintf("the bundled %q (key %s)", bundle.MustGetType(key).Name, key), true
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
func (s *Service) requireLiveProperty(spaceId, input string) (propertyEntry, error) {
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
func (s *Service) requireLiveType(spaceId, input, path string) (typeEntry, error) {
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
func (s *Service) propertySlugConflict(slug string, entries []propertyEntry) (slugHolder, bool) {
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
func (s *Service) typeSlugConflict(slug string, entries []typeEntry) (slugHolder, bool) {
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

// There is exactly ONE authority for the wire spelling of a key: servedKeyOf
// below. The rival propertyEntry.publicKey/typeEntry.publicKey pair used to
// live here — "the stored slug when the stored key is a BSON, the stored key
// otherwise" — with NONE of servedKeyOf's three round-trip guards, and dead
// repo-wide. Methods never trip an unused-symbol check, so it would have sat
// here until the next listing picked it up and re-opened the class of defect
// this file exists to close. Deleted deliberately: if a listing needs a wire
// spelling, it calls servedKey/servedTypeKeyOf. Its only helper,
// isBsonLikeKey, went with it — the BSON-or-not distinction was the rival
// rule's whole basis, and servedKeyOf does not make it.

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
// BSON as the only address. The transform itself lives in pkg/lib/bundle
// beside ApiSlug — the slug grammar is one thing, and the heart-side mint
// and the apiObjectKey backfill apply the same one.
func sanitizeApiSlug(raw string) string {
	return bundle.SanitizeApiSlug(raw, maxV2KeyLength)
}

// servedKeySets primes the two maps the served-spelling rule needs from one
// live set: every live stored key, and the stored keys HOLDING each slug
// (hidden holders don't participate in the slug namespace — a hidden twin
// must not downgrade the visible row's spelling). Holders, not a count:
// a bundled key's slug is DERIVED, so the round-trip test is "does anyone
// ELSE answer to this spelling", which a count of zero cannot express.
func servedPropertyKeySets(entries []propertyEntry) (keys map[string]bool, slugHolders map[string][]string) {
	keys = make(map[string]bool, len(entries))
	slugHolders = map[string][]string{}
	for _, entry := range entries {
		keys[entry.Key] = true
		if !entry.Hidden && entry.Slug != "" {
			slugHolders[entry.Slug] = append(slugHolders[entry.Slug], entry.Key)
		}
	}
	return keys, slugHolders
}

func servedTypeKeySets(entries []typeEntry) (keys map[string]bool, slugHolders map[string][]string) {
	keys = make(map[string]bool, len(entries))
	slugHolders = map[string][]string{}
	for _, entry := range entries {
		keys[entry.Key] = true
		if !entry.Hidden && entry.Slug != "" {
			slugHolders[entry.Slug] = append(slugHolders[entry.Slug], entry.Key)
		}
	}
	return keys, slugHolders
}

// servedKey is the wire spelling of one live entry's key under the §7.5a
// surface rule — **the slug, always**, from whichever authority owns it:
//
//   - a BUNDLED key spells as its DERIVED slug (`dueDate` → `due_date`),
//     because the table in code is that key's authority in every space and
//     offline (§7.5a-1); no stored detail is consulted or needed;
//   - every other key spells as its STORED slug (apiObjectKey), when it has
//     one. Pre-backfill entities have none and keep the stored key — the
//     honest degradation, not a second vocabulary.
//
// The round-trip guard is unchanged in spirit and sharper in fact: an
// address the API serves MUST resolve back to the row it labels. All THREE
// ways it can fail are checked, one per chain step — a live stored key wins
// the spelling at step 1; any OTHER live holder makes it ambiguous at step 2;
// and the bundled table resolving it to a different key is the §7.5a-6 shadow
// at step 3, which resolvePropertyInput refuses as ambiguous. The third was
// missing: with the bundled relation NOT installed, nothing in the space
// revealed the clash, so a listing advertised `due_date` for a squatter and
// the very next request to /properties/due_date 400'd.
func servedKey(storedKey, slug string, keyTaken map[string]bool, slugHolders map[string][]string) string {
	return servedKeyOf(storedKey, slug, keyTaken, slugHolders,
		bundle.HasRelation(domain.RelationKey(storedKey)), shadowedBundledProperty)
}

// servedTypeKeyOf is servedKey for the type namespace (its bundled tests are
// the type tables, not the relation ones).
func servedTypeKeyOf(storedKey, slug string, keyTaken map[string]bool, slugHolders map[string][]string) string {
	return servedKeyOf(storedKey, slug, keyTaken, slugHolders,
		bundle.HasObjectTypeByKey(domain.TypeKey(storedKey)), shadowedBundledType)
}

func servedKeyOf(storedKey, slug string, keyTaken map[string]bool, slugHolders map[string][]string, bundled bool, shadowed func(input, matchedKey string) (string, bool)) string {
	candidate := slug
	if bundled {
		candidate = bundle.ApiSlug(storedKey)
	}
	if candidate == "" || candidate == storedKey {
		return storedKey
	}
	if keyTaken[candidate] {
		return storedKey // a live stored key wins the spelling at chain step 1
	}
	for _, holder := range slugHolders[candidate] {
		if holder != storedKey {
			return storedKey // someone else answers to it — ambiguous, so honest
		}
	}
	if _, isShadow := shadowed(candidate, storedKey); isShadow {
		return storedKey // the bundled table answers to it — the input side refuses it
	}
	return candidate
}

// corpseFlagged reports whether an index row carries any lifecycle-exit
// flag: isUninstalled (UI delete), isArchived (v2 DELETE — §8.41 made it
// refuse writes like an uninstall) or isDeleted (the re-derived local flag a
// prod corpse always carries).
func corpseFlagged(details *domain.Details) bool {
	return details.GetBool(bundle.RelationKeyIsUninstalled) ||
		details.GetBool(bundle.RelationKeyIsArchived) ||
		details.GetBool(bundle.RelationKeyIsDeleted)
}

// relationObjectHoldingKey returns the ID of the relation object holding the
// stored key — live, corpse or tombstoned — and is the one probe that
// deliberately sees EVERY store shape a relation row can have.
//
// A corpse has THREE store shapes, not two (§8.41). A UI delete sets
// isUninstalled, the same Apply stamps isDeleted (smartblock's
// detailsinject, since GO-1978), and BeforeDelete then TOMBSTONES the index
// row down to {id, spaceId, isDeleted} — no relationKey, no resolvedLayout —
// until the next space load re-indexes the surviving tree with full details
// and both flags. So:
//
//   - the query below (both injected defaults suppressed via the no-op
//     Condition None clauses) sees the full-detail shapes, flag-only and
//     two-flag alike;
//   - the tombstone, which no key-filtered query can return, is found by its
//     ID instead: a derived object's id is a pure function of (space, kind,
//     internal key) — ADDRESSING §2.4, verified for every relation creation
//     path — so RelationIdByKey computes where the row MUST be and a point
//     lookup answers whether it is there. The tree the tombstone stands for
//     still exists; the row's absence of fields is a window, not a fact.
//
// When several rows hold one key (a live relation beside a corpse), the LIVE
// row wins — callers reach this probe after the live resolution chain
// misses, but that ordering is a convention of today's call sites, not a
// contract this probe may lean on. Ties break by id for determinism.
//
// The error return is load-bearing: PropertyId's mint decision consults this
// probe, and a probe that swallowed a store or derivation error would turn
// "could not look" into "not held" and mint a duplicate of a property that
// exists (§7.5a-2's fail-closed rule).
func (s *Service) relationObjectHoldingKey(ctx context.Context, spaceId, key string) (string, bool, error) {
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyRelationKey,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.String(key),
			},
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_relation)),
			},
			{
				RelationKey: bundle.RelationKeyIsArchived,
				Condition:   model.BlockContentDataviewFilter_None,
			},
			{
				RelationKey: bundle.RelationKeyIsDeleted,
				Condition:   model.BlockContentDataviewFilter_None,
			},
		},
	})
	if err != nil {
		return "", false, fmt.Errorf("query relation objects holding %q in space %s: %w", key, spaceId, err)
	}
	best := ""
	bestLive := false
	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyId)
		if id == "" {
			continue
		}
		live := !corpseFlagged(record.Details)
		switch {
		case best == "",
			live && !bestLive,
			live == bestLive && id < best:
			best, bestLive = id, live
		}
	}
	if best != "" {
		return best, true, nil
	}
	// tombstone window: the row may exist with nothing but {id, isDeleted}
	details, id, err := s.derivedRelationRow(ctx, spaceId, key)
	if err != nil {
		return "", false, err
	}
	if details != nil && details.GetString(bundle.RelationKeyId) != "" {
		return id, true, nil
	}
	return "", false, nil
}

// derivedRelationRow computes the id the relation object for `key` MUST have
// in this space (derived identity, ADDRESSING §2.4) and point-looks-up its
// index row, bypassing every injected default. A nil details return means
// "no row at all" — the relation was never installed here. Requires the
// creator port (id derivation runs in the space); a read-only service has
// none and reports no row, which the write paths that consult this never
// reach.
func (s *Service) derivedRelationRow(ctx context.Context, spaceId, key string) (*domain.Details, string, error) {
	if s.creator == nil {
		return nil, "", nil
	}
	id, err := s.creator.RelationIdByKey(ctx, spaceId, domain.RelationKey(key))
	if err != nil {
		return nil, "", fmt.Errorf("derive relation id for %q in space %s: %w", key, spaceId, err)
	}
	details, err := s.store.SpaceIndex(spaceId).GetDetails(id)
	if err != nil {
		return nil, "", fmt.Errorf("read relation row %s in space %s: %w", id, spaceId, err)
	}
	return details, id, nil
}

// propertyKeyHeldByAnyRelation reports whether ANY relation object holds the
// stored key — live, corpse or tombstoned. This is the create path's
// round-trip tolerance (§8.29); it is never an ADDRESS — nothing resolves a
// corpse key to a property object, and no listing advertises it.
//
// The tolerance is DELIBERATELY a bare existence probe. It cannot tell a
// pasted read body (the clone loop it was written for) from a fresh value
// authored onto the corpse key of a brand-new object: the API has no
// provenance signal, and the §8.41 review settled that none is worth
// building — a custom corpse's stored key is a BSON id that resolves
// nowhere, so the worst a fresh value can do is join the dormant freight the
// clone loop already carries. Rows with only isDeleted set (no
// isUninstalled/isArchived) pass too, and that is intended: whatever exotic
// path exited the relation, a document value under its stored key is the
// same inert freight. On a probe error the key reads as not held and create
// refuses — fail closed, never a silent mint of presence.
func (s *Service) propertyKeyHeldByAnyRelation(ctx context.Context, spaceId, key string) bool {
	_, held, err := s.relationObjectHoldingKey(ctx, spaceId, key)
	return err == nil && held
}

// bundledRemovalSet is the set of BUNDLED relation keys this space has
// REMOVED: a relation object exists and carries isUninstalled (UI delete) or
// isArchived (v2 DELETE — §8.41: the API's own delete verb must not leave a
// property that 404s on its route yet accepts writes). One bounded query per
// request (§7.5a-2), never one per reference; both injected defaults are
// suppressed so every full-detail corpse shape is seen.
//
// The set is deliberately narrow. A bundled relation that was NEVER
// installed has no object at all and is absent here — install-on-write stays
// correct, and is the common case in a fresh space. Only removals land in
// this set, which is the whole distinction the refusal rests on: "not
// installed yet" and "you deleted it" look identical through
// bundle.HasRelation and could not be told apart without this probe. The
// TOMBSTONE shape is invisible to this query too (no relationKey field) —
// per-key consultation goes through bundledPropertyRemoved, which adds the
// derived-id probe for that window.
func (s *Service) bundledRemovalSet(spaceId string) (map[string]bool, error) {
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_relation)),
			},
			{RelationKey: bundle.RelationKeyIsArchived, Condition: model.BlockContentDataviewFilter_None},
			{RelationKey: bundle.RelationKeyIsDeleted, Condition: model.BlockContentDataviewFilter_None},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query removed relations of space %s: %w", spaceId, err)
	}
	removed := map[string]bool{}
	for _, record := range records {
		if !corpseFlagged(record.Details) {
			continue
		}
		key := record.Details.GetString(bundle.RelationKeyRelationKey)
		// custom corpses are NOT in this set: their stored key is a BSON id
		// no bundled table knows, they can never be reinstalled, and the
		// §8.29 tolerance carries their in-document values instead
		if key != "" && bundle.HasRelation(domain.RelationKey(key)) {
			removed[key] = true
		}
	}
	return removed, nil
}

// bundledPropertyRemoved is the per-key removal verdict for a BUNDLED
// property key: the space explicitly removed it (bundledRemovalSet), or its
// relation object sits in the post-delete tombstone window — a row at the
// derived id carrying isDeleted and no relationKey, which no query-built set
// can contain. A live installed entry always outvotes; a missing row means
// never-installed and keeps install-on-write working.
func (s *Service) bundledPropertyRemoved(ctx context.Context, spaceId string, entries []propertyEntry, removed map[string]bool, key string) (bool, error) {
	if propertyKeyRemovedIn(entries, removed, key) {
		return true, nil
	}
	if !bundle.HasRelation(domain.RelationKey(key)) || propertyKeyInstalledIn(entries, key) {
		return false, nil
	}
	details, _, err := s.derivedRelationRow(ctx, spaceId, key)
	if err != nil {
		return false, err
	}
	if details == nil || details.GetString(bundle.RelationKeyId) == "" {
		return false, nil // no row: never installed
	}
	if _, hasKey := details.TryString(bundle.RelationKeyRelationKey); hasKey {
		return false, nil // a full-detail row belongs to the query-built sets
	}
	return details.GetBool(bundle.RelationKeyIsDeleted), nil
}

// bundledTypeRemovalSet is bundledRemovalSet for the TYPE namespace: bundled
// type keys whose type object exists and carries a removal flag. Same
// query discipline, same never-installed boundary, same tombstone blind spot
// (bundledTypeRemoved owns that window).
func (s *Service) bundledTypeRemovalSet(spaceId string) (map[string]bool, error) {
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(model.ObjectType_objectType)),
			},
			{RelationKey: bundle.RelationKeyIsArchived, Condition: model.BlockContentDataviewFilter_None},
			{RelationKey: bundle.RelationKeyIsDeleted, Condition: model.BlockContentDataviewFilter_None},
		},
	})
	if err != nil {
		return nil, fmt.Errorf("query removed types of space %s: %w", spaceId, err)
	}
	removed := map[string]bool{}
	for _, record := range records {
		if !corpseFlagged(record.Details) {
			continue
		}
		key, err := domain.GetTypeKeyFromRawUniqueKey(record.Details.GetString(bundle.RelationKeyUniqueKey))
		if err != nil {
			continue
		}
		if bundle.HasObjectTypeByKey(key) {
			removed[string(key)] = true
		}
	}
	return removed, nil
}

// typeKeyInstalledIn is propertyKeyInstalledIn for the type namespace.
func typeKeyInstalledIn(entries []typeEntry, key string) bool {
	for _, entry := range entries {
		if entry.Key == key {
			return true
		}
	}
	return false
}

// bundledTypeRemoved is bundledPropertyRemoved for the type namespace.
func (s *Service) bundledTypeRemoved(ctx context.Context, spaceId string, entries []typeEntry, removed map[string]bool, key string) (bool, error) {
	if removed[key] && !typeKeyInstalledIn(entries, key) {
		return true, nil
	}
	if !bundle.HasObjectTypeByKey(domain.TypeKey(key)) || typeKeyInstalledIn(entries, key) {
		return false, nil
	}
	if s.creator == nil {
		return false, nil
	}
	id, err := s.creator.TypeIdByKey(ctx, spaceId, domain.TypeKey(key))
	if err != nil {
		return false, fmt.Errorf("derive type id for %q in space %s: %w", key, spaceId, err)
	}
	details, err := s.store.SpaceIndex(spaceId).GetDetails(id)
	if err != nil {
		return false, fmt.Errorf("read type row %s in space %s: %w", id, spaceId, err)
	}
	if details.GetString(bundle.RelationKeyId) == "" {
		return false, nil // no row: never installed
	}
	if _, hasKey := details.TryString(bundle.RelationKeyUniqueKey); hasKey {
		return false, nil // a full-detail row belongs to the query-built set
	}
	return details.GetBool(bundle.RelationKeyIsDeleted), nil
}

// canonicalizeDocumentKeys rewrites an inbound document's addressing terms
// to their canonical stored spellings BEFORE validation and import: the
// envelope's type/templateFor (slug → internal type key — the import path
// derives `ot-<key>` URLs from them) and the properties-map keys (slug →
// stored relation key — they become detail keys verbatim). Terms already
// canonical, or resolving to nothing (the R9 validation owns that refusal),
// pass through verbatim so errors keep the caller's spelling. Ambiguity is
// a path-addressed 400; two spellings canonicalizing onto one key is too.
//
// The second return maps every REWRITTEN property key back to the spelling
// the caller sent (canonical → original), so validation that runs after the
// rewrite can address its refusals to the request as sent (§8.41-10).
func (s *Service) canonicalizeDocumentKeys(spaceId string, body []byte) ([]byte, map[string]string, error) {
	spellings := map[string]string{}
	fields, err := parseEnvelope(body)
	if err != nil {
		return body, spellings, nil // not an object — the document validator owns this
	}
	changed := false

	var typeEntries []typeEntry
	for _, field := range []string{"type", "template_for"} {
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
				return nil, nil, err
			}
		}
		entry, ok, ambiguous := s.resolveTypeInput(term, typeEntries)
		if len(ambiguous) > 0 {
			return nil, nil, ambiguousKeyError("type key", term, "/"+field, ambiguous)
		}
		if ok && entry.Key != term {
			if fields[field], err = rawJSON(entry.Key); err != nil {
				return nil, nil, err
			}
			changed = true
		}
	}

	if raw, ok := fields["properties"]; ok {
		var props map[string]json.RawMessage
		if err := json.Unmarshal(raw, &props); err == nil && len(props) > 0 {
			propEntries, err := s.liveProperties(spaceId)
			if err != nil {
				return nil, nil, err
			}
			renames := map[string]string{}
			for _, key := range sortedKeys(props) {
				entry, ok, ambiguous := s.resolvePropertyInput(key, propEntries)
				if len(ambiguous) > 0 {
					return nil, nil, ambiguousKeyError("property key", key, "/properties/"+key, ambiguous)
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
						spellings[canonical] = key
					}
					if _, dup := rewritten[canonical]; dup {
						return nil, nil, v2model.ValidationFailed("duplicate property key",
							v2model.Issue{Path: "/properties/" + key,
								Message: fmt.Sprintf("%q and another spelling both address property %q — keep one", key, canonical)})
					}
					rewritten[canonical] = props[key]
				}
				if fields["properties"], err = rawJSON(rewritten); err != nil {
					return nil, nil, err
				}
				changed = true
			}
		}
	}

	if !changed {
		return body, spellings, nil
	}
	body, err = encodeEnvelope(fields)
	return body, spellings, err
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
