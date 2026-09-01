package systemobjectreviser

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/anyproto/any-sync/commonspace/object/acl/list"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/util/slice"
)

const MName = "SystemObjectReviser"

const revisionKey = bundle.RelationKeyRevision

var (
	systemObjectFilterKeys = []domain.RelationKey{
		bundle.RelationKeyName,
		bundle.RelationKeyIsReadonly,
		bundle.RelationKeyIsHidden,
		bundle.RelationKeyRevision,
		bundle.RelationKeyRelationReadonlyValue,
		bundle.RelationKeyRelationMaxCount,
		bundle.RelationKeyIconEmoji,
		bundle.RelationKeyIconOption,
		bundle.RelationKeyIconName,
		bundle.RelationKeyPluralName,
		bundle.RelationKeyRecommendedLayout,
		bundle.RelationKeyRelationFormatIncludeTime,
	}

	customObjectFilterKeys = []domain.RelationKey{
		bundle.RelationKeyRevision,
		bundle.RelationKeyIconOption,
		bundle.RelationKeyIconName,
		bundle.RelationKeyPluralName,
	}

	// nonSystemRelationFilterKeys lists the only details the reviser is allowed to touch on
	// bundled NON-system relations. Kept deliberately narrow: non-system relations are fully
	// user-modifiable, so the reviser only records the bundle revision and — guarded by
	// previousBundledRelationNames — propagates bundled renames.
	nonSystemRelationFilterKeys = []domain.RelationKey{
		bundle.RelationKeyRevision,
		bundle.RelationKeyName,
	}
)

// previousBundledRelationNames maps a bundled NON-system relation key to every display name
// it had in earlier releases. The reviser applies the current bundled name to an installed
// relation only if its local name still equals one of these previous bundled names; any other
// local name is a user's own rename and is kept. Whenever a bundled non-system relation is
// renamed, append its OLD name here and bump the relation's revision in relations.json —
// without both, the rename never reaches existing spaces. System relations do not need
// entries: users cannot rename them, so the system path applies names unconditionally.
var previousBundledRelationNames = map[domain.RelationKey][]string{
	bundle.RelationKeyAudioGenre:            {"Genre"},
	bundle.RelationKeyHeaderRelationsLayout: {"Header relations layout"},
}

// Migration SystemObjectReviser performs revision of all system object types and relations, so after Migration
// objects installed in space should correspond to bundled objects from library.
// To modify relations of system objects relation revision should be incremented in types.json or relations.json
// Bundled non-system relations are also reachable, but only for revision and name
// (see nonSystemRelationFilterKeys and previousBundledRelationNames).
// For more info see 'System Objects Update' section of docs/Flow.md
type Migration struct{}

func (m Migration) Name() string {
	return MName
}

func (m Migration) Run(ctx context.Context, log logger.CtxLogger, store dependencies.QueryableStore, space dependencies.SpaceWithCtx) (toMigrate, migrated int, err error) {
	spaceObjects, err := listAllTypesAndRelations(store)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get relations and types from client space: %w", err)
	}

	for _, details := range spaceObjects {
		shouldBeRevised, e := reviseObject(ctx, log, space, details)
		if !shouldBeRevised {
			continue
		}
		toMigrate++
		if e != nil {
			// A space this account may only READ answers the same way for
			// every object in it — the verdict is the ACL's, not the
			// object's — so one refusal ends the pass. Without this the
			// migration attempted every pending object on every space load,
			// logged an error for each, and left each one carrying the new
			// value in memory until it was evicted: an unwritable change is
			// applied to the loaded document before the push is refused, so
			// a reader of a shared space saw a value that would never
			// persist. Measured on a subscribed space: twelve failed writes
			// and thirteen documents whose two consecutive exports differed.
			//
			// Not an error to report. A reader cannot revise the system
			// objects of a space they do not own, and saying so once per
			// load is the whole of what is true.
			if errors.Is(e, list.ErrInsufficientPermissions) {
				log.Debug("skipping system object revision: this account may only read the space",
					zap.String("space", space.Id()))
				return toMigrate, migrated, nil
			}
			err = errors.Join(err, fmt.Errorf("failed to revise object: %w", e))
		} else {
			migrated++
		}
	}
	return
}

func listAllTypesAndRelations(store dependencies.QueryableStore) (map[string]*domain.Details, error) {
	records, err := store.Query(database.Query{
		Filters: []database.FilterRequest{
			{
				RelationKey: bundle.RelationKeyResolvedLayout,
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       domain.Int64List([]model.ObjectTypeLayout{model.ObjectType_objectType, model.ObjectType_relation}),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	details := make(map[string]*domain.Details, len(records))
	for _, record := range records {
		id := record.Details.GetString(bundle.RelationKeyId)
		details[id] = record.Details
	}
	return details, nil
}

func reviseObject(ctx context.Context, log logger.CtxLogger, space dependencies.SpaceWithCtx, localObject *domain.Details) (toRevise bool, err error) {
	uniqueKeyRaw := localObject.GetString(bundle.RelationKeyUniqueKey)

	uk, err := domain.UnmarshalUniqueKey(uniqueKeyRaw)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal unique key '%s': %w", uniqueKeyRaw, err)
	}

	// compare raw bundle revision first: constructing full bundled details for every
	// type/relation on every space load is pure waste when nothing changed
	bundleRevision, isBundled := getBundleObjectRevision(uk)
	if !isBundled || bundleRevision <= localObject.GetInt64(revisionKey) {
		return false, nil
	}

	bundleObject, isSystem := getBundleObjectDetails(uk)
	if bundleObject == nil {
		return false, nil
	}
	details := buildDiffDetails(bundleObject, localObject, uk, isSystem)

	// non-system relations are user-modifiable, so only the narrow filtered diff
	// (revision + guarded name) may be applied to them
	if isSystem || uk.SmartblockType() != coresb.SmartBlockTypeRelation {
		recRelsDetails, err := checkRecommendedRelations(ctx, space, bundleObject, localObject, uk)
		if err != nil {
			log.Error("failed to check recommended relations", zap.Error(err))
		}

		for _, recRelsDetail := range recRelsDetails {
			details.Set(recRelsDetail.Key, recRelsDetail.Value)
		}
	}

	if isSystem {
		relFormatOTDetail, err := checkRelationFormatObjectTypes(ctx, space, bundleObject, localObject)
		if err != nil {
			log.Error("failed to check relation format object types", zap.Error(err))
		}

		if relFormatOTDetail != nil {
			details.Set(relFormatOTDetail.Key, relFormatOTDetail.Value)
		}
	}

	if details.Len() > 0 {
		log.Debug("updating system object", zap.String("key", uk.InternalKey()), zap.String("space", space.Id()))
		if err := space.DoCtx(ctx, localObject.GetString(bundle.RelationKeyId), func(sb smartblock.SmartBlock) error {
			st := sb.NewState()
			st.SetChangeType(domain.ChangeTypeSystemObjectReviserMigration)
			for key, value := range details.Iterate() {
				st.SetDetail(key, value)
			}
			return sb.Apply(st)
		}); err != nil {
			return true, fmt.Errorf("failed to update system object '%s' in space '%s': %w", uk.InternalKey(), space.Id(), err)
		}
	}
	return true, nil
}

// getBundleObjectRevision returns the revision of the bundled counterpart without building
// its details. Mirrors getBundleObjectDetails: ok is false for non-bundled objects, which
// are not revisable. Bundled non-system relations answer too, so that a bundled rename with
// a revision bump reaches them; almost all of them have revision 0, so the caller's
// revision guard short-circuits before any details are built.
func getBundleObjectRevision(uk domain.UniqueKey) (revision int64, ok bool) {
	switch uk.SmartblockType() {
	case coresb.SmartBlockTypeObjectType:
		objectType, err := bundle.GetType(domain.TypeKey(uk.InternalKey()))
		if err != nil {
			return 0, false
		}
		return objectType.Revision, true
	case coresb.SmartBlockTypeRelation:
		relation, err := bundle.GetRelation(domain.RelationKey(uk.InternalKey()))
		if err != nil {
			return 0, false
		}
		return relation.Revision, true
	default:
		return 0, false
	}
}

// getBundleObjectDetails returns nil if the object with provided unique key is not a bundled type or relation
func getBundleObjectDetails(uk domain.UniqueKey) (details *domain.Details, isSystem bool) {
	switch uk.SmartblockType() {
	case coresb.SmartBlockTypeObjectType:
		typeKey := domain.TypeKey(uk.InternalKey())
		objectType, err := bundle.GetType(typeKey)
		if err != nil {
			// not bundled type, no need to revise
			return nil, false
		}
		return (&relationutils.ObjectType{ObjectType: objectType}).BundledTypeDetails(), isSystemType(uk)
	case coresb.SmartBlockTypeRelation:
		relation, err := bundle.GetRelation(domain.RelationKey(uk.InternalKey()))
		if err != nil {
			// not bundled relation, no need to revise
			return nil, false
		}
		return (&relationutils.Relation{Relation: relation}).ToDetails(), isSystemRelation(uk)
	default:
		return nil, false
	}
}

func buildDiffDetails(origin, current *domain.Details, uk domain.UniqueKey, isSystem bool) *domain.Details {
	isNonSystemRelation := !isSystem && uk.SmartblockType() == coresb.SmartBlockTypeRelation

	filterKeys := systemObjectFilterKeys
	if isNonSystemRelation {
		// non-system bundled relations only record the revision and, guardedly, bundled renames
		filterKeys = nonSystemRelationFilterKeys
	} else if !isSystem {
		// non-system bundled types are going to update only icons and plural names for now
		filterKeys = customObjectFilterKeys
	}
	diff, _ := domain.StructDiff(current, origin)
	diff = diff.CopyOnlyKeys(filterKeys...)

	if isNonSystemRelation {
		if !canApplyBundledRelationName(domain.RelationKey(uk.InternalKey()), current.GetString(bundle.RelationKeyName)) {
			diff.Delete(bundle.RelationKeyName)
		}
	} else if cannotApplyPluralName(isSystem, current, origin) {
		diff.Delete(bundle.RelationKeyName)
		diff.Delete(bundle.RelationKeyPluralName)
	}
	return diff
}

// canApplyBundledRelationName reports whether the bundled name may overwrite the local one:
// only when the local name is still a previous bundled name from previousBundledRelationNames
// (or is empty). Any other local name is the user's own rename and must be kept.
func canApplyBundledRelationName(key domain.RelationKey, currentName string) bool {
	if currentName == "" {
		return true
	}
	return lo.Contains(previousBundledRelationNames[key], currentName)
}

func cannotApplyPluralName(isSystem bool, current, origin *domain.Details) bool {
	// we cannot set plural name to custom types with custom name
	return !isSystem && current.GetString(bundle.RelationKeyName) != origin.GetString(bundle.RelationKeyName)
}

func checkRelationFormatObjectTypes(
	ctx context.Context, space dependencies.SpaceWithCtx, origin, current *domain.Details,
) (newValue *domain.Detail, err error) {
	localIds := current.GetStringList(bundle.RelationKeyRelationFormatObjectTypes)
	bundledIds := origin.GetStringList(bundle.RelationKeyRelationFormatObjectTypes)

	newIds := make([]string, 0, len(bundledIds))
	for _, bundledId := range bundledIds {
		if !strings.HasPrefix(bundledId, addr.BundledObjectTypeURLPrefix) {
			return nil, fmt.Errorf("invalid object id: %s. %s prefix is expected", bundledId, addr.BundledObjectTypeURLPrefix)
		}
		key := strings.TrimPrefix(bundledId, addr.BundledObjectTypeURLPrefix)
		uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeObjectType, key)
		if err != nil {
			return nil, err
		}

		// we should add only system objects to detail, because non-system objects could be not installed to space yet
		if isSystemType(uk) {
			continue
		}

		id, err := space.DeriveObjectID(ctx, uk)
		if err != nil {
			return nil, fmt.Errorf("failed to derive system object with key '%s': %w", key, err)
		}

		newIds = append(newIds, id)
	}

	_, added := slice.DifferenceRemovedAdded(localIds, newIds)
	if len(added) == 0 {
		return nil, nil
	}

	return &domain.Detail{
		Key:   bundle.RelationKeyRelationFormatObjectTypes,
		Value: domain.StringList(append(localIds, added...)),
	}, nil
}

func checkRecommendedRelations(
	ctx context.Context, space dependencies.SpaceWithCtx, origin, current *domain.Details, uk domain.UniqueKey,
) (newValues []*domain.Detail, err error) {
	details := origin.CopyOnlyKeys(
		bundle.RelationKeyRecommendedRelations,
		bundle.RelationKeyRecommendedLayout,
		bundle.RelationKeyUniqueKey,
	)

	_, filled, err := relationutils.FillRecommendedRelations(ctx, space, details, domain.TypeKey(uk.InternalKey()))
	if filled {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	var allNewIds []string
	for _, key := range []domain.RelationKey{
		bundle.RelationKeyRecommendedFeaturedRelations,
		bundle.RelationKeyRecommendedFileRelations,
		bundle.RelationKeyRecommendedHiddenRelations,
		bundle.RelationKeyRecommendedRelations,
	} {
		localIds := current.GetStringList(key)
		newIds := details.GetStringList(key)
		allNewIds = append(allNewIds, newIds...)

		removed, added := slice.DifferenceRemovedAdded(localIds, newIds)
		if len(added) != 0 || len(removed) != 0 {
			if key == bundle.RelationKeyRecommendedRelations {
				// we should not miss relations that were set to recommended by user
				removedFromAll, _ := slice.DifferenceRemovedAdded(removed, allNewIds)
				newIds = append(newIds, removedFromAll...)
			}
			newValues = append(newValues, &domain.Detail{
				Key:   key,
				Value: domain.StringList(newIds),
			})
		}
	}

	return newValues, nil
}

func isSystemType(uk domain.UniqueKey) bool {
	return lo.Contains(bundle.SystemTypes, domain.TypeKey(uk.InternalKey()))
}

func isSystemRelation(uk domain.UniqueKey) bool {
	return bundle.IsSystemRelation(domain.RelationKey(uk.InternalKey()))
}
