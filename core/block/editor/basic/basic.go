package basic

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

type Basic interface {
	Create(ctx *state.Context, req pb.RpcBlockCreateRequest) (id string, err error)
	Duplicate(req pb.RpcBlockListDuplicateRequest) (newIds []string, err error)
	Unlink(id ...string) (err error)
	Move(ctx *state.Context, req pb.RpcBlockListMoveRequest) error
	Replace(id string, block *model.Block) (newId string, err error)
	SetFields(fields ...*pb.RpcBlockListSetFieldsRequestBlockField) (err error)
	Update(apply func(b simple.Block) error, blockIds ...string) (err error)
	SetDivStyle(ctx *state.Context, style model.BlockContentDivStyle, ids ...string) (err error)
	InternalCut(ctx *state.Context, req pb.RpcBlockListMoveRequest) (blocks []simple.Block, err error)
	InternalPaste(blocks []simple.Block) (err error)
}

func (bs *basic) InternalCut(ctx *state.Context, req pb.RpcBlockListMoveRequest) (blocks []simple.Block, err error) {
	contextState := bs.NewStateCtx(ctx)

	for _, bId := range req.BlockIds {
		b := contextState.Pick(bId)
		if b != nil {
			descendants := bs.getAllDescendants(b, []simple.Block{})
			blocks = append(blocks, descendants...)
			contextState.Remove(b.Model().Id)
		}
	}

	err = bs.Apply(contextState)
	if err != nil {
		return blocks, err
	}

	return blocks, err
}

func (bs *basic) InternalPaste(blocks []simple.Block) (err error) {
	targetState := bs.NewState()
	idToIsChild := make(map[string]bool)
	for _, b := range blocks {
		for _, cId := range b.Model().ChildrenIds {
			idToIsChild[cId] = true
		}
	}

	for _, b := range blocks {
		targetState.Add(b)
		if idToIsChild[b.Model().Id] != true {
			err := targetState.InsertTo("", model.Block_Inner, b.Model().Id)
			if err != nil {
				return err
			}
		}
		if err != nil {
			return err
		}
	}

	return bs.Apply(targetState)
}

func NewBasic(sb smartblock.SmartBlock) Basic {
	return &basic{sb}
}

type basic struct {
	smartblock.SmartBlock
}

func (bs *basic) Create(ctx *state.Context, req pb.RpcBlockCreateRequest) (id string, err error) {
	s := bs.NewStateCtx(ctx)
	block := simple.New(req.Block)
	s.Add(block)
	if err = s.InsertTo(req.TargetId, req.Position, block.Model().Id); err != nil {
		return
	}
	if err = bs.Apply(s); err != nil {
		return
	}
	return block.Model().Id, nil
}

func (bs *basic) Duplicate(req pb.RpcBlockListDuplicateRequest) (newIds []string, err error) {
	s := bs.NewState()
	pos := req.Position
	targetId := req.TargetId
	for _, id := range req.BlockIds {
		copyId, e := bs.copy(s, id)
		if e != nil {
			return nil, e
		}
		if err = s.InsertTo(targetId, pos, copyId); err != nil {
			return
		}
		pos = model.Block_Bottom
		targetId = copyId
		newIds = append(newIds, copyId)
	}
	if err = bs.Apply(s); err != nil {
		return
	}
	return
}

func (bs *basic) copy(s *state.State, sourceId string) (id string, err error) {
	b := s.Get(sourceId)
	if bs == nil {
		return "", smartblock.ErrSimpleBlockNotFound
	}
	m := b.Copy().Model()
	m.Id = "" // reset id
	copy := simple.New(m)
	s.Add(copy)
	for i, childrenId := range copy.Model().ChildrenIds {
		if copy.Model().ChildrenIds[i], err = bs.copy(s, childrenId); err != nil {
			return
		}
	}
	return copy.Model().Id, nil
}

func (bs *basic) Unlink(ids ...string) (err error) {
	s := bs.NewState()
	for _, id := range ids {
		if !s.Remove(id) {
			return smartblock.ErrSimpleBlockNotFound
		}
	}
	return bs.Apply(s)
}

func (bs *basic) Move(ctx *state.Context, req pb.RpcBlockListMoveRequest) (err error) {
	s := bs.NewStateCtx(ctx)
	for _, id := range req.BlockIds {
		s.Unlink(id)
	}
	if err = s.InsertTo(req.DropTargetId, req.Position, req.BlockIds...); err != nil {
		return
	}
	return bs.Apply(s)
}

func (bs *basic) Replace(id string, block *model.Block) (newId string, err error) {
	s := bs.NewState()

	new := simple.New(block)
	newId = new.Model().Id
	s.Add(new)
	if err = s.InsertTo(id, model.Block_Replace, newId); err != nil {
		return
	}
	if err = bs.Apply(s); err != nil {
		return
	}
	return
}

func (bs *basic) SetFields(fields ...*pb.RpcBlockListSetFieldsRequestBlockField) (err error) {
	s := bs.NewState()
	for _, fr := range fields {
		if b := s.Get(fr.BlockId); b != nil {
			b.Model().Fields = fr.Fields
		}
	}
	return bs.Apply(s, smartblock.NoHistory)
}

func (bs *basic) Update(apply func(b simple.Block) error, blockIds ...string) (err error) {
	s := bs.NewState()
	for _, id := range blockIds {
		if b := s.Get(id); b != nil {
			if err = apply(b); err != nil {
				return
			}
		} else {
			return smartblock.ErrSimpleBlockNotFound
		}
	}
	return bs.Apply(s)
}

func (bs *basic) SetDivStyle(ctx *state.Context, style model.BlockContentDivStyle, ids ...string) (err error) {
	s := bs.NewStateCtx(ctx)
	for _, id := range ids {
		b := s.Get(id)
		if b == nil {
			return smartblock.ErrSimpleBlockNotFound
		}
		if div, ok := b.(base.DivBlock); ok {
			div.SetStyle(style)
		} else {
			return fmt.Errorf("unexpected block type: %T (want Div)", b)
		}
	}
	return bs.Apply(s)
}

func (bs *basic) getAllDescendants(block simple.Block, blocks []simple.Block) []simple.Block {
	blocks = append(blocks, block)
	for _, cId := range block.Model().ChildrenIds {
		blocks = bs.getAllDescendants(bs.Pick(cId), blocks)
	}

	return blocks
}
