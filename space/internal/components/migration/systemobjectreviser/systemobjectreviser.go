package systemobjectreviser

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/any-sync/app/logger"
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
)

// Migration SystemObjectReviser performs revision of all system object types and relations, so after Migration
// objects installed in space should correspond to bundled objects from library.
// To modify relations of system objects relation revision should be incremented in types.json or relations.json
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

	bundleObject, isSystem := getBundleObjectDetails(uk)
	if bundleObject == nil {
		return false, nil
	}

	if bundleObject.GetInt64(revisionKey) <= localObject.GetInt64(revisionKey) {
		return false, nil
	}
	details := buildDiffDetails(bundleObject, localObject, isSystem)

	recRelsDetails, err := checkRecommendedRelations(ctx, space, bundleObject, localObject, uk)
	if err != nil {
		log.Error("failed to check recommended relations", zap.Error(err))
	}

	for _, recRelsDetail := range recRelsDetails {
		details.Set(recRelsDetail.Key, recRelsDetail.Value)
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
			st.SetChangeType(domain.SystemObjectReviserMigration)
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

// getBundleObjectDetails returns nil if the object with provided unique key is not either system relation or bundled type
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
		if !isSystemRelation(uk) {
			// non system relation, no need to revise
			return nil, false
		}
		relationKey := domain.RelationKey(uk.InternalKey())
		relation := bundle.MustGetRelation(relationKey)
		return (&relationutils.Relation{Relation: relation}).ToDetails(), true
	default:
		return nil, false
	}
}

func buildDiffDetails(origin, current *domain.Details, isSystem bool) *domain.Details {
	// non-system bundled types are going to update only icons and plural names for now
	filterKeys := customObjectFilterKeys
	if isSystem {
		filterKeys = systemObjectFilterKeys
	}
	diff, _ := domain.StructDiff(current, origin)
	diff = diff.CopyOnlyKeys(filterKeys...)

	if cannotApplyPluralName(isSystem, current, origin) {
		diff.Delete(bundle.RelationKeyName)
		diff.Delete(bundle.RelationKeyPluralName)
	}
	return diff
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
