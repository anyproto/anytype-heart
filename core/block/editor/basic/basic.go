package basic

import (
	"fmt"

	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/state"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/template"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/base"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple/relation"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/pkg/lib/pb/model"
	"github.com/globalsign/mgo/bson"
)

type Basic interface {
	Create(ctx *state.Context, groupId string, req pb.RpcBlockCreateRequest) (id string, err error)
	Duplicate(ctx *state.Context, req pb.RpcBlockListDuplicateRequest) (newIds []string, err error)
	Unlink(ctx *state.Context, id ...string) (err error)
	Move(ctx *state.Context, req pb.RpcBlockListMoveRequest) error
	Replace(ctx *state.Context, id string, block *model.Block) (newId string, err error)
	SetFields(ctx *state.Context, fields ...*pb.RpcBlockListSetFieldsRequestBlockField) (err error)
	Update(ctx *state.Context, apply func(b simple.Block) error, blockIds ...string) (err error)
	SetDivStyle(ctx *state.Context, style model.BlockContentDivStyle, ids ...string) (err error)
	InternalCut(ctx *state.Context, req pb.RpcBlockListMoveRequest) (blocks []simple.Block, err error)
	InternalPaste(blocks []simple.Block) (err error)
	SetRelationKey(ctx *state.Context, req pb.RpcBlockRelationSetKeyRequest) error
	AddRelationAndSet(ctx *state.Context, req pb.RpcBlockRelationAddRequest) error
}

var ErrNotSupported = fmt.Errorf("operation not supported for this type of smartblock")

func (bs *basic) InternalCut(ctx *state.Context, req pb.RpcBlockListMoveRequest) (blocks []simple.Block, err error) {
	s := bs.NewStateCtx(ctx)

	for _, bId := range req.BlockIds {
		b := s.Pick(bId)
		if b != nil {
			descendants := bs.getAllDescendants(b.Copy(), []simple.Block{})
			blocks = append(blocks, descendants...)
			s.Unlink(b.Model().Id)
		}
	}

	err = bs.Apply(s)
	if err != nil {
		return blocks, err
	}

	return blocks, err
}

func (bs *basic) InternalPaste(blocks []simple.Block) (err error) {
	s := bs.NewState()
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
	return bs.Apply(s)
}

func NewBasic(sb smartblock.SmartBlock) Basic {
	return &basic{sb}
}

type basic struct {
	smartblock.SmartBlock
}

func (bs *basic) Create(ctx *state.Context, groupId string, req pb.RpcBlockCreateRequest) (id string, err error) {
	if err = bs.Restrictions().Object.Check(model.Restrictions_Blocks); err != nil {
		return
	}
	if bs.Type() == model.SmartBlockType_Set {
		return "", ErrNotSupported
	}
	s := bs.NewStateCtx(ctx).SetGroupId(groupId)
	if req.TargetId != "" {
		if s.IsChild(template.HeaderLayoutId, req.TargetId) {
			req.Position = model.Block_Bottom
			req.TargetId = template.HeaderLayoutId
		}
	}
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

func (bs *basic) Duplicate(ctx *state.Context, req pb.RpcBlockListDuplicateRequest) (newIds []string, err error) {
	if bs.Type() == model.SmartBlockType_Set {
		return nil, ErrNotSupported
	}

	s := bs.NewStateCtx(ctx)
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

func (bs *basic) Unlink(ctx *state.Context, ids ...string) (err error) {
	if bs.Type() == model.SmartBlockType_Set {
		return ErrNotSupported
	}

	s := bs.NewStateCtx(ctx)
	for _, id := range ids {
		if !s.Unlink(id) {
			return smartblock.ErrSimpleBlockNotFound
		}
	}
	return bs.Apply(s)
}

func (bs *basic) Move(ctx *state.Context, req pb.RpcBlockListMoveRequest) (err error) {
	if bs.Type() == model.SmartBlockType_Set {
		return ErrNotSupported
	}

	s := bs.NewStateCtx(ctx)
	if req.DropTargetId != "" {
		if s.IsChild(template.HeaderLayoutId, req.DropTargetId) || req.DropTargetId == template.HeaderLayoutId {
			req.Position = model.Block_Bottom
			req.DropTargetId = template.HeaderLayoutId
		}
	}
	for _, id := range req.BlockIds {
		if b := s.Pick(id); b != nil {
			s.Unlink(id)
		}
	}
	if err = s.InsertTo(req.DropTargetId, req.Position, req.BlockIds...); err != nil {
		return
	}
	return bs.Apply(s)
}

func (bs *basic) Replace(ctx *state.Context, id string, block *model.Block) (newId string, err error) {
	if bs.Type() == model.SmartBlockType_Set {
		return "", ErrNotSupported
	}

	s := bs.NewStateCtx(ctx)
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

func (bs *basic) SetFields(ctx *state.Context, fields ...*pb.RpcBlockListSetFieldsRequestBlockField) (err error) {
	s := bs.NewStateCtx(ctx)
	for _, fr := range fields {
		if b := s.Get(fr.BlockId); b != nil {
			b.Model().Fields = fr.Fields
		}
	}
	return bs.Apply(s)
}

func (bs *basic) Update(ctx *state.Context, apply func(b simple.Block) error, blockIds ...string) (err error) {
	s := bs.NewStateCtx(ctx)
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

func (bs *basic) SetRelationKey(ctx *state.Context, req pb.RpcBlockRelationSetKeyRequest) (err error) {
	s := bs.NewStateCtx(ctx)
	b := s.Get(req.BlockId)
	if b == nil {
		return smartblock.ErrSimpleBlockNotFound
	}
	if !bs.HasRelation(req.Key) {
		return fmt.Errorf("relation with given key not found")
	}
	if rel, ok := b.(relation.Block); ok {
		rel.SetKey(req.Key)
	} else {
		return fmt.Errorf("unexpected block type: %T (want relation)", b)
	}
	return bs.Apply(s)
}

func (bs *basic) AddRelationAndSet(ctx *state.Context, req pb.RpcBlockRelationAddRequest) (err error) {
	s := bs.NewStateCtx(ctx)
	b := s.Get(req.BlockId)
	if b == nil {
		return smartblock.ErrSimpleBlockNotFound
	}
	key := req.Relation.Key
	if !s.HasRelation(key) {
		if req.Relation.Key == "" {
			req.Relation.Key = bson.NewObjectId().Hex()
		}
		s.AddRelation(req.Relation)
	}
	if rel, ok := b.(relation.Block); ok {
		rel.SetKey(req.Relation.Key)
	} else {
		return fmt.Errorf("unexpected block type: %T (want relation)", b)
	}
	return bs.Apply(s)
}

func (bs *basic) getAllDescendants(block simple.Block, blocks []simple.Block) []simple.Block {
	blocks = append(blocks, block)
	for _, cId := range block.Model().ChildrenIds {
		blocks = bs.getAllDescendants(bs.Pick(cId).Copy(), blocks)
	}
	return blocks
}
