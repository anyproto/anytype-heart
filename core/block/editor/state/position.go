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

type childrenInheritableOnReplace interface {
	CanInheritChildrenOnReplace()
}

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
		target = s.Get(s.RootId())
		if target == nil {
			return fmt.Errorf("target (root) block not found")
		}
		// target block is root, so we should support only inner insertions
		if reqPos != model.Block_InnerFirst {
			reqPos = model.Block_Inner
		}
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
		s.insertChildrenIds(targetParentM, pos, ids...)
	case model.Block_Top:
		pos = targetPos
		s.insertChildrenIds(targetParentM, pos, ids...)
	case model.Block_Left, model.Block_Right:
		if err = s.moveFromSide(target, s.Get(targetParentM.Id), reqPos, ids...); err != nil {
			return
		}
	case model.Block_Inner:
		s.prependChildrenIds(target.Model(), ids...)
	case model.Block_Replace:
		s.insertReplace(target, targetParentM, targetPos, ids...)
	case model.Block_InnerFirst:
		s.appendChildrenIds(target.Model(), ids...)
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
			cb.Add(targetID, pos, blockToAdd.Copy().Model())
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
	column := s.addNewColumn(opId, ids)

	targetPos := slice.FindPos(row.Model().ChildrenIds, target.Model().Id)
	if targetPos == -1 {
		return fmt.Errorf("target[%s] is not a child of row[%s]", target.Model().Id, row.Model().Id)
	}

	columnPos := targetPos
	if pos == model.Block_Right {
		columnPos += 1
	}
	s.insertChildrenIds(row.Model(), columnPos, column.Model().Id)
	s.changesStructureIgnoreIds = append(s.changesStructureIgnoreIds, "cd-"+opId, "ct-"+opId, "r-"+opId, row.Model().Id)
	return
}

func (s *State) wrapToRow(opId string, parent, b simple.Block) (row simple.Block, err error) {
	column := s.addNewBlockAndWrapToColumn(opId, b)
	row = s.addNewColumnToRow(opId, column)
	pos := slice.FindPos(parent.Model().ChildrenIds, b.Model().Id)
	if pos == -1 {
		return nil, fmt.Errorf("creating row: can't find child[%s] in given parent[%s]", b.Model().Id, parent.Model().Id)
	}
	// do not need to remove from cache
	parent.Model().ChildrenIds[pos] = row.Model().Id
	return
}

func (s *State) setChildrenIds(parent *model.Block, childrenIds []string) {
	parent.ChildrenIds = childrenIds
}

// do not use this method outside of normalization
func (s *State) SetChildrenIds(parent *model.Block, childrenIds []string) {
	s.setChildrenIds(parent, childrenIds)
}

func (s *State) removeChildren(parent *model.Block, childrenId string) {
	parent.ChildrenIds = slice.RemoveMut(parent.ChildrenIds, childrenId)
}

func (s *State) prependChildrenIds(block *model.Block, ids ...string) {
	s.setChildrenIds(block, append(block.ChildrenIds, ids...))
}

func (s *State) appendChildrenIds(block *model.Block, ids ...string) {
	s.setChildrenIds(block, append(ids, block.ChildrenIds...))
}

func (s *State) insertChildrenIds(block *model.Block, pos int, ids ...string) {
	s.setChildrenIds(block, slice.Insert(block.ChildrenIds, pos, ids...))
}

func (s *State) addNewColumn(opId string, ids []string) simple.Block {
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
	return column
}

func (s *State) addNewColumnToRow(opId string, column simple.Block) simple.Block {
	row := simple.New(&model.Block{
		Id:          "r-" + opId,
		ChildrenIds: []string{column.Model().Id},
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Row,
			},
		},
	})
	s.Add(row)
	return row
}

func (s *State) addNewBlockAndWrapToColumn(opId string, b simple.Block) simple.Block {
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
	return column
}

func (s *State) insertReplace(target simple.Block, targetParentM *model.Block, targetPos int, ids ...string) {
	if len(ids) == 0 {
		return
	}
	id0Block := s.Get(ids[0])
	_, canInheritChildren := id0Block.(childrenInheritableOnReplace)
	targetHasChildren := false
	pos := targetPos + 1
	if !canInheritChildren {
		pos = targetPos
	}
	if len(id0Block.Model().ChildrenIds) == 0 {
		var idsIsChild bool
		if targetChild := target.Model().ChildrenIds; len(targetChild) > 0 {
			targetHasChildren = true
			for _, id := range ids {
				if slice.FindPos(targetChild, id) != -1 {
					idsIsChild = true
					break
				}
			}
		}
		if !idsIsChild && canInheritChildren {
			s.setChildrenIds(id0Block.Model(), target.Model().ChildrenIds)
		}
	}
	s.insertChildrenIds(targetParentM, pos, ids...)
	if canInheritChildren || !targetHasChildren {
		s.Unlink(target.Model().Id)
	}
}
