package dataview

import (
	"errors"
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/proto"
	"github.com/google/uuid"
)

var _ Block = (*Dataview)(nil)
var (
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

	AddRelation(relation model.Relation)
	GetRelation(relationKey string) (*model.Relation, error)
	UpdateRelation(relationKey string, relation model.Relation) error
	DeleteRelation(relationKey string) error

	AddRelationOption(relationKey string, opt model.RelationOption) error
	UpdateRelationOption(relationKey string, opt model.RelationOption) error
	DeleteRelationOption(relationKey string, optId string) error

	GetSource() []string
	SetSource(source []string) error
	SetActiveView(activeView string)

	FillSmartIds(ids []string) []string
	HasSmartIds() bool
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

func (d *Dataview) Diff(b simple.Block) (msgs []simple.EventMessage, err error) {
	dv, ok := b.(*Dataview)
	if !ok {
		return nil, fmt.Errorf("can't make diff with different block type")
	}
	if msgs, err = d.Base.Diff(dv); err != nil {
		return
	}

	// @TODO: rewrite for optimised compare
	for _, view2 := range dv.content.Views {
		var found bool
		var changed bool
		for _, view1 := range d.content.Views {
			if view1.Id == view2.Id {
				found = true
				changed = !proto.Equal(view1, view2)
				break
			}
		}

		if !found || changed {
			msgs = append(msgs,
				simple.EventMessage{
					Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockDataviewViewSet{
						&pb.EventBlockDataviewViewSet{
							Id:     dv.Id,
							ViewId: view2.Id,
							View:   view2,
							Offset: 0,
							Limit:  0,
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

	for _, rel2 := range dv.content.Relations {
		var found bool
		var changed bool
		for _, rel1 := range d.content.Relations {
			if rel1.Key == rel2.Key {
				found = true
				changed = !pbtypes.RelationEqual(rel1, rel2)
				break
			}
		}

		if !found || changed {
			msgs = append(msgs,
				simple.EventMessage{
					Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockDataviewRelationSet{
						&pb.EventBlockDataviewRelationSet{
							Id:          dv.Id,
							RelationKey: rel2.Key,
							Relation:    rel2,
						}}}})
		}
	}
	for _, rel1 := range d.content.Relations {
		var found bool
		for _, rel2 := range dv.content.Relations {
			if rel1.Key == rel2.Key {
				found = true
				break
			}
		}

		if !found {
			msgs = append(msgs,
				simple.EventMessage{Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockDataviewRelationDelete{
					&pb.EventBlockDataviewRelationDelete{
						Id:          dv.Id,
						RelationKey: rel1.Key,
					}}}})
		}
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
			s.content.Views = append(s.content.Views[:i], s.content.Views[i+1:]...)
			break
		}
	}

	if !found {
		return fmt.Errorf("view not found")
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

			break
		}
	}

	if !found {
		return fmt.Errorf("view not found")
	}

	return nil
}

func (s *Dataview) GetRelation(relationKey string) (*model.Relation, error) {
	for _, v := range s.content.Relations {
		if v.Key == relationKey {
			return v, nil
		}
	}
	return nil, ErrRelationNotFound
}

func (s *Dataview) UpdateRelation(relationKey string, rel model.Relation) error {
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

func (l *Dataview) getActiveView() *model.BlockContentDataviewView {
	for i, view := range l.GetDataview().Views {
		if view.Id == l.content.ActiveView {
			return l.GetDataview().Views[i]
		}
	}

	return nil
}

func (l *Dataview) FillSmartIds(ids []string) []string {
	relationsWithObjFormat := l.relationsWithObjectFormat()
	activeView := l.getActiveView()

	ids = append(ids, l.GetSource()...)
	if activeView == nil {
		// shouldn't be a case
		return ids
	}

	for _, filter := range activeView.Filters {
		if slice.FindPos(relationsWithObjFormat, filter.RelationKey) >= 0 {
			for _, objId := range pbtypes.GetStringListValue(filter.Value) {
				if objId != "" && slice.FindPos(ids, objId) == -1 {
					ids = append(ids, objId)
				}
			}
		}
	}

	return ids
}

func (l *Dataview) HasSmartIds() bool {
	relationsWithObjFormat := l.relationsWithObjectFormat()
	activeView := l.getActiveView()
	if len(l.GetSource()) > 0 {
		return true
	}
	if activeView == nil {
		// shouldn't be a case
		return false
	}

	for _, filter := range activeView.Filters {
		if slice.FindPos(relationsWithObjFormat, filter.RelationKey) >= 0 {
			if len(pbtypes.GetStringListValue(filter.Value)) > 0 {
				return true
			}
		}
	}

	return false
}

func (td *Dataview) ModelToSave() *model.Block {
	b := pbtypes.CopyBlock(td.Model())
	for _, rel := range b.Content.(*model.BlockContentOfDataview).Dataview.Relations {
		// reset all selectDict
		rel.SelectDict = nil
	}
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

func (d *Dataview) AddRelation(relation model.Relation) {
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

func (d *Dataview) DeleteRelation(relationKey string) error {
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

func (d *Dataview) DetailsInit(s simple.DetailsService) {
	//todo: inject setOf
}

func (d *Dataview) OnDetailsChange(s simple.DetailsService) {
	// empty
}

func (d *Dataview) DetailsApply(s simple.DetailsService) {

}

func (d *Dataview) SetActiveView(activeView string) {
	d.content.ActiveView = activeView
}

func (d *Dataview) AddRelationOption(relationKey string, option model.RelationOption) error {
	var relFound *model.Relation
	for _, rel := range d.content.Relations {
		if rel.Key != relationKey {
			continue
		}
		if rel.Format != model.RelationFormat_status && rel.Format != model.RelationFormat_tag {
			return fmt.Errorf("relation has incorrect format")
		}

		relFound = pbtypes.CopyRelation(rel)
		for _, opt := range rel.SelectDict {
			if option.Id == opt.Id {
				return fmt.Errorf("option already exists")
			}
		}
		if option.Scope != model.RelationOption_local {
			return fmt.Errorf("incorrect option scope")
		}
		optionCopy := option
		rel.SelectDict = append(rel.SelectDict, &optionCopy)
		break
	}

	if relFound == nil {
		return fmt.Errorf("relation not found")
	}

	// add this option with format scope to other dataview relations
	for _, rel := range d.content.Relations {
		if rel.Key == relationKey {
			continue
		}

		if rel.Format != relFound.Format {
			continue
		}
		optionCopy := option

		optionCopy.Scope = model.RelationOption_format
		rel.SelectDict = append(rel.SelectDict, &optionCopy)
	}

	return nil
}

func (d *Dataview) UpdateRelationOption(relationKey string, option model.RelationOption) error {
	if option.Scope != model.RelationOption_local {
		return fmt.Errorf("incorrect option scope")
	}

	relFound := pbtypes.GetRelation(d.content.Relations, relationKey)

	for _, rel := range d.content.Relations {
		if rel.Key != relationKey {
			if relFound.Format != rel.Format {
				continue
			}
			option.Scope = model.RelationOption_format
		} else {
			option.Scope = model.RelationOption_local
		}

		var found bool
		for i, opt := range rel.SelectDict {
			if option.Id == opt.Id {
				optionCopy := option
				rel.SelectDict[i] = &optionCopy
				found = true
				break
			}
		}

		if !found && rel.Key != relationKey {
			rel.SelectDict = append(rel.SelectDict, &option)
		}
	}
	return nil
}

func (d *Dataview) DeleteRelationOption(relationKey string, optId string) error {
	for relI, rel := range d.content.Relations {
		if rel.Key != relationKey {
			continue
		}
		var filtered = make([]*model.RelationOption, 0, len(rel.SelectDict))
		for optI, opt := range rel.SelectDict {
			if optId != opt.Id {
				filtered = append(filtered, rel.SelectDict[optI])
			}
		}

		if len(filtered) == len(rel.SelectDict) {
			return ErrOptionNotExists
		}

		d.content.Relations[relI].SelectDict = filtered
		return nil
	}

	return fmt.Errorf("relation not found")
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

func mergeSelectOptions(opts1, opts2 []*model.RelationOption) []*model.RelationOption {
	var opts []*model.RelationOption
	for _, opt1 := range opts1 {
		opts = append(opts, &*opt1)
	}

	for _, opt2 := range opts2 {
		var found bool
		for _, opt := range opts {
			if opt.Id != opt2.Id {
				continue
			}
			opt.Text = opt2.Text
			opt2.Color = opt2.Color
			found = true
		}

		if !found {
			opts = append(opts, &*opt2)
		}
	}
	return opts
}
