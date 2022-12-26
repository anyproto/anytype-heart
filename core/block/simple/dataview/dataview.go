package dataview

import (
	"errors"
	"fmt"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/proto"
	"github.com/google/uuid"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/localstore/addr"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

var _ Block = (*Dataview)(nil)

var (
	ErrRelationExists   = fmt.Errorf("relation exists")
	ErrViewNotFound     = errors.New("view not found")
	ErrRelationNotFound = fmt.Errorf("relation not found")
	ErrOptionNotExists  = errors.New("option not exists")
)

func init() {
	simple.RegisterCreator(NewDataview)
}

func NewDataview(m *model.Block) simple.Block {
	if link := m.GetDataview(); link != nil {
		return &Dataview{
			Base:    base.NewBase(m).(*base.Base),
			content: link,
		}
	}
	return nil
}

type Block interface {
	simple.Block
	GetView(viewID string) (*model.BlockContentDataviewView, error)
	SetView(viewID string, view model.BlockContentDataviewView) error
	SetViewFields(viewID string, view *model.BlockContentDataviewView) error
	AddView(view model.BlockContentDataviewView)
	DeleteView(viewID string) error
	SetViewOrder(ids []string)
	SetViewGroupOrder(order *model.BlockContentDataviewGroupOrder)
	SetViewObjectOrder(order []*model.BlockContentDataviewObjectOrder)

	AddRelation(relation *model.RelationLink) error
	DeleteRelation(relationKey string) error

	GetSource() []string
	SetSource(source []string) error
	SetActiveView(activeView string)

	FillSmartIds(ids []string) []string
	HasSmartIds() bool

	// AddRelationOld DEPRECATED
	AddRelationOld(relation model.Relation)
	// UpdateRelationOld DEPRECATED
	UpdateRelationOld(relationKey string, relation model.Relation) error
	// DeleteRelationOld DEPRECATED
	DeleteRelationOld(relationKey string) error

	ApplyViewUpdate(upd *pb.EventBlockDataviewViewUpdate)
	ApplyObjectOrderUpdate(upd *pb.EventBlockDataviewObjectOrderUpdate)

	AddFilter(viewID string, filter *model.BlockContentDataviewFilter) error
	RemoveFilters(viewID string, filterIDs []string) error
	ReplaceFilter(viewID string, filterID string, filter *model.BlockContentDataviewFilter) error
	ReorderFilters(viewID string, ids []string) error

	AddSort(viewID string, sort *model.BlockContentDataviewSort) error
	RemoveSorts(viewID string, relationKeys []string) error
	ReplaceSort(viewID string, relationKey string, sort *model.BlockContentDataviewSort) error
	ReorderSorts(viewID string, relationKeys []string) error

	AddViewRelation(viewID string, relation *model.BlockContentDataviewRelation) error
	RemoveViewRelations(viewID string, relationKeys []string) error
	ReplaceViewRelation(viewID string, relationKey string, relation *model.BlockContentDataviewRelation) error
	ReorderViewRelations(viewID string, relationKeys []string) error
}

type Dataview struct {
	*base.Base
	content *model.BlockContentDataview
}

func (d *Dataview) Copy() simple.Block {
	copy := pbtypes.CopyBlock(d.Model())
	return &Dataview{
		Base:    base.NewBase(copy).(*base.Base),
		content: copy.GetDataview(),
	}
}

// Validate TODO: add validation rules
func (d *Dataview) Validate() error {
	return nil
}

func (d *Dataview) Diff(b simple.Block) (msgs []simple.EventMessage, err error) {
	dv, ok := b.(*Dataview)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = d.Base.Diff(dv); err != nil {
		return
	}

	for _, order2 := range dv.content.GroupOrders {
		var found bool
		var changed bool
		for _, order1 := range d.content.GroupOrders {
			if order1.ViewId == order2.ViewId {
				found = true
				changed = !proto.Equal(order1, order2)
				break
			}
		}

		if !found || changed {
			msgs = append(msgs,
				simple.EventMessage{
					Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockDataViewGroupOrderUpdate{
						&pb.EventBlockDataviewGroupOrderUpdate{
							Id:         dv.Id,
							GroupOrder: order2,
						}}}})
		}
	}

	for _, order2 := range dv.content.ObjectOrders {
		var found bool
		var changes []*pb.EventBlockDataviewSliceChange
		for _, order1 := range d.content.ObjectOrders {
			if order1.ViewId == order2.ViewId && order1.GroupId == order2.GroupId {
				found = true
				changes = diffViewObjectOrder(order1, order2)
				break
			}
		}

		if !found {
			msgs = append(msgs,
				simple.EventMessage{
					Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockDataViewObjectOrderUpdate{
						&pb.EventBlockDataviewObjectOrderUpdate{
							Id:           dv.Id,
							ViewId:       order2.ViewId,
							GroupId:      order2.GroupId,
							SliceChanges: []*pb.EventBlockDataviewSliceChange{{Op: pb.EventBlockDataview_SliceOperationAdd, Ids: order2.ObjectIds}},
						}}}})
		}

		if len(changes) > 0 {
			msgs = append(msgs,
				simple.EventMessage{
					Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockDataViewObjectOrderUpdate{
						&pb.EventBlockDataviewObjectOrderUpdate{
							Id:           dv.Id,
							ViewId:       order2.ViewId,
							GroupId:      order2.GroupId,
							SliceChanges: changes,
						}}}})
		}
	}

	// @TODO: rewrite for optimised compare
	for _, view2 := range dv.content.Views {
		var found bool
		var (
			viewFilterChanges   []*pb.EventBlockDataviewViewUpdateFilter
			viewRelationChanges []*pb.EventBlockDataviewViewUpdateRelation
			viewSortChanges     []*pb.EventBlockDataviewViewUpdateSort
			viewFieldsChange    *pb.EventBlockDataviewViewUpdateFields
		)

		for _, view1 := range d.content.Views {
			if view1.Id == view2.Id {
				found = true

				viewFieldsChange = diffViewFields(view1, view2)
				viewFilterChanges = diffViewFilters(view1, view2)
				viewRelationChanges = diffViewRelations(view1, view2)
				viewSortChanges = diffViewSorts(view1, view2)

				break
			}
		}

		if len(viewFilterChanges) > 0 || len(viewRelationChanges) > 0 || len(viewSortChanges) > 0 || viewFieldsChange != nil {
			msgs = append(msgs,
				simple.EventMessage{
					Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockDataviewViewUpdate{
						BlockDataviewViewUpdate: &pb.EventBlockDataviewViewUpdate{
							Id:       dv.Id,
							ViewId:   view2.Id,
							Fields:   viewFieldsChange,
							Filter:   viewFilterChanges,
							Relation: viewRelationChanges,
							Sort:     viewSortChanges,
						},
					}}})
		}

		if !found {
			msgs = append(msgs,
				simple.EventMessage{
					Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockDataviewViewSet{
						&pb.EventBlockDataviewViewSet{
							Id:     dv.Id,
							ViewId: view2.Id,
							View:   view2,
						}}}})
		}
	}
	for _, view1 := range d.content.Views {
		var found bool
		for _, view2 := range dv.content.Views {
			if view1.Id == view2.Id {
				found = true
				break
			}
		}

		if !found {
			msgs = append(msgs,
				simple.EventMessage{Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockDataviewViewDelete{
					&pb.EventBlockDataviewViewDelete{
						Id:     dv.Id,
						ViewId: view1.Id,
					}}}})
		}
	}

	added, removed := pbtypes.RelationLinks(dv.content.RelationLinks).Diff(d.content.RelationLinks)
	if len(removed) > 0 {
		msgs = append(msgs, simple.EventMessage{
			Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockDataviewRelationDelete{
				BlockDataviewRelationDelete: &pb.EventBlockDataviewRelationDelete{
					Id:           dv.Id,
					RelationKeys: removed,
				},
			}},
		})
	}
	if len(added) > 0 {
		msgs = append(msgs, simple.EventMessage{
			Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockDataviewRelationSet{
				BlockDataviewRelationSet: &pb.EventBlockDataviewRelationSet{
					Id:            dv.Id,
					RelationLinks: added,
				},
			}},
		})
	}

	if !slice.UnsortedEquals(dv.content.Source, d.content.Source) {
		msgs = append(msgs,
			simple.EventMessage{Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockDataviewSourceSet{
				&pb.EventBlockDataviewSourceSet{
					Id:     dv.Id,
					Source: dv.content.Source,
				}}}})
	}

	var viewIds1, viewIds2 []string
	for _, v := range d.content.Views {
		viewIds1 = append(viewIds1, v.Id)
	}
	for _, v := range dv.content.Views {
		viewIds2 = append(viewIds2, v.Id)
	}
	if !slice.SortedEquals(viewIds1, viewIds2) {
		msgs = append(msgs,
			simple.EventMessage{Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockDataviewViewOrder{
				&pb.EventBlockDataviewViewOrder{
					Id:      dv.Id,
					ViewIds: viewIds2,
				}}}})
	}
	return
}

// AddView adds a view to the dataview. It doesn't fills any missing field excepting id
func (s *Dataview) AddView(view model.BlockContentDataviewView) {
	if view.Id == "" {
		view.Id = uuid.New().String()
	}
	for _, f := range view.Filters {
		if f.Id == "" {
			f.Id = bson.NewObjectId().Hex()
		}
	}

	s.content.Views = append(s.content.Views, &view)
}

func (s *Dataview) GetView(viewId string) (*model.BlockContentDataviewView, error) {
	for _, view := range s.GetDataview().Views {
		if view.Id == viewId {
			return view, nil
		}
	}

	return nil, fmt.Errorf("view '%s' not found", viewId)
}

func (s *Dataview) DeleteView(viewID string) error {
	var found bool
	for i, v := range s.content.Views {
		if v.Id == viewID {
			found = true
			s.content.Views[i] = nil
			s.content.Views = append(s.content.Views[:i], s.content.Views[i+1:]...)
			break
		}
	}

	if !found {
		return ErrViewNotFound
	}

	return nil
}

func (s *Dataview) SetView(viewID string, view model.BlockContentDataviewView) error {
	v, err := s.GetView(viewID)
	if err != nil {
		return nil
	}

	v.Relations = view.Relations
	v.Sorts = view.Sorts
	v.Filters = view.Filters

	v.Name = view.Name
	v.Type = view.Type
	v.CoverRelationKey = view.CoverRelationKey
	v.HideIcon = view.HideIcon
	v.CoverFit = view.CoverFit
	v.CardSize = view.CardSize
	v.GroupRelationKey = view.GroupRelationKey
	v.GroupBackgroundColors = view.GroupBackgroundColors

	return nil
}

// SetViewFields updates only simple fields of a view. It doesn't update filters, relations, sorts.
func (d *Dataview) SetViewFields(viewID string, view *model.BlockContentDataviewView) error {
	v, err := d.GetView(viewID)
	if err != nil {
		return err
	}

	v.Name = view.Name
	v.Type = view.Type
	v.CoverRelationKey = view.CoverRelationKey
	v.HideIcon = view.HideIcon
	v.CoverFit = view.CoverFit
	v.CardSize = view.CardSize
	v.GroupRelationKey = view.GroupRelationKey
	v.GroupBackgroundColors = view.GroupBackgroundColors

	return nil
}

func (l *Dataview) getActiveView() *model.BlockContentDataviewView {
	for i, view := range l.GetDataview().Views {
		if view.Id == l.content.ActiveView {
			return l.GetDataview().Views[i]
		}
	}
	return nil
}

func (l *Dataview) FillSmartIds(ids []string) []string {
	for _, rl := range l.content.RelationLinks {
		ids = append(ids, addr.RelationKeyToIdPrefix+rl.Key)
	}
	return ids
}

func (l *Dataview) HasSmartIds() bool {
	return len(l.content.RelationLinks) > 0
}

func (d *Dataview) AddRelation(relation *model.RelationLink) error {
	if pbtypes.RelationLinks(d.content.RelationLinks).Has(relation.Key) {
		return ErrRelationExists
	}
	d.content.RelationLinks = append(d.content.RelationLinks, relation)
	return nil
}

func (d *Dataview) DeleteRelation(relationKey string) error {
	d.content.RelationLinks = pbtypes.RelationLinks(d.content.RelationLinks).Remove(relationKey)

	for _, view := range d.content.Views {
		var filteredFilters []*model.BlockContentDataviewFilter
		for _, filter := range view.Filters {
			if filter.RelationKey != relationKey {
				filteredFilters = append(filteredFilters, filter)
			}
		}
		view.Filters = filteredFilters

		var filteredSorts []*model.BlockContentDataviewSort
		for _, sort := range view.Sorts {
			if sort.RelationKey != relationKey {
				filteredSorts = append(filteredSorts, sort)
			}
		}
		view.Sorts = filteredSorts
	}
	return nil
}

func (td *Dataview) ModelToSave() *model.Block {
	b := pbtypes.CopyBlock(td.Model())
	b.Content.(*model.BlockContentOfDataview).Dataview.Relations = nil
	b.Content.(*model.BlockContentOfDataview).Dataview.ActiveView = ""
	return b
}

func (d *Dataview) SetSource(source []string) error {
	d.content.Source = source
	return nil
}

func (d *Dataview) GetSource() []string {
	return d.content.Source
}

func (d *Dataview) AddRelationOld(relation model.Relation) {
	if relation.Key == "" {
		relation.Key = bson.NewObjectId().Hex()
	}

	for i := range relation.SelectDict {
		if relation.SelectDict[i].Id == "" {
			relation.SelectDict[i].Id = bson.NewObjectId().Hex()
		}
	}

	d.content.Relations = append(d.content.Relations, &relation)
}

func (d *Dataview) DeleteRelationOld(relationKey string) error {
	var found bool
	for i, r := range d.content.Relations {
		if r.Key == relationKey {
			found = true
			d.content.Relations = append(d.content.Relations[:i], d.content.Relations[i+1:]...)
			break
		}
	}

	for _, view := range d.content.Views {
		var filteredFilters []*model.BlockContentDataviewFilter
		for _, filter := range view.Filters {
			if filter.RelationKey != relationKey {
				filteredFilters = append(filteredFilters, filter)
			}
		}
		view.Filters = filteredFilters

		var filteredSorts []*model.BlockContentDataviewSort
		for _, sort := range view.Sorts {
			if sort.RelationKey != relationKey {
				filteredSorts = append(filteredSorts, sort)
			}
		}
		view.Sorts = filteredSorts
	}

	if !found {
		return fmt.Errorf("relation not found")
	}

	return nil
}

func (d *Dataview) SetActiveView(activeView string) {
	d.content.ActiveView = activeView
}

func (d *Dataview) SetViewOrder(viewIds []string) {
	var newViews = make([]*model.BlockContentDataviewView, 0, len(viewIds))
	for _, viewId := range viewIds {
		if view, err := d.GetView(viewId); err == nil {
			newViews = append(newViews, view)
		}
	}
	// if some view not exists in viewIds - add it to end
	for _, view := range d.content.Views {
		if slice.FindPos(viewIds, view.Id) == -1 {
			newViews = append(newViews, view)
		}
	}
	d.content.Views = newViews
}

func (d *Dataview) SetViewGroupOrder(order *model.BlockContentDataviewGroupOrder) {
	isExist := false
	for _, groupOrder := range d.Model().GetDataview().GroupOrders {
		if groupOrder.ViewId == order.ViewId {
			isExist = true
			groupOrder.ViewGroups = order.ViewGroups
			break
		}
	}
	if !isExist {
		d.Model().GetDataview().GroupOrders = append(d.Model().GetDataview().GroupOrders, order)
	}
}

func (d *Dataview) SetViewObjectOrder(orders []*model.BlockContentDataviewObjectOrder) {
	for _, reqOrder := range orders {
		isExist := false
		for _, existOrder := range d.Model().GetDataview().ObjectOrders {
			if reqOrder.ViewId == existOrder.ViewId && reqOrder.GroupId == existOrder.GroupId {
				isExist = true
				existOrder.ObjectIds = reqOrder.ObjectIds
				break
			}
		}
		if !isExist {
			d.Model().GetDataview().ObjectOrders = append(d.Model().GetDataview().ObjectOrders, reqOrder)
		}
	}
}
func (s *Dataview) GetRelation(relationKey string) (*model.Relation, error) {
	for _, v := range s.content.Relations {
		if v.Key == relationKey {
			return v, nil
		}
	}
	return nil, ErrRelationNotFound
}

func (s *Dataview) UpdateRelationOld(relationKey string, rel model.Relation) error {
	var found bool
	if relationKey != rel.Key {
		return fmt.Errorf("changing key of existing relation is retricted")
	}

	for i, v := range s.content.Relations {
		if v.Key == relationKey {
			found = true

			s.content.Relations[i] = pbtypes.CopyRelation(&rel)
			break
		}
	}

	if !found {
		return ErrRelationNotFound
	}

	return nil
}

func (l *Dataview) relationsWithObjectFormat() []string {
	var relationsWithObjFormat []string
	for _, rel := range l.GetDataview().Relations {
		if rel.Format == model.RelationFormat_file || rel.Format == model.RelationFormat_object {
			relationsWithObjFormat = append(relationsWithObjFormat, rel.Key)
		}
	}
	return relationsWithObjFormat
}
