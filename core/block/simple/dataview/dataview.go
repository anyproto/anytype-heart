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

type withID[T any] struct {
	item T

	id string
}

func (w withID[T]) GetId() string {
	return w.id
}

func wrapWithIDs[T any](items []T, calcID func(item T) string) []withID[T] {
	wrapped := make([]withID[T], len(items))
	for i, item := range items {
		wrapped[i] = withID[T]{
			item: item,
			id:   calcID(item),
		}
	}
	return wrapped
}

// unwrap items from withID wrapper
func unwrapItems[T any](items []withID[T]) []T {
	res := make([]T, len(items))
	for i, it := range items {
		res[i] = it.item
	}
	return res
}

func unwrapChanges[T, R any](
	changes []slice.Change[withID[T]],
	add func(afterID string, items []T) R,
	remove func(ids []string) R,
	move func(afterID string, ids []string) R,
	update func(id string, item T) R,
) []R {
	res := make([]R, 0, len(changes))
	for _, c := range changes {
		if v := c.Add(); v != nil {
			res = append(res, add(v.AfterId, unwrapItems(v.Items)))
		}
		if v := c.Remove(); v != nil {
			res = append(res, remove(v.IDs))
		}
		if v := c.Move(); v != nil {
			res = append(res, move(v.AfterId, v.IDs))
		}
		if v := c.Replace(); v != nil {
			res = append(res, update(v.ID, v.Item.item))
		}
	}
	return res
}

func (d *Dataview) Diff(b simple.Block) (msgs []simple.EventMessage, err error) {
	fmt.Println("DIFF", b.Model().GetId())

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
		var changes []slice.Change[slice.ID]
		for _, order1 := range d.content.ObjectOrders {
			if order1.ViewId == order2.ViewId && order1.GroupId == order2.GroupId {
				found = true
				changes = slice.Diff(slice.StringsToIDs(order1.ObjectIds), slice.StringsToIDs(order2.ObjectIds), slice.CompareID)
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
							SliceChanges: pbtypes.SliceChangeToEvents(changes),
						}}}})
		}
	}

	// @TODO: rewrite for optimised compare
	for _, view2 := range dv.content.Views {
		var found bool
		var changed bool
		var viewFilterChanges []*pb.EventBlockDataviewViewUpdateFilter

		for _, view1 := range d.content.Views {
			if view1.Id == view2.Id {
				found = true
				changed = !proto.Equal(view1, view2)

				viewFilterChanges = diffViewFilters(view1, view2)

				{

					calcID := func(s *model.BlockContentDataviewSort) string {
						// TODO temp
						return s.RelationKey
					}
					res := slice.Diff(wrapWithIDs(view1.Sorts, calcID), wrapWithIDs(view2.Sorts, calcID), func(a, b withID[*model.BlockContentDataviewSort]) bool {
						return a.item.RelationKey == b.item.RelationKey
					})
					if len(res) > 0 {
						fmt.Println("sorts")
					}
					for _, x := range res {
						fmt.Printf("%s\n", x)
					}
				}
				{

					calcID := func(s *model.BlockContentDataviewRelation) string {
						// TODO temp
						return s.Key
					}
					res := slice.Diff(wrapWithIDs(view1.Relations, calcID), wrapWithIDs(view2.Relations, calcID), func(a, b withID[*model.BlockContentDataviewRelation]) bool {
						if a.item.Key != b.item.Key {
							return false
						}
						if a.item.IsVisible != b.item.IsVisible {
							return false
						}
						return true
					})
					if len(res) > 0 {
						fmt.Println("relations")
					}
					for _, x := range res {
						fmt.Printf("%s\n", x)
					}
				}

				break
			}

		}

		if len(viewFilterChanges) > 0 {
			msgs = append(msgs,
				simple.EventMessage{
					Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockDataviewViewUpdate{
						&pb.EventBlockDataviewViewUpdate{
							Filter: viewFilterChanges,
						},
					}}})
		}

		if !found || changed {
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
	var found bool
	for _, v := range s.content.Views {
		if v.Id == viewID {
			found = true

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

			break
		}
	}

	if !found {
		return ErrViewNotFound
	}

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
