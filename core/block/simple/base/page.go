package base

import (
	"fmt"

	"github.com/anytypeio/go-anytype-library/pb/model"
	"github.com/anytypeio/go-anytype-middleware/core/block/simple"
	"github.com/anytypeio/go-anytype-middleware/pb"
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
	i.Model().GetPage().IsArchived = isArchived
	return
}

func (i *Page) Diff(block simple.Block) (msgs []*pb.EventMessage, err error) {
	if block.Model().GetPage() == nil {
		return nil, fmt.Errorf("can't diff page with %T", block.Model().Content)
	}
	if msgs, err = i.Base.Diff(block); err != nil {
		return
	}
	if newName := block.Model().GetPage().IsArchived; newName != i.GetPage().IsArchived {
		msgs = append(msgs, &pb.EventMessage{
			Value: &pb.EventMessageValueOfBlockSetPage{
				BlockSetPage: &pb.EventBlockSetPage{
					Id:         i.Model().Id,
					IsArchived: &pb.EventBlockSetPageIsArchived{Value: newName},
				},
			},
		})
	}
	return
}

func (i *Page) Copy() simple.Block {
	return NewPage(deepcopy.Copy(i.Model()).(*model.Block))
}
