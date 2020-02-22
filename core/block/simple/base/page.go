package base

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/gogo/protobuf/types"
	"github.com/mohae/deepcopy"
)

func NewPage(m *model.Block) simple.Block {
	return &Page{
		Base: NewBase(m).(*Base),
	}
}

type PageBlock interface {
	simple.Block
	SetPageIsArchived(isArchived bool)
}

type Page struct {
	*Base
}

func (i *Page) SetPageIsArchived(isArchived bool) {
	fields := i.Model().Fields
	if fields.Fields == nil {
		fields.Fields = make(map[string]*types.Value)
	}
	fields.Fields["isArchived"] = &types.Value{
		Kind: &types.Value_BoolValue{BoolValue: isArchived},
	}
	return
}

func (i *Page) Diff(block simple.Block) (msgs []*pb.EventMessage, err error) {
	return i.Base.Diff(block)
}

func (i *Page) Copy() simple.Block {
	return NewPage(deepcopy.Copy(i.Model()).(*model.Block))
}
