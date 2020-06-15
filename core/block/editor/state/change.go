package state

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
	"github.com/anytypeio/go-anytype-middleware/util/slice"
	"github.com/gogo/protobuf/types"
	"github.com/mb0/diff"
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
		case ch.GetDetailsSet() != nil:
			if err = s.changeBlockDetailsSet(ch.GetDetailsSet()); err != nil {
				return
			}
		case ch.GetDetailsUnset() != nil:
			if err = s.changeBlockDetailsUnset(ch.GetDetailsUnset()); err != nil {
				return
			}
		}
	}
	return
}

func (s *State) changeBlockDetailsSet(set *pb.ChangeDetailsSet) error {
	det := s.Details()
	if det == nil {
		det = &types.Struct{
			Fields: make(map[string]*types.Value),
		}
	}
	s.details = pbtypes.CopyStruct(det)
	s.details.Fields[set.Key] = set.Value
	return nil
}

func (s *State) changeBlockDetailsUnset(unset *pb.ChangeDetailsUnset) error {
	det := s.Details()
	if det == nil {
		det = &types.Struct{
			Fields: make(map[string]*types.Value),
		}
	}
	s.details = pbtypes.CopyStruct(det)
	delete(s.details.Fields, unset.Key)
	return nil
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
		s.Unlink(id)
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

func (s *State) Merge(st *State) *State {
	result := s.NewState()
	// TODO:
	return result
}

func (s *State) GetChanges() []*pb.ChangeContent {
	return s.changes
}

func (s *State) fillChanges(msgs []*pb.EventMessage) {
	var updMsgs = make([]*pb.EventMessage, 0, len(msgs))
	var delIds []string
	var structMsgs = make([]*pb.EventBlockSetChildrenIds, 0, len(msgs))
	for _, msg := range msgs {
		switch o := msg.Value.(type) {
		case *pb.EventMessageValueOfBlockSetChildrenIds:
			structMsgs = append(structMsgs, o.BlockSetChildrenIds)
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
				if len(b.ChildrenIds) > 0 {
					structMsgs = append(structMsgs, &pb.EventBlockSetChildrenIds{
						Id:          b.Id,
						ChildrenIds: b.ChildrenIds,
					})
				}
			}
		default:
			log.Errorf("unexpected event - can't convert to changes: %T", msg)
		}
	}
	var cb = &changeBuilder{changes: s.changes}
	if len(structMsgs) > 0 {
		s.fillStructureChanges(cb, structMsgs)
	}
	if len(delIds) > 0 {
		cb.AddChange(&pb.ChangeContent{
			Value: &pb.ChangeContentValueOfBlockRemove{
				BlockRemove: &pb.ChangeBlockRemove{
					Ids: delIds,
				},
			},
		})
	}
	if len(updMsgs) > 0 {
		cb.AddChange(&pb.ChangeContent{
			Value: &pb.ChangeContentValueOfBlockUpdate{
				BlockUpdate: &pb.ChangeBlockUpdate{
					Events: updMsgs,
				},
			},
		})
	}
	s.changes = cb.Build()
	s.changes = append(s.changes, s.makeDetailsChanges()...)
}

func (s *State) fillStructureChanges(cb *changeBuilder, msgs []*pb.EventBlockSetChildrenIds) {
	for _, msg := range msgs {
		s.makeStructureChanges(cb, msg)
	}
}

func (s *State) makeStructureChanges(cb *changeBuilder, msg *pb.EventBlockSetChildrenIds) (ch []*pb.ChangeContent) {
	if slice.FindPos(s.changesStructureIgnoreIds, msg.Id) != -1 {
		return
	}
	var before []string
	orig := s.PickOrigin(msg.Id)
	if orig != nil {
		before = orig.Model().ChildrenIds
	}

	ds := &dstrings{a: before, b: msg.ChildrenIds}
	d := diff.Diff(len(ds.a), len(ds.b), ds)
	var (
		targetId  string
		targetPos model.BlockPosition
	)
	var makeTarget = func(pos int) {
		if pos == 0 {
			if len(ds.a) == 0 {
				targetId = msg.Id
				targetPos = model.Block_Inner
			} else {
				targetId = ds.a[0]
				targetPos = model.Block_Top
			}
		} else {
			targetId = ds.b[pos-1]
			targetPos = model.Block_Bottom
		}
	}
	for _, c := range d {
		if c.Ins > 0 {
			prevOp := 0
			for ins := 0; ins < c.Ins; ins++ {
				pos := c.B + ins
				id := ds.b[pos]
				if slice.FindPos(s.newIds, id) != -1 {
					if prevOp != 1 {
						makeTarget(pos)
					}
					cb.Add(targetId, targetPos, s.Pick(id).Copy().Model())
					prevOp = 1
				} else {
					if prevOp != 2 {
						makeTarget(pos)
					}
					cb.Move(targetId, targetPos, id)
					prevOp = 2
				}
			}
		}
	}
	return
}

func (s *State) makeDetailsChanges() (ch []*pb.ChangeContent) {
	if s.details == nil || s.details.Fields == nil {
		return nil
	}
	var prev *types.Struct
	if s.parent != nil {
		prev = s.parent.Details()
	}
	if prev == nil || prev.Fields == nil {
		prev = &types.Struct{Fields: make(map[string]*types.Value)}
	}
	for k, v := range s.details.Fields {
		pv, ok := prev.Fields[k]
		if !ok || !pv.Equal(v) {
			ch = append(ch, &pb.ChangeContent{
				Value: &pb.ChangeContentValueOfDetailsSet{
					DetailsSet: &pb.ChangeDetailsSet{Key: k, Value: v},
				},
			})
		}
	}
	for k := range prev.Fields {
		if _, ok := s.details.Fields[k]; !ok {
			ch = append(ch, &pb.ChangeContent{
				Value: &pb.ChangeContentValueOfDetailsUnset{
					DetailsUnset: &pb.ChangeDetailsUnset{Key: k},
				},
			})
		}
	}
	return
}

type dstrings struct{ a, b []string }

func (d *dstrings) Equal(i, j int) bool { return d.a[i] == d.b[j] }

type changeBuilder struct {
	changes []*pb.ChangeContent

	isLastAdd    bool
	lastTargetId string
	lastPosition model.BlockPosition
	lastIds      []string
	lastBlocks   []*model.Block
}

func (cb *changeBuilder) Move(targetId string, pos model.BlockPosition, id string) {
	if cb.isLastAdd || cb.lastTargetId != targetId || cb.lastPosition != pos {
		cb.Flush()
	}
	cb.isLastAdd = false
	cb.lastTargetId = targetId
	cb.lastPosition = pos
	cb.lastIds = append(cb.lastIds, id)
}

func (cb *changeBuilder) Add(targetId string, pos model.BlockPosition, m *model.Block) {
	if !cb.isLastAdd || cb.lastTargetId != targetId || cb.lastPosition != pos {
		cb.Flush()
	}
	m.ChildrenIds = nil
	cb.isLastAdd = true
	cb.lastTargetId = targetId
	cb.lastPosition = pos
	cb.lastBlocks = append(cb.lastBlocks, m)
}

func (cb *changeBuilder) AddChange(ch ...*pb.ChangeContent) {
	cb.Flush()
	cb.changes = append(cb.changes, ch...)
}

func (cb *changeBuilder) Flush() {
	if cb.lastTargetId == "" {
		return
	}
	if cb.isLastAdd && len(cb.lastBlocks) > 0 {
		var create = &pb.ChangeBlockCreate{
			TargetId: cb.lastTargetId,
			Position: cb.lastPosition,
			Blocks:   cb.lastBlocks,
		}
		cb.changes = append(cb.changes, &pb.ChangeContent{
			Value: &pb.ChangeContentValueOfBlockCreate{
				BlockCreate: create,
			},
		})
	} else if !cb.isLastAdd && len(cb.lastIds) > 0 {
		var move = &pb.ChangeBlockMove{
			TargetId: cb.lastTargetId,
			Position: cb.lastPosition,
			Ids:      cb.lastIds,
		}
		cb.changes = append(cb.changes, &pb.ChangeContent{
			Value: &pb.ChangeContentValueOfBlockMove{
				BlockMove: move,
			},
		})
	}
	cb.lastTargetId = ""
	cb.lastBlocks = nil
	cb.lastIds = nil
}

func (cb *changeBuilder) Build() []*pb.ChangeContent {
	cb.Flush()
	return cb.changes
}
