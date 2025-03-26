package block

import (
	"context"
	"fmt"
	"slices"

	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/basic"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/widget"
	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/domain"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/bundle"
	"github.com/anyproto/anytype-heart/pkg/lib/database"
	"github.com/anyproto/anytype-heart/pkg/lib/localstore/addr"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

var skippedTypesForAutoWidget = []domain.TypeKey{
	bundle.TypeKeyTemplate,
	bundle.TypeKeyObjectType,
	bundle.TypeKeyDate,
	bundle.TypeKeyRelation,
	bundle.TypeKeyRelationOption,
	bundle.TypeKeyDashboard,
	bundle.TypeKeyChatDerived,
}

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
	if slices.Contains(skippedTypesForAutoWidget, key) {
		return nil
	}
	space, err := s.spaceService.Get(ctx, spaceId)
	if err != nil {
		return err
	}
	typeId, err := space.GetTypeIdByKey(ctx, key)
	if err != nil {
		return err
	}
	widgetObjectId := space.DerivedIDs().Widgets
	widgetDetails, err := s.objectStore.SpaceIndex(space.Id()).GetDetails(widgetObjectId)
	if err == nil {
		keys := widgetDetails.Get(bundle.RelationKeyAutoWidgetTargets).StringList()
		if slices.Contains(keys, typeId) {
			// widget was created before
			return nil
		}
	}
	// this is not optimal, maybe it should be some cheaper way
	records, err := s.objectStore.SpaceIndex(space.Id()).QueryRaw(&database.Filters{FilterObj: database.FiltersAnd{
		database.FilterEq{
			Key:   bundle.RelationKeyType,
			Cond:  model.BlockContentDataviewFilter_Equal,
			Value: domain.String(typeId),
		},
		database.FilterEq{
			Key:   bundle.RelationKeyIsHiddenDiscovery,
			Cond:  model.BlockContentDataviewFilter_NotEqual,
			Value: domain.Bool(true),
		},
		database.FilterEq{
			Key:   bundle.RelationKeyOrigin,
			Cond:  model.BlockContentDataviewFilter_NotEqual,
			Value: domain.Int64(model.ObjectOrigin_usecase),
		},
	}}, 1, 1)
	if err != nil {
		log.Warnf("failed to query records for type '%s' in space '%s': %v", key, spaceId, err)
	}
	if len(records) > 0 {
		// only create widget if this was the first object of this type created
		return nil
	}
	return cache.DoState(s, widgetObjectId, func(st *state.State, w widget.Widget) (err error) {
		return w.AddAutoWidget(st, typeId, key.String(), addr.ObjectTypeAllViewId, model.BlockContentWidget_View)
	})
	return err
}
