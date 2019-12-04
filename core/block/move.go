package block

import (
	"fmt"

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
	targetParent := p.findParentOf(req.DropTargetId, blocks, p.versions)
	if targetParent == nil {
		return fmt.Errorf("target has not parent")
	}
	targetParent = targetParent.Copy()

	

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
