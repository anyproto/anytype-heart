package editor

import (
	"errors"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
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
}

func (f *ObjectFactory) newRelationOption(spaceId string, sb smartblock.SmartBlock) *RelationOption {
	store := f.objectStore.SpaceIndex(spaceId)
	return &RelationOption{
		SmartBlock:     sb,
		ChangeReceiver: sb.(source.ChangeReceiver),
		AllOperations:  basic.NewBasic(sb, store, f.layoutConverter, f.fileObjectService),
		IHistory:       basic.NewHistory(sb),
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

func (ro *RelationOption) SetOrder(previousOrderId string) (string, error) {
	st := ro.NewState()
	var orderId string
	if previousOrderId == "" {
		// For the first element, use a lexid with huge padding
		orderId = lx.Middle()
	} else {
		orderId = lx.Next(previousOrderId)
	}
	st.SetDetail(bundle.RelationKeyOptionOrder, domain.String(orderId))
	return orderId, ro.Apply(st)
}

func (ro *RelationOption) SetAfterOrder(orderId string) error {
	st := ro.NewState()
	currentOrderId := st.Details().GetString(bundle.RelationKeyOptionOrder)
	if orderId > currentOrderId {
		currentOrderId = lx.Next(orderId)
		st.SetDetail(bundle.RelationKeyOptionOrder, domain.String(currentOrderId))
		return ro.Apply(st)
	}
	return nil
}

func (ro *RelationOption) SetBetweenOrders(previousOrderId, afterOrderId string) error {
	st := ro.NewState()
	var before string
	var err error

	if previousOrderId == "" {
		// Insert before the first existing element
		before = lx.Prev(afterOrderId)
	} else {
		// Insert between two existing elements
		before, err = lx.NextBefore(previousOrderId, afterOrderId)
	}

	if err != nil {
		return errors.Join(ErrLexidInsertionFailed, err)
	}
	st.SetDetail(bundle.RelationKeyOptionOrder, domain.String(before))
	return ro.Apply(st)
}

func (ro *RelationOption) UnsetOrder() error {
	st := ro.NewState()
	st.RemoveDetail(bundle.RelationKeyOptionOrder)
	return ro.Apply(st)
}

func (ro *RelationOption) GetOrder() string {
	return ro.Details().GetString(bundle.RelationKeyOptionOrder)
}
