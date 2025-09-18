package editor

import (
	"context"
	"errors"

	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/converter"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/migration"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
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
	basic.DetailsSettable
}

func NewWidgetObject(
	sb smartblock.SmartBlock,
	objectStore spaceindex.Store,
	layoutConverter converter.LayoutConverter,
) *WidgetObject {
	bs := basic.NewBasic(sb, objectStore, layoutConverter, nil)
	return &WidgetObject{
		SmartBlock:      sb,
		Movable:         bs,
		Updatable:       bs,
		DetailsSettable: bs,
		IHistory:        basic.NewHistory(sb),
		Widget:          widget.NewWidget(sb),
	}
}

func (w *WidgetObject) Init(ctx *smartblock.InitContext) (err error) {
	if err = w.SmartBlock.Init(ctx); err != nil {
		return
	}

	// cleanup broken
	var removeIds []string
	_ = ctx.State.Iterate(func(b simple.Block) (isContinue bool) {
		if wc, ok := b.Model().Content.(*model.BlockContentOfLink); ok {
			if wc.Link.TargetBlockId == addr.MissingObject {
				removeIds = append(removeIds, b.Model().Id)
				return true
			}
		}
		return true
	})

	if len(removeIds) > 0 {
		// we need to avoid these situations, so lets log it
		log.Warnf("widget: removing %d broken links", len(removeIds))
	}
	for _, id := range removeIds {
		ctx.State.Unlink(id)
	}
	// now remove empty widget wrappers
	removeIds = removeIds[:0]
	_ = ctx.State.Iterate(func(b simple.Block) (isContinue bool) {
		if _, ok := b.Model().Content.(*model.BlockContentOfWidget); ok {
			if len(b.Model().GetChildrenIds()) == 0 {
				removeIds = append(removeIds, b.Model().Id)
				return true
			}
		}
		return true
	})
	if len(removeIds) > 0 {
		log.Warnf("widget: removing %d empty wrappers", len(removeIds))
	}
	for _, id := range removeIds {
		ctx.State.Unlink(id)
	}
	return nil
}

func (w *WidgetObject) CreationStateMigration(ctx *smartblock.InitContext) migration.Migration {
	return migration.Migration{
		Version: 3,
		Proc: func(st *state.State) {
			// we purposefully do not add the ALl Objects widget here(as in migration3), because for new users we don't want to auto-create it
			template.InitTemplate(st,
				template.WithEmpty,
				template.WithObjectTypes([]domain.TypeKey{bundle.TypeKeyDashboard}),
				template.WithLayout(model.ObjectType_dashboard),
				template.WithDetail(bundle.RelationKeyIsHidden, domain.Bool(true)),
			)
		},
	}
}

func replaceWidgetTarget(st *state.State, targetFrom string, targetTo string, viewId string, layout model.BlockContentWidgetLayout) {
	st.Iterate(func(b simple.Block) (isContinue bool) {
		if wc, ok := b.Model().Content.(*model.BlockContentOfWidget); ok {
			// get child
			if len(b.Model().GetChildrenIds()) > 0 {
				child := st.Get(b.Model().GetChildrenIds()[0])
				childBlock := st.Get(child.Model().Id)
				if linkBlock, ok := childBlock.Model().Content.(*model.BlockContentOfLink); ok {
					if linkBlock.Link.TargetBlockId == targetFrom {
						linkBlock.Link.TargetBlockId = targetTo
						wc.Widget.ViewId = viewId
						wc.Widget.Layout = layout
						return false
					}
				}
			}
		}
		return true
	})
}
func (w *WidgetObject) StateMigrations() migration.Migrations {
	return migration.MakeMigrations([]migration.Migration{
		{
			Version: 2,
			Proc: func(s *state.State) {
				spc := w.Space()
				setTypeId, err := spc.GetTypeIdByKey(context.Background(), bundle.TypeKeySet)
				if err != nil {
					return
				}
				collectionTypeId, err := spc.GetTypeIdByKey(context.Background(), bundle.TypeKeyCollection)
				if err != nil {
					return
				}
				replaceWidgetTarget(s, widget.DefaultWidgetCollection, collectionTypeId, addr.ObjectTypeAllViewId, model.BlockContentWidget_View)
				replaceWidgetTarget(s, widget.DefaultWidgetSet, setTypeId, addr.ObjectTypeAllViewId, model.BlockContentWidget_View)

			},
		},
		{
			Version: 3,
			Proc: func(s *state.State) {
				// add All Objects widget for existing spaces
				_, err := w.CreateBlock(s, &pb.RpcBlockCreateWidgetRequest{
					ContextId:    s.RootId(),
					WidgetLayout: model.BlockContentWidget_Link,
					Position:     model.Block_InnerFirst,
					TargetId:     s.RootId(),
					ViewId:       "",
					Block: &model.Block{
						Id: widget.DefaultWidgetAll, // this is correct, to avoid collisions when applied on many devices
						Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
							TargetBlockId: widget.DefaultWidgetAll,
						}},
					},
				})
				if errors.Is(err, widget.ErrWidgetAlreadyExists) {
					return
				}
				if err != nil {
					log.Warnf("all objects migration failed: %s", err.Error())
				}
			},
		},
	},
	)
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
