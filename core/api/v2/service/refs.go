package v2service

// refs.go is the referential validation layer: on
// create, type and property references are validated against the space and
// rejected with path-addressed did-you-mean errors listing the actual keys —
// the schema-linking hallucination guard. Create-missing surfaces (select
// option names, typeProperties) are exempt by design; see resolver.go for
// the full policy table.

import (
	"context"
	"fmt"
	"net/http"
	"slices"
	"sort"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
)

// maxListedKeys bounds how many actual keys an error message names.
const maxListedKeys = 15

//
// ---- the error vocabulary (?keys — APIV2_VOCABULARY.md §4.3) ----
//
// A referential refusal speaks the requesting surface's vocabulary: the
// known-key lists and did-you-mean suggestions a caller is told to retry
// with must be spellings that caller was taught. The slug default is the
// zero value, so every existing caller, client and test reads the exact
// bytes it always has; name mode exists only for a request that asked
// `?keys=name` — the same switch that already selects the served body
// vocabulary, extended to the errors because a repair hint in a vocabulary
// the caller never sees is unactionable (the tool wrapper is that caller).

// errKeys carries the vocabulary referential errors spell keys in.
type errKeys struct{ names bool }

// errKeysFor reads the request's ?keys choice (keyshape.go).
func errKeysFor(ctx context.Context) errKeys {
	return errKeys{names: nameKeysRequested(ctx)}
}

// propertiesWord / typesWord label a key LIST ("known <word>: ...").
func (v errKeys) propertiesWord() string {
	if v.names {
		return "properties"
	}
	return "property keys"
}

func (v errKeys) typesWord() string {
	if v.names {
		return "types"
	}
	return "type keys"
}

// propertyWord / typeWord label a single reference ("unknown <word> %q").
func (v errKeys) propertyWord() string {
	if v.names {
		return "property"
	}
	return "property key"
}

func (v errKeys) typeWord() string {
	if v.names {
		return "type"
	}
	return "type key"
}

// spell picks one entry's spelling: the display name in name mode (the
// served key standing in for an unnamed entry — an unnamed entry has no
// other spelling), the served key otherwise.
func (v errKeys) spell(served, name string) string {
	if v.names && name != "" {
		return name
	}
	return served
}

// typeIdInSpace resolves a type key to its object id when the type object
// exists in the space; ok is false otherwise.
func (s *Service) typeIdInSpace(spaceId, typeKey string) (string, bool) {
	uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, typeKey)
	if err != nil {
		return "", false
	}
	details, err := s.store.SpaceIndex(spaceId).GetObjectByUniqueKey(uk)
	if err != nil {
		return "", false
	}
	id := details.GetString(bundle.RelationKeyId)
	return id, id != ""
}

// typeKeyExists reports whether a type key resolves LIVE in the space or in
// the bundle (bundled types install on first use — the create adapter does
// it). Corpse-aware and chain-aware: a UI-deleted type must not gate object
// or template creates through (review cause 2), and the served slug must
// resolve here like everywhere else. Fail closed on a load error.
func (s *Service) typeKeyExists(spaceId, typeKey string) bool {
	entries, err := s.liveTypes(spaceId)
	if err != nil {
		return false
	}
	_, ok, ambiguous := s.resolveTypeInput(spaceId, typeKey, entries)
	return ok && len(ambiguous) == 0
}

// knownTypeKeys lists the space's LIVE type keys in their SERVED spelling
// (the address the input chain resolves right back) — a corpse must never
// be suggested as a remedy (§7.5-2), and a candidate list must never
// advertise a spelling the routes reject (review cause 3). Hint-only, so a
// load error degrades to an empty list rather than failing the request.
func (s *Service) knownTypeKeys(spaceId string, v errKeys) []string {
	entries, err := s.liveTypes(spaceId)
	if err != nil {
		return nil
	}
	keyTaken, slugHolders := servedTypeKeySets(entries)
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, v.spell(servedTypeKeyOf(entry.Key, entry.Slug, keyTaken, slugHolders), entry.Name))
	}
	return sortedDistinct(keys)
}

// unknownTypeKeyError is the R9 did-you-mean 400 for a type reference.
func (s *Service) unknownTypeKeyError(spaceId, typeKey, path string, v errKeys) error {
	// the spelling a read served for a REMOVED space-minted type (its
	// objects keep it — R4-1): say removed, not unknown with a guess
	if issue, removed := s.removedTypeRefusal(spaceId, typeKey, path, v); removed {
		return v2model.ValidationFailed(fmt.Sprintf("removed %s", v.typeWord()), issue)
	}
	known := s.knownTypeKeys(spaceId, v)
	return v2model.ValidationFailed(
		fmt.Sprintf("type %q not found in space %q", typeKey, spaceId),
		v2model.Issue{
			Path:    path,
			Message: fmt.Sprintf("unknown %s %q — %s", v.typeWord(), typeKey, listKnown(v.typesWord(), known)),
		}.WithHint(didYouMean(typeKey, known, v2model.Hintf("list all with %s", v2model.RefListTypes(spaceId)))))
}

// typeNotFoundError is the 404 for a type-KEY lookup miss on the routes that
// address a type directly (GET/PATCH/DELETE types/{type}). It lists the
// space's actual keys plus the nearest match — the same steering the R9
// create path gives (unknownTypeKeyError): a candidate-less tip is a dead
// end for a small model (§8.21 — given the bare "not found" text, a
// benchmarked 4B did not retry at all, while the key-listing property tip
// repaired on the first retry in the same run).
func (s *Service) typeNotFoundError(spaceId, typeKey string, v errKeys) error {
	// a type route addressed by the spelling its objects still serve: 404
	// still (the type is not addressable), but saying why (R4-1)
	if issue, removed := s.removedTypeRefusal(spaceId, typeKey, "type", v); removed {
		// the diagnosis once, in the message; the issue carries the
		// consequences and the live-type reference
		known := s.knownTypeKeys(spaceId, v)
		issue.Message = "existing objects keep this type; creating objects with it and filtering by it are unavailable"
		return v2model.NewError(http.StatusNotFound, v2model.CodeNotFound,
			fmt.Sprintf("type %q not found in space %q — it was removed; %s", typeKey, spaceId, listKnown(v.typesWord(), known)), issue)
	}
	return notFoundWithKeys(
		fmt.Sprintf("type %q not found in space %q", typeKey, spaceId),
		"type", typeKey, v.typesWord(), s.knownTypeKeys(spaceId, v),
		v2model.Hintf("list all with %s", v2model.RefListTypes(spaceId)))
}

// propertyNotFoundError is typeNotFoundError's sibling for property-KEY
// routes (options listing, PATCH/DELETE properties/{key}).
func (s *Service) propertyNotFoundError(spaceId, propertyKey string, v errKeys) error {
	return notFoundWithKeys(
		fmt.Sprintf("property %q not found in space %q", propertyKey, spaceId),
		"key", propertyKey, v.propertiesWord(), s.knownPropertyKeys(spaceId, v),
		v2model.Hintf("%s lists user-visible properties only; hidden addressable properties are excluded and contribute to the total above", v2model.RefListProperties(spaceId)))
}

// notFoundWithKeys composes a 404 that is repairable from the error alone:
// subject, the known keys (capped), and a did-you-mean when a close key
// exists. The list operation rides along, as an issue on the path parameter
// `param`, only when the key list was truncated and no suggestion fired —
// the one case where the message alone cannot show every candidate.
func notFoundWithKeys(subject, param, input, what string, known []string, list v2model.Hint) error {
	msg := subject + " — " + listKnown(what, known)
	if hint := didYouMean(input, known, v2model.Hint{}); hint.Text != "" {
		return v2model.NotFound(msg + "; " + hint.Text)
	}
	if len(known) > maxListedKeys {
		return v2model.NotFound(msg,
			v2model.Issue{Path: param, Message: "not among the " + what + " listed"}.WithHint(list))
	}
	return v2model.NotFound(msg)
}

// propertyKeyExistsIn reports whether a property key resolves in a primed
// live set or the bundle. LIVE relations only (§7.5-2 corpse policy): a
// UI-deleted property must reject with did-you-mean, not resolve as a
// corpse. The primed form is the loop shape (§7.5a-2: one bounded query
// per request, never one per reference).
func propertyKeyExistsIn(entries []propertyEntry, key string) bool {
	return propertyKeyInstalledIn(entries, key) || bundle.HasRelation(domain.RelationKey(key))
}

// servedPropertyEntry finds the live property whose SERVED spelling is `key` —
// the api key a caller reads back from GET /properties and addresses a
// property by everywhere else on this surface.
//
// propertyKeyExistsIn is not enough on its own: it matches the STORED key, and
// a space-minted property stores a bson id while serving a slug. So a caller
// who sent the only spelling the API ever showed them was told the key was
// unknown — by an error that then listed that same slug among the known keys
// and suggested it back, because knownPropertyKeysIn lists served spellings.
// Matching here on the served spelling is what makes the check and the error
// speak one vocabulary.
//
// Hidden entries are skipped for the same reason they are invisible to the
// slug namespace everywhere else (propertyEntry.Hidden).
func servedPropertyEntry(entries []propertyEntry, key string) (propertyEntry, bool) {
	keyTaken, slugHolders := servedPropertyKeySets(entries)
	for _, entry := range entries {
		if entry.Hidden {
			continue
		}
		if servedKey(entry.Key, entry.Slug, keyTaken, slugHolders) == key {
			return entry, true
		}
	}
	return propertyEntry{}, false
}

// propertyKeyInstalledIn is the live-entry half of propertyKeyExistsIn: the
// key belongs to a relation object this space actually has. Split out
// because the BUNDLED half answers for keys with no object at all, and the
// two halves need different corpse verdicts (propertyKeyRemovedIn).
func propertyKeyInstalledIn(entries []propertyEntry, key string) bool {
	for _, entry := range entries {
		if entry.Key == key {
			return true
		}
	}
	return false
}

// propertyKeyRemovedIn reports whether a key that propertyKeyExistsIn just
// waved through only exists because of the BUNDLED table, while this space
// has explicitly uninstalled that bundled relation — the user deleted it.
// `removed` comes from uninstalledBundledKeys and holds bundled keys only,
// so a live entry is the sole thing that can outvote it.
func propertyKeyRemovedIn(entries []propertyEntry, removed map[string]bool, key string) bool {
	return removed[key] && !propertyKeyInstalledIn(entries, key)
}

// removedPropertyIssue refuses a write that names a bundled property the
// space removed. Asymmetric with the §8.29 corpse tolerance ON PURPOSE, and
// the asymmetry is in the entities, not the policy: a custom corpse's stored
// key is a BSON id that can never be reinstalled or re-derived, so a
// document value on it is inert freight the tolerance carries; a bundled
// corpse's key is reinstallable, so a value landing there resurrects into a
// property the user deleted the moment it comes back.
//
// The refusal names the repair that actually works, per §8.34 — and for the
// clone loop (§8.41: GET serves the key, POST of those bytes lands here) the
// working repair is REMOVING the key from the body, so the hint leads with
// it. `spelledAs` is the caller's own spelling — canonicalization runs
// before validation on these channels, and a hint naming a spelling the
// request never contained is unactionable; the message keeps the served
// slug, the one spelling every listing agrees on.
// removedCustomProperty finds the REMOVED space-minted property a key names
// — by the slug the surface serves for it or by its stored key — so a write
// to it can be refused as removed rather than as unknown (the served
// spelling must be understood back: a caller who read `gamma` off an object
// and writes `gamma` is told what happened to it). One bounded query, and
// only on the unknown-key path, which is rare.
func (s *Service) removedCustomProperty(spaceId, key string) (propertyEntry, bool) {
	removed, err := s.removedProperties(spaceId)
	if err != nil {
		return propertyEntry{}, false
	}
	for _, e := range removed {
		if e.Key == key || (e.Slug != "" && e.Slug == key) {
			return e, true
		}
	}
	return propertyEntry{}, false
}

// removedCustomPropertyIssue is removedPropertyIssue for a space-minted
// property: the same repair, with the spelling the surface serves for it.
func removedCustomPropertyIssue(spaceId string, entry propertyEntry, spelledAs, path string, v errKeys) v2model.Issue {
	spelling := entry.Slug
	if spelling == "" {
		spelling = entry.Key
	}
	if v.names && entry.Name != "" {
		spelling = entry.Name
	}
	return v2model.Issue{
		Path:    path,
		Message: fmt.Sprintf("property %q was removed from this space — set_properties gives no object that does not already hold a value of it one", spelling),
	}.Hintf("remove %q from the request — values objects already hold stay readable, and reappear if the property is restored; for a different property, list them with %s",
		spelledAs, v2model.RefListProperties(spaceId))
}

func removedPropertyIssue(spaceId, key, spelledAs, path string, v errKeys) v2model.Issue {
	slug := bundle.ApiSlug(key)
	spelling := slug
	if v.names {
		// a removed bundled property still has its bundled display name —
		// the spelling a name-mode caller was taught
		if rel, err := bundle.GetRelation(domain.RelationKey(key)); err == nil && rel.Name != "" {
			spelling = rel.Name
		}
	}
	return v2model.Issue{
		Path:    path,
		Message: fmt.Sprintf("property %q was removed from this space — set_properties gives no object that does not already hold a value of it one", spelling),
	}.Hintf("remove %q from the request — values objects already hold stay readable, and reappear if the property is restored; for a different property, list them with %s",
		spelledAs, v2model.RefListProperties(spaceId))
}

// removedTypeIssue is removedPropertyIssue for the TYPE namespace (§8.41):
// an uninstalled or archived bundled type stays a resolvable key through the
// bundled table forever, so without this a create landed a new object in a
// type the user deleted — GET /types/{key} 404ing beside it — and a
// reinstall lit the type back up with the new object already in it. The
// repair differs from the property one: a create cannot simply drop its
// type, so the hint steers to the live type list.
func removedTypeIssue(spaceId, key, path string, v errKeys) v2model.Issue {
	slug := bundle.TypeApiSlug(key)
	spelling := slug
	if v.names {
		if t, err := bundle.GetType(domain.TypeKey(key)); err == nil && t.Name != "" {
			spelling = t.Name
		}
	}
	return v2model.Issue{
		Path:    path,
		Message: fmt.Sprintf("type %q was removed from this space — nothing new is created in a removed type", spelling),
	}.Hintf("use a live type instead — list them with %s", v2model.RefListTypes(spaceId))
}

// propertyKeyExists is the single-lookup form; loops prime entries once and
// use propertyKeyExistsIn.
func (s *Service) propertyKeyExists(spaceId, key string) bool {
	entries, err := s.liveProperties(spaceId)
	if err != nil {
		return false // fail closed: an unverifiable key is not "known good"
	}
	return propertyKeyExistsIn(entries, key)
}

// knownPropertyKeys lists the space's LIVE property keys in their SERVED
// spelling (see knownTypeKeys) — corpse-free, load-error-tolerant
// (hint-only).
func (s *Service) knownPropertyKeys(spaceId string, v errKeys) []string {
	entries, err := s.liveProperties(spaceId)
	if err != nil {
		return nil
	}
	return knownPropertyKeysIn(entries, v)
}

// knownPropertyKeysIn is knownPropertyKeys over a primed set.
func knownPropertyKeysIn(entries []propertyEntry, v errKeys) []string {
	keyTaken, slugHolders := servedPropertyKeySets(entries)
	keys := make([]string, 0, len(entries))
	for _, entry := range entries {
		keys = append(keys, v.spell(servedKey(entry.Key, entry.Slug, keyTaken, slugHolders), entry.Name))
	}
	return sortedDistinct(keys)
}

// unknownPropertyIssue builds one path-addressed did-you-mean issue for an
// unknown property key.
func unknownPropertyIssue(key, path string, known []string, list v2model.Hint, v errKeys) v2model.Issue {
	return v2model.Issue{
		Path:    path,
		Message: fmt.Sprintf("unknown %s %q — %s", v.propertyWord(), key, listKnown(v.propertiesWord(), known)),
	}.WithHint(didYouMean(key, known, list))
}

// propertyListHint is the repair every unknown-property refusal ends on:
// list the space's properties, or create the missing one.
func propertyListHint(spaceId string) v2model.Hint {
	return v2model.Hintf("list all with %s, or create it with %s",
		v2model.RefListProperties(spaceId), v2model.RefCreateProperty(spaceId))
}

// listKnown renders "known <what>: a, b, c…" capped at maxListedKeys.
func listKnown(what string, known []string) string {
	if len(known) == 0 {
		return "the space has no " + what + " yet"
	}
	listed := known
	suffix := ""
	if len(listed) > maxListedKeys {
		suffix = fmt.Sprintf(", … (%d total)", len(known))
		listed = listed[:maxListedKeys]
	}
	return "known " + what + ": " + strings.Join(listed, ", ") + suffix
}

// didYouMean picks the closest known keys for the hint; fallback steers to
// the discovery list, and rides along behind the guess — a guess is a
// question, and the caller whose answer is "no" needs the list as much as
// one who got no guess at all (round-two eval F6: the did-you-mean branch
// dropped the reference the list-all branch carried).
func didYouMean(input string, known []string, fallback v2model.Hint) v2model.Hint {
	suggestions := closestKeys(input, known, 3)
	if len(suggestions) == 0 {
		return fallback
	}
	guess := "did you mean " + strings.Join(suggestions, ", ") + "?"
	if fallback.Text == "" {
		return v2model.Plain(guess)
	}
	return v2model.Hint{Text: guess + " — if not, " + fallback.Text, Refs: fallback.Refs}
}

// closestKeys ranks known keys by simple similarity to input:
// case-insensitive equality, then prefix, then substring containment either
// way, then small edit distance (typos). Deterministic (rank, then
// alphabetical).
func closestKeys(input string, known []string, max int) []string {
	in := strings.ToLower(input)
	type scored struct {
		key  string
		rank int
	}
	var out []scored
	for _, k := range known {
		lk := strings.ToLower(k)
		switch {
		case lk == in:
			out = append(out, scored{k, 0})
		case strings.HasPrefix(lk, in) || strings.HasPrefix(in, lk):
			out = append(out, scored{k, 1})
		case strings.Contains(lk, in) || strings.Contains(in, lk):
			out = append(out, scored{k, 2})
		case editDistanceAtMost(lk, in, 2):
			out = append(out, scored{k, 3})
		}
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].rank != out[j].rank {
			return out[i].rank < out[j].rank
		}
		return out[i].key < out[j].key
	})
	if len(out) > max {
		out = out[:max]
	}
	keys := make([]string, len(out))
	for i, sc := range out {
		keys[i] = sc.key
	}
	return keys
}

// editDistanceAtMost reports whether the Levenshtein distance between a and
// b is ≤ bound. Inputs longer than 64 runes never match (cost guard; keys
// are short).
func editDistanceAtMost(a, b string, bound int) bool {
	ra, rb := []rune(a), []rune(b)
	if len(ra) > 64 || len(rb) > 64 {
		return false
	}
	if abs(len(ra)-len(rb)) > bound {
		return false
	}
	prev := make([]int, len(rb)+1)
	curr := make([]int, len(rb)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(ra); i++ {
		curr[0] = i
		for j := 1; j <= len(rb); j++ {
			cost := 1
			if ra[i-1] == rb[j-1] {
				cost = 0
			}
			curr[j] = minInt(prev[j]+1, curr[j-1]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(rb)] <= bound
}

func abs(v int) int {
	if v < 0 {
		return -v
	}
	return v
}

func minInt(values ...int) int {
	out := values[0]
	for _, v := range values[1:] {
		if v < out {
			out = v
		}
	}
	return out
}

// typeRecommendedListKeys are the four recommended-relation lists a type
// object carries — the §2a typeProperties storage.
var typeRecommendedListKeys = []domain.RelationKey{
	bundle.RelationKeyRecommendedFeaturedRelations,
	bundle.RelationKeyRecommendedRelations,
	bundle.RelationKeyRecommendedFileRelations,
	bundle.RelationKeyRecommendedHiddenRelations,
}

// typePropertyKeys collects the property keys a type actually recommends
// (its four recommended-relation lists, resolved id→key) — the R9 reference
// set for set filters and sorts.
func (s *Service) typePropertyKeys(spaceId, typeId string) []string {
	details, err := s.store.SpaceIndex(spaceId).GetDetails(typeId)
	if err != nil {
		return nil
	}
	reads := storeresolver.New(s.store.SpaceIndex(spaceId))
	var out []string
	seen := map[string]bool{}
	for _, listKey := range typeRecommendedListKeys {
		for _, id := range details.GetStringList(listKey) {
			def, ok := reads.PropertyById(id)
			if !ok || def.Key == "" || seen[string(def.Key)] {
				continue
			}
			seen[string(def.Key)] = true
			out = append(out, string(def.Key))
		}
	}
	sort.Strings(out)
	return out
}

// recommendedRelationIds reads the relation object ids a type's four
// recommended lists carry RIGHT NOW — the echo baseline for the
// typeProperties removal gate (creatingResolvers.echoPropertyIds): an entry
// already referenced resolves as an identity even when its relation is
// removed, because refusing the type's own read back would force-delete the
// reference (§8.34/§8.41). Hint-grade lookup: an unreadable type yields an
// empty baseline, and the write path then refuses rather than echoes —
// fail closed.
func (s *Service) recommendedRelationIds(spaceId, typeId string) map[string]bool {
	details, err := s.store.SpaceIndex(spaceId).GetDetails(typeId)
	if err != nil {
		return nil
	}
	out := map[string]bool{}
	for _, listKey := range typeRecommendedListKeys {
		for _, id := range details.GetStringList(listKey) {
			out[id] = true
		}
	}
	return out
}

// typeListedKeys is the stored key set a type recommends, resolved from the
// key a document spells the type by — a bundled key (ot-page), or the api
// slug this surface serves for a space-minted type. Nil when the type cannot
// be resolved: then no F16 warning is issued, because a warning the code
// cannot substantiate is worse than none.
func (s *Service) typeListedKeys(spaceId, typeKey string) map[string]bool {
	if typeKey == "" {
		return nil
	}
	typeId, ok := s.typeIdInSpace(spaceId, typeKey)
	if !ok {
		entries, err := s.liveTypes(spaceId)
		if err != nil {
			return nil
		}
		entry, found, ambiguous := s.resolveTypeInput(spaceId, typeKey, entries)
		if !found || len(ambiguous) > 0 || entry.Id == "" {
			return nil
		}
		typeId = entry.Id
	}
	keys := map[string]bool{}
	for _, key := range s.typePropertyKeys(spaceId, typeId) {
		keys[key] = true
	}
	return keys
}

// offTypeCandidate reports whether a stored property key is one the F16
// warning applies to: not a key every type's queries accept (name and the
// system query keys), and not a hidden bundled relation (icon, layout and
// the like — system fields, never listed on a type).
func offTypeCandidate(key string) bool {
	if key == bundle.RelationKeyName.String() || slices.Contains(v2SystemQueryKeys, key) {
		return false
	}
	if rel, err := bundle.GetRelation(domain.RelationKey(key)); err == nil && rel.Hidden {
		return false
	}
	return true
}

// offTypePropertyIssue is the F16 warning: the value lands, but the type's
// own surfaces do not reach it.
func offTypePropertyIssue(spelling, typeKey, path string) v2model.Issue {
	return v2model.Issue{
		Path:    path,
		Message: fmt.Sprintf("property %q is not on type %q — the value is stored and served on the object, but type-scoped search and queries over %q refuse the key and the type's default columns omit it", spelling, typeKey, typeKey),
	}.Hintf("list it on the type with the add_property op (%s)", v2model.RefGetOpSchema("add_property"))
}

// removedTypeRefusal is the issue for a REMOVED space-minted type addressed
// by the spelling a read served for it. When a live type has since taken
// the removed one's slug, the caller addressed the old type by its stored
// key, and the refusal must not call the live slug removed.
func (s *Service) removedTypeRefusal(spaceId, input, path string, v errKeys) (v2model.Issue, bool) {
	entry, removed, err := s.removedTypeBySpelling(spaceId, input)
	if err != nil || !removed {
		return v2model.Issue{}, false
	}
	spelling := entry.Slug
	if spelling == "" {
		spelling = entry.Key
	}
	if v.names && entry.Name != "" {
		spelling = entry.Name
	}
	if entry.Slug != "" && input != entry.Slug {
		// a live type OWNS the removed one's slug only when it is the one
		// visible holder and serves it unchanged (servedTypeKeyOf's guards)
		if live, err := s.liveTypes(spaceId); err == nil {
			keyTaken, holders := servedTypeKeySets(live)
			if owners := holders[entry.Slug]; len(owners) == 1 && servedTypeKeyOf(owners[0], entry.Slug, keyTaken, holders) == entry.Slug {
				return v2model.Issue{
					Path:    path,
					Message: fmt.Sprintf("type %q (formerly %q) was removed from this space, and %q now names a different type — the old type's objects keep it under its stored key; nothing new is created in it", input, entry.Slug, entry.Slug),
				}.Hintf("use a live type instead — list them with %s", v2model.RefListTypes(spaceId)), true
			}
		}
	}
	return v2model.Issue{
		Path:    path,
		Message: fmt.Sprintf("type %q was removed from this space — its objects keep it, but nothing new is created in it and it is not filterable", spelling),
	}.Hintf("use a live type instead — list them with %s", v2model.RefListTypes(spaceId)), true
}
