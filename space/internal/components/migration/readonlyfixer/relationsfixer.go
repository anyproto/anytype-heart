package readonlyfixer

import (
	"context"
	"errors"
	"fmt"

	"github.com/anyproto/any-sync/app/logger"
	"go.uber.org/zap"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

type detailsSettable interface {
	SetDetails(ctx session.Context, details []*model.Detail, showEvent bool) (err error)
}

const MName = "ReadonlyRelationsFixer"

// Migration ReadonlyRelationsFixer performs setting readOnlyValue relation to true for all relations with Status and Tag format
// This migration was implemented to fix relations in accounts of users that are not able to modify its value (GO-2331)
type Migration struct{}

func (Migration) Name() string {
	return MName
}

func (Migration) Run(ctx context.Context, log logger.CtxLogger, store, _ dependencies.QueryableStore, space dependencies.SpaceWithCtx) (toMigrate, migrated int, err error) {
	spaceId := space.Id()

	relations, err := listReadonlyTagAndStatusRelations(store, spaceId)
	toMigrate = len(relations)

	if err != nil {
		return toMigrate, 0, fmt.Errorf("failed to list all relations with tag and status format in space %s: %w", spaceId, err)
	}

	if toMigrate != 0 {
		log.Debug(fmt.Sprintf("space %s contains %d relations of tag and status format with relationReadonlyValue=true", spaceId, toMigrate), zap.String("migration", MName))
	}

	for _, r := range relations {
		var (
			name = r.Details.GetString(bundle.RelationKeyName)
			uk   = r.Details.GetString(bundle.RelationKeyUniqueKey)
		)

		format := model.RelationFormat_name[int32(r.Details.GetInt64(bundle.RelationKeyRelationFormat))]
		log.Debug("setting relationReadonlyValue to FALSE for relation", zap.String("name", name), zap.String("uniqueKey", uk), zap.String("format", format), zap.String("migration", MName))

		det := []*model.Detail{{
			Key:   bundle.RelationKeyRelationReadonlyValue.String(),
			Value: pbtypes.Bool(false),
		}}
		e := space.DoCtx(ctx, r.Details.GetString(bundle.RelationKeyId), func(sb smartblock.SmartBlock) error {
			if ds, ok := sb.(detailsSettable); ok {
				return ds.SetDetails(nil, det, false)
			}
			return nil
		})
		if e != nil {
			err = errors.Join(err, fmt.Errorf("failed to set readOnlyValue=true to relation %s in space %s: %w", uk, spaceId, e))
		} else {
			migrated++
		}
	}
	return
}

func listReadonlyTagAndStatusRelations(store dependencies.QueryableStore, spaceId string) ([]database.Record, error) {
	return store.Query(database.Query{Filters: []database.FilterRequest{
		{
			RelationKey: bundle.RelationKeyRelationFormat,
			Condition:   model.BlockContentDataviewFilter_In,
			Value:       domain.Int64List([]model.RelationFormat{model.RelationFormat_status, model.RelationFormat_tag}),
		},
		{
			RelationKey: bundle.RelationKeyRelationReadonlyValue,
			Condition:   model.BlockContentDataviewFilter_Equal,
			Value:       domain.Bool(true),
		},
	}})
}
