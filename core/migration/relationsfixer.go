package migration

import (
	"context"
	"fmt"

	"github.com/hashicorp/go-multierror"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type readonlyRelationsFixer struct{}

func (readonlyRelationsFixer) Name() string {
	return "ReadonlyRelationsFixer"
}

func (readonlyRelationsFixer) Run(ctx context.Context, store storeWithCtx, space spaceWithCtx) (toMigrate, migrated int, err error) {
	spaceId := space.Id()

	relations, err := listReadonlyTagAndStatusRelations(ctx, store, spaceId)
	toMigrate = len(relations)

	if err != nil {
		return toMigrate, 0, fmt.Errorf("failed to list all relations with tag and status format in space %s: %w", spaceId, err)
	}

	if toMigrate != 0 {
		log.Infof("space %s contains %d relations of tag and status format with relationReadonlyValue=true", spaceId, toMigrate)
	}

	for _, r := range relations {
		var (
			name = pbtypes.GetString(r.Details, bundle.RelationKeyName.String())
			uk   = pbtypes.GetString(r.Details, bundle.RelationKeyUniqueKey.String())
		)

		format := model.RelationFormat_name[int32(pbtypes.GetInt64(r.Details, bundle.RelationKeyRelationFormat.String()))]
		log.Infof("setting relationReadonlyValue to FALSE for relation %s (uniqueKey='%s', format='%s')", name, uk, format)

		det := []*model.Detail{{
			Key:   bundle.RelationKeyRelationReadonlyValue.String(),
			Value: pbtypes.Bool(false),
		}}
		e := space.DoCtx(ctx, pbtypes.GetString(r.Details, bundle.RelationKeyId.String()), func(sb smartblock.SmartBlock) error {
			if ds, ok := sb.(basic.DetailsSettable); ok {
				return ds.SetDetails(nil, det, false)
			}
			return nil
		})
		if e != nil {
			err = multierror.Append(err, fmt.Errorf("failed to set readOnlyValue=true to relation %s in space %s: %w", uk, spaceId, e))
		} else {
			migrated++
		}
	}
	return
}

func listReadonlyTagAndStatusRelations(ctx context.Context, store storeWithCtx, spaceId string) ([]database.Record, error) {
	return store.QueryWithContext(ctx, database.Query{Filters: []*model.BlockContentDataviewFilter{
		{
			RelationKey: bundle.RelationKeyRelationFormat.String(),
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       pbtypes.IntList(int(model.RelationFormat_status), int(model.RelationFormat_tag)),
		},
		{
			RelationKey: bundle.RelationKeySpaceId.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.String(spaceId),
		},
		{
			RelationKey: bundle.RelationKeyRelationReadonlyValue.String(),
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       pbtypes.Bool(true),
		},
	}})
}
