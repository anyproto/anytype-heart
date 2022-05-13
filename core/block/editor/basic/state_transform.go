package basic

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/globalsign/mgo/bson"
)

func CreateBlock(s *state.State, groupId string, req pb.RpcBlockCreateRequest) (id string, err error) {
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

func CutBlocks(s *state.State, blockIds []string) (blocks []simple.Block) {
	visited := map[string]struct{}{}
	for _, id := range blockIds {
		b := s.Pick(id)
		if b == nil {
			continue
		}

		queue := append(s.Descendants(id), b)
		for _, b := range queue {
			if _, ok := visited[b.Model().Id]; ok {
				continue
			}
			blocks = append(blocks, b.Copy())
			visited[b.Model().Id] = struct{}{}
			s.Unlink(b.Model().Id)
		}
	}
	return blocks
}

func PasteBlocks(s *state.State, blocks []simple.Block) error {
	childIdsRewrite := make(map[string]string)
	for _, b := range blocks {
		for i, cId := range b.Model().ChildrenIds {
			newId := bson.NewObjectId().Hex()
			childIdsRewrite[cId] = newId
			b.Model().ChildrenIds[i] = newId
		}
	}
	for _, b := range blocks {
		var child bool
		if newId, ok := childIdsRewrite[b.Model().Id]; ok {
			b.Model().Id = newId
			child = true
		} else {
			b.Model().Id = bson.NewObjectId().Hex()
		}
		s.Add(b)
		if !child {
			err := s.InsertTo("", model.Block_Inner, b.Model().Id)
			if err != nil {
				return err
			}
		}
	}
	return nil
}
