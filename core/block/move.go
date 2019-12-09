package block

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (p *commonSmart) Move(req pb.RpcBlockListMoveRequest) (err error) {
	p.m.Lock()
	defer p.m.Unlock()

	s := p.newState()

	if findPosInSlice(req.BlockIds, req.DropTargetId) != -1 {
		return fmt.Errorf("blockIds contains targetId")
	}

	if err = p.cut(s, req.BlockIds...); err != nil {
		return
	}

	target := s.get(req.DropTargetId)
	if target == nil {
		return fmt.Errorf("target block %s not found", req.DropTargetId)
	}
	targetParent := s.findParentOf(req.DropTargetId)
	if targetParent == nil {
		return fmt.Errorf("target has not parent")
	}
	targetParentM := targetParent.Model()

	targetPos := findPosInSlice(targetParentM.ChildrenIds, target.Model().Id)
	if targetPos == -1 {
		return fmt.Errorf("target[%s] is not a child of parent[%s]", target.Model().Id, targetParentM.Id)
	}
	var pos int
	switch req.Position {
	case model.Block_After:
		pos = targetPos + 1
	case model.Block_Before:
		pos = targetPos
	case model.Block_Inner:
		pos = -1
		targetParent = target
		targetParentM = target.Model()
	default:
		return fmt.Errorf("unexpected position")
	}

	if pos != -1 {
		for _, id := range req.BlockIds {
			targetParentM.ChildrenIds = insertToSlice(targetParentM.ChildrenIds, id, pos)
			pos++
		}
	} else {
		targetParentM.ChildrenIds = append(targetParentM.ChildrenIds, req.BlockIds...)
	}
	return p.applyAndSendEvent(s)
}

func (p *commonSmart) cut(s *state, ids ...string) (err error) {
	for _, id := range ids {
		if b := s.get(id); b != nil {
			if parent := s.findParentOf(id); parent != nil {
				parent.Model().ChildrenIds = removeFromSlice(parent.Model().ChildrenIds, id)
			}
		} else {
			err = fmt.Errorf("block '%s' not found", id)
			return
		}
	}
	return
}
