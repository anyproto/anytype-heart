package block

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

func (p *commonSmart) Move(req pb.RpcBlockListMoveRequest) (err error) {
	p.m.Lock()
	defer p.m.Unlock()

	if findPosInSlice(req.BlockIds, req.DropTargetId) != -1 {
		return fmt.Errorf("blockIds contains targetId")
	}

	blocks, err := p.cut(req.BlockIds...)
	if err != nil {
		return
	}

	target, ok := p.versions[req.DropTargetId]
	if ! ok {
		return fmt.Errorf("target block %s not found", req.DropTargetId)
	}
	target = target.Copy()
	parent := p.findParentOf(req.DropTargetId, blocks, p.versions)
	if parent == nil {
		return fmt.Errorf("target has not parent")
	}
	targetParent := parent.Copy()
	targetParentM := targetParent.Model()

	targetPos := findPosInSlice(parent.Model().ChildrenIds, target.Model().Id)
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
	blocks[targetParentM.Id] = targetParent

	var msgs []*pb.EventMessage
	var updBlocks []*model.Block
	for id, b := range blocks {
		diff, err := p.versions[id].Diff(b)
		if err != nil {
			return err
		}
		if len(diff) > 0 {
			msgs = append(msgs, diff...)
			if ! b.Virtual() {
				updBlocks = append(updBlocks, p.toSave(b.Model()))
			}
		}
	}
	if _, err := p.block.AddVersions(updBlocks); err != nil {
		return err
	}
	for _, b := range updBlocks {
		p.versions[b.Id] = blocks[b.Id]
	}

	p.s.sendEvent(&pb.Event{
		Messages:  msgs,
		ContextId: p.GetId(),
	})

	return nil
}

func (p *commonSmart) cut(ids ...string) (blocks map[string]simple.Block, err error) {
	blocks = make(map[string]simple.Block)
	for _, id := range ids {
		if b, ok := p.versions[id]; ok {
			blocks[id] = b.Copy()
			if parent := p.findParentOf(id); parent != nil {
				parent = parent.Copy()
				parent.Model().ChildrenIds = removeFromSlice(parent.Model().ChildrenIds, id)
				blocks[parent.Model().Id] = parent
			}
		} else {
			err = fmt.Errorf("block '%s' not found", id)
			return
		}
	}
	return
}
