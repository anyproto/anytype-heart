package block

import (
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/simple"
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
