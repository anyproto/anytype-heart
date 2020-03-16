package basic

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/editor/smartblock"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
)

type Basic interface {
	Create(req pb.RpcBlockCreateRequest) (id string, err error)
	CreatePage(req pb.RpcBlockCreatePageRequest) (id, targetId string, err error)
	Duplicate(req pb.RpcBlockListDuplicateRequest) (newIds []string, err error)
	Unlink(id ...string) (err error)
	Move(req pb.RpcBlockListMoveRequest) error
	Replace(id string, block *model.Block) (newId string, err error)
}

func NewBasic(sb smartblock.SmartBlock) Basic {
	return &basic{
		sb: sb,
	}
}

type basic struct {
	smartblock.SmartBlock
}

func (b *basic) Create(req pb.RpcBlockCreateRequest) (id string, err error) {
	b.Lock()
	defer b.Unlock()
	s := b.New()
	block := simple.New(req.Block)
	s.Add(block)
	if err = s.InsertTo(req.TargetId, req.Position, block.Model().Id); err != nil {
		return
	}
	s.Apply()
	return block.Model().Id, nil
}

func (b *basic) CreatePage(req pb.RpcBlockCreatePageRequest) (id, targetId string, err error) {
	panic("implement me")
}

func (b *basic) Duplicate(req pb.RpcBlockListDuplicateRequest) (newIds []string, err error) {
	panic("implement me")
}

func (b *basic) Unlink(id ...string) (err error) {
	panic("implement me")
}

func (b *basic) Move(req pb.RpcBlockListMoveRequest) error {
	panic("implement me")
}

func (b *basic) Replace(id string, block *model.Block) (newId string, err error) {
	panic("implement me")
}
