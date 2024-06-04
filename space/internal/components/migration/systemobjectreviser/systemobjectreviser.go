package systemobjectreviser

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/gogo/protobuf/types"
	"github.com/samber/lo"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

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
	spaceObjects, err := listAllTypesAndRelations(store, space.Id())
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get relations and types from client space: %w", err)
	}

	marketObjects, err := listAllTypesAndRelations(store, addr.AnytypeMarketplaceWorkspace)
	if err != nil {
		return 0, 0, fmt.Errorf("failed to get relations from marketplace space: %w", err)
	}

	for _, details := range spaceObjects {
		shouldBeRevised, e := reviseSystemObject(ctx, log, space, details, marketObjects)
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

func listAllTypesAndRelations(store dependencies.QueryableStore, spaceId string) (map[string]*types.Struct, error) {
	records, err := store.Query(database.Query{
		Filters: []*model.BlockContentDataviewFilter{
			{
				RelationKey: bundle.RelationKeyLayout.String(),
				Condition:   model.BlockContentDataviewFilter_In,
				Value:       pbtypes.IntList(int(model.ObjectType_objectType), int(model.ObjectType_relation)),
			},
			{
				RelationKey: bundle.RelationKeySpaceId.String(),
				Condition:   model.BlockContentDataviewFilter_Equal,
				Value:       pbtypes.String(spaceId),
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

func reviseSystemObject(ctx context.Context, log logger.CtxLogger, space dependencies.SpaceWithCtx, localObject *types.Struct, marketObjects map[string]*types.Struct) (toRevise bool, err error) {
	source := pbtypes.GetString(localObject, bundle.RelationKeySourceObject.String())
	marketObject, found := marketObjects[source]
	if !found || !isSystemObject(localObject) || pbtypes.GetInt64(marketObject, revisionKey) <= pbtypes.GetInt64(localObject, revisionKey) {
		return false, nil
	}
	details := buildDiffDetails(marketObject, localObject)
	if len(details) != 0 {
		log.Debug("updating system object", zap.String("source", source), zap.String("space", space.Id()))
		if err := space.DoCtx(ctx, pbtypes.GetString(localObject, bundle.RelationKeyId.String()), func(sb smartblock.SmartBlock) error {
			if ds, ok := sb.(basic.DetailsSettable); ok {
				return ds.SetDetails(nil, details, false)
			}
			return nil
		}); err != nil {
			return true, fmt.Errorf("failed to update system object %s in space %s: %w", source, space.Id(), err)
		}
	}
	return true, nil
}

func isSystemObject(details *types.Struct) bool {
	rawKey := pbtypes.GetString(details, bundle.RelationKeyUniqueKey.String())
	uk, err := domain.UnmarshalUniqueKey(rawKey)
	if err != nil {
		return false
	}
	switch uk.SmartblockType() {
	case coresb.SmartBlockTypeObjectType:
		return lo.Contains(bundle.SystemTypes, domain.TypeKey(uk.InternalKey()))
	case coresb.SmartBlockTypeRelation:
		return lo.Contains(bundle.SystemRelations, domain.RelationKey(uk.InternalKey()))
	}
	return false
}

func buildDiffDetails(origin, current *types.Struct) (details []*model.Detail) {
	diff := pbtypes.StructDiff(current, origin)
	diff = pbtypes.StructFilterKeys(diff, []string{
		bundle.RelationKeyName.String(), bundle.RelationKeyDescription.String(),
		bundle.RelationKeyIsReadonly.String(), bundle.RelationKeyIsHidden.String(),
		bundle.RelationKeyRevision.String(), bundle.RelationKeyRelationReadonlyValue.String(),
		bundle.RelationKeyRelationMaxCount.String(), bundle.RelationKeyTargetObjectType.String(),
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
