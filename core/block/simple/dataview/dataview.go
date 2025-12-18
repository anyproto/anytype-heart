package dataview

import (
	"errors"
	"fmt"
	"slices"

	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/types"
	"github.com/google/uuid"
	"github.com/samber/lo"

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
	ListViews() []*model.BlockContentDataviewView
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
	DeleteRelations(relationKeys ...string)
	SetRelations(relationLinks []*model.RelationLink)
	ListRelationLinks() []*model.RelationLink

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

func (d *Dataview) ListViews() []*model.BlockContentDataviewView {
	return d.GetDataview().Views
}

// AddView adds a view to the dataview. It doesn't fill any missing field except id
func (d *Dataview) AddView(view model.BlockContentDataviewView) {
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

	d.content.Views = append(d.content.Views, &view)
}

func (d *Dataview) GetView(viewId string) (*model.BlockContentDataviewView, error) {
	for _, view := range d.GetDataview().Views {
		if view.Id == viewId {
			return view, nil
		}
	}

	return nil, fmt.Errorf("view '%s' not found", viewId)
}

func (d *Dataview) DeleteView(viewID string) error {
	var found bool
	for i, v := range d.content.Views {
		if v.Id == viewID {
			found = true
			d.content.Views[i] = nil
			d.content.Views = append(d.content.Views[:i], d.content.Views[i+1:]...)
			break
		}
	}

	if !found {
		return ErrViewNotFound
	}

	return nil
}

func (d *Dataview) SetView(viewID string, view model.BlockContentDataviewView) error {
	v, err := d.GetView(viewID)
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
	v.EndRelationKey = view.EndRelationKey
	v.WrapContent = view.WrapContent

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
	v.EndRelationKey = view.EndRelationKey
	v.WrapContent = view.WrapContent

	return nil
}

func (d *Dataview) FillSmartIds(ids []string) []string {
	// for _ := range l.content.RelationLinks {
	// todo: add relationIds from relationLinks
	// }

	if d.content.TargetObjectId != "" {
		ids = append(ids, d.content.TargetObjectId)
	}

	for _, view := range d.content.Views {
		if view.DefaultObjectTypeId != "" {
			ids = append(ids, view.DefaultObjectTypeId)
		}
		if view.DefaultTemplateId != "" {
			ids = append(ids, view.DefaultTemplateId)
		}
		ids = append(ids, getIdsFromFilters(view.Filters)...)
	}

	return ids
}

func (d *Dataview) MigrateFile(migrateFunc func(oldHash string) (newHash string)) {
	for _, view := range d.content.Views {
		for _, filter := range view.Filters {
			d.migrateFilesInFilter(filter, migrateFunc)
		}
	}
}

func (d *Dataview) migrateFilesInFilter(filter *model.BlockContentDataviewFilter, migrateFunc func(oldHash string) (newHash string)) {
	if filter.Format != model.RelationFormat_object && filter.Format != model.RelationFormat_file {
		return
	}
	if filter.Value == nil {
		return
	}
	switch v := filter.Value.Kind.(type) {
	case *types.Value_StringValue:
		filter.Value = pbtypes.String(migrateFunc(v.StringValue))
	case *types.Value_ListValue:
		var changed bool
		ids := pbtypes.ListValueToStrings(v.ListValue)
		for i, id := range ids {
			newId := migrateFunc(id)
			if newId != id {
				ids[i] = newId
				changed = true
			}
		}
		if changed {
			filter.Value = pbtypes.StringList(ids)
		}
	}
}

func getIdsFromFilters(filters []*model.BlockContentDataviewFilter) (ids []string) {
	for _, filter := range filters {
		if filter.Format != model.RelationFormat_object &&
			filter.Format != model.RelationFormat_status &&
			filter.Format != model.RelationFormat_tag {
			continue
		}

		id := filter.Value.GetStringValue()
		if id != "" {
			ids = append(ids, id)
			continue
		}

		list := filter.Value.GetListValue()
		if list == nil {
			continue
		}
		for _, value := range list.Values {
			ids = append(ids, value.GetStringValue())
		}
	}

	return ids
}

func (d *Dataview) ReplaceLinkIds(replacer func(oldId string) (newId string)) {
	if d.content.TargetObjectId != "" {
		d.content.TargetObjectId = replacer(d.content.TargetObjectId)
	}
	return
}

func (d *Dataview) HasSmartIds() bool {
	for _, view := range d.content.Views {
		if view.DefaultObjectTypeId != "" || view.DefaultTemplateId != "" {
			return true
		}
	}
	return len(d.content.RelationLinks) > 0 || d.content.TargetObjectId != ""
}

func (d *Dataview) AddRelation(relation *model.RelationLink) error {
	if pbtypes.RelationLinks(d.content.RelationLinks).Has(relation.Key) {
		return ErrRelationExists
	}
	d.content.RelationLinks = append(d.content.RelationLinks, relation)
	return nil
}

func (d *Dataview) DeleteRelations(relationKeys ...string) {
	d.content.RelationLinks = pbtypes.RelationLinks(d.content.RelationLinks).Remove(relationKeys...)
	d.removeSortsAndFiltersByKeys(relationKeys...)
}

func (d *Dataview) SetRelations(relationLinks []*model.RelationLink) {
	_, removed := pbtypes.RelationLinks(relationLinks).Diff(d.content.RelationLinks)
	d.content.RelationLinks = relationLinks
	d.removeSortsAndFiltersByKeys(removed...)
}

func (d *Dataview) removeSortsAndFiltersByKeys(keys ...string) {
	if keys == nil {
		return
	}
	for _, view := range d.content.Views {
		view.Filters = lo.Filter(view.Filters, func(filter *model.BlockContentDataviewFilter, _ int) bool {
			return !slices.Contains(keys, filter.RelationKey)
		})
		view.Sorts = lo.Filter(view.Sorts, func(filter *model.BlockContentDataviewSort, _ int) bool {
			return !slices.Contains(keys, filter.RelationKey)
		})
	}
}

func (d *Dataview) ListRelationLinks() []*model.RelationLink {
	return d.content.RelationLinks
}

func (d *Dataview) ModelToSave() *model.Block {
	b := pbtypes.CopyBlock(d.Model())
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

func (d *Dataview) UpdateRelationOld(relationKey string, rel model.Relation) error {
	var found bool
	if relationKey != rel.Key {
		return fmt.Errorf("changing key of existing relation is retricted")
	}

	for i, v := range d.content.Relations {
		if v.Key == relationKey {
			found = true

			d.content.Relations[i] = pbtypes.CopyRelation(&rel)
			break
		}
	}

	if !found {
		return ErrRelationNotFound
	}

	return nil
}

func (d *Dataview) IsEmpty() bool {
	return d.content.TargetObjectId == "" && len(d.content.Views) == 0
}
