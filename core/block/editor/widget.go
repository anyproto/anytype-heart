package editor

import (
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/objectstore/spaceindex"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

type WidgetObject struct {
	smartblock.SmartBlock
	basic.IHistory
	basic.Movable
	basic.Unlinkable
	basic.Updatable
	widget.Widget
}

func NewWidgetObject(
	sb smartblock.SmartBlock,
	objectStore spaceindex.Store,
	layoutConverter converter.LayoutConverter,
) *WidgetObject {
	bs := basic.NewBasic(sb, objectStore, layoutConverter, nil, nil)
	return &WidgetObject{
		SmartBlock: sb,
		Movable:    bs,
		Updatable:  bs,
		IHistory:   basic.NewHistory(sb),
		Widget:     widget.NewWidget(sb),
	}
}

func (w *WidgetObject) Init(ctx *smartblock.InitContext) (err error) {
	if err = w.SmartBlock.Init(ctx); err != nil {
		return
	}

	return nil
}

func (w *WidgetObject) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 1,
		Proc: func(st *state.State) {
			template.InitTemplate(st,
				template.WithEmpty,
				template.WithObjectTypesAndLayout([]domain.TypeKey{bundle.TypeKeyDashboard}, model.ObjectType_dashboard),
				template.WithDetail(bundle.RelationKeyIsHidden, domain.Bool(true)),
			)
		},
	}
}

func (w *WidgetObject) StateMigrations() migration.Migrations {
	return migration.MakeMigrations(nil)
}

func (w *WidgetObject) Unlink(ctx session.Context, ids ...string) (err error) {
	st := w.NewStateCtx(ctx)
	for _, id := range ids {
		if p := st.PickParentOf(id); p != nil && p.Model().GetWidget() != nil {
			st.Unlink(p.Model().Id)
		}
		st.Unlink(id)
	}
	return w.Apply(st)
}
