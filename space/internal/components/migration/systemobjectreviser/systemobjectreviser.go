package systemobjectreviser

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/util/slice"
)

type detailsSettable interface {
	SetDetails(ctx session.Context, details []*model.Detail, showEvent bool) (err error)
}

const MName = "SystemObjectReviser"

const revisionKey = bundle.RelationKeyRevision

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

	bundleObject := getBundleSystemObjectDetails(uk)
	if bundleObject == nil {
		return false, nil
	}

	if bundleObject.GetInt64(revisionKey) <= localObject.GetInt64(revisionKey) {
		return false, nil
	}
	details := buildDiffDetails(bundleObject, localObject)

	recRelsDetails, err := checkRecommendedRelations(ctx, space, bundleObject, localObject)
	if err != nil {
		log.Error("failed to check recommended relations", zap.Error(err))
	}

	for _, recRelsDetail := range recRelsDetails {
		details.Set(recRelsDetail.Key, recRelsDetail.Value)
	}

	if details.Len() > 0 {
		log.Debug("updating system object", zap.String("key", uk.InternalKey()), zap.String("space", space.Id()))
		if err := space.DoCtx(ctx, localObject.GetString(bundle.RelationKeyId), func(sb smartblock.SmartBlock) error {
			st := sb.NewState()
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

// getBundleSystemObjectDetails returns nil if the object with provided unique key is not either system relation or system type
func getBundleSystemObjectDetails(uk domain.UniqueKey) *domain.Details {
	switch uk.SmartblockType() {
	case coresb.SmartBlockTypeObjectType:
		typeKey := domain.TypeKey(uk.InternalKey())
		if !lo.Contains(bundle.SystemTypes, typeKey) {
			// non system object type, no need to revise
			return nil
		}
		objectType := bundle.MustGetType(typeKey)
		return (&relationutils.ObjectType{ObjectType: objectType}).BundledTypeDetails()
	case coresb.SmartBlockTypeRelation:
		relationKey := domain.RelationKey(uk.InternalKey())
		if !lo.Contains(bundle.SystemRelations, relationKey) {
			// non system relation, no need to revise
			return nil
		}
		relation := bundle.MustGetRelation(relationKey)
		return (&relationutils.Relation{Relation: relation}).ToDetails()
	default:
		return nil
	}
}

func buildDiffDetails(origin, current *domain.Details) *domain.Details {
	diff, _ := domain.StructDiff(current, origin)
	diff = diff.CopyOnlyKeys(
		bundle.RelationKeyName,
		bundle.RelationKeyDescription,
		bundle.RelationKeyIsReadonly,
		bundle.RelationKeyIsHidden,
		bundle.RelationKeyRevision,
		bundle.RelationKeyRelationReadonlyValue,
		bundle.RelationKeyRelationMaxCount,
		bundle.RelationKeyTargetObjectType,
		bundle.RelationKeyIconEmoji,
	)

	details := domain.NewDetails()
	for key, value := range diff.Iterate() {
		if key == bundle.RelationKeyTargetObjectType {
			// special case. We don't want to remove the types that was set by user, so only add ones that we have
			currentList := current.GetStringList(bundle.RelationKeyTargetObjectType)
			missedInCurrent, _ := lo.Difference(origin.GetStringList(bundle.RelationKeyTargetObjectType), currentList)
			currentList = append(currentList, missedInCurrent...)
			value = domain.StringList(currentList)
		}
		details.Set(key, value)
	}
	return details
}

func checkRecommendedRelations(
	ctx context.Context, space dependencies.SpaceWithCtx, origin, current *domain.Details,
) (newValues []*domain.Detail, err error) {
	details := origin.CopyOnlyKeys(
		bundle.RelationKeyRecommendedRelations,
		bundle.RelationKeyRecommendedLayout,
		bundle.RelationKeyUniqueKey,
	)

	_, filled, err := relationutils.FillRecommendedRelations(ctx, space, details)
	if filled {
		return nil, nil
	}

	if err != nil {
		return nil, err
	}

	for _, key := range []domain.RelationKey{
		bundle.RelationKeyRecommendedRelations,
		bundle.RelationKeyRecommendedFeaturedRelations,
		bundle.RelationKeyRecommendedFileRelations,
	} {
		localIds := current.GetStringList(key)
		newIds := details.GetStringList(key)

		removed, added := slice.DifferenceRemovedAdded(localIds, newIds)
		if len(added) != 0 || len(removed) != 0 {
			newValues = append(newValues, &domain.Detail{
				Key:   key,
				Value: domain.StringList(newIds),
			})
		}
	}

	return newValues, nil
}
