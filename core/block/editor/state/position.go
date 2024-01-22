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
		s.cacheInsert(targetParentM, pos, ids)
	case model.Block_Top:
		pos = targetPos
		s.cacheInsert(targetParentM, pos, ids)
	case model.Block_Left, model.Block_Right:
		if err = s.moveFromSide(target, s.Get(targetParentM.Id), reqPos, ids...); err != nil {
			return
		}
	case model.Block_Inner:
		target.Model().ChildrenIds = s.cacheAppendStart(target, ids)
	case model.Block_Replace:
		pos = targetPos + 1
		id0Block := s.Get(ids[0]).Model()
		if len(ids) > 0 && len(id0Block.ChildrenIds) == 0 {
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
				s.cacheParent(id0Block, target.Model().ChildrenIds)
			}
		}
		s.cacheInsert(targetParentM, pos, ids)
		s.Unlink(target.Model().Id)
	case model.Block_InnerFirst:
		s.cacheAppendEnd(target.Model(), ids)
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
	column := s.cacheColumnCreation(opId, ids)
	s.Add(column)

	targetPos := slice.FindPos(row.Model().ChildrenIds, target.Model().Id)
	if targetPos == -1 {
		return fmt.Errorf("target[%s] is not a child of row[%s]", target.Model().Id, row.Model().Id)
	}

	columnPos := targetPos
	if pos == model.Block_Right {
		columnPos += 1
	}
	row.Model().ChildrenIds = s.cacheInsertMoveFromSide(row.Model(), columnPos, column.Model())
	s.changesStructureIgnoreIds = append(s.changesStructureIgnoreIds, "cd-"+opId, "ct-"+opId, "r-"+opId, row.Model().Id)
	return
}

func (s *State) wrapToRow(opId string, parent, b simple.Block) (row simple.Block, err error) {
	column := s.cacheWrapToColumn(opId, b)
	s.Add(column)
	row = s.cacheWriteToRaw(opId, column)
	s.Add(row)
	pos := slice.FindPos(parent.Model().ChildrenIds, b.Model().Id)
	if pos == -1 {
		return nil, fmt.Errorf("creating row: can't find child[%s] in given parent[%s]", b.Model().Id, parent.Model().Id)
	}
	parent.Model().ChildrenIds[pos] = row.Model().Id
	return
}

func (s *State) cacheParent(parent *model.Block, childrenIds []string) {
	s.cacheParentUntilSame(parent, childrenIds, false)
}

func (s *State) cacheParentUntilSame(parent *model.Block, childrenIds []string, untilSame bool) {
	parent.ChildrenIds = childrenIds
	if s.isIdsCacheInited() {
		cache := s.getSubIdsCache()
		for _, childId := range childrenIds {
			if untilSame {
				if curParentId, ok := cache[childId]; ok && curParentId == parent.Id {
					break
				}
			}
			cache[childId] = parent.Id
		}
	}
}

func (s *State) cacheAppendStart(target simple.Block, ids []string) []string {
	result := append(target.Model().ChildrenIds, ids...)
	s.cacheParent(target.Model(), result)
	return result
}

func (s *State) cacheAppendEnd(target *model.Block, ids []string) {
	result := append(ids, target.ChildrenIds...)
	s.cacheParent(target, result)
}

func (s *State) cacheInsert(target *model.Block, pos int, ids []string) {
	result := slice.Insert(target.ChildrenIds, pos, ids...)
	s.cacheParent(target, result)
}

func (s *State) cacheColumnCreation(opId string, ids []string) simple.Block {
	result := simple.New(&model.Block{
		Id:          "cd-" + opId,
		ChildrenIds: ids,
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Column,
			},
		},
	})
	s.cacheParent(result.Model(), ids)
	return result
}

func (s *State) cacheInsertMoveFromSide(row *model.Block, columnPos int, column *model.Block) []string {
	result := slice.Insert(row.ChildrenIds, columnPos, column.Id)
	s.cacheParent(row, result)
	return result
}

func (s *State) cacheWriteToRaw(opId string, column simple.Block) simple.Block {
	result := simple.New(&model.Block{
		Id:          "r-" + opId,
		ChildrenIds: []string{column.Model().Id},
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Row,
			},
		},
	})
	s.cacheParent(result.Model(), result.Model().ChildrenIds)
	return result
}

func (s *State) cacheWrapToColumn(opId string, b simple.Block) simple.Block {
	result := simple.New(&model.Block{
		Id:          "ct-" + opId,
		ChildrenIds: []string{b.Model().Id},
		Content: &model.BlockContentOfLayout{
			Layout: &model.BlockContentLayout{
				Style: model.BlockContentLayout_Column,
			},
		},
	})
	s.cacheParent(result.Model(), result.Model().ChildrenIds)
	return result
}
