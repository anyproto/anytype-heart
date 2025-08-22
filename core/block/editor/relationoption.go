package editor

import (
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/order"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/block/source"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var relationOptionRequiredRelations = []domain.RelationKey{
	bundle.RelationKeyApiObjectKey,
}

type RelationOption struct {
	smartblock.SmartBlock
	basic.AllOperations
	basic.IHistory
	source.ChangeReceiver
	order.OrderSettable
}

func (f *ObjectFactory) newRelationOption(spaceId string, sb smartblock.SmartBlock) *RelationOption {
	store := f.objectStore.SpaceIndex(spaceId)
	return &RelationOption{
		SmartBlock:     sb,
		ChangeReceiver: sb.(source.ChangeReceiver),
		AllOperations:  basic.NewBasic(sb, store, f.layoutConverter, f.fileObjectService),
		IHistory:       basic.NewHistory(sb),
		OrderSettable:  order.NewOrderSettable(sb, bundle.RelationKeyRelationOptionOrder),
	}
}

func (ro *RelationOption) Init(ctx *smartblock.InitContext) (err error) {
	ctx.RequiredInternalRelationKeys = append(ctx.RequiredInternalRelationKeys, relationOptionRequiredRelations...)

	if err = ro.SmartBlock.Init(ctx); err != nil {
		return
	}
	return nil
}

func (ro *RelationOption) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 1,
		Proc: func(s *state.State) {
			template.InitTemplate(s,
				template.WithEmpty,
				template.WithObjectTypes([]domain.TypeKey{bundle.TypeKeyRelationOption}),
				template.WithTitle,
				template.WithLayout(model.ObjectType_relationOption),
			)
		},
	}
}

func (ro *RelationOption) StateMigrations() migration.Migrations {
	return migration.MakeMigrations([]migration.Migration{
		{
			Version: 1,
			Proc:    func(s *state.State) {},
		},
	})
}
