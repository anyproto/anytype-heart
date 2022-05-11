package basic

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
)

type StateTransform struct {
	*state.State
}

func (s StateTransform) CreateBlock(groupId string, req pb.RpcBlockCreateRequest) (id string, err error) {
	if req.TargetId != "" {
		if s.IsChild(template.HeaderLayoutId, req.TargetId) {
			req.Position = model.Block_Bottom
			req.TargetId = template.HeaderLayoutId
		}
	}
	if req.Block.GetContent() == nil {
		err = fmt.Errorf("no block content")
		return
	}
	req.Block.Id = ""
	block := simple.New(req.Block)
	block.Model().ChildrenIds = nil
	err = block.Validate()
	if err != nil {
		return
	}
	s.Add(block)
	if err = s.InsertTo(req.TargetId, req.Position, block.Model().Id); err != nil {
		return
	}
	return block.Model().Id, nil
}

func (s StateTransform) CutBlocks(blockIds []string) (blocks []simple.Block) {
	var uniqMap = make(map[string]struct{})
	for _, bId := range blockIds {
		b := s.Pick(bId)
		if b != nil {
			descendants := s.getAllDescendants(uniqMap, b.Copy(), []simple.Block{})
			blocks = append(blocks, descendants...)
			s.Unlink(b.Model().Id)
		}
	}
	return blocks
}

func (s StateTransform) getAllDescendants(uniqMap map[string]struct{}, block simple.Block, blocks []simple.Block) []simple.Block {
	if _, ok := uniqMap[block.Model().Id]; ok {
		return blocks
	}
	blocks = append(blocks, block)
	uniqMap[block.Model().Id] = struct{}{}
	for _, cId := range block.Model().ChildrenIds {
		blocks = s.getAllDescendants(uniqMap, s.Pick(cId).Copy(), blocks)
	}
	return blocks
}
