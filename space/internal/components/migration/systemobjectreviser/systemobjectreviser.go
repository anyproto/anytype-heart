package systemobjectreviser

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/ethereum/go-ethereum/log"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/relationutils"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

type detailsSettable interface {
	SetDetails(ctx session.Context, details []*model.Detail, showEvent bool) (err error)
}

const MName = "SystemObjectReviser"

var revisionKey = bundle.RelationKeyRevision.String()

// Migration SystemObjectReviser performs revision of all system object types and relations, so after Migration
// objects installed in space should correspond to bundled objects from library.
// To modify relations of system objects relation revision should be incremented in types.json or relations.json
// For more info see 'System Objects Update' section of docs/Flow.md
type Migration struct{}

func (Migration) Name() string {
	return MName
}

func (Migration) Run(ctx context.Context, log logger.CtxLogger, store dependencies.QueryableStore, space dependencies.SpaceWithCtx) (toMigrate, migrated int, err error) {
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

func listAllTypesAndRelations(store dependencies.QueryableStore) (map[string]*types.Struct, error) {
	records, err := store.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.IntList(int(model.ObjectType_objectType), int(model.ObjectType_relation)),
			},
		},
	})
	if err != nil {
		return nil, err
	}

	details := make(map[string]*types.Struct, len(records))
	for _, record := range records {
		id := pbtypes.GetString(record.Details, bundle.RelationKeyId.String())
		details[id] = record.Details
	}
	return details, nil
}

func reviseObject(ctx context.Context, log logger.CtxLogger, space dependencies.SpaceWithCtx, localObject *types.Struct) (toRevise bool, err error) {
	uniqueKeyRaw := pbtypes.GetString(localObject, bundle.RelationKeyUniqueKey.String())

	uk, err := domain.UnmarshalUniqueKey(uniqueKeyRaw)
	if err != nil {
		return false, fmt.Errorf("failed to unmarshal unique key '%s': %w", uniqueKeyRaw, err)
	}

	bundleObject := getBundleSystemObjectDetails(uk)
	if bundleObject == nil {
		return false, nil
	}

	if pbtypes.GetInt64(bundleObject, revisionKey) <= pbtypes.GetInt64(localObject, revisionKey) {
		return false, nil
	}
	details := buildDiffDetails(bundleObject, localObject)

	recRelsDetail, err := checkRecommendedRelations(ctx, space, bundleObject, localObject)
	if err != nil {
		log.Error("failed to check recommended relations", zap.Error(err))
	}

	if recRelsDetail != nil {
		details = append(details, recRelsDetail)
	}

	if len(details) != 0 {
		log.Debug("updating system object", zap.String("key", uk.InternalKey()), zap.String("space", space.Id()))
		if err := space.DoCtx(ctx, pbtypes.GetString(localObject, bundle.RelationKeyId.String()), func(sb smartblock.SmartBlock) error {
			if ds, ok := sb.(detailsSettable); ok {
				return ds.SetDetails(nil, details, false)
			}
			return nil
		}); err != nil {
			return true, fmt.Errorf("failed to update system object '%s' in space '%s': %w", uk.InternalKey(), space.Id(), err)
		}
	}
	return true, nil
}

// getBundleSystemObjectDetails returns nil if the object with provided unique key is not either system relation or system type
func getBundleSystemObjectDetails(uk domain.UniqueKey) *types.Struct {
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
		return (&relationutils.Relation{Relation: relation}).ToStruct()
	default:
		return nil
	}
}

func buildDiffDetails(origin, current *types.Struct) (details []*model.Detail) {
	diff := pbtypes.StructDiff(current, origin)
	diff = pbtypes.StructFilterKeys(diff, []string{
		bundle.RelationKeyName.String(), bundle.RelationKeyDescription.String(),
		bundle.RelationKeyIsReadonly.String(), bundle.RelationKeyIsHidden.String(),
		bundle.RelationKeyRevision.String(), bundle.RelationKeyRelationReadonlyValue.String(),
		bundle.RelationKeyRelationMaxCount.String(), bundle.RelationKeyTargetObjectType.String(),
		bundle.RelationKeyIconEmoji.String(),
	})

	for key, value := range diff.Fields {
		if key == bundle.RelationKeyTargetObjectType.String() {
			// special case. We don't want to remove the types that was set by user, so only add ones that we have
			currentList := pbtypes.GetStringList(current, bundle.RelationKeyTargetObjectType.String())
			missedInCurrent, _ := lo.Difference(pbtypes.GetStringList(origin, bundle.RelationKeyTargetObjectType.String()), currentList)
			currentList = append(currentList, missedInCurrent...)
			value = pbtypes.StringList(currentList)
		}
		details = append(details, &model.Detail{Key: key, Value: value})
	}
	return
}

func checkRecommendedRelations(ctx context.Context, space dependencies.SpaceWithCtx, origin, current *types.Struct) (newValue *model.Detail, err error) {
	localIds := pbtypes.GetStringList(current, bundle.RelationKeyRecommendedRelations.String())
	bundledIds := pbtypes.GetStringList(origin, bundle.RelationKeyRecommendedRelations.String())

	newIds := make([]string, 0, len(bundledIds))
	for _, bundledId := range bundledIds {
		if !strings.HasPrefix(bundledId, addr.BundledRelationURLPrefix) {
			return nil, fmt.Errorf("invalid recommended bundled relation id: %s. %s prefix is expected",
				bundledId, addr.BundledRelationURLPrefix)
		}
		key := strings.TrimPrefix(bundledId, addr.BundledRelationURLPrefix)
		uk, err := domain.NewUniqueKey(coresb.SmartBlockTypeRelation, key)
		if err != nil {
			return nil, err
		}

		// we should add only system relations to object types, because non-system could be not installed to space yet
		if !lo.Contains(bundle.SystemRelations, domain.RelationKey(uk.InternalKey())) {
			log.Debug("recommended relation is not system, so we are not adding it to the type object", zap.String("relation key", key))
			continue
		}

		id, err := space.DeriveObjectID(ctx, uk)
		if err != nil {
			return nil, fmt.Errorf("failed to derive recommended relation with key '%s': %w", key, err)
		}

		newIds = append(newIds, id)
	}

	_, added := slice.DifferenceRemovedAdded(localIds, newIds)
	if len(added) == 0 {
		return nil, nil
	}

	return &model.Detail{
		Key:   bundle.RelationKeyRecommendedRelations.String(),
		Value: pbtypes.StringList(append(localIds, added...)),
	}, nil
}
