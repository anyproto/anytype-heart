package dataview

import (
	"errors"
	"fmt"

	"github.com/globalsign/mgo/bson"
	"github.com/google/uuid"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/core/block/simple/base"
	"github.com/anyproto/anytype-heart/pb"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/pbtypes"
	"github.com/anyproto/anytype-heart/util/slice"
)

var _ Block = (*Dataview)(nil)

var (
	ErrRelationExists   = fmt.Errorf("relation exists")
	ErrViewNotFound     = errors.New("view not found")
	ErrRelationNotFound = fmt.Errorf("relation not found")
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
	MoveObjectsInView(req *pb.RpcBlockDataviewObjectOrderMoveRequest) error

	AddRelation(relation *model.RelationLink) error
	DeleteRelation(relationKey string) error

	GetSource() []string
	SetSource(source []string) error
	SetActiveView(activeView string)
	SetTargetObjectID(targetObjectID string)
	SetIsCollection(value bool)

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
	RemoveSorts(viewID string, ids []string) error
	ReplaceSort(viewID string, id string, sort *model.BlockContentDataviewSort) error
	ReorderSorts(viewID string, ids []string) error

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
	for _, view := range d.content.Views {
		if view.Id == "" {
			view.Id = bson.NewObjectId().Hex()
		}
	}

	return nil
}

// AddView adds a view to the dataview. It doesn't fill any missing field except id
func (s *Dataview) AddView(view model.BlockContentDataviewView) {
	if view.Id == "" {
		view.Id = uuid.New().String()
	}
	for _, f := range view.Filters {
		if f.Id == "" {
			f.Id = bson.NewObjectId().Hex()
		}
	}
	for _, s := range view.Sorts {
		if s.Id == "" {
			s.Id = bson.NewObjectId().Hex()
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
		return err
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
	v.PageLimit = view.PageLimit
	v.DefaultTemplateId = view.DefaultTemplateId
	v.DefaultObjectTypeId = view.DefaultObjectTypeId

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
	v.PageLimit = view.PageLimit
	v.DefaultTemplateId = view.DefaultTemplateId
	v.DefaultObjectTypeId = view.DefaultObjectTypeId

	return nil
}

func (l *Dataview) FillSmartIds(ids []string) []string {
	// for _ := range l.content.RelationLinks {
	// todo: add relationIds from relationLinks
	// }

	if l.content.TargetObjectId != "" {
		ids = append(ids, l.content.TargetObjectId)
	}

	for _, view := range l.content.Views {
		if view.DefaultObjectTypeId != "" {
			ids = append(ids, view.DefaultObjectTypeId)
		}
		if view.DefaultTemplateId != "" {
			ids = append(ids, view.DefaultTemplateId)
		}
	}

	return ids
}

func (l *Dataview) ReplaceSmartIds(f func(id string) (newId string, replaced bool)) (anyReplaced bool) {
	if l.content.TargetObjectId != "" {
		newId, replaced := f(l.content.TargetObjectId)
		if replaced {
			l.content.TargetObjectId = newId
			return true
		}
	}

	return
}

func (l *Dataview) HasSmartIds() bool {
	for _, view := range l.content.Views {
		if view.DefaultObjectTypeId != "" || view.DefaultTemplateId != "" {
			return true
		}
	}
	return len(l.content.RelationLinks) > 0 || l.content.TargetObjectId != ""
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

func (d *Dataview) SetTargetObjectID(targetObjectID string) {
	d.content.TargetObjectId = targetObjectID
}

func (d *Dataview) SetIsCollection(value bool) {
	d.content.IsCollection = value
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

func (d *Dataview) MoveObjectsInView(req *pb.RpcBlockDataviewObjectOrderMoveRequest) error {
	var found bool
	for _, order := range d.content.ObjectOrders {
		if order.ViewId == req.ViewId && order.GroupId == req.GroupId {
			order.ObjectIds = slice.Difference(order.ObjectIds, req.ObjectIds)

			pos := slice.FindPos(order.ObjectIds, req.AfterId)
			order.ObjectIds = slice.Insert(order.ObjectIds, pos+1, req.ObjectIds...)

			found = true
			break
		}
	}
	if !found {
		return fmt.Errorf("object order is not found")
	}
	return nil
}

func (d *Dataview) resetObjectOrderForView(viewId string) {
	orders := d.content.ObjectOrders
	for _, order := range orders {
		if order.ViewId == viewId {
			order.ObjectIds = nil
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
