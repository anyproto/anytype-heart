package v2service

// refs.go is the Phase-2 referential validation layer (APIV2.md R9): on
// create, type and property references are validated against the space and
// rejected with path-addressed did-you-mean errors listing the actual keys —
// the schema-linking hallucination guard. Create-missing surfaces (select
// option names, typeProperties) are exempt by design; see resolver.go for
// the full policy table.

import (
	"fmt"
	"sort"
	"strings"

	v2model "github.com/anyproto/anytype-heart/core/api/v2/model"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/anyblockjson/storeresolver"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// maxListedKeys bounds how many actual keys an error message names.
const maxListedKeys = 15

// typeIdInSpace resolves a type key to its object id when the type object
// exists in the space; ok is false otherwise.
func (s *V2Service) typeIdInSpace(spaceId, typeKey string) (string, bool) {
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

// typeKeyExists reports whether a type key resolves in the space or the
// bundle (bundled types install on first use — the create adapter does it).
func (s *V2Service) typeKeyExists(spaceId, typeKey string) bool {
	if _, ok := s.typeIdInSpace(spaceId, typeKey); ok {
		return true
	}
	return bundle.HasObjectTypeByKey(domain.TypeKey(typeKey))
}

// knownTypeKeys lists the space's type keys (for did-you-mean).
func (s *V2Service) knownTypeKeys(spaceId string) []string {
	keys, _ := s.typeKeysById(spaceId)
	out := make([]string, 0, len(keys))
	seen := map[string]bool{}
	for _, key := range keys {
		if key != "" && !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

// unknownTypeKeyError is the R9 did-you-mean 400 for a type reference.
func (s *V2Service) unknownTypeKeyError(spaceId, typeKey, path string) error {
	known := s.knownTypeKeys(spaceId)
	return v2model.ValidationFailed(
		fmt.Sprintf("type %q not found in space %q", typeKey, spaceId),
		v2model.Issue{
			Path:    path,
			Message: fmt.Sprintf("unknown type key %q — %s", typeKey, listKnown("type keys", known)),
			Hint:    didYouMean(typeKey, known, fmt.Sprintf("list all with GET /v2/spaces/%s/types", spaceId)),
		})
}

// typeNotFoundError is the 404 for a type-KEY lookup miss on the routes that
// address a type directly (GET/PATCH/DELETE types/{type}). It lists the
// space's actual keys plus the nearest match — the same steering the R9
// create path gives (unknownTypeKeyError): a candidate-less tip is a dead
// end for a small model (§8.21 — given the bare "not found" text, a
// benchmarked 4B did not retry at all, while the key-listing property tip
// repaired on the first retry in the same run).
func (s *V2Service) typeNotFoundError(spaceId, typeKey string) error {
	return v2model.NotFound(notFoundWithKeys(
		fmt.Sprintf("type %q not found in space %q", typeKey, spaceId),
		typeKey, "type keys", s.knownTypeKeys(spaceId),
		fmt.Sprintf("list all with GET /v2/spaces/%s/types", spaceId)))
}

// propertyNotFoundError is typeNotFoundError's sibling for property-KEY
// routes (options listing, PATCH/DELETE properties/{key}).
func (s *V2Service) propertyNotFoundError(spaceId, propertyKey string) error {
	return v2model.NotFound(notFoundWithKeys(
		fmt.Sprintf("property %q not found in space %q", propertyKey, spaceId),
		propertyKey, "property keys", s.knownPropertyKeys(spaceId),
		fmt.Sprintf("list all with GET /v2/spaces/%s/properties", spaceId)))
}

// notFoundWithKeys composes a not-found message that is repairable from the
// error alone: subject, the known keys (capped), and a did-you-mean when a
// close key exists. The list route rides along only when the key list was
// truncated and no suggestion fired — the one case where the message alone
// cannot show every candidate.
func notFoundWithKeys(subject, input, what string, known []string, listRoute string) string {
	msg := subject + " — " + listKnown(what, known)
	if hint := didYouMean(input, known, ""); hint != "" {
		return msg + "; " + hint
	}
	if len(known) > maxListedKeys {
		return msg + "; " + listRoute
	}
	return msg
}

// propertyKeyExists reports whether a property key resolves in the space or
// the bundle.
func (s *V2Service) propertyKeyExists(spaceId, key string) bool {
	if _, err := s.store.SpaceIndex(spaceId).GetRelationByKey(key); err == nil {
		return true
	}
	return bundle.HasRelation(domain.RelationKey(key))
}

// knownPropertyKeys lists the space's visible property keys (did-you-mean).
func (s *V2Service) knownPropertyKeys(spaceId string) []string {
	records, err := s.store.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Int64(int64(model.ObjectType_relation)),
		}},
	})
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(records))
	seen := map[string]bool{}
	for _, record := range records {
		key := record.Details.GetString(bundle.RelationKeyRelationKey)
		if key != "" && !seen[key] {
			seen[key] = true
			out = append(out, key)
		}
	}
	sort.Strings(out)
	return out
}

// unknownPropertyIssue builds one path-addressed did-you-mean issue for an
// unknown property key.
func unknownPropertyIssue(key, path string, known []string, listUrl string) v2model.Issue {
	return v2model.Issue{
		Path:    path,
		Message: fmt.Sprintf("unknown property key %q — %s", key, listKnown("property keys", known)),
		Hint:    didYouMean(key, known, listUrl),
	}
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
// the discovery list.
func didYouMean(input string, known []string, fallback string) string {
	suggestions := closestKeys(input, known, 3)
	if len(suggestions) == 0 {
		return fallback
	}
	return "did you mean " + strings.Join(suggestions, ", ") + "?"
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

// typePropertyKeys collects the property keys a type actually recommends
// (its four recommended-relation lists, resolved id→key) — the R9 reference
// set for set filters and sorts.
func (s *V2Service) typePropertyKeys(spaceId, typeId string) []string {
	details, err := s.store.SpaceIndex(spaceId).GetDetails(typeId)
	if err != nil {
		return nil
	}
	reads := storeresolver.New(s.store.SpaceIndex(spaceId))
	var out []string
	seen := map[string]bool{}
	for _, listKey := range []domain.RelationKey{
		bundle.RelationKeyRecommendedFeaturedRelations,
		bundle.RelationKeyRecommendedRelations,
		bundle.RelationKeyRecommendedFileRelations,
		bundle.RelationKeyRecommendedHiddenRelations,
	} {
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
