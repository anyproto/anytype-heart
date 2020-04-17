package base

import (
	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
	"github.com/anytypeio/go-anytype-middleware/util/pbtypes"
)

func NewPage(m *model.Block) simple.Block {
	return &Page{
		Base: NewBase(m).(*Base),
	}
}

type PageBlock interface {
	simple.Block
}

type Page struct {
	*Base
}

func (i *Page) Diff(block simple.Block) (msgs []*pb.EventMessage, err error) {
	return i.Base.Diff(block)
}

func (i *Page) Copy() simple.Block {
	return NewPage(pbtypes.CopyBlock(i.Model()))
}
