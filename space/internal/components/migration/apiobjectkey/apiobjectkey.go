// Package apiobjectkey backfills the `apiObjectKey` slug for types and
// properties that predate it.
//
// The slug is API v2's entire key surface for a BSON-keyed type or property
// (ADDRESSING.md §7.5a): the internal key never appears on the wire, so an
// entity with no stored slug has **no stable bare-op address at all** — it
// can be read (documents pin the stored key) but not named in a
// `setProperties`, a filter, a sort or a route. `systemobjectreviser` never
// filled it in: its revised-keys list carries no `ApiObjectKey`, and it
// only ever visits bundled/system objects anyway, which are exactly the
// ones that do not need a stored slug (their slug is DERIVED in code —
// `pkg/lib/bundle/apislug.go` is the authority, §7.5a-1).
//
// This is the §7.5-requirement-5 backfill, and it is a data migration, so:
//
//   - **Idempotent.** It only ever fills an EMPTY slug; it never re-points
//     one. `apiObjectKey` is mutable and v1-visible (v1 addresses properties
//     by it), so overwriting a stored slug would silently re-aim a shipped
//     address — the exact failure class the identity work exists to kill.
//   - **Safe to re-run.** A run that skips an object (see below) leaves it
//     untouched, so a later run picks it up if the obstacle is gone.
//   - **Convergent.** Candidates are processed in ascending object-id order,
//     so two devices migrating the same space independently derive the same
//     slugs; store order never decides an address (§7.1's minting rule).
//   - **Explicit about the taken case.** When the slug a candidate would
//     take is ALREADY HELD by another entity in the space, this migration
//     deliberately does NOTHING for that candidate. See takenSlugPolicy.
package apiobjectkey

import (
	"context"
	"errors"
	"fmt"
	"sort"

	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
)

const MName = "ApiObjectKeyBackfill"

// takenSlugPolicy — WHY a collision is a no-op rather than a suffix.
//
// The mint (`objectcreator.ensureUniqueApiObjectKey`) suffixes on collision
// because a UI create must succeed and has a name the user just chose. A
// backfill has neither: it runs unattended over data whose owner is not
// present, and a suffixed slug (`due_date_2`) would be a NEW permanent,
// user-visible, v1-addressable name invented by an ordering accident.
//
// Skipping is the reversible choice — the entity keeps exactly the
// addressability it has today (its stored key), no new name is invented,
// and the decision stays open. Which resolution the colliding case should
// ultimately get — suffix here, a deterministic re-slug sweep, or leaving
// the loud-ambiguity lookup as the floor — is ADDRESSING.md §8 open
// question 3 ("twin-slug repair"), whose recorded lean is "floor first,
// sweep if telemetry shows real collisions". This migration implements the
// floor and counts the skips so the telemetry exists.
const takenSlugPolicy = "skip"

// maxSlugLength matches the v2 key grammar's maxLength.
const maxSlugLength = 256

// Migration ApiObjectKeyBackfill fills apiObjectKey for non-bundled types
// and properties that have none (GO-7383, ADDRESSING.md §7.5 requirement 5).
type Migration struct{}

func (m Migration) Name() string {
	return MName
}

// entity is one type or property as this migration sees it.
type entity struct {
	id        string
	key       string // stored relation key / type uniqueKey internal part
	slug      string // stored apiObjectKey ("" = the backfill's candidates)
	name      string
	isType    bool
	isBundled bool
}

func (m Migration) Run(ctx context.Context, log logger.CtxLogger, store dependencies.QueryableStore, space dependencies.SpaceWithCtx) (toMigrate, migrated int, err error) {
	spaceId := space.Id()

	// the cheap steady-state probe: after the first successful run a space
	// answers this with nothing and the migration costs one query
	unslugged, err := listUnslugged(store)
	if err != nil {
		return 0, 0, fmt.Errorf("list types and properties without an api key in space %s: %w", spaceId, err)
	}
	candidates := make([]entity, 0, len(unslugged))
	for _, e := range unslugged {
		// bundled keys need no stored slug: the derived table in code is
		// their authority, in every space and offline (§7.5a-1). Stamping
		// one would add nothing and could only manufacture a twin.
		if e.isBundled || e.key == "" {
			continue
		}
		candidates = append(candidates, e)
	}
	toMigrate = len(candidates)
	if toMigrate == 0 {
		return 0, 0, nil
	}
	sort.Slice(candidates, func(i, j int) bool { return candidates[i].id < candidates[j].id })

	all, err := listTypesAndProperties(store)
	if err != nil {
		return toMigrate, 0, fmt.Errorf("list types and properties of space %s: %w", spaceId, err)
	}
	namespaces := newNamespaces(all)

	skipped := 0
	for _, candidate := range candidates {
		if e := ctx.Err(); e != nil {
			return toMigrate, migrated, errors.Join(err, e)
		}
		slug := deriveSlug(candidate)
		if slug == "" {
			// nothing derivable (a name that transliterates away): the
			// stored key stays the only address, as at mint
			skipped++
			continue
		}
		if namespaces.taken(slug, candidate) {
			// takenSlugPolicy: leave it alone, loudly enough to count
			skipped++
			log.Debug("api key backfill skipped a taken slug",
				zap.String("migration", MName), zap.String("spaceId", spaceId),
				zap.String("objectId", candidate.id), zap.String("slug", slug))
			continue
		}
		if e := writeSlug(ctx, space, candidate.id, slug); e != nil {
			err = errors.Join(err, fmt.Errorf("set api key %q on %s in space %s: %w", slug, candidate.id, spaceId, e))
			continue
		}
		namespaces.occupy(slug, candidate)
		migrated++
	}
	if skipped > 0 {
		log.Info("api key backfill left objects without a slug",
			zap.String("migration", MName), zap.String("spaceId", spaceId), zap.Int("skipped", skipped))
	}
	return toMigrate, migrated, err
}

// deriveSlug is the backfill's mint. A readable stored key is its own slug
// (it is already addressable through resolution chain step 1, and making the
// slug agree keeps the served spelling stable); an opaque BSON key has no
// readable content, so the slug comes from the display name — derived ONCE
// and stored, which is precisely what makes it stable under a later rename.
// (Deriving it at READ time instead is the HTML-anchor anti-pattern the
// dossier rejects — §7.5a-6.)
func deriveSlug(e entity) string {
	if !isBsonLikeKey(e.key) {
		return bundle.SanitizeApiSlug(bundle.ApiSlug(e.key), maxSlugLength)
	}
	return bundle.SanitizeApiSlug(bundle.ApiSlugFromName(e.name), maxSlugLength)
}

// namespaces holds the two occupied slug namespaces (types and properties
// are separate — different routes, different document slots) as
// name -> holding stored key, so an entity never collides with itself.
type namespaces struct {
	types      map[string]string
	properties map[string]string
}

func newNamespaces(all []entity) *namespaces {
	ns := &namespaces{types: map[string]string{}, properties: map[string]string{}}
	for _, e := range all {
		if e.key == "" {
			continue
		}
		ns.occupy(e.key, e)
		if e.slug != "" {
			ns.occupy(e.slug, e)
		}
	}
	return ns
}

func (n *namespaces) space(e entity) map[string]string {
	if e.isType {
		return n.types
	}
	return n.properties
}

func (n *namespaces) occupy(name string, e entity) {
	m := n.space(e)
	if _, ok := m[name]; !ok {
		m[name] = e.key
	}
}

// taken reports whether name is held by anything other than e — including
// the bundled vocabulary, whose derived slugs are addressable in every space
// whether or not the object is installed (§7.5a-1/-5 step 3).
func (n *namespaces) taken(name string, e entity) bool {
	if holder, ok := n.space(e)[name]; ok && holder != e.key {
		return true
	}
	if e.isType {
		if key, ok := bundle.TypeKeyByApiSlug(name); ok && string(key) != e.key {
			return true
		}
		return name != e.key && bundle.HasObjectTypeByKey(domain.TypeKey(name))
	}
	if key, ok := bundle.RelationKeyByApiSlug(name); ok && string(key) != e.key {
		return true
	}
	return name != e.key && bundle.HasRelation(domain.RelationKey(name))
}

func writeSlug(ctx context.Context, space dependencies.SpaceWithCtx, objectId, slug string) error {
	return space.DoCtx(ctx, objectId, func(sb smartblock.SmartBlock) error {
		st := sb.NewState()
		st.SetChangeType(domain.ChangeTypeApiObjectKeyBackfill)
		st.SetDetail(bundle.RelationKeyApiObjectKey, domain.String(slug))
		return sb.Apply(st)
	})
}

// liveFilters is the §7.5-requirement-2 corpse policy, matched to v2's
// liveProperties/liveTypes: a UI-deleted (isUninstalled) object vacates the
// slug namespace, so it neither gets a slug nor holds one against anyone
// else. isArchived/isDeleted ride the store's injected query defaults.
func liveFilters() []database.FilterRequest {
	return []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyResolvedLayout,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.Int64List([]model.ObjectTypeLayout{model.ObjectType_objectType, model.ObjectType_relation}),
		},
		{
			RelationKey: bundle.RelationKeyIsUninstalled,
			Condition:   model.BlockContentDataviewFilter_NotEqual,
			Value:       domain.Bool(true),
		},
	}
}

func listUnslugged(store dependencies.QueryableStore) ([]entity, error) {
	return list(store, append(liveFilters(), database.FilterRequest{
		RelationKey: bundle.RelationKeyApiObjectKey,
		Condition:   model.BlockContentDataviewFilter_Empty,
	}))
}

func listTypesAndProperties(store dependencies.QueryableStore) ([]entity, error) {
	return list(store, liveFilters())
}

func list(store dependencies.QueryableStore, filters []database.FilterRequest) ([]entity, error) {
	records, err := store.Query(database.Query{Filters: filters})
	if err != nil {
		return nil, err
	}
	out := make([]entity, 0, len(records))
	for _, record := range records {
		e := entity{
			id:     record.Details.GetString(bundle.RelationKeyId),
			slug:   record.Details.GetString(bundle.RelationKeyApiObjectKey),
			name:   record.Details.GetString(bundle.RelationKeyName),
			isType: record.Details.GetInt64(bundle.RelationKeyResolvedLayout) == int64(model.ObjectType_objectType),
		}
		if e.isType {
			key, keyErr := domain.GetTypeKeyFromRawUniqueKey(record.Details.GetString(bundle.RelationKeyUniqueKey))
			if keyErr != nil {
				continue
			}
			e.key = string(key)
			e.isBundled = bundle.HasObjectTypeByKey(key)
		} else {
			e.key = record.Details.GetString(bundle.RelationKeyRelationKey)
			e.isBundled = bundle.HasRelation(domain.RelationKey(e.key))
		}
		out = append(out, e)
	}
	return out, nil
}

// isBsonLikeKey reports whether a stored key is a minted BSON ObjectId hex —
// 24 hex chars with at least one digit (the same heuristic v2 and v1 use).
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
