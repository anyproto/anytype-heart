package dataview

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
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

	FillSmartIds(ids []string) []string
	HasSmartIds() bool
}

type Dataview struct {
	*base.Base
	content      *model.BlockContentDataview
	recordIDs    []string
	activeViewID string
	offset       int
	limit        int
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
					Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockSetDataviewView{
						&pb.EventBlockSetDataviewView{
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
				simple.EventMessage{Msg: &pb.EventMessage{Value: &pb.EventMessageValueOfBlockDeleteDataviewView{
					&pb.EventBlockDeleteDataviewView{
						Id:     dv.Id,
						ViewId: view1.Id,
					}}}})
		}
	}

	// @TODO: rewrite for optimised compare

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

func (l *Dataview) FillSmartIds(ids []string) []string {
	//@todo: fill from recordIDs
	return ids
}

func (l *Dataview) HasSmartIds() bool {
	return len(l.recordIDs) > 0
}
