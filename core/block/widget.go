package block

import (
	"context"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

func (s *Service) SetWidgetBlockTargetId(ctx session.Context, req *pb.RpcBlockWidgetSetTargetIdRequest) error {
	return cache.Do(s, req.ContextId, func(b smartblock.SmartBlock) error {
		st := b.NewStateCtx(ctx)
		root := st.Get(req.BlockId)
		if root == nil {
			return fmt.Errorf("failed to find block '%s' in widget object", req.BlockId)
		}
		if len(root.Model().ChildrenIds) == 0 {
			return fmt.Errorf("failed to get child block of widget block '%s' as ChildrenIds is empty", req.BlockId)
		}
		link := st.Get(root.Model().ChildrenIds[0])
		if lc, ok := link.Model().Content.(*model.BlockContentOfLink); ok {
			lc.Link.TargetBlockId = req.TargetId
			return b.Apply(st)
		}
		return fmt.Errorf("failed to update target block of widget block '%s' as its child is not link", req.BlockId)
	})
}

func (s *Service) SetWidgetBlockLayout(ctx session.Context, req *pb.RpcBlockWidgetSetLayoutRequest) error {
	return cache.Do(s, req.ContextId, func(b basic.Updatable) error {
		return b.Update(ctx, func(b simple.Block) error {
			if wc, ok := b.Model().Content.(*model.BlockContentOfWidget); ok {
				wc.Widget.Layout = req.Layout
			}
			return nil
		}, req.BlockId)
	})
}

func (s *Service) SetWidgetBlockLimit(ctx session.Context, req *pb.RpcBlockWidgetSetLimitRequest) error {
	return cache.Do(s, req.ContextId, func(b basic.Updatable) error {
		return b.Update(ctx, func(b simple.Block) error {
			if wc, ok := b.Model().Content.(*model.BlockContentOfWidget); ok {
				wc.Widget.Limit = req.Limit
			}
			return nil
		}, req.BlockId)
	})
}

func (s *Service) SetWidgetBlockViewId(ctx session.Context, req *pb.RpcBlockWidgetSetViewIdRequest) error {
	return cache.Do(s, req.ContextId, func(b basic.Updatable) error {
		return b.Update(ctx, func(b simple.Block) error {
			if wc, ok := b.Model().Content.(*model.BlockContentOfWidget); ok {
				wc.Widget.ViewId = req.ViewId
			}
			return nil
		}, req.BlockId)
	})
}

func (s *Service) CreateTypeWidgetIfMissing(ctx context.Context, spaceId string, key domain.TypeKey) error {
	space, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return err
	}
	widgetObjectId := space.DerivedIDs().Widgets
	typeId, err := space.GetTypeIdByKey(ctx, key)
	if err != nil {
		return err
	}
	widgetBlockId := "type_" + key.String()
	return cache.DoState(s, widgetObjectId, func(st *state.State, w widget.Widget) (err error) {
		var typeBlockAlreadyExists bool

		err = st.Iterate(func(b simple.Block) (isContinue bool) {
			link := b.Model().GetLink()
			if link == nil {
				return true
			}
			if link.TargetBlockId == typeId {
				// check by targetBlockId in case user created the same block manually
				typeBlockAlreadyExists = true
				return false
			}
			return true
		})

		if err != nil {
			return err
		}
		if typeBlockAlreadyExists {
			log.Debug("favorite widget block is already presented")
			return nil
		}

		_, err = w.CreateBlock(st, &pb.RpcBlockCreateWidgetRequest{
			ContextId:    widgetObjectId,
			ObjectLimit:  6,
			WidgetLayout: model.BlockContentWidget_View,
			Position:     model.Block_Bottom,
			Block: &model.Block{
				Id: widgetBlockId, // hardcode id to avoid duplicates
				Content: &model.BlockContentOfLink{Link: &model.BlockContentLink{
					TargetBlockId: typeId,
				}},
			},
		})
		return err
	})
}
