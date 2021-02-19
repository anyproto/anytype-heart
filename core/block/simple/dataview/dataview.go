package dataview

import (
	"fmt"
	"strings"

	"github.com/anytypeio/go-anytype-middleware/core/block/database/objects"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	pbrelation "github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/relation"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/globalsign/mgo/bson"
	"github.com/gogo/protobuf/proto"
	"github.com/google/uuid"
)

var _ Block = (*Dataview)(nil)

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
	GetView(viewID string) *model.BlockContentDataviewView
	SetView(viewID string, view model.BlockContentDataviewView) error
	AddView(view model.BlockContentDataviewView)
	DeleteView(viewID string) error

	AddRelation(relation pbrelation.Relation)
	UpdateRelation(relationKey string, relation pbrelation.Relation) error
	DeleteRelation(relationKey string) error

	AddRelationOption(relationKey string, opt pbrelation.RelationOption) error
	UpdateRelationOption(relationKey string, opt pbrelation.RelationOption) error
	DeleteRelationOption(relationKey string, optId string) error

	GetSource() string
	SetSource(source string) error
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

	return
}

// AddView adds a view to the dataview. It doesn't fills any missing field excepting id
func (s *Dataview) AddView(view model.BlockContentDataviewView) {
	if view.Id == "" {
		view.Id = uuid.New().String()
	}

	s.content.Views = append(s.content.Views, &view)
}

func (s *Dataview) GetView(viewId string) *model.BlockContentDataviewView {
	for _, view := range s.GetDataview().Views {
		if view.Id == viewId {
			return view
		}
	}

	return nil
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

			break
		}
	}

	if !found {
		return fmt.Errorf("view not found")
	}

	return nil
}

func (s *Dataview) UpdateRelation(relationKey string, rel pbrelation.Relation) error {
	var found bool
	if relationKey != rel.Key {
		return fmt.Errorf("changing key of existing relation is retricted")
	}

	for i, v := range s.content.Relations {
		if v.Key == relationKey {
			found = true

			if v.Format != rel.Format {
				return fmt.Errorf("changing format of existing relation is retricted")
			}

			if v.DataSource != rel.DataSource {
				return fmt.Errorf("changing data source of existing relation is retricted")
			}

			if v.Hidden != rel.Hidden {
				return fmt.Errorf("changing hidden flag of existing relation is retricted")
			}

			if rel.Format == pbrelation.RelationFormat_status {
				for i := range rel.SelectDict {
					if rel.SelectDict[i].Id == "" {
						rel.SelectDict[i].Id = bson.NewObjectId().Hex()
					}
				}
			}

			s.content.Relations[i] = &rel

			break
		}
	}

	if !found {
		return fmt.Errorf("relation not found")
	}

	return nil
}

func (l *Dataview) relationsWithObjectFormat() []string {
	var relationsWithObjFormat []string
	for _, rel := range l.GetDataview().Relations {
		if rel.Format == pbrelation.RelationFormat_file || rel.Format == pbrelation.RelationFormat_object {
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
	if activeView == nil {
		// shouldn't be a case
		return ids
	}

	for _, filter := range activeView.Filters {
		if slice.FindPos(relationsWithObjFormat, filter.RelationKey) >= 0 {
			for _, objId := range pbtypes.GetStringListValue(filter.Value) {
				if slice.FindPos(ids, objId) == -1 {
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

func (d *Dataview) SetSource(source string) error {
	if !strings.HasPrefix(source, objects.BundledObjectTypeURLPrefix) && !strings.HasPrefix(source, objects.CustomObjectTypeURLPrefix) {
		return fmt.Errorf("invalid source URL")
	}

	d.content.Source = source
	return nil
}

func (d *Dataview) GetSource() string {
	return d.content.Source
}

func (d *Dataview) AddRelation(relation pbrelation.Relation) {
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

func (d *Dataview) AddRelationOption(relationKey string, option pbrelation.RelationOption) error {
	var relFound *pbrelation.Relation
	for _, rel := range d.content.Relations {
		if rel.Key != relationKey {
			continue
		}
		if rel.Format != pbrelation.RelationFormat_status && rel.Format != pbrelation.RelationFormat_tag {
			return fmt.Errorf("relation has incorrect format")
		}

		relFound = pbtypes.CopyRelation(rel)
		for _, opt := range rel.SelectDict {
			if option.Id == opt.Id {
				return fmt.Errorf("option already exists")
			}
		}
		if option.Scope != pbrelation.RelationOption_local {
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

		optionCopy.Scope = pbrelation.RelationOption_format
		rel.SelectDict = append(rel.SelectDict, &optionCopy)
	}

	return nil
}

func (d *Dataview) UpdateRelationOption(relationKey string, option pbrelation.RelationOption) error {
	if option.Scope != pbrelation.RelationOption_local {
		return fmt.Errorf("incorrect option scope")
	}

	relFound := pbtypes.GetRelation(d.content.Relations, relationKey)

	for _, rel := range d.content.Relations {
		if rel.Key != relationKey{
			if relFound.Format != rel.Format {
				continue
			}
			option.Scope = pbrelation.RelationOption_format
		} else {
			option.Scope = pbrelation.RelationOption_local
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
	for _, rel := range d.content.Relations {
		if rel.Key != relationKey {
			continue
		}

		for i, opt := range rel.SelectDict {
			if optId == opt.Id {
				rel.SelectDict = append(rel.SelectDict[:i], rel.SelectDict[i+1:]...)
				return nil
			}
		}

		return fmt.Errorf("option not exists")
	}
	return nil
}
