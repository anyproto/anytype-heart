package profilemigrator

import (
	"context"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	coresb "github.com/anyproto/anytype-heart/pkg/lib/core/smartblock"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const MName = "common.migration.profilemigrator"

type Migration struct {
	TechSpace techspace.TechSpace
}

func (m Migration) Run(ctx context.Context, logger logger.CtxLogger, store dependencies.QueryableStore, space dependencies.SpaceWithCtx) (toMigrate, migrated int, err error) {
	uniqueKey, err := domain.NewUniqueKey(coresb.SmartBlockTypeProfilePage, "")
	if err != nil {
		return
	}
	profileObjectId, err := space.DeriveObjectID(ctx, uniqueKey)
	if err != nil {
		return
	}
	// TODO: [PS] add icon image migration
	var details *types.Struct
	err = space.DoCtx(ctx, profileObjectId, func(sb smartblock.SmartBlock) error {
		details = pbtypes.CopyStructFields(sb.CombinedDetails(),
			bundle.RelationKeyName.String(),
			bundle.RelationKeyDescription.String(),
			bundle.RelationKeyIconImage.String(),
			bundle.RelationKeyGlobalName.String())
		return nil
	})
	if err != nil {
		return
	}
	err = m.TechSpace.DoAccountObject(ctx, func(accountObject techspace.AccountObject) error {
		if accountObject.CombinedDetails().GetFields()[bundle.RelationKeyName.String()].GetStringValue() != "" {
			return nil
		}
		return accountObject.SetProfileDetails(details)
	})
	return
}

func (m Migration) Name() string {
	return MName
}
