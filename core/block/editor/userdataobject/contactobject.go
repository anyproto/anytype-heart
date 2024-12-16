package userdataobject

import (
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ContactObject struct {
	basic.DetailsSettable
	smartblock.SmartBlock
	store spaceindex.Store
}

func NewContactObject(smartBlock smartblock.SmartBlock, store spaceindex.Store) *ContactObject {
	return &ContactObject{SmartBlock: smartBlock, store: store}
}

func (co *ContactObject) Init(ctx *smartblock.InitContext) error {
	err := co.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}
	records, err := co.store.QueryByIds([]string{co.Id()})
	if err != nil {
		return err
	}
	if len(records) > 0 {
		ctx.State.SetDetails(records[0].Details)
	}
	template.InitTemplate(ctx.State,
		template.WithEmpty,
		template.WithTitle,
		template.WithForcedObjectTypes([]domain.TypeKey{bundle.TypeKeyContact}),
		template.WithLayout(model.ObjectType_contact))
	return nil
}

func (co *ContactObject) SetDetails(ctx session.Context, details []*model.Detail, showEvent bool) (err error) {
	state := co.NewState()
	for _, detail := range details {
		state.SetDetail(detail.Key, detail.Value)
	}
	return co.Apply(state)

}
