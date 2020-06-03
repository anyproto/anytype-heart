package state

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
)

func NewDocFromSnapshot(rootId string, snapshot *pb.ChangeSnapshot) Doc {
	blocks := make(map[string]simple.Block)
	for _, b := range snapshot.Data.Blocks {
		blocks[b.Id] = simple.New(b)
	}
	return &State{
		rootId:  rootId,
		blocks:  blocks,
		details: snapshot.Data.Details,
	}
}

func (s *State) SetChangeId(id string) {
	s.changeId = id
}

func (s *State) ChangeId() string {
	return s.changeId
}

func (s *State) AddChanges(changes ...*pb.ChangeContent) *State {
	s.DisableAutoChanges()
	s.changes = append(s.changes, changes...)
	return s
}

func (s *State) ApplyChange(changes ...*pb.ChangeContent) (err error) {
	for _, ch := range changes {
		switch {
		case ch.GetBlockCreate() != nil:
			if err = s.changeBlockCreate(ch.GetBlockCreate()); err != nil {
				return
			}
		case ch.GetBlockRemove() != nil:
			if err = s.changeBlockRemove(ch.GetBlockRemove()); err != nil {
				return
			}
		case ch.GetBlockUpdate() != nil:
			if err = s.changeBlockUpdate(ch.GetBlockUpdate()); err != nil {
				return
			}
		case ch.GetBlockMove() != nil:
			if err = s.changeBlockMove(ch.GetBlockMove()); err != nil {
				return
			}
		case ch.GetBlockDuplicate() != nil:
			if err = s.changeBlockDuplicate(ch.GetBlockDuplicate()); err != nil {
				return
			}
		}
	}
	return
}

func (s *State) changeBlockCreate(bc *pb.ChangeBlockCreate) (err error) {
	var bIds = make([]string, len(bc.Blocks))
	for i, m := range bc.Blocks {
		b := simple.New(m)
		bIds[i] = b.Model().Id
		s.Add(b)
	}
	return s.InsertTo(bc.TargetId, bc.Position, bIds...)
}

func (s *State) changeBlockRemove(remove *pb.ChangeBlockRemove) error {
	for _, id := range remove.Ids {
		s.Remove(id)
	}
	return nil
}

func (s *State) changeBlockUpdate(update *pb.ChangeBlockUpdate) error {
	for _, ev := range update.Events {
		if err := s.applyEvent(ev); err != nil {
			return err
		}
	}
	return nil
}

func (s *State) changeBlockMove(move *pb.ChangeBlockMove) error {
	for _, id := range move.Ids {
		s.Unlink(id)
	}
	return s.InsertTo(move.TargetId, move.Position, move.Ids...)
}

func (s *State) changeBlockDuplicate(duplicate *pb.ChangeBlockDuplicate) error {
	// TODO:
	return nil
}

func (s *State) Merge(st *State) *State {
	result := s.NewState()
	// TODO:
	return result
}

func (s *State) GetChanges() []*pb.ChangeContent {
	res := s.changes
	if s.parent != nil {
		res = append(res, s.parent.GetChanges()...)
	}
	return res
}

func (s *State) DisableAutoChanges() *State {
	s.noAutoChanges = true
	return s
}

func (s *State) fillAutoChange(msgs []*pb.EventMessage) {
	var updMsgs = make([]*pb.EventMessage, 0, len(msgs))
	var delIds []string
	for _, msg := range msgs {
		switch o := msg.Value.(type) {
		case *pb.EventMessageValueOfBlockSetAlign:
			updMsgs = append(updMsgs, msg)
		case *pb.EventMessageValueOfBlockSetBackgroundColor:
			updMsgs = append(updMsgs, msg)
		case *pb.EventMessageValueOfBlockSetBookmark:
			updMsgs = append(updMsgs, msg)
		case *pb.EventMessageValueOfBlockSetDiv:
			updMsgs = append(updMsgs, msg)
		case *pb.EventMessageValueOfBlockSetText:
			updMsgs = append(updMsgs, msg)
		case *pb.EventMessageValueOfBlockSetFields:
			updMsgs = append(updMsgs, msg)
		case *pb.EventMessageValueOfBlockSetFile:
			updMsgs = append(updMsgs, msg)
		case *pb.EventMessageValueOfBlockSetLink:
			updMsgs = append(updMsgs, msg)
		case *pb.EventMessageValueOfBlockDelete:
			delIds = append(delIds, o.BlockDelete.BlockIds...)
		case *pb.EventMessageValueOfBlockAdd:
			for _, b := range o.BlockAdd.Blocks {
				s.changes = append(s.changes, s.makeCreateChange(b.Id))
			}
		}
	}
	if len(updMsgs) > 0 {
		s.changes = append(s.changes, &pb.ChangeContent{
			Value: &pb.ChangeContentValueOfBlockUpdate{
				BlockUpdate: &pb.ChangeBlockUpdate{
					Events: updMsgs,
				},
			},
		})
	}
	if len(delIds) > 0 {
		s.changes = append(s.changes, &pb.ChangeContent{
			Value: &pb.ChangeContentValueOfBlockRemove{
				BlockRemove: &pb.ChangeBlockRemove{
					Ids: delIds,
				},
			},
		})
	}
}

func (s *State) makeCreateChange(id string) (ch *pb.ChangeContent) {
	var create = &pb.ChangeBlockCreate{}
	ch = &pb.ChangeContent{
		Value: &pb.ChangeContentValueOfBlockCreate{
			BlockCreate: create,
		},
	}
	create.Blocks = []*model.Block{s.Pick(id).Copy().Model()}
	parent := s.PickParentOf(id)
	if parent == nil {
		return
	}
	pm := parent.Model()
	pos := slice.FindPos(pm.ChildrenIds, id)
	if pos == 0 {
		if len(pm.ChildrenIds) == 1 {
			create.TargetId = pm.Id
			create.Position = model.Block_Inner
			return
		}
		create.TargetId = pm.ChildrenIds[1]
		create.Position = model.Block_Top
		return
	}
	if pos > 0 {
		create.TargetId = pm.ChildrenIds[pos-1]
		create.Position = model.Block_Bottom
	}
	return
}

func newChangeMove(targetId string, pos model.BlockPosition, ids ...string) *pb.ChangeContent {
	return &pb.ChangeContent{
		Value: &pb.ChangeContentValueOfBlockMove{
			BlockMove: &pb.ChangeBlockMove{
				TargetId: targetId,
				Position: pos,
				Ids:      ids,
			},
		},
	}
}
