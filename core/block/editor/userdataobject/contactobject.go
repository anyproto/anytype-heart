package userdataobject

import (
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type ContactObject struct {
	basic.DetailsSettable
	smartblock.SmartBlock
}

func (co *ContactObject) Init(ctx *smartblock.InitContext) error {
	err := co.SmartBlock.Init(ctx)
	if err != nil {
		return err
	}
	template.InitTemplate(ctx.State,
		template.WithEmpty,
		template.WithDescription,
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
