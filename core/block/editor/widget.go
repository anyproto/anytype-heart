package editor

import (
	"fmt"
	
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/basic"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/widget"
	"github.com/anytypeio/go-anytype-middleware/core/session"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/bundle"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

const BlockAdditionError = "failed to add widget '%s': %w"

type WidgetObject struct {
	smartblock.SmartBlock
	basic.IHistory
	basic.Movable
	basic.Unlinkable
	basic.Updatable
	widget.Widget
}

func NewWidgetObject() *WidgetObject {
	sb := smartblock.New()
	bs := basic.NewBasic(sb)
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

	defaultWidgetBlocks := []string{
		widget.DefaultWidgetFavorite,
		widget.DefaultWidgetSet,
		widget.DefaultWidgetRecent,
	}

	for _, id := range defaultWidgetBlocks {
		if !w.isLinkBlockIncluded(ctx.State, id) {
			if _, err = w.CreateBlock(ctx.State, &pb.RpcBlockCreateWidgetRequest{
				TargetId:     "",
				Position:     model.Block_Bottom,
				WidgetLayout: widget.LayoutList,
				Block: &model.Block{
					Id:          "",
					ChildrenIds: nil,
					Content: &model.BlockContentOfLink{
						Link: &model.BlockContentLink{
							TargetBlockId: id,
							Style:         model.BlockContentLink_Page,
							IconSize:      model.BlockContentLink_SizeNone,
							CardStyle:     model.BlockContentLink_Text,
							Description:   model.BlockContentLink_None,
						},
					},
				},
			}); err != nil {
				return fmt.Errorf(BlockAdditionError, widget.DefaultWidgetFavorite, err)
			}
		}
	}

	return smartblock.ObjectApplyTemplate(w, ctx.State,
		template.WithEmpty,
		template.WithObjectTypesAndLayout([]string{bundle.TypeKeyDashboard.URL()}, model.ObjectType_basic),
	)
}

func (w *WidgetObject) Unlink(ctx *session.Context, ids ...string) (err error) {
	st := w.NewStateCtx(ctx)
	for _, id := range ids {
		if p := st.PickParentOf(id); p != nil && p.Model().GetWidget() != nil {
			st.Unlink(p.Model().Id)
		}
		st.Unlink(id)
	}
	return w.Apply(st)
}

func (w *WidgetObject) isLinkBlockIncluded(s *state.State, id string) bool {
	for _, b := range s.Blocks() {
		if link, ok := b.Content.(*model.BlockContentOfLink); ok {
			if link.Link.TargetBlockId == id {
				return true
			}
		}
	}
	return false
}
