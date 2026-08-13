package objectcreator

// apikey.go — mint-time uniqueness for the api key slug (`apiObjectKey`).
//
// The slug is identity-bearing for API v2: it is the ONLY key the surface
// speaks for a BSON-keyed type or property (ADDRESSING.md §7.5a), so two
// entities answering to one slug is not a cosmetic duplicate — it is an
// address that means two things. Until this file existed, the heart-side
// mint (`injectApiObjectKey`) derived the slug from the create-time name and
// checked NOTHING (§2.3-1): two properties named "Manual property" both took
// `manual_property`, and — the collision that matters — a UI property named
// "Due Date" took `due_date`, which is exactly bundled `dueDate`'s derived
// slug, shadowing the bundled property for every subsequent v2 request.
//
// The check is the UNION §7.5a-6 names, and nothing less:
//
//	1. the space's live stored api slugs   (apiObjectKey)
//	2. the space's live stored keys        (relationKey / uniqueKey internal)
//	3. the bundled derived slugs           (pkg/lib/bundle/apislug.go, both
//	   directions — the authority for bundled keys, which old spaces never
//	   stored as details)
//
// Arms 1 and 2 come from ONE bounded listing per create (the §7.5a-2
// resolver shape — tens to low hundreds of rows, never a query per
// candidate); arm 3 is in-memory.
//
// Two deliberate asymmetries with v2's mint (APIV2.md §8.22):
//
//   - **Bundled installs skip the check entirely.** A bundled key's slug is
//     DERIVED, not minted — the table in code is its authority and is
//     invariant — so there is nothing to disambiguate, and convergence is
//     the install mechanism (§2.4-1). Skipping also keeps first-space setup
//     free of a store query per installed relation.
//   - **A collision suffixes; it never refuses.** v2's POST refuses because
//     it has a caller to steer ("explicit beats silent" — §8.22). A UI
//     create has nobody to steer and must not fail because a NAME collided,
//     so the mint disambiguates: `due_date`, `due_date_2`, `due_date_3`…
//     (the `_N` spelling is what strcase itself produces for `dueDate2`, so
//     the suffixed form stays in the advertised `^[a-zA-Z0-9_]+$` grammar).
//     Loud-at-the-address beats silent-at-the-data, exactly as §7.5 argues.

import (
	"fmt"
	"strconv"

	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

// maxApiKeySuffix bounds the disambiguation walk. Beyond it the mint gives
// up and stores no slug at all: an empty apiObjectKey means "no derivable
// address, the minted BSON is the only one" — the same convention the v2
// mint uses for a name with no derivable slug — and it is strictly better
// than stamping a slug that already means something else.
const maxApiKeySuffix = 99

// apiKeyKind selects the namespace a slug must be unique within. Types and
// properties are separate namespaces (a type `task` and a property `task`
// never collide — they are addressed through different routes and different
// document slots), and options are not in this layer at all: option identity
// is name + pin, per property, and `apiObjectKey` on options stays a v1-only
// surface (§8-closed).
type apiKeyKind int

const (
	apiKeyKindRelation apiKeyKind = iota
	apiKeyKindType
)

// apiKeyNamespace is one space's occupied slug namespace for one kind: the
// stored arms as a name -> holding key map, plus the bundled arm consulted
// as a point lookup (the table is large and invariant; enumerating it per
// create would be waste). Every arm records WHO holds a name, so an entity
// re-minting its OWN slug — a bundled reinstall, a re-create of the same
// stored key — is never told it collides with itself.
type apiKeyNamespace struct {
	kind   apiKeyKind
	holder map[string]string // occupied name -> the stored key holding it
}

// taken reports whether name is held by anyone other than owner.
func (n apiKeyNamespace) taken(name, owner string) bool {
	if holder, ok := n.holder[name]; ok && holder != owner {
		return true
	}
	// arm 3: the bundled vocabulary — its derived slug, and (for a
	// caller-supplied slug that is spelled like an internal key) the key
	// itself. Consulted by lookup, not enumeration.
	if n.kind == apiKeyKindType {
		if key, ok := bundle.TypeKeyByApiSlug(name); ok && string(key) != owner {
			return true
		}
		return name != owner && bundle.HasObjectTypeByKey(domain.TypeKey(name))
	}
	if key, ok := bundle.RelationKeyByApiSlug(name); ok && string(key) != owner {
		return true
	}
	return name != owner && bundle.HasRelation(domain.RelationKey(name))
}

// occupy records name as held by key unless it is already held.
func (n apiKeyNamespace) occupy(name, key string) {
	if name == "" {
		return
	}
	if _, ok := n.holder[name]; !ok {
		n.holder[name] = key
	}
}

// loadApiKeyNamespace builds the union for one space and kind: one bounded
// listing for the stored arms, the bundled derived table for the third.
//
// "Live" is the §7.5-requirement-2 corpse policy, matched to v2's
// `liveProperties`/`liveTypes`: a UI-deleted (isUninstalled) object vacates
// the slug namespace, so delete-then-recreate mints the same clean slug
// again. isArchived/isDeleted ride the store's injected query defaults.
//
// A store error is returned, never swallowed: an empty-looking namespace
// would wave every collision through, and the whole point of this file is
// that a wrong slug is a wrong address.
func (s *service) loadApiKeyNamespace(spaceId string, kind apiKeyKind) (apiKeyNamespace, error) {
	ns := apiKeyNamespace{kind: kind, holder: map[string]string{}}

	layout := model.ObjectType_relation
	if kind == apiKeyKindType {
		layout = model.ObjectType_objectType
	}
	records, err := s.objectStore.SpaceIndex(spaceId).Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       domain.Int64(int64(layout)),
			},
			{
				RelationKey: bundle.RelationKeyIsUninstalled,
				Condition:   model.BlockContentDataviewFilter_NotEqual,
				Value:       domain.Bool(true),
			},
		},
	})
	if err != nil {
		return ns, fmt.Errorf("list %s of space %s: %w", kindName(kind), spaceId, err)
	}
	for _, record := range records {
		key := storedKeyOf(record.Details, kind)
		if key == "" {
			continue
		}
		ns.occupy(key, key)
		ns.occupy(record.Details.GetString(bundle.RelationKeyApiObjectKey), key)
	}
	return ns, nil
}

func storedKeyOf(details *domain.Details, kind apiKeyKind) string {
	if kind == apiKeyKindType {
		key, err := domain.GetTypeKeyFromRawUniqueKey(details.GetString(bundle.RelationKeyUniqueKey))
		if err != nil {
			return ""
		}
		return string(key)
	}
	return details.GetString(bundle.RelationKeyRelationKey)
}

func kindName(kind apiKeyKind) string {
	if kind == apiKeyKindType {
		return "types"
	}
	return "relations"
}

// isBundledKey reports whether a stored key is a bundled one, whose slug is
// derived rather than minted and therefore never disambiguated.
func isBundledKey(key string, kind apiKeyKind) bool {
	if key == "" {
		return false
	}
	if kind == apiKeyKindType {
		return bundle.HasObjectTypeByKey(domain.TypeKey(key))
	}
	return bundle.HasRelation(domain.RelationKey(key))
}

// disambiguateApiKey returns the first free spelling of slug in ns for the
// entity stored under key: slug itself, else slug_2, slug_3… Empty means the
// walk gave up (see maxApiKeySuffix).
func disambiguateApiKey(slug, key string, ns apiKeyNamespace) string {
	if slug == "" || !ns.taken(slug, key) {
		return slug
	}
	for i := 2; i <= maxApiKeySuffix; i++ {
		candidate := slug + "_" + strconv.Itoa(i)
		if !ns.taken(candidate, key) {
			return candidate
		}
	}
	return ""
}

// ensureUniqueApiObjectKey is the mint-time half of the union check: it
// takes the slug injectApiObjectKey just derived (or a caller-supplied one)
// and replaces it with a free spelling when something else in the space
// already answers to it.
//
// key is the entity's stored key when it is already decided ("" when the
// create is about to mint a fresh BSON — nothing can then be "its own"
// slug). A load error leaves the derived slug untouched and is returned for
// the caller to log: failing a user's property create because the store
// hiccuped would be a worse trade than a slug that may need repair, and the
// v2 resolution layer's ambiguity-loud lookups are the standing backstop.
func (s *service) ensureUniqueApiObjectKey(spaceId string, object *domain.Details, key string, kind apiKeyKind) error {
	slug := object.GetString(bundle.RelationKeyApiObjectKey)
	if slug == "" || isBundledKey(key, kind) {
		return nil
	}
	ns, err := s.loadApiKeyNamespace(spaceId, kind)
	if err != nil {
		return err
	}
	unique := disambiguateApiKey(slug, key, ns)
	if unique != slug {
		object.SetString(bundle.RelationKeyApiObjectKey, unique)
	}
	return nil
}
