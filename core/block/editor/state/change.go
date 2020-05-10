package state

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/merge/change/chmodel"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (s *State) SetChangeId(id string) {
	s.changeId = id
}

func (s *State) ChangeId() string {
	return s.changeId
}

func (s *State) ApplyChange(change chmodel.Change) (err error) {
	switch change.Type {
	case chmodel.TypeAdd:
		return s.changeAdd(change.Value.(chmodel.ChangeValueBlockPosition))
	case chmodel.TypeMove:
		return s.changeMove(change.Value.(chmodel.ChangeValueBlockPosition))
	case chmodel.TypeUpdate:
		return s.changeUpdate(change.Value.([]pb.EventMessage))
	}
	return
}

func (s *State) changeAdd(p chmodel.ChangeValueBlockPosition) (err error) {
	p.BlockIds = make([]string, len(p.Blocks))
	for i, b := range p.Blocks {
		sb := simple.New(b)
		p.BlockIds[i] = sb.Model().Id
		s.Add(sb)
	}
	return s.InsertTo(p.TargetId, p.Position, p.BlockIds...)
}

func (s *State) changeMove(p chmodel.ChangeValueBlockPosition) (err error) {
	for _, bid := range p.BlockIds {
		s.Unlink(bid)
	}
	return s.InsertTo(p.TargetId, p.Position, p.BlockIds...)
}

func (s *State) changeUpdate(events []pb.EventMessage) (err error) {
	getBlock := func(id string) (b simple.Block, err error) {
		if b = s.Get(id); b == nil {
			err = fmt.Errorf("simple block not found")
		}
		return
	}
	for _, e := range events {
		switch {
		case e.GetBlockSetBackgroundColor() != nil:
			bc := e.GetBlockSetBackgroundColor()
			b, er := getBlock(bc.Id)
			if er != nil {
				return er
			}
			b.Model().BackgroundColor = bc.BackgroundColor
		case e.GetBlockSetAlign() != nil:
			bc := e.GetBlockSetAlign()
			b, er := getBlock(bc.Id)
			if er != nil {
				return er
			}
			b.Model().Align = bc.Align
		}
	}
	return
}


func (s *State) Merge(st *State) *State {
	result := s.NewState()

	return result
}
