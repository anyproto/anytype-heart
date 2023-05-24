package state

import (
	"crypto/md5"
	"encoding/binary"
	"encoding/hex"
	"fmt"

	"github.com/anyproto/anytype-heart/core/block/simple"
	"github.com/anyproto/anytype-heart/pkg/lib/pb/model"
	"github.com/anyproto/anytype-heart/util/slice"
)

func (s *State) InsertTo(targetId string, reqPos model.BlockPosition, ids ...string) (err error) {
	var (
		target        simple.Block
		targetParentM *model.Block
		targetPos     int
	)

	if len(ids) == 0 {
		return
	}

	if targetId == "" {
		reqPos = model.Block_Inner
		target = s.Get(s.RootId())
	} else {
		target = s.Get(targetId)
		if target == nil {
			return fmt.Errorf("target block not found")
		}
		if reqPos != model.Block_Inner && reqPos != model.Block_InnerFirst {
			if pv := s.GetParentOf(targetId); pv != nil {
				targetParentM = pv.Model()
			} else {
				return fmt.Errorf("target without parent")
			}
			targetPos = slice.FindPos(targetParentM.ChildrenIds, target.Model().Id)
		}
	}

	if targetId != "" && slice.FindPos(ids, targetId) != -1 {
		return fmt.Errorf("blockIds contains target")
	}
	if targetParentM != nil && slice.FindPos(ids, targetParentM.Id) != -1 {
		return fmt.Errorf("blockIds contains parent")
	}

	if targetPos == -1 {
		return fmt.Errorf("target[%s] is not a child of parent[%s]", target.Model().Id, targetParentM.Id)
	}

	var pos int
	switch reqPos {
	case model.Block_Bottom:
		pos = targetPos + 1
		targetParentM.ChildrenIds = slice.Insert(targetParentM.ChildrenIds, pos, ids...)
	case model.Block_Top:
		pos = targetPos
		targetParentM.ChildrenIds = slice.Insert(targetParentM.ChildrenIds, pos, ids...)
	case model.Block_Left, model.Block_Right:
		if err = s.moveFromSide(target, s.Get(targetParentM.Id), reqPos, ids...); err != nil {
			return
		}
	case model.Block_Inner:
		target.Model().ChildrenIds = append(target.Model().ChildrenIds, ids...)
	case model.Block_Replace:
		pos = targetPos + 1
		if len(ids) > 0 && len(s.Get(ids[0]).Model().ChildrenIds) == 0 {
			var idsIsChild bool
			if targetChild := target.Model().ChildrenIds; len(targetChild) > 0 {
				for _, id := range ids {
					if slice.FindPos(targetChild, id) != -1 {
						idsIsChild = true
						break
					}
				}
			}
			if !idsIsChild {
				s.Get(ids[0]).Model().ChildrenIds = target.Model().ChildrenIds
			}
		}
		targetParentM.ChildrenIds = slice.Insert(targetParentM.ChildrenIds, pos, ids...)
		s.Unlink(target.Model().Id)
	case model.Block_InnerFirst:
		target.Model().ChildrenIds = append(ids, target.Model().ChildrenIds...)
	default:
		return fmt.Errorf("unexpected position")
	}
	return
}

func makeOpId(target simple.Block, pos model.BlockPosition, ids ...string) string {
	var del = [...]byte{'-'}
	h := md5.New()
	h.Write([]byte(target.Model().Id))
	h.Write(del[:])
	binary.Write(h, binary.LittleEndian, pos)
	h.Write(del[:])
	for _, id := range ids {
		h.Write([]byte(id))
		h.Write(del[:])
	}
	return hex.EncodeToString(h.Sum(nil))
}

// addChangesForSideMoving adds changes for moving blocks to side of another blocks.
// It creates the first change with position=Left or position=Right to create row-column structure, then
// other changes are created with position=Bottom to only add blocks into existing row-column structure.
func (s *State) addChangesForSideMoving(targetID string, pos model.BlockPosition, ids ...string) {
	type operation int
	const (
		operationNone operation = iota
		operationAdd
		operationMove
	)
	cb := &changeBuilder{changes: s.changes}
	lastTargetID := targetID
	lastOperation := operationNone
	for _, id := range ids {
		if s.parent.Exists(id) {
			if lastOperation == operationAdd {
				targetID = lastTargetID
				pos = model.Block_Bottom
			}
			cb.Move(targetID, pos, id)
			lastOperation = operationMove
			lastTargetID = id
		} else if blockToAdd := s.Get(id); blockToAdd != nil {
			if lastOperation == operationMove {
				targetID = lastTargetID
				pos = model.Block_Bottom
			}
			cb.Add(targetID, pos, blockToAdd.Model())
			lastOperation = operationAdd
			lastTargetID = id
		} else {
			log.With("rootID", s.RootId()).Errorf("side moving: trying to add change for missing block: %s", id)
		}
	}
	s.changes = cb.Build()
}

func (s *State) moveFromSide(target, parent simple.Block, pos model.BlockPosition, ids ...string) (err error) {
	s.addChangesForSideMoving(target.Model().Id, pos, ids...)

	opId := makeOpId(target, pos, ids...)
	row := parent
	if row == nil {
		return fmt.Errorf("target block has not parent")
	}
	if s.Exists("cd-" + opId) {
		return fmt.Errorf("nothing to do")
	}
	if row.Model().GetLayout() == nil || row.Model().GetLayout().Style != model.BlockContentLayout_Row {
		s.changesStructureIgnoreIds = append(s.changesStructureIgnoreIds, row.Model().Id)
		if row, err = s.wrapToRow(opId, row, target); err != nil {
			return
		}
		target = s.Get(row.Model().ChildrenIds[0])
	}
	column := simple.New(&model.Block{
		Id:          "cd-" + opId,
		ChildrenIds: ids,
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Column,
			},
		},
	})
	s.Add(column)

	targetPos := slice.FindPos(row.Model().ChildrenIds, target.Model().Id)
	if targetPos == -1 {
		return fmt.Errorf("target[%s] is not a child of row[%s]", target.Model().Id, row.Model().Id)
	}

	columnPos := targetPos
	if pos == model.Block_Right {
		columnPos += 1
	}
	row.Model().ChildrenIds = slice.Insert(row.Model().ChildrenIds, columnPos, column.Model().Id)
	s.changesStructureIgnoreIds = append(s.changesStructureIgnoreIds, "cd-"+opId, "ct-"+opId, "r-"+opId, row.Model().Id)
	return
}

func (s *State) wrapToRow(opId string, parent, b simple.Block) (row simple.Block, err error) {
	column := simple.New(&model.Block{
		Id:          "ct-" + opId,
		ChildrenIds: []string{b.Model().Id},
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Column,
			},
		},
	})
	s.Add(column)
	row = simple.New(&model.Block{
		Id:          "r-" + opId,
		ChildrenIds: []string{column.Model().Id},
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Row,
			},
		},
	})
	s.Add(row)
	pos := slice.FindPos(parent.Model().ChildrenIds, b.Model().Id)
	if pos == -1 {
		return nil, fmt.Errorf("creating row: can't find child[%s] in given parent[%s]", b.Model().Id, parent.Model().Id)
	}
	parent.Model().ChildrenIds[pos] = row.Model().Id
	return
}
