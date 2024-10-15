package dataviewservice

import (
	"fmt"

	"github.com/anyproto/any-sync/app"

	"github.com/anyproto/anytype-heart/core/block/cache"
	"github.com/anyproto/anytype-heart/core/block/editor/dataview"
	"github.com/anyproto/anytype-heart/core/block/editor/smartblock"
	"github.com/anyproto/anytype-heart/core/block/editor/state"
	"github.com/anyproto/anytype-heart/core/block/editor/template"
	"github.com/anyproto/anytype-heart/core/session"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
)

const CName = "dataview.service"

type Service interface {
	app.Component
	AddDataviewFilter(ctx session.Context, objectId, blockId, viewId string, filter *model.BlockContentDataviewFilter) error
	RemoveDataviewFilters(ctx session.Context, objectId, blockId, viewId string, filterIDs []string) error
	ReplaceDataviewFilter(ctx session.Context, objectId, blockId, viewId, filterID string, filter *model.BlockContentDataviewFilter) error
	ReorderDataviewFilters(ctx session.Context, objectId, blockId, viewId string, filterIDs []string) error

	AddDataviewSort(ctx session.Context, objectId, blockId, viewId string, sort *model.BlockContentDataviewSort) error
	RemoveDataviewSorts(ctx session.Context, objectId, blockId, viewId string, ids []string) error
	ReplaceDataviewSort(ctx session.Context, objectId, blockId, viewId, sortId string, sort *model.BlockContentDataviewSort) error
	ReorderDataviewSorts(ctx session.Context, objectId, blockId, viewId string, ids []string) error

	AddDataviewViewRelation(ctx session.Context, objectId, blockId, viewId string, relation *model.BlockContentDataviewRelation) error
	RemoveDataviewViewRelations(ctx session.Context, objectId, blockId, viewId string, relationKeys []string) error
	ReplaceDataviewViewRelation(ctx session.Context, objectId, blockId, viewId, relationKey string, relation *model.BlockContentDataviewRelation) error
	ReorderDataviewViewRelations(ctx session.Context, objectId, blockId, viewId string, relationKeys []string) error

	UpdateDataviewView(ctx session.Context, req pb.RpcBlockDataviewViewUpdateRequest) error
	UpdateDataviewGroupOrder(ctx session.Context, req pb.RpcBlockDataviewGroupOrderUpdateRequest) error
	UpdateDataviewObjectOrder(ctx session.Context, req pb.RpcBlockDataviewObjectOrderUpdateRequest) error
	DataviewMoveObjectsInView(ctx session.Context, req *pb.RpcBlockDataviewObjectOrderMoveRequest) error
	DeleteDataviewView(ctx session.Context, req pb.RpcBlockDataviewViewDeleteRequest) error
	SetDataviewActiveView(ctx session.Context, req pb.RpcBlockDataviewViewSetActiveRequest) error
	SetDataviewViewPosition(ctx session.Context, req pb.RpcBlockDataviewViewSetPositionRequest) error
	CreateDataviewView(ctx session.Context, req pb.RpcBlockDataviewViewCreateRequest) (id string, err error)

	AddDataviewRelation(ctx session.Context, req pb.RpcBlockDataviewRelationAddRequest) error
	DeleteDataviewRelation(ctx session.Context, req pb.RpcBlockDataviewRelationDeleteRequest) error
	SetDataviewSource(ctx session.Context, contextId, blockId string, source []string) error

	CopyDataviewToBlock(ctx session.Context, req *pb.RpcBlockDataviewCreateFromExistingObjectRequest) ([]*model.BlockContentDataviewView, error)

	SetSourceToSet(ctx session.Context, objectId string, source []string) error
}

func New() Service {
	return &service{}
}

type service struct {
	getter cache.ObjectGetter
}

func (s *service) Init(a *app.App) error {
	s.getter = app.MustComponent[cache.ObjectGetter](a)
	return nil
}

func (s *service) Name() string {
	return CName
}

func (s *service) AddDataviewFilter(
	ctx session.Context,
	objectId, blockId, viewId string,
	filter *model.BlockContentDataviewFilter,
) (err error) {
	return cache.DoStateCtx(s.getter, ctx, objectId, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockId)
		if err != nil {
			return err
		}

		return dv.AddFilter(viewId, filter)
	})
}

func (s *service) RemoveDataviewFilters(
	ctx session.Context,
	objectId, blockId, viewId string,
	filterIDs []string,
) (err error) {
	return cache.DoStateCtx(s.getter, ctx, objectId, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockId)
		if err != nil {
			return err
		}

		return dv.RemoveFilters(viewId, filterIDs)
	})
}

func (s *service) ReplaceDataviewFilter(
	ctx session.Context,
	objectId, blockId, viewId string,
	filterID string,
	filter *model.BlockContentDataviewFilter,
) (err error) {
	return cache.DoStateCtx(s.getter, ctx, objectId, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockId)
		if err != nil {
			return err
		}

		return dv.ReplaceFilter(viewId, filterID, filter)
	})
}

func (s *service) ReorderDataviewFilters(
	ctx session.Context,
	objectId, blockId, viewId string,
	filterIDs []string,
) (err error) {
	return cache.DoStateCtx(s.getter, ctx, objectId, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockId)
		if err != nil {
			return err
		}

		return dv.ReorderFilters(viewId, filterIDs)
	})
}

func (s *service) AddDataviewSort(
	ctx session.Context,
	objectId, blockId, viewId string,
	sort *model.BlockContentDataviewSort,
) (err error) {
	return cache.DoStateCtx(s.getter, ctx, objectId, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockId)
		if err != nil {
			return err
		}

		return dv.AddSort(viewId, sort)
	})
}

func (s *service) RemoveDataviewSorts(
	ctx session.Context,
	objectId, blockId, viewId string,
	ids []string,
) (err error) {
	return cache.DoStateCtx(s.getter, ctx, objectId, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockId)
		if err != nil {
			return err
		}

		return dv.RemoveSorts(viewId, ids)
	})
}

func (s *service) ReplaceDataviewSort(
	ctx session.Context,
	objectId, blockId, viewId string,
	id string,
	sort *model.BlockContentDataviewSort,
) (err error) {
	return cache.DoStateCtx(s.getter, ctx, objectId, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockId)
		if err != nil {
			return err
		}

		return dv.ReplaceSort(viewId, id, sort)
	})
}

func (s *service) ReorderDataviewSorts(
	ctx session.Context,
	objectId, blockId, viewId string,
	ids []string,
) (err error) {
	return cache.DoStateCtx(s.getter, ctx, objectId, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockId)
		if err != nil {
			return err
		}

		return dv.ReorderSorts(viewId, ids)
	})
}

func (s *service) AddDataviewViewRelation(
	ctx session.Context,
	objectId, blockId, viewId string,
	relation *model.BlockContentDataviewRelation,
) (err error) {
	return cache.DoStateCtx(s.getter, ctx, objectId, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockId)
		if err != nil {
			return err
		}

		return dv.AddViewRelation(viewId, relation)
	})
}

func (s *service) RemoveDataviewViewRelations(
	ctx session.Context,
	objectId, blockId, viewId string,
	relationKeys []string,
) (err error) {
	return cache.DoStateCtx(s.getter, ctx, objectId, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockId)
		if err != nil {
			return err
		}

		return dv.RemoveViewRelations(viewId, relationKeys)
	})
}

func (s *service) ReplaceDataviewViewRelation(
	ctx session.Context,
	objectId, blockId, viewId string,
	relationKey string,
	relation *model.BlockContentDataviewRelation,
) (err error) {
	return cache.DoStateCtx(s.getter, ctx, objectId, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockId)
		if err != nil {
			return err
		}

		return dv.ReplaceViewRelation(viewId, relationKey, relation)
	})
}

func (s *service) ReorderDataviewViewRelations(
	ctx session.Context,
	objectId, blockId, viewId string,
	relationKeys []string,
) (err error) {
	return cache.DoStateCtx(s.getter, ctx, objectId, func(s *state.State, d dataview.Dataview) error {
		dv, err := d.GetDataviewBlock(s, blockId)
		if err != nil {
			return err
		}

		return dv.ReorderViewRelations(viewId, relationKeys)
	})
}

func (s *service) UpdateDataviewView(ctx session.Context, req pb.RpcBlockDataviewViewUpdateRequest) error {
	return cache.Do(s.getter, req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateView(ctx, req.BlockId, req.ViewId, req.View, true)
	})
}

func (s *service) UpdateDataviewGroupOrder(ctx session.Context, req pb.RpcBlockDataviewGroupOrderUpdateRequest) error {
	return cache.Do(s.getter, req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateViewGroupOrder(ctx, req.BlockId, req.GroupOrder)
	})
}

func (s *service) UpdateDataviewObjectOrder(
	ctx session.Context, req pb.RpcBlockDataviewObjectOrderUpdateRequest,
) error {
	return cache.Do(s.getter, req.ContextId, func(b dataview.Dataview) error {
		return b.UpdateViewObjectOrder(ctx, req.BlockId, req.ObjectOrders)
	})
}

func (s *service) DataviewMoveObjectsInView(
	ctx session.Context, req *pb.RpcBlockDataviewObjectOrderMoveRequest,
) error {
	return cache.Do(s.getter, req.ContextId, func(b dataview.Dataview) error {
		return b.DataviewMoveObjectsInView(ctx, req)
	})
}

func (s *service) DeleteDataviewView(ctx session.Context, req pb.RpcBlockDataviewViewDeleteRequest) error {
	return cache.Do(s.getter, req.ContextId, func(b dataview.Dataview) error {
		return b.DeleteView(ctx, req.BlockId, req.ViewId, true)
	})
}

func (s *service) SetDataviewActiveView(ctx session.Context, req pb.RpcBlockDataviewViewSetActiveRequest) error {
	return cache.Do(s.getter, req.ContextId, func(b dataview.Dataview) error {
		return b.SetActiveView(ctx, req.BlockId, req.ViewId)
	})
}

func (s *service) SetDataviewViewPosition(ctx session.Context, req pb.RpcBlockDataviewViewSetPositionRequest) error {
	return cache.Do(s.getter, req.ContextId, func(b dataview.Dataview) error {
		return b.SetViewPosition(ctx, req.BlockId, req.ViewId, req.Position)
	})
}

func (s *service) CreateDataviewView(
	ctx session.Context, req pb.RpcBlockDataviewViewCreateRequest,
) (id string, err error) {
	err = cache.Do(s.getter, req.ContextId, func(b dataview.Dataview) error {
		if req.View == nil {
			req.View = &model.BlockContentDataviewView{CardSize: model.BlockContentDataviewView_Medium}
		}
		view, e := b.CreateView(ctx, req.BlockId, *req.View, req.Source)
		if e != nil {
			return e
		}
		id = view.Id
		return nil
	})
	return
}

func (s *service) AddDataviewRelation(ctx session.Context, req pb.RpcBlockDataviewRelationAddRequest) error {
	return cache.Do(s.getter, req.ContextId, func(b dataview.Dataview) error {
		return b.AddRelations(ctx, req.BlockId, req.RelationKeys, true)
	})
}

func (s *service) DeleteDataviewRelation(ctx session.Context, req pb.RpcBlockDataviewRelationDeleteRequest) error {
	return cache.Do(s.getter, req.ContextId, func(b dataview.Dataview) error {
		return b.DeleteRelations(ctx, req.BlockId, req.RelationKeys, true)
	})
}

func (s *service) SetDataviewSource(ctx session.Context, contextId, blockId string, source []string) error {
	return cache.Do(s.getter, contextId, func(b dataview.Dataview) error {
		return b.SetSource(ctx, blockId, source)
	})
}

func (s *service) CopyDataviewToBlock(
	ctx session.Context,
	req *pb.RpcBlockDataviewCreateFromExistingObjectRequest,
) ([]*model.BlockContentDataviewView, error) {

	var targetDvContent *model.BlockContentDataview

	err := cache.Do(s.getter, req.TargetObjectId, func(d dataview.Dataview) error {
		var err error
		targetDvContent, err = d.GetDataview(template.DataviewBlockId)
		return err
	})
	if err != nil {
		return nil, err
	}

	err = cache.Do(s.getter, req.ContextId, func(b smartblock.SmartBlock) error {
		st := b.NewStateCtx(ctx)
		block := st.Get(req.BlockId)
		if block == nil {
			return fmt.Errorf("block is not found")
		}

		dvContent, ok := block.Model().Content.(*model.BlockContentOfDataview)
		if !ok {
			return fmt.Errorf("block must contain dataView content")
		}

		dvContent.Dataview.Views = targetDvContent.Views
		dvContent.Dataview.RelationLinks = targetDvContent.RelationLinks
		dvContent.Dataview.GroupOrders = targetDvContent.GroupOrders
		dvContent.Dataview.ObjectOrders = targetDvContent.ObjectOrders
		dvContent.Dataview.TargetObjectId = req.TargetObjectId
		dvContent.Dataview.IsCollection = targetDvContent.IsCollection

		return b.Apply(st)
	})
	if err != nil {
		return nil, err
	}

	return targetDvContent.Views, err
}

func (s *service) SetSourceToSet(ctx session.Context, objectId string, source []string) error {
	return cache.Do(s.getter, objectId, func(dv dataview.Dataview) error {
		return dv.SetSourceInSet(ctx, source)
	})
}
