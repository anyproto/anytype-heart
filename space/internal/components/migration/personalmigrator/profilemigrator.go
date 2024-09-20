package personalmigrator

import (
	"context"

	"github.com/anyproto/any-sync/app/logger"
	"github.com/gogo/protobuf/types"

	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/space/internal/components/dependencies"
	"github.com/anyproto/anytype-heart/space/techspace"
	"github.com/anyproto/anytype-heart/util/pbtypes"
)

const MName = "common.migration.personalmigrator"

type Migration struct {
	TechSpace techspace.TechSpace
}

func (m Migration) Run(ctx context.Context, logger logger.CtxLogger, store dependencies.QueryableStore, space dependencies.SpaceWithCtx) (toMigrate, migrated int, err error) {
	ids := space.DerivedIDs()
	var details *types.Struct
	err = space.DoCtx(ctx, ids.Profile, func(sb smartblock.SmartBlock) error {
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
	var analyticsId string
	err = space.DoCtx(ctx, ids.Workspace, func(sb smartblock.SmartBlock) error {
		analyticsId = sb.NewState().GetSetting(state.SettingsAnalyticsId).GetStringValue()
		return nil
	})
	if err != nil {
		return
	}
	err = m.TechSpace.DoAccountObject(ctx, func(accountObject techspace.AccountObject) error {
		if accountObject.CombinedDetails().GetFields()[bundle.RelationKeyName.String()].GetStringValue() != "" {
			return nil
		}
		err = accountObject.SetAnalyticsId(analyticsId)
		if err != nil {
			return err
		}
		return accountObject.SetProfileDetails(details)
	})
	return
}

func (m Migration) Name() string {
	return MName
}
