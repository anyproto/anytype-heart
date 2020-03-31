package basic

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

type Basic interface {
	Create(req pb.RpcBlockCreateRequest) (id string, err error)
	Duplicate(req pb.RpcBlockListDuplicateRequest) (newIds []string, err error)
	Unlink(id ...string) (err error)
	Move(req pb.RpcBlockListMoveRequest) error
	Replace(id string, block *model.Block) (newId string, err error)
	SetFields(fields ...*pb.RpcBlockListSetFieldsRequestBlockField) (err error)
	Update(apply func(b simple.Block) error, blockIds ...string) (err error)
}

func NewBasic(sb smartblock.SmartBlock) Basic {
	return &basic{sb}
}

type basic struct {
	smartblock.SmartBlock
}

func (bs *basic) Create(req pb.RpcBlockCreateRequest) (id string, err error) {
	s := bs.NewState()
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
		if ! s.Remove(id) {
			return smartblock.ErrSimpleBlockNotFound
		}
	}
	return bs.Apply(s)
}

func (bs *basic) Move(req pb.RpcBlockListMoveRequest) (err error) {
	s := bs.NewState()
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
	return bs.Apply(s)
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
